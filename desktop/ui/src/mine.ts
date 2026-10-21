//! Vista de mineração (A4): pedido de prova → prova TLSNotary → aprovação →
//! tx não assinada → assinatura local → broadcast.
//!
//! A **seed** assina no Rust; aqui só se conduz a interface. A aprovação chega
//! por **evento** (`mine-preview`) porque o prover fica bloqueado à espera da
//! decisão — assim a sessão do notário mantém-se viva.

import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { setError } from "./errors";
import { t } from "./i18n";
import { walletRefresh } from "./wallet";

type Provider = { id: string; name: string };
type CaptureReply = { trn: number; seq_flow: number | null; receipt: string };
type Reveal = { direction: string; label: string; action: string; preview: string };
type Preview = {
  session_id: string;
  status: number;
  response_body: string;
  descriptors: Reveal[];
};
type MineResult = {
  tx_hash: string;
  payment_amount: string | null;
  tax_entity: string | null;
  cancelled: boolean;
};

const DEFAULT_API = "http://127.0.0.1:8082";
const DEFAULT_NOTARY = "http://127.0.0.1:7047";

// Não são segredos: guardar no WebView chega e sobrevive a reinícios.
const API_KEY = "luggom.mine.api_url";
const NOTARY_KEY = "luggom.mine.notary_url";

const el = <T extends HTMLElement>(id: string): T => {
  const node = document.getElementById(id);
  if (!node) throw new Error(`elemento #${id} em falta`);
  return node as T;
};

const errorBox = el<HTMLParagraphElement>("mine-error");
const doneBox = el<HTMLParagraphElement>("mine-done");
const statusBox = el<HTMLParagraphElement>("mine-status");
const runButton = el<HTMLButtonElement>("mine-run");
const approval = el<HTMLDivElement>("mine-approval");
const lockedNotice = el<HTMLDivElement>("mine-locked");

let running = false;
let locked = true;

async function call<T>(cmd: string, args?: Record<string, unknown>): Promise<T | null> {
  setError(errorBox, null);
  try {
    return await invoke<T>(cmd, args);
  } catch (err) {
    setError(errorBox, err);
    return null;
  }
}

function api(): string {
  return el<HTMLInputElement>("mine-api").value.trim() || DEFAULT_API;
}

function notary(): string {
  return el<HTMLInputElement>("mine-notary").value.trim() || DEFAULT_NOTARY;
}

function provider(): string {
  return el<HTMLSelectElement>("mine-provider").value;
}

function localError(message: string): void {
  setError(errorBox, { kind: "internal", message });
}

function short(value: string): string {
  return value.length > 20 ? `${value.slice(0, 10)}…${value.slice(-6)}` : value;
}

// ---------------------------------------------------------------------------
// Bancos
// ---------------------------------------------------------------------------

async function loadProviders(): Promise<void> {
  const select = el<HTMLSelectElement>("mine-provider");
  const placeholder = (label: string) => {
    select.innerHTML = "";
    const option = document.createElement("option");
    option.value = "";
    option.textContent = label;
    select.append(option);
  };

  placeholder(t("mine.providers.loading"));

  let providers: Provider[] = [];
  try {
    providers = await invoke<Provider[]>("mine_providers", { apiUrl: api() });
  } catch {
    placeholder(t("mine.providers.failed"));
    return;
  }

  if (providers.length === 0) return placeholder(t("mine.providers.empty"));

  select.innerHTML = "";
  for (const item of providers) {
    const option = document.createElement("option");
    option.value = item.id;
    option.textContent = item.name || item.id;
    select.append(option);
  }
  // Viacredi é o banco com rail provado — fica selecionado por omissão.
  const viacredi = [...select.options].find((o) => o.value === "viacredi");
  if (viacredi) select.value = "viacredi";
}

// ---------------------------------------------------------------------------
// Aprovação
// ---------------------------------------------------------------------------

function renderApproval(preview: Preview): void {
  el<HTMLElement>("mine-approval-status").textContent = t("mine.approval.status", {
    status: preview.status,
  });

  const list = el<HTMLUListElement>("mine-descriptors");
  list.innerHTML = "";
  for (const descriptor of preview.descriptors) {
    const item = document.createElement("li");
    item.className = descriptor.action === "REVEAL" ? "reveal" : "hash";

    const label = document.createElement("strong");
    label.textContent = `${descriptor.direction} · ${descriptor.label}`;

    const body = document.createElement("code");
    body.textContent = descriptor.preview.replace(/\s+/g, " ").slice(0, 240);

    item.append(label, body);
    list.append(item);
  }

  approval.hidden = false;
  statusBox.hidden = true;
}

