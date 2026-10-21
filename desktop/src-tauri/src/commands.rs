//! Comandos expostos à UI.
//!
//! A seed **nunca** atravessa a ponte: entra (criar/importar) e sai só uma vez,
//! na criação. Para o resto, o frontend fala em endereços e assinaturas.

use serde::Serialize;
use tauri::State;

use luggom_core::{
    restore_from_backup, OsKeychain, SecretStore, Vault, VaultConfig, MIN_PIN_LEN,
};
use luggom_wallet::Wallet;

use crate::error::CommandError;
use crate::paths::vault_path;
use crate::state::{AppState, Unlocked};

/// Mnemónico BIP39 público (o do standard). Serve para provar, sem tocar em
/// nenhuma carteira real, que a ponte chama **mesmo** o assinador do APK.
const PROBE_MNEMONIC: &str =
    "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";

#[derive(Debug, Serialize)]
pub struct ProbeInfo {
    pub address: String,
    pub public_key_hex: String,
}

/// Comando de teste do A1: deriva um endereço pelo `signer.rs` real.
///
/// O valor esperado está fixado no KAT contra o `luggomd` — se este endereço
/// mudar, a derivação do desktop divergiu da chain.
#[tauri::command]
pub fn signer_probe() -> Result<ProbeInfo, CommandError> {
    let wallet = Wallet::from_mnemonic(PROBE_MNEMONIC)?;
    Ok(ProbeInfo {
        address: wallet.address().to_string(),
        public_key_hex: hex(wallet.public_key()),
    })
}

#[derive(Debug, Serialize)]
pub struct VaultStatus {
    pub vault_path: String,
    pub exists: bool,
    pub unlocked: bool,
    pub address: Option<String>,
    /// Tentativas gratuitas antes do próximo bloqueio.
    pub attempts_left: Option<u32>,
    /// Segundos que faltam, se estiver bloqueado.
    pub locked_for_seconds: Option<u64>,
    /// Mínimo do PIN — a UI mostra-o no rótulo do campo.
    pub min_pin_length: usize,
    /// Esta máquina tem keychain utilizável?
    pub keychain_available: bool,
    /// A carteira tem o atalho "abrir sem PIN" ligado?
    pub keychain_enabled: bool,
}

#[derive(Debug, Serialize)]
pub struct CreatedVault {
    /// A frase. É a **única** vez que sai do processo — o utilizador tem de a guardar.
    pub mnemonic: String,
    pub address: String,
}

#[derive(Debug, Serialize)]
pub struct AddressReply {
    pub address: String,
}

#[tauri::command]
pub fn vault_status(state: State<'_, AppState>) -> Result<VaultStatus, CommandError> {
    let vault = Vault::at(vault_path());
    let exists = vault.exists();

    let (unlocked, address) = state.with(|slot| match slot {
        Some(open) => (true, Some(open.address().to_string())),
        None => (false, None),
    });

    let (attempts_left, locked_for_seconds, keychain_enabled) = if exists {
        let lockout = vault.lockout_state().map_err(CommandError::from)?;
        (
            Some(lockout.attempts_left()),
            lockout.seconds_remaining(now_unix()),
            vault.has_keychain_unlock().map_err(CommandError::from)?,
        )
    } else {
        (None, None, false)
    };

    Ok(VaultStatus {
        vault_path: vault.path().display().to_string(),
        exists,
        unlocked,
        address,
        attempts_left,
        locked_for_seconds,
        min_pin_length: MIN_PIN_LEN,
        keychain_available: OsKeychain.is_available(),
        keychain_enabled,
    })
}

#[tauri::command]
pub fn vault_create(pin: String, state: State<'_, AppState>) -> Result<CreatedVault, CommandError> {
    let vault = Vault::at(vault_path());
    let phrase = vault.create(&pin, VaultConfig::default())?;
    let wallet = Wallet::from_mnemonic(phrase.expose())?;

    let address = wallet.address().to_string();
    let mnemonic = phrase.expose().to_string();
    state.with(|slot| *slot = Some(Unlocked { phrase, wallet }));

    Ok(CreatedVault { mnemonic, address })
}

#[tauri::command]
pub fn vault_import(
    pin: String,
    mnemonic: String,
    state: State<'_, AppState>,
) -> Result<AddressReply, CommandError> {
    let vault = Vault::at(vault_path());
    let phrase = vault.import(&pin, &mnemonic, VaultConfig::default())?;
    let wallet = Wallet::from_mnemonic(phrase.expose())?;

    let address = wallet.address().to_string();
    state.with(|slot| *slot = Some(Unlocked { phrase, wallet }));

    Ok(AddressReply { address })
}

