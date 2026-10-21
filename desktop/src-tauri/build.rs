//! Build do desktop.
//!
//! Os comandos da app são declarados no manifesto para o **ACL** os poder
//! restringir: a partir daqui deixam de ser auto-permitidos e passam a exigir
//! uma permissão (`allow-<comando>`) numa capability.
//! - `capabilities/default.json` — a janela principal (todos, menos a captura);
//! - `capabilities/bank.json` — a janela do banco (página **remota**): **só**
//!   `bank_capture`.
//!
//! ⚠️ Um comando que não esteja nesta lista (ou sem permissão na sua janela)
//! passa a ser recusado em execução — foi o que aconteceu ao esquecer os
//! restantes. Manter esta lista alinhada com o `invoke_handler` do `lib.rs`.

const APP_COMMANDS: &[&str] = &[
    // cofre / assinador
    "signer_probe",
    "vault_status",
    "vault_create",
    "vault_import",
    "vault_unlock",
    "vault_delete",
    "vault_lock",
    "vault_export_backup",
    "vault_restore",
    "vault_unlock_keychain",
    "vault_enable_keychain",
    "vault_disable_keychain",
    // chain / carteira
    "chain_node_url",
    "chain_set_node_url",
    "chain_info",
    "wallet_balance",
    "wallet_history",
    "mining_history",
    "wallet_send_plan",
    "wallet_send",
    // mineração
    "mine_providers",
    "mine_load_capture",
    "mine_run",
    "mine_decide",
    // banco — pelo Firefox; a captura chega pela extensão
    "bank_open_firefox",
    "bank_status",
    "bank_forget",
];

fn main() {
    tauri_build::try_build(
        tauri_build::Attributes::new()
            .app_manifest(tauri_build::AppManifest::new().commands(APP_COMMANDS)),
    )
    .expect("falha no build do Tauri");
}