listen<Preview>("mine-preview", (event) => renderApproval(event.payload));

el<HTMLButtonElement>("mine-approve").addEventListener("click", () => void decide(true));
el<HTMLButtonElement>("mine-reject").addEventListener("click", () => void decide(false));

async function decide(approved: boolean): Promise<void> {
  approval.hidden = true;
  const approve = el<HTMLButtonElement>("mine-approve");
  const reject = el<HTMLButtonElement>("mine-reject");
  approve.disabled = true;
  reject.disabled = true;
  try {
    await call<null>("mine_decide", { approved });
  } finally {
    approve.disabled = false;
    reject.disabled = false;
  }
}

// ---------------------------------------------------------------------------
// Banco, pelo Firefox (a extensão coletora envia; o app só recebe)
// ---------------------------------------------------------------------------

const bankInfo = el<HTMLElement>("mine-bank-info");

let bankPoll: number | null = null;

/** Comando que não devolve nada: `true` se correu, `false` se falhou. */
async function invokeOrError(cmd: string, args?: Record<string, unknown>): Promise<boolean> {
  setError(errorBox, null);
  try {
    await invoke(cmd, args);
    return true;
  } catch (err) {
    setError(errorBox, err);
    return false;
  }
}

/** O que a captura recebida já identifica (sem mexer na caixa de erro). */
async function refreshBankInfo(): Promise<CaptureReply | null> {
  try {
    const info = await invoke<CaptureReply | null>("bank_status");
    if (!info) {
      bankInfo.textContent = "";
      return null;
    }
    bankInfo.textContent = t("mine.capture.ok", {
      receipt: `${info.trn}${info.seq_flow ? `/${info.seq_flow}` : ""} · ${info.receipt}`,
    });
    return info;
  } catch {
    return null;
  }
}

type BankOpenReply = { profile: string; extension: boolean };

/** Abre o login no Firefox, no perfil do app. A captura chega sozinha. */
async function openBank(): Promise<void> {
  const providerId = provider();
  if (!providerId) {
    localError(t("mine.providers.empty"));
    return;
  }
  setError(errorBox, null);
  try {
    const reply = await invoke<BankOpenReply>("bank_open_firefox", { providerId });
    // Sem a extensão no perfil, o utilizador tem de a instalar uma vez.
    const hint = document.getElementById("mine-bank-hint");
    if (hint) {
      hint.textContent = reply.extension
        ? ""
        : t("mine.bank.no_extension", { profile: reply.profile });
    }
  } catch (err) {
    setError(errorBox, err);
  }
}

async function forgetBank(): Promise<void> {
  await invokeOrError("bank_forget");
  await refreshBankInfo();
}

el<HTMLButtonElement>("mine-bank-open").addEventListener("click", () => void openBank());
el<HTMLButtonElement>("mine-bank-forget").addEventListener("click", () => void forgetBank());

/** A captura pode chegar a qualquer momento: consulta-se em ciclo. */
function startBankPolling(): void {
  if (bankPoll !== null) return;
  bankPoll = window.setInterval(() => void refreshBankInfo(), 2000);
}

startBankPolling();

// ---------------------------------------------------------------------------
// Minerações — o extrato: quanto se pagou (FIAT) e quanto virou em LUG
// ---------------------------------------------------------------------------

type MiningRow = {
  tx_hash: string;
  height: number;
  timestamp: string;
  tax_entity: string;
  amount: string;
  currency: string;
  lug: string;
};

const miningList = el<HTMLUListElement>("mine-history");
const miningEmpty = el<HTMLElement>("mine-history-empty");

/** Lê as minerações **da chain** e desenha a lista. Nunca inventa valores. */
async function refreshMiningHistory(): Promise<MiningRow[]> {
  try {
    const rows = await invoke<MiningRow[]>("mining_history", { limit: 20 });
    miningList.innerHTML = "";
    for (const row of rows) {
      const item = document.createElement("li");
      item.className = "history-row";

      const paid = document.createElement("span");
      paid.textContent = `${row.amount} ${row.currency}`;

      const mined = document.createElement("strong");
      mined.textContent = `${row.lug} LUG`;

      item.append(paid, mined);
      miningList.append(item);
    }
    miningEmpty.hidden = rows.length > 0;
    return rows;
  } catch (err) {
    // Não há lista porque **não se conseguiu ler** (carteira fechada, nó em
    // baixo) — diz-se isso, em vez de fingir que não há minerações.
    miningList.innerHTML = "";
    miningEmpty.hidden = true;
    setError(errorBox, err);
    return [];
  }
}

