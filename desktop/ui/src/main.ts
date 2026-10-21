//! Casca da aplicação: idioma, abas e o cofre da carteira.
//!
//! A seed nunca passa por aqui: criar/importar entram e a frase sai **uma vez**
//! (na criação); depois só se fala em endereços.

import { invoke } from "@tauri-apps/api/core";
import { setError } from "./errors";
import { applyTranslations, getLang, onLangChange, setLang, t, type Lang } from "./i18n";
import { initMineView, mineReload, mineSetLocked } from "./mine";
import { initWalletView, setWalletAddress, walletReload } from "./wallet";

type VaultStatus = {
  vault_path: string;
  exists: boolean;
  unlocked: boolean;
  address: string | null;
  attempts_left: number | null;
  locked_for_seconds: number | null;
  min_pin_length: number;
  keychain_available: boolean;
  keychain_enabled: boolean;
};

type CreatedVault = { mnemonic: string; address: string };
type AddressReply = { address: string };

const el = <T extends HTMLElement>(id: string): T => {
  const node = document.getElementById(id);
  if (!node) throw new Error(`elemento #${id} em falta`);
  return node as T;
};

const errorBox = el<HTMLParagraphElement>("error");
const vaultBox = el<HTMLDivElement>("vault");
const langBox = el<HTMLDivElement>("lang");
const tabs = el<HTMLElement>("tabs");

let status: VaultStatus | null = null;

/** Frase acabada de criar, à espera que o confirme. Só vive em memória. */
let pendingMnemonic: string | null = null;

// ---------------------------------------------------------------------------
// Abas
// ---------------------------------------------------------------------------

function showTab(name: string): void {
  for (const button of tabs.querySelectorAll<HTMLButtonElement>("button")) {
    button.classList.toggle("active", button.dataset.tab === name);
  }
  for (const panel of document.querySelectorAll<HTMLElement>("[data-panel]")) {
    panel.hidden = panel.dataset.panel !== name;
  }
}

tabs.addEventListener("click", (event) => {
  const button = (event.target as HTMLElement).closest("button");
  if (button?.dataset.tab) showTab(button.dataset.tab);
});

// Outras vistas podem pedir uma aba (ex.: minerar com a carteira fechada).
document.addEventListener("luggom:tab", (event) => {
  const name = (event as CustomEvent<string>).detail;
  if (name) showTab(name);
});

// ---------------------------------------------------------------------------
// Idioma
// ---------------------------------------------------------------------------

function renderLangSwitch(): void {
  langBox.innerHTML = "";
  for (const lang of ["pt", "en"] as Lang[]) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = lang.toUpperCase();
    button.lang = lang;
    button.className = lang === getLang() ? "active" : "";
    button.addEventListener("click", () => setLang(lang));
    langBox.append(button);
  }
}

onLangChange(() => {
  renderLangSwitch();
  render();
  walletReload();
  mineReload();
});

// ---------------------------------------------------------------------------
// Comandos
// ---------------------------------------------------------------------------

async function call<T>(cmd: string, args?: Record<string, unknown>): Promise<T | null> {
  setError(errorBox, null);
  try {
    return await invoke<T>(cmd, args);
  } catch (err) {
    setError(errorBox, err);
    return null;
  }
}

/** Comandos sem valor de retorno: `true` só quando o `invoke` não falhou. */
async function run(cmd: string, args?: Record<string, unknown>): Promise<boolean> {
  setError(errorBox, null);
  try {
    await invoke(cmd, args);
    return true;
  } catch (err) {
    setError(errorBox, err);
    return false;
  }
}

// ---------------------------------------------------------------------------
// Cofre
// ---------------------------------------------------------------------------

async function refresh(): Promise<void> {
  const fetched = await call<VaultStatus>("vault_status");
  if (!fetched) return;
  status = fetched;
  render();
}

function render(): void {
  const current = status;
  if (!current) return;

  // A vista da carteira e a mineração só existem com a carteira aberta.
  setWalletAddress(current.unlocked ? (current.address ?? null) : null);
  mineSetLocked(!current.unlocked, current.address);

  if (!current.exists) return renderCreate();
  if (!current.unlocked) return renderUnlock(current);
  renderUnlocked(current);
}

