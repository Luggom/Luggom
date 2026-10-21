//! Textos da interface, em português e inglês.
//!
//! O utilizador escolhe o idioma e a escolha fica gravada neste computador
//! (`localStorage`). Nada aqui é secreto — a seed nunca entra nesta camada.

export type Lang = "pt" | "en";

const PT = {
  "tab.wallet": "Carteira",
  "tab.mine": "Minerar",
  "tab.net": "Rede",

  "vault.create.title": "Criar carteira",
  "vault.create.hint":
    "A frase é gerada aqui e cifrada com a sua senha. Só a vê uma vez — guarde-a.",
  "vault.create.action": "Criar carteira",
  "vault.import.summary": "Já tenho uma frase",
  "vault.import.phrase": "Frase (12/24 palavras)",
  "vault.import.action": "Importar",
  "vault.unlock.title": "Entrar",
  "vault.unlock.hint": "A sua senha abre a carteira neste computador.",
  "vault.unlock.action": "Entrar",
  "vault.lock": "Sair",
  "vault.created.warning":
    "Esta é a única vez que a frase aparece. Escreva-a no papel e guarde-a fora deste computador.",
  "vault.created.done": "Já guardei a frase",
  "vault.backup.toggle": "Restaurar de um backup",
  "vault.backup.path": "Ficheiro de backup",
  "vault.backup.pin": "Senha do backup",
  "vault.backup.new_pin": "Senha nova",
  "vault.backup.restore": "Restaurar",
  "vault.export.title": "Exportar backup",
  "vault.export.to": "Guardar em",
  "vault.export.action": "Exportar",
  "vault.export.done": "Backup criado. Guarde-o fora deste computador.",
  "vault.delete.title": "Começar de novo",
  "vault.delete.hint":
    "Apaga a carteira deste computador. Só o consegue com a senha; sem a frase guardada, os fundos perdem-se.",
  "vault.delete.action": "Apagar carteira",
  "vault.delete.confirm":
    "Apagar a carteira deste computador? Uma carteira nova gera uma frase nova.",

  "password.label": "Senha",
  "password.repeat": "Repetir senha",
  "password.blocked": "Bloqueado — tente daqui a {n} s",

  "keychain.title": "Entrar sem senha",
  "keychain.warning":
    "Quem tiver a sessão deste computador aberta poderá entrar sem a senha. A frase continua a não ficar guardada.",
  "keychain.state.on": "Ativado neste computador.",
  "keychain.enable": "Ativar",
  "keychain.disable": "Desativar",
  "keychain.unlock": "Entrar com o sistema",

  "balance.title": "Saldo",
  "balance.none": "Ainda não há conta na chain.",

  "receive.title": "Receber",
  "receive.hint": "Este é o teu endereço. Partilha-o para receber LUG.",
  "receive.copy": "Copiar endereço",
  "receive.copied": "Endereço copiado.",

  "send.title": "Enviar",
  "send.to": "Para",
  "send.amount": "Valor (LUG)",
  "send.plan": "Valor {amount} · gás {gas} · taxa {fee} · total {total} LUG",
  "send.action": "Enviar",
  "send.confirm": "Confirmas? Saem {total} LUG da tua carteira.",
  "send.done": "Enviado. Transação {hash}",

  "history.title": "Extrato",
  "history.empty": "Sem movimentos.",
  "history.in": "Recebido de",
  "history.out": "Enviado para",

  "mine.title": "Minerar",
  "mine.hint":
    "Paga imposto e mina LUG. A prova do comprovante é feita aqui e assinada com a tua carteira — a API só monta a transação e transmite.",
  "mine.locked": "Entra na carteira (aba Carteira) para minerar.",
  "mine.wallet.open": "Carteira aberta: {address}",
  "mine.provider": "Banco",
  "mine.advanced": "Avançado (API e notário)",
  "mine.api": "API",
  "mine.notary": "Notário",
  "mine.bank.open": "Abrir o banco no Firefox",
  "mine.bank.forget": "Esquecer captura",
  "mine.bank.hint":
    "Abre o login no Firefox (é lá que a extensão coletora corre) e entra. Abre o comprovante do imposto — a captura chega sozinha ao app, sem ficheiros.",
  "mine.bank.no_extension":
    "A extensão coletora ainda não está no perfil do app ({profile}). Instala-a uma vez em about:debugging, ou coloca o .xpi assinado em ~/.luggom/.",
  "mine.capture": "Sessão por ficheiro (coletor)",
  "mine.capture.load": "Carregar",
  "mine.capture.hint":
    "Modo avançado: usa uma captura do coletor em vez da janela do banco. A app assina e envia o pedido na hora — o banco só aceita um carimbo de tempo fresco.",
  "mine.capture.ok": "Comprovante a pedir: {receipt}",
  "mine.run": "Minerar",
  "mine.working": "A gerar a prova… isto pode demorar.",
  "mine.providers.loading": "A carregar bancos…",
  "mine.providers.empty": "Sem bancos na whitelist.",
  "mine.providers.failed": "Não foi possível ler a whitelist de bancos.",
  "mine.approval.title": "Confirma os dados a revelar",
  "mine.approval.status": "Resposta do banco: HTTP {status}",
  "mine.approval.approve": "Confirmar e minerar",
  "mine.approval.reject": "Cancelar",
  "mine.cancelled": "Cancelado. Nada foi revelado.",
  "mine.done": "Minerado (valor {amount}) · transação {hash}",
  "mine.done.plain": "Minerado. Transação {hash}",
  "mine.done.detail": "Minerado: {amount} {currency} → {lug} LUG · {hash}",
  "mine.history.title": "Minerações",
  "mine.history.hint":
    "O que pagaste de imposto e quanto entrou na tua carteira em LUG. Os valores são lidos da chain.",
  "mine.history.empty": "Ainda não minaste nenhum imposto.",

  "chain.title": "Rede",
  "chain.hint": "O nó a que a carteira se liga (REST/LCD).",
  "chain.node": "Nó",
  "chain.save": "Ligar",
  "chain.connected": "Ligado a {chain} · altura {height}",
  "chain.disconnected": "Sem ligação ao nó.",

  "error.pins_mismatch": "As senhas não coincidem.",
  "error.backup_path": "Indique o caminho do ficheiro.",
  "error.wrong_pin": "Senha incorreta.",
  "error.locked_out": "Bloqueado por tentativas erradas. Tente daqui a {n} s.",
  "error.weak_pin": "A senha tem de ter pelo menos {n} caracteres.",
  "error.already_exists": "Já existe uma carteira neste computador.",
  "error.not_found": "Não existe carteira neste computador.",
  "error.invalid_mnemonic": "A frase não é um mnemónico BIP39 válido.",
  "error.not_a_backup": "O ficheiro escolhido não é um backup do Luggom.",
  "error.keychain_unavailable": "O keychain deste computador não está disponível.",
  "error.keychain_not_enabled": "A entrada pelo sistema não está ativada.",
  "error.locked": "A carteira está fechada.",
  "error.invalid_amount": "Valor inválido.",
  "error.invalid_address": "Endereço inválido.",
  "error.network": "Não foi possível falar com o nó.",
  "error.insufficient_funds": "Saldo insuficiente para o valor mais as taxas.",
  "error.tx_rejected": "A chain recusou a transação.",
  "error.invalid_key": "Chave inválida.",
  "error.internal": "Erro inesperado.",
  "error.mine_rejected": "A mineração foi recusada ({n}).",
  "error.mine_capture": "Indique a captura da sessão do banco.",
  "error.bank_unsupported": "Este banco ainda não tem janela dentro da app.",
  "error.capture":
    "A captura ainda não tem uma sessão do banco utilizável. Abre o banco na app, entra, abre o comprovante do imposto e volta a tentar.",
  "error.mine_1202": "Este comprovante já foi minado. Cada um mina uma só vez.",
  "error.mine_1206": "Entidades governamentais não podem minerar LUG.",
  "error.mine_1207": "O comprovante não é uma arrecadação de imposto.",
  "error.mine_1208": "O banco está fora da whitelist on-chain.",
} as const;