#[tauri::command]
pub fn vault_unlock(pin: String, state: State<'_, AppState>) -> Result<AddressReply, CommandError> {
    let vault = Vault::at(vault_path());
    let phrase = vault.unlock(&pin)?;
    let wallet = Wallet::from_mnemonic(phrase.expose())?;

    let address = wallet.address().to_string();
    state.with(|slot| *slot = Some(Unlocked { phrase, wallet }));

    Ok(AddressReply { address })
}

/// Apaga a carteira deste computador. Exige a senha: sem ela não se apaga o
/// cofre (a frase continua a ser a única forma de recuperar os fundos).
#[tauri::command]
pub fn vault_delete(pin: String, state: State<'_, AppState>) -> Result<(), CommandError> {
    let vault = Vault::at(vault_path());
    vault.delete(&pin)?;
    state.with(|slot| *slot = None);
    Ok(())
}

/// Fecha a sessão e larga a frase da memória (o `SecretPhrase` zera-se ao cair).
#[tauri::command]
pub fn vault_lock(state: State<'_, AppState>) -> Result<(), CommandError> {
    state.with(|slot| *slot = None);
    Ok(())
}

#[tauri::command]
pub fn vault_export_backup(pin: String, dest: String) -> Result<(), CommandError> {
    let vault = Vault::at(vault_path());
    vault
        .export_backup(&pin, dest, VaultConfig::default())
        .map_err(CommandError::from)
}

/// Recupera uma carteira a partir de um backup, com um PIN novo.
///
/// É o caminho do vector `T-APP-3`: exportar → apagar → importar, e o endereço
/// tem de voltar a ser o mesmo.
#[tauri::command]
pub fn vault_restore(
    backup: String,
    backup_pin: String,
    new_pin: String,
    state: State<'_, AppState>,
) -> Result<AddressReply, CommandError> {
    let phrase = restore_from_backup(
        &backup,
        &backup_pin,
        vault_path(),
        &new_pin,
        VaultConfig::default(),
    )?;
    let wallet = Wallet::from_mnemonic(phrase.expose())?;

    let address = wallet.address().to_string();
    state.with(|slot| *slot = Some(Unlocked { phrase, wallet }));

    Ok(AddressReply { address })
}

/// Abre o cofre com a chave guardada no keychain do sistema (sem PIN).
#[tauri::command]
pub fn vault_unlock_keychain(state: State<'_, AppState>) -> Result<AddressReply, CommandError> {
    let vault = Vault::at(vault_path());
    let phrase = vault.unlock_with_keychain(&OsKeychain)?;
    let wallet = Wallet::from_mnemonic(phrase.expose())?;

    let address = wallet.address().to_string();
    state.with(|slot| *slot = Some(Unlocked { phrase, wallet }));

    Ok(AddressReply { address })
}

/// Liga o atalho "abrir sem PIN" (opt-in; guarda uma chave, nunca a seed).
#[tauri::command]
pub fn vault_enable_keychain(pin: String) -> Result<(), CommandError> {
    let vault = Vault::at(vault_path());
    vault
        .enable_keychain_unlock(&pin, &OsKeychain)
        .map_err(CommandError::from)
}

/// Desliga o atalho e remove a chave do keychain.
#[tauri::command]
pub fn vault_disable_keychain(pin: String) -> Result<(), CommandError> {
    let vault = Vault::at(vault_path());
    vault
        .disable_keychain_unlock(&pin, &OsKeychain)
        .map_err(CommandError::from)
}

fn now_unix() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Aceite do A1: a ponte chama o assinador real e devolve um `luggom1…`,
    /// com o endereço do KAT contra o `luggomd`.
    #[test]
    fn signer_probe_returns_the_canonical_address() {
        let probe = signer_probe().unwrap();
        assert_eq!(
            probe.address,
            "luggom19rl4cm2hmr8afy4kldpxz3fka4jguq0amgkgeu"
        );
        assert_eq!(
            probe.public_key_hex,
            "024f4e2ad99c34d60b9ba6283c9431a8418af8673212961f97a77b6377fcd05b62"
        );
        assert!(probe.address.starts_with("luggom1"));
    }
}