function renderCreate(): void {
  vaultBox.innerHTML = `
    <section class="card">
      <h2 data-i18n="vault.create.title"></h2>
      <p class="hint" data-i18n="vault.create.hint"></p>
      <div class="field">
        <label for="pin" data-i18n="password.label"></label>
        <input id="pin" type="password" autocomplete="new-password" />
      </div>
      <div class="field">
        <label for="pin2" data-i18n="password.repeat"></label>
        <input id="pin2" type="password" autocomplete="new-password" />
      </div>
      <button type="button" id="create" class="primary big" data-i18n="vault.create.action"></button>
      <details class="advanced">
        <summary data-i18n="vault.import.summary"></summary>
        <div class="field">
          <label for="mnemonic" data-i18n="vault.import.phrase"></label>
          <textarea id="mnemonic" rows="3" spellcheck="false" autocomplete="off"></textarea>
        </div>
        <button type="button" id="import" data-i18n="vault.import.action"></button>
      </details>
    </section>`;
  applyTranslations(vaultBox);

  el<HTMLButtonElement>("create").addEventListener("click", async () => {
    const pin = el<HTMLInputElement>("pin").value;
    const confirm = el<HTMLInputElement>("pin2").value;
    if (pin !== confirm) {
      errorBox.textContent = translationsMismatch();
      errorBox.hidden = false;
      return;
    }
    const created = await call<CreatedVault>("vault_create", { pin });
    if (!created) return;
    pendingMnemonic = created.mnemonic;
    await refresh();
  });

  el<HTMLButtonElement>("import").addEventListener("click", async () => {
    const pin = el<HTMLInputElement>("pin").value;
    const mnemonic = el<HTMLTextAreaElement>("mnemonic").value;
    if (await call<AddressReply>("vault_import", { pin, mnemonic })) {
      el<HTMLTextAreaElement>("mnemonic").value = "";
      await refresh();
    }
  });
}

function renderUnlock(vault: VaultStatus): void {
  const locked = vault.locked_for_seconds ?? 0;
  const blocked = locked > 0;

  const keychain =
    vault.keychain_enabled && vault.keychain_available
      ? `<button type="button" id="unlock-keychain" data-i18n="keychain.unlock"></button>`
      : "";

  vaultBox.innerHTML = `
    <section class="card">
      <h2 data-i18n="vault.unlock.title"></h2>
      <p class="hint" data-i18n="vault.unlock.hint"></p>
      <div class="field">
        <label for="pin" data-i18n="${blocked ? "password.blocked" : "password.label"}"></label>
        <input id="pin" type="password" autocomplete="current-password" />
      </div>
      <button type="button" id="unlock" class="primary big" data-i18n="vault.unlock.action"></button>
      ${keychain}
      <details class="advanced">
        <summary data-i18n="vault.backup.toggle"></summary>
        <div class="field">
          <label for="backup" data-i18n="vault.backup.path"></label>
          <input id="backup" type="text" spellcheck="false" />
        </div>
        <div class="field">
          <label for="backup-pin" data-i18n="vault.backup.pin"></label>
          <input id="backup-pin" type="password" autocomplete="off" />
        </div>
        <div class="field">
          <label for="new-pin" data-i18n="vault.backup.new_pin"></label>
          <input id="new-pin" type="password" autocomplete="new-password" />
        </div>
        <button type="button" id="restore" data-i18n="vault.backup.restore"></button>
      </details>
      <details class="advanced">
        <summary data-i18n="vault.delete.title"></summary>
        <p class="hint">${escapeHtml(t("vault.delete.hint"))}</p>
        <button type="button" id="delete-vault" data-i18n="vault.delete.action"></button>
      </details>
    </section>`;
  applyTranslations(vaultBox);

  if (blocked) {
    const label = vaultBox.querySelector("label[for=pin]");
    if (label) label.textContent = label.textContent?.replace("{n}", String(locked)) ?? "";
    el<HTMLInputElement>("pin").disabled = true;
    el<HTMLButtonElement>("unlock").disabled = true;
  }

  el<HTMLButtonElement>("unlock").addEventListener("click", async () => {
    const pin = el<HTMLInputElement>("pin").value;
    if (await call<AddressReply>("vault_unlock", { pin })) await refresh();
  });

  document.getElementById("unlock-keychain")?.addEventListener("click", async () => {
    if (await call<AddressReply>("vault_unlock_keychain")) await refresh();
  });

  el<HTMLButtonElement>("restore").addEventListener("click", async () => {
    const args = {
      backup: el<HTMLInputElement>("backup").value,
      backupPin: el<HTMLInputElement>("backup-pin").value,
      newPin: el<HTMLInputElement>("new-pin").value,
    };
    if (!args.backup) {
      errorBox.textContent = translationsBackupPath();
      errorBox.hidden = false;
      return;
    }
    if (await call<AddressReply>("vault_restore", args)) await refresh();
  });

  el<HTMLButtonElement>("delete-vault").addEventListener("click", async () => {
    const pin = el<HTMLInputElement>("pin").value;
    if (!pin) {
      errorBox.textContent = t("error.wrong_pin");
      errorBox.hidden = false;
      return;
    }
    if (!window.confirm(t("vault.delete.confirm"))) return;
    if (await run("vault_delete", { pin })) await refresh();
  });
}