export type Key = keyof typeof PT;

const EN: Record<Key, string> = {
  "tab.wallet": "Wallet",
  "tab.mine": "Mine",
  "tab.net": "Network",

  "vault.create.title": "Create wallet",
  "vault.create.hint":
    "The phrase is generated here and encrypted with your password. You only see it once — write it down.",
  "vault.create.action": "Create wallet",
  "vault.import.summary": "I already have a phrase",
  "vault.import.phrase": "Phrase (12/24 words)",
  "vault.import.action": "Import",
  "vault.unlock.title": "Unlock",
  "vault.unlock.hint": "Your password opens the wallet on this computer.",
  "vault.unlock.action": "Unlock",
  "vault.lock": "Lock",
  "vault.created.warning":
    "This is the only time the phrase appears. Write it on paper and keep it off this computer.",
  "vault.created.done": "I have written it down",
  "vault.backup.toggle": "Restore from a backup",
  "vault.backup.path": "Backup file",
  "vault.backup.pin": "Backup password",
  "vault.backup.new_pin": "New password",
  "vault.backup.restore": "Restore",
  "vault.export.title": "Export backup",
  "vault.export.to": "Save to",
  "vault.export.action": "Export",
  "vault.export.done": "Backup created. Keep it off this computer.",
  "vault.delete.title": "Start over",
  "vault.delete.hint":
    "Deletes the wallet on this computer. Only possible with the password; without the saved phrase the funds are lost.",
  "vault.delete.action": "Delete wallet",
  "vault.delete.confirm":
    "Delete the wallet on this computer? A new wallet generates a new phrase.",

  "password.label": "Password",
  "password.repeat": "Repeat password",
  "password.blocked": "Locked — try again in {n} s",

  "keychain.title": "Unlock without password",
  "keychain.warning":
    "Anyone with this computer's session unlocked will be able to open the wallet without the password. The phrase is still not stored.",
  "keychain.state.on": "Enabled on this computer.",
  "keychain.enable": "Enable",
  "keychain.disable": "Disable",
  "keychain.unlock": "Unlock with system",

  "balance.title": "Balance",
  "balance.none": "No account on the chain yet.",

  "receive.title": "Receive",
  "receive.hint": "This is your address. Share it to receive LUG.",
  "receive.copy": "Copy address",
  "receive.copied": "Address copied.",

  "send.title": "Send",
  "send.to": "To",
  "send.amount": "Amount (LUG)",
  "send.plan": "Amount {amount} · gas {gas} · fee {fee} · total {total} LUG",
  "send.action": "Send",
  "send.confirm": "Confirm? {total} LUG will leave your wallet.",
  "send.done": "Sent. Transaction {hash}",

  "history.title": "Statement",
  "history.empty": "No movements.",
  "history.in": "Received from",
  "history.out": "Sent to",

  "mine.title": "Mine",
  "mine.hint":
    "Pay tax and mine LUG. The receipt proof is made here and signed with your wallet — the API only builds the transaction and broadcasts it.",
  "mine.locked": "Unlock the wallet (Wallet tab) to mine.",
  "mine.wallet.open": "Wallet unlocked: {address}",
  "mine.provider": "Bank",
  "mine.advanced": "Advanced (API and notary)",
  "mine.api": "API",
  "mine.notary": "Notary",
  "mine.bank.open": "Open the bank in Firefox",
  "mine.bank.forget": "Forget capture",
  "mine.bank.hint":
    "Opens the login in Firefox (where the collector extension runs) and you log in. Open the tax receipt — the capture reaches the app on its own, no files.",
  "mine.bank.no_extension":
    "The collector extension is not in the app profile yet ({profile}). Install it once from about:debugging, or drop the signed .xpi in ~/.luggom/.",
  "mine.capture": "Session from file (collector)",
  "mine.capture.load": "Load",
  "mine.capture.hint":
    "Advanced mode: use a collector capture instead of the bank window. The app signs and sends the request on the spot — the bank only accepts a fresh timestamp.",
  "mine.capture.ok": "Receipt to request: {receipt}",
  "mine.run": "Mine",
  "mine.working": "Generating the proof… this can take a while.",
  "mine.providers.loading": "Loading banks…",
  "mine.providers.empty": "No banks in the whitelist.",
  "mine.providers.failed": "Could not read the bank whitelist.",
  "mine.approval.title": "Confirm the data to reveal",
  "mine.approval.status": "Bank response: HTTP {status}",
  "mine.approval.approve": "Confirm and mine",
  "mine.approval.reject": "Cancel",
  "mine.cancelled": "Cancelled. Nothing was revealed.",
  "mine.done": "Mined (value {amount}) · transaction {hash}",
  "mine.done.plain": "Mined. Transaction {hash}",
  "mine.done.detail": "Mined: {amount} {currency} → {lug} LUG · {hash}",
  "mine.history.title": "Minings",
  "mine.history.hint":
    "What you paid in tax and how much reached your wallet in LUG. The figures are read from the chain.",
  "mine.history.empty": "You have not mined any tax yet.",

  "chain.title": "Network",
  "chain.hint": "The node the wallet connects to (REST/LCD).",
  "chain.node": "Node",
  "chain.save": "Connect",
  "chain.connected": "Connected to {chain} · height {height}",
  "chain.disconnected": "No connection to the node.",

  "error.pins_mismatch": "The passwords do not match.",
  "error.backup_path": "Provide the file path.",
  "error.wrong_pin": "Wrong password.",
  "error.locked_out": "Locked after failed attempts. Try again in {n} s.",
  "error.weak_pin": "The password must be at least {n} characters.",
  "error.already_exists": "A wallet already exists on this computer.",
  "error.not_found": "There is no wallet on this computer.",
  "error.invalid_mnemonic": "That phrase is not a valid BIP39 mnemonic.",
  "error.not_a_backup": "The chosen file is not a Luggom backup.",
  "error.keychain_unavailable": "This computer's keychain is not available.",
  "error.keychain_not_enabled": "Unlocking through the system is not enabled.",
  "error.locked": "The wallet is locked.",
  "error.invalid_amount": "Invalid amount.",
  "error.invalid_address": "Invalid address.",
  "error.network": "Could not reach the node.",
  "error.insufficient_funds": "Not enough balance for the amount plus fees.",
  "error.tx_rejected": "The chain rejected the transaction.",
  "error.invalid_key": "Invalid key.",
  "error.internal": "Unexpected error.",
  "error.mine_rejected": "Mining was rejected ({n}).",
  "error.mine_capture": "Provide the bank session capture.",
  "error.bank_unsupported": "This bank does not have an in-app window yet.",
  "error.capture":
    "The capture does not have a usable bank session yet. Open the bank in the app, log in, open the tax receipt and try again.",
  "error.mine_1202": "This receipt has already been mined. Each one mines only once.",
  "error.mine_1206": "Government entities cannot mine LUG.",
  "error.mine_1207": "The receipt is not a tax collection.",
  "error.mine_1208": "The bank is not in the on-chain whitelist.",
};

