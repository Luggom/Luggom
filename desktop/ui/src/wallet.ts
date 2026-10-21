//! Vista da carteira: rede, saldo, receber, enviar e extrato (A3).

import { invoke } from "@tauri-apps/api/core";
import { setError } from "./errors";
import { t } from "./i18n";

type ChainInfo = {
  node_url: string;
  reachable: boolean;
  chain_id: string | null;
  height: number | null;
  error: string | null;
};

type BalanceReply = {
  address: string;
  balance_uluggom: string;
  balance_lug: string;
  exists: boolean;
};

type SendPlanReply = {
  amount_lug: string;
  gas_fee_lug: string;
  flat_fee_lug: string;
  total_lug: string;
};

type TransferReply = {
  tx_hash: string;
  height: number;
  timestamp: string;
  direction: "in" | "out";
  counterparty: string;
  amount_lug: string;
};

type SendReceipt = { tx_hash: string; code: number };

const el = <T extends HTMLElement>(id: string): T => {
  const node = document.getElementById(id);
  if (!node) throw new Error(`elemento #${id} em falta`);
  return node as T;
};

const errorBox = el<HTMLParagraphElement>("wallet-error");
const walletView = el<HTMLDivElement>("wallet-view");

let address: string | null = null;
let chainInfo: ChainInfo | null = null;
let history: TransferReply[] = [];
let plan: SendPlanReply | null = null;

async function call<T>(cmd: string, args?: Record<string, unknown>): Promise<T | null> {
  setError(errorBox, null);
  try {
    return await invoke<T>(cmd, args);
  } catch (err) {
    setError(errorBox, err);
    return null;
  }
}

// ---------------------------------------------------------------------------
// Rede
// ---------------------------------------------------------------------------

function renderNode(): void {
  const info = chainInfo;
  el<HTMLInputElement>("node-url").value = info?.node_url ?? "";
  el<HTMLElement>("chain-state").textContent = !info
    ? ""
    : info.reachable
      ? t("chain.connected", {
          chain: info.chain_id ?? "?",
          height: info.height ?? "?",
        })
      : t("chain.disconnected");
}

async function loadChain(): Promise<void> {
  chainInfo = await call<ChainInfo>("chain_info");
  renderNode();
}

// ---------------------------------------------------------------------------
// Saldo e receber
// ---------------------------------------------------------------------------

async function loadBalance(): Promise<void> {
  if (!address) return;
  const reply = await call<BalanceReply>("wallet_balance");
  if (!reply) return;

  el<HTMLElement>("balance-lug").textContent = `${reply.balance_lug} LUG`;
  // O texto do aviso vem do `data-i18n`; aqui só se decide se aparece.
  el<HTMLParagraphElement>("balance-note").hidden = reply.exists;
}

function renderReceive(): void {
  el<HTMLElement>("receive-address").textContent = address ?? "—";
}

// ---------------------------------------------------------------------------
// Enviar
// ---------------------------------------------------------------------------

function renderPlan(): void {
  const box = el<HTMLElement>("send-plan");
  if (!plan) {
    box.textContent = "";
    return;
  }
  box.textContent = t("send.plan", {
    amount: plan.amount_lug,
    gas: plan.gas_fee_lug,
    fee: plan.flat_fee_lug,
    total: plan.total_lug,
  });
}

/** Recalcula o custo enquanto se escreve o valor (sem tocar na rede). */
async function updatePlan(): Promise<void> {
  const raw = el<HTMLInputElement>("send-amount").value.trim();
  if (!raw) {
    plan = null;
    return renderPlan();
  }
  plan = await call<SendPlanReply>("wallet_send_plan", { amountLug: raw });
  renderPlan();
}

// ---------------------------------------------------------------------------
// Extrato
// ---------------------------------------------------------------------------

