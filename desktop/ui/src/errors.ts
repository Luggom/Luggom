//! Tradução de erros do backend.
//!
//! O Rust devolve `kind` (estável) + campos estruturados; a mensagem em texto é
//! só o último recurso. É assim que os erros acompanham o idioma escolhido.

import { t, type Key } from "./i18n";

export type CommandError = {
  kind: string;
  message: string;
  retry_after_seconds?: number;
  min_pin_length?: number;
  mine_code?: number;
};

export function translateCommandError(
  error: Partial<CommandError>,
  fallbackKey: Key = "error.internal",
): string {
  switch (error.kind) {
    case "wrong_pin":
      return t("error.wrong_pin");
    case "locked_out":
      return error.retry_after_seconds !== undefined
        ? t("error.locked_out", { n: error.retry_after_seconds })
        : t(fallbackKey);
    case "weak_pin":
      return error.min_pin_length !== undefined
        ? t("error.weak_pin", { n: error.min_pin_length })
        : t(fallbackKey);
    case "already_exists":
      return t("error.already_exists");
    case "not_found":
      return t("error.not_found");
    case "invalid_mnemonic":
      return t("error.invalid_mnemonic");
    case "not_a_backup":
      return t("error.not_a_backup");
    case "keychain_unavailable":
      return t("error.keychain_unavailable");
    case "keychain_not_enabled":
      return t("error.keychain_not_enabled");
    case "locked":
      return t("error.locked");
    case "invalid_amount":
      return t("error.invalid_amount");
    case "invalid_address":
      return t("error.invalid_address");
    case "network":
      return t("error.network");
    case "insufficient_funds":
      return t("error.insufficient_funds");
    case "tx_rejected":
      return t("error.tx_rejected");
    case "invalid_key":
      return t("error.invalid_key");
    case "mine_rejected":
      return mineRejected(error.mine_code, error.message, fallbackKey);
    case "capture":
      return t("error.capture");
    default:
      return error.message ?? t(fallbackKey);
  }
}

/** A recusa da mineração traz um `code` da chain — dá-se-lhe texto próprio. */
function mineRejected(code: number | undefined, message: string | undefined, fallbackKey: Key): string {
  switch (code) {
    case 1202:
      return t("error.mine_1202");
    case 1206:
      return t("error.mine_1206");
    case 1207:
      return t("error.mine_1207");
    case 1208:
      return t("error.mine_1208");
    default:
      return code !== undefined ? t("error.mine_rejected", { n: code }) : (message ?? t(fallbackKey));
  }
}

/** Mostra o erro numa caixa; `null` limpa-a. */
export function setError(
  box: HTMLElement,
  error: unknown | null,
  fallbackKey: Key = "error.internal",
): void {
  if (!error) {
    box.hidden = true;
    box.textContent = "";
    return;
  }
  box.textContent = translateCommandError(error as Partial<CommandError>, fallbackKey);
  box.hidden = false;
}