function renderUnlocked(vault: VaultStatus): void {
  const warning = pendingMnemonic
    ? `<section class="card">
         <div class="notice">${escapeHtml(translationsCreatedWarning())}</div>
         <code class="mono block" id="pending-mnemonic"></code>
         <button type="button" id="dismiss" class="primary" data-i18n="vault.created.done"></button>
       </section>`
    : "";

  const keychain = vault.keychain_available
    ? `<details class="advanced">
         <summary data-i18n="keychain.title"></summary>
         <p class="hint">${escapeHtml(
           vault.keychain_enabled ? translationsKeychainOn() : translationsKeychainWarning(),
         )}</p>
         <div class="field">
           <label for="keychain-pin" data-i18n="password.label"></label>
           <input id="keychain-pin" type="password" autocomplete="current-password" />
         </div>
         <button type="button" id="keychain-toggle" data-i18n="${
           vault.keychain_enabled ? "keychain.disable" : "keychain.enable"
         }"></button>
       </details>`
    : "";

  vaultBox.innerHTML = `
    <section class="card">
      <div class="row">
        <button type="button" id="lock" data-i18n="vault.lock"></button>
        <span class="hint muted mono small" id="vault-address"></span>
      </div>
      <details class="advanced">
        <summary data-i18n="vault.export.title"></summary>
        <div class="field">
          <label for="backup-dest" data-i18n="vault.export.to"></label>
          <input id="backup-dest" type="text" spellcheck="false" placeholder="/caminho/backup.luggom" />
        </div>
        <div class="field">
          <label for="export-pin" data-i18n="password.label"></label>
          <input id="export-pin" type="password" autocomplete="current-password" />
        </div>
        <button type="button" id="export" data-i18n="vault.export.action"></button>
        <p id="exported" class="ok" hidden></p>
      </details>
      ${keychain}
    </section>
    ${warning}`;
  applyTranslations(vaultBox);

  el<HTMLElement>("vault-address").textContent = vault.address ?? "";

  if (pendingMnemonic) {
    el<HTMLElement>("pending-mnemonic").textContent = pendingMnemonic;
  }

  document.getElementById("dismiss")?.addEventListener("click", () => {
    pendingMnemonic = null;
    render();
  });

  el<HTMLButtonElement>("lock").addEventListener("click", async () => {
    await call<null>("vault_lock");
    pendingMnemonic = null;
    await refresh();
  });

  el<HTMLButtonElement>("export").addEventListener("click", async () => {
    const dest = el<HTMLInputElement>("backup-dest").value;
    if (!dest) {
      errorBox.textContent = translationsBackupPath();
      errorBox.hidden = false;
      return;
    }
    const pin = el<HTMLInputElement>("export-pin").value;
    if (await run("vault_export_backup", { pin, dest })) {
      el<HTMLInputElement>("export-pin").value = "";
      const done = el<HTMLParagraphElement>("exported");
      done.textContent = translationsExported();
      done.hidden = false;
    }
  });

  document.getElementById("keychain-toggle")?.addEventListener("click", async () => {
    const pin = el<HTMLInputElement>("keychain-pin").value;
    const command = vault.keychain_enabled ? "vault_disable_keychain" : "vault_enable_keychain";
    if (await run(command, { pin })) {
      el<HTMLInputElement>("keychain-pin").value = "";
      await refresh();
    }
  });
}

// Pequenos atalhos para textos que não estão em `data-i18n`.
const translationsMismatch = () => t("error.pins_mismatch");
const translationsBackupPath = () => t("error.backup_path");
const translationsCreatedWarning = () => t("vault.created.warning");
const translationsExported = () => t("vault.export.done");
const translationsKeychainOn = () => t("keychain.state.on");
const translationsKeychainWarning = () => t("keychain.warning");

function escapeHtml(text: string): string {
  const div = document.createElement("div");
  div.textContent = text;
  return div.innerHTML;
}

// ---------------------------------------------------------------------------
// Arranque
// ---------------------------------------------------------------------------

renderLangSwitch();
applyTranslations();
initWalletView();
initMineView();
showTab("wallet");
void refresh();