function renderHistory(): void {
  const list = el<HTMLUListElement>("history");
  list.innerHTML = "";

  el<HTMLParagraphElement>("history-empty").hidden = history.length > 0;

  for (const transfer of history) {
    const item = document.createElement("li");

    const label = document.createElement("span");
    label.textContent = `${transfer.direction === "in" ? t("history.in") : t("history.out")} `;
    label.className = "muted";

    const who = document.createElement("code");
    who.className = "mono small";
    who.textContent = short(transfer.counterparty);

    const amount = document.createElement("strong");
    amount.className = transfer.direction === "in" ? "in" : "out";
    amount.textContent = `${transfer.direction === "in" ? "+" : "−"}${transfer.amount_lug} LUG`;

    item.append(label, who, amount);
    list.append(item);
  }
}

function short(value: string): string {
  return value.length > 20 ? `${value.slice(0, 10)}…${value.slice(-6)}` : value;
}

async function loadHistory(): Promise<void> {
  if (!address) return;
  history = (await call<TransferReply[]>("wallet_history", { limit: 25 })) ?? [];
  renderHistory();
}

// ---------------------------------------------------------------------------
// Ações
// ---------------------------------------------------------------------------

el<HTMLButtonElement>("node-save").addEventListener("click", async () => {
  const url = el<HTMLInputElement>("node-url").value.trim();
  await call<null>("chain_set_node_url", { url });
  await loadChain();
  if (address) {
    await loadBalance();
    await loadHistory();
  }
});

el<HTMLButtonElement>("receive-copy").addEventListener("click", async () => {
  if (!address) return;
  await navigator.clipboard.writeText(address);
  const done = el<HTMLParagraphElement>("receive-copied");
  done.textContent = t("receive.copied");
  done.hidden = false;
});

el<HTMLInputElement>("send-amount").addEventListener("input", () => {
  void updatePlan();
});

el<HTMLButtonElement>("send").addEventListener("click", async () => {
  if (!address) return;

  const to = el<HTMLInputElement>("send-to").value.trim();
  const amount = el<HTMLInputElement>("send-amount").value.trim();
  if (!to || !amount) return;

  const total = plan?.total_lug ?? amount;
  if (!window.confirm(t("send.confirm", { total }))) return;

  // Desativar durante o envio evita um duplo clique a criar duas transações.
  const button = el<HTMLButtonElement>("send");
  button.disabled = true;
  try {
    const receipt = await call<SendReceipt>("wallet_send", { to, amountLug: amount });
    if (!receipt) return;

    el<HTMLInputElement>("send-to").value = "";
    el<HTMLInputElement>("send-amount").value = "";
    plan = null;
    renderPlan();

    const done = el<HTMLParagraphElement>("send-done");
    done.textContent = t("send.done", { hash: short(receipt.tx_hash) });
    done.hidden = false;

    await loadBalance();
    await loadHistory();
  } finally {
    button.disabled = false;
  }
});

// ---------------------------------------------------------------------------
// Interface com o resto da aplicação
// ---------------------------------------------------------------------------

/** Chamado quando a carteira abre (`address`) ou fecha (`null`). */
export function setWalletAddress(value: string | null): void {
  if (value === address) return; // nada mudou desde o último render

  address = value;
  walletView.hidden = value === null;
  if (value === null) {
    history = [];
    plan = null;
    renderHistory();
    renderPlan();
    return;
  }
  renderReceive();
  void loadBalance();
  void loadHistory();
}

/** Chamado quando o idioma muda. */
export function walletReload(): void {
  renderNode();
  renderPlan();
  renderHistory();

  // Mensagens de resultado foram escritas no idioma antigo: esconder é mais
  // honesto do que deixá-las desatualizadas.
  el<HTMLParagraphElement>("send-done").hidden = true;
  el<HTMLParagraphElement>("receive-copied").hidden = true;
}

/** Rebusca saldo e extrato (ex.: depois de uma mineração). */
export function walletRefresh(): void {
  if (!address) return;
  renderReceive();
  void loadBalance();
  void loadHistory();
}

/** Arranque: estado do nó (não precisa de carteira aberta). */
export function initWalletView(): void {
  void loadChain();
}