void refreshMiningHistory();

// ---------------------------------------------------------------------------
// Correr
// ---------------------------------------------------------------------------

function capturePath(): string {
  return el<HTMLInputElement>("mine-capture").value.trim();
}

/** Carrega a captura e mostra o comprovante que ela identifica. */
async function loadCapture(): Promise<boolean> {
  const path = capturePath();
  if (!path) {
    localError(t("error.mine_capture"));
    return false;
  }
  const info = await call<CaptureReply>("mine_load_capture", { path });
  if (!info) {
    el<HTMLElement>("mine-capture-info").textContent = "";
    return false;
  }
  el<HTMLElement>("mine-capture-info").textContent = t("mine.capture.ok", {
    receipt: `${info.trn}${info.seq_flow ? `/${info.seq_flow}` : ""} · ${info.receipt}`,
  });
  return true;
}

async function run(): Promise<void> {
  if (running) return;

  // A carteira fechada não assina nada: em vez de um botão morto, explica e
  // leva o utilizador ao sítio onde se abre.
  if (locked) {
    localError(t("mine.locked"));
    document.dispatchEvent(new CustomEvent("luggom:tab", { detail: "wallet" }));
    return;
  }

  doneBox.hidden = true;
  approval.hidden = true;

  const providerId = provider();
  if (!providerId) {
    localError(t("mine.providers.empty"));
    return;
  }

  // A captura vem da janela do banco; o ficheiro (modo avançado) é alternativa.
  const path = capturePath();
  if (!path && !(await refreshBankInfo())) {
    localError(t("error.mine_capture"));
    return;
  }

  running = true;
  runButton.disabled = true;
  runButton.textContent = t("mine.working");
  statusBox.textContent = t("mine.working");
  statusBox.hidden = false;
  try {
    const result = await call<MineResult>("mine_run", {
      capturePath: path || null,
      providerId,
      notaryUrl: notary(),
      apiUrl: api(),
    });
    if (!result) return;

    if (result.cancelled) {
      doneBox.textContent = t("mine.cancelled");
      doneBox.hidden = false;
      return;
    }

    // O que se pagou e o que entrou estão na chain: lê-se de lá (não se inventa).
    const rows = await refreshMiningHistory();
    const row = rows.find((entry) => entry.tx_hash === result.tx_hash);
    doneBox.textContent = row
      ? t("mine.done.detail", {
          amount: row.amount,
          currency: row.currency,
          lug: row.lug,
          hash: short(result.tx_hash),
        })
      : t("mine.done.plain", { hash: short(result.tx_hash) });
    doneBox.hidden = false;

    // O saldo e o extrato mudaram — refletir isso já.
    walletRefresh();
  } finally {
    running = false;
    runButton.disabled = false;
    runButton.textContent = t("mine.run");
    statusBox.hidden = true;
  }
}

el<HTMLButtonElement>("mine-run").addEventListener("click", () => void run());

el<HTMLButtonElement>("mine-capture-load").addEventListener("click", () => void loadCapture());

// ---------------------------------------------------------------------------
// Interface com o resto da aplicação
// ---------------------------------------------------------------------------

/** Arranque: valores por omissão e whitelist de bancos. */
export function initMineView(): void {
  const apiInput = el<HTMLInputElement>("mine-api");
  const notaryInput = el<HTMLInputElement>("mine-notary");

  apiInput.value = localStorage.getItem(API_KEY) ?? DEFAULT_API;
  notaryInput.value = localStorage.getItem(NOTARY_KEY) ?? DEFAULT_NOTARY;

  apiInput.addEventListener("change", () => {
    localStorage.setItem(API_KEY, api());
    void loadProviders();
  });
  notaryInput.addEventListener("change", () => {
    localStorage.setItem(NOTARY_KEY, notary());
  });

  void loadProviders();
}

/** Estado da carteira, sempre visível no cartão de mineração. */
export function mineSetLocked(value: boolean, address: string | null): void {
  locked = value;
  lockedNotice.hidden = false;
  lockedNotice.className = value ? "notice" : "ok";
  lockedNotice.textContent = value
    ? t("mine.locked")
    : t("mine.wallet.open", { address: short(address ?? "") });
  // O botão fica sempre clicável: se estiver fechada, explica e leva à aba.
  runButton.disabled = running;
  // Ao abrir a carteira, a lista de minerações passa a poder ser lida.
  if (!value) void refreshMiningHistory();
}

/** Chamado quando o idioma muda. */
export function mineReload(): void {
  if (!running) runButton.textContent = t("mine.run");
  doneBox.hidden = true;
}