const DICTS: Record<Lang, Record<Key, string>> = { pt: PT, en: EN };

const STORAGE_KEY = "luggom.lang";

function detect(): Lang {
  const saved = localStorage.getItem(STORAGE_KEY);
  if (saved === "pt" || saved === "en") return saved;
  return navigator.language?.toLowerCase().startsWith("en") ? "en" : "pt";
}

let current: Lang = detect();
const listeners = new Set<() => void>();

export function getLang(): Lang {
  return current;
}

export function setLang(lang: Lang): void {
  if (lang === current) return;
  current = lang;
  localStorage.setItem(STORAGE_KEY, lang);
  document.documentElement.lang = lang;
  applyTranslations();
  for (const listener of listeners) listener();
}

export function onLangChange(listener: () => void): void {
  listeners.add(listener);
}

type Vars = Record<string, string | number>;

/** Texto de `key`, com `{n}` substituído pelos valores de `vars`. */
export function t(key: Key, vars?: Vars): string {
  let text: string = DICTS[current][key];
  if (vars) {
    for (const [name, value] of Object.entries(vars)) {
      text = text.replaceAll(`{${name}}`, String(value));
    }
  }
  return text;
}

/** Traduz os elementos marcados com `data-i18n` / `data-i18n-placeholder`. */
export function applyTranslations(root: ParentNode = document): void {
  for (const node of root.querySelectorAll<HTMLElement>("[data-i18n]")) {
    node.textContent = t(node.dataset.i18n as Key);
  }
  for (const node of root.querySelectorAll<HTMLElement>("[data-i18n-placeholder]")) {
    (node as HTMLInputElement).placeholder = t(node.dataset.i18nPlaceholder as Key);
  }
}

document.documentElement.lang = current;
