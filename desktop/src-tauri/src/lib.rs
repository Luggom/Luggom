//! Desktop Luggom — minerador + carteira com a seed.
//!
//! Camadas: `luggom-core` (cofre da seed) e `luggom-wallet` (derivação e
//! assinatura com o assinador Rust real). Este crate é só a ponte para a UI —
//! não tem criptografia própria.

mod chain_cmds;
mod collect_cmds;
mod commands;
mod error;
mod mine_cmds;
mod paths;
mod settings;
mod state;

pub use error::CommandError;
pub use state::AppState;

/// Arranca a aplicação.
pub fn run() {
    tauri::Builder::default()
        .manage(AppState::default())
        .setup(|app| {
            // Fica à escuta da captura que a extensão do Firefox envia
            // (`127.0.0.1:8099/collect`) — é por aqui que a sessão do banco entra.
            collect_cmds::spawn_listener(app.handle().clone());
            Ok(())
        })
        .invoke_handler(tauri::generate_handler![
            commands::signer_probe,
            commands::vault_status,
            commands::vault_create,
            commands::vault_import,
            commands::vault_unlock,
            commands::vault_delete,
            commands::vault_lock,
            commands::vault_export_backup,
            commands::vault_restore,
            commands::vault_unlock_keychain,
            commands::vault_enable_keychain,
            commands::vault_disable_keychain,
            chain_cmds::chain_node_url,
            chain_cmds::chain_set_node_url,
            chain_cmds::chain_info,
            chain_cmds::wallet_balance,
            chain_cmds::wallet_history,
            chain_cmds::mining_history,
            chain_cmds::wallet_send_plan,
            chain_cmds::wallet_send,
            mine_cmds::mine_providers,
            mine_cmds::mine_load_capture,
            mine_cmds::mine_run,
            mine_cmds::mine_decide,
            collect_cmds::bank_open_firefox,
            collect_cmds::bank_status,
            collect_cmds::bank_forget,
        ])
        .run(tauri::generate_context!())
        .expect("erro ao arrancar o desktop Luggom");
}
