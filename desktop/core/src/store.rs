//! Guarda de segredos do sistema operativo (keychain).
//!
//! ⚠️ **O keychain nunca guarda a seed.** Guarda uma **chave aleatória de 32
//! bytes** que fecha uma segunda cópia do mesmo conteúdo do cofre. É o
//! "abrir sem PIN", opcional e desligado por omissão: o que se ganha é
//! conveniência (a chave é protegida pelo login do sistema), o que se perde é
//! que **quem tiver a sessão do sistema aberta abre a carteira sem saber o PIN**.
//! Por isso é opt-in explícito, e o PIN continua a ser o caminho normal.

use crate::error::VaultError;

/// Nome do serviço no keychain (agrupa as entradas do Luggom).
pub const SERVICE: &str = "com.luggom.desktop";

/// Conta da chave que abre o cofre sem PIN.
pub const ACCOUNT: &str = "vault-unlock-key";

/// Acesso a uma guarda de segredos do sistema.
///
/// Existe como trait para que os testes corram sem um keychain real, e para que
/// uma máquina sem keychain (servidor, sessão sem D-Bus) possa falhar de forma
/// explícita em vez de rebentar.
pub trait SecretStore: Send + Sync {
    /// Devolve `Ok(None)` quando não há nada guardado para este par.
    fn get(&self, service: &str, account: &str) -> Result<Option<Vec<u8>>, VaultError>;

    fn set(&self, service: &str, account: &str, secret: &[u8]) -> Result<(), VaultError>;

    /// Apagar algo que não existe **não** é erro.
    fn delete(&self, service: &str, account: &str) -> Result<(), VaultError>;

    /// Se a guarda está utilizável nesta máquina.
    fn is_available(&self) -> bool {
        true
    }
}

/// O keychain real do sistema operativo.
#[derive(Debug, Default, Clone, Copy)]
pub struct OsKeychain;

impl SecretStore for OsKeychain {
    fn get(&self, service: &str, account: &str) -> Result<Option<Vec<u8>>, VaultError> {
        use base64::Engine;
        let entry = entry(service, account)?;
        match entry.get_password() {
            Ok(encoded) => {
                let secret = base64::engine::general_purpose::STANDARD
                    .decode(encoded)
                    .map_err(|e| VaultError::KeychainUnavailable(format!("entrada ilegível: {e}")))?;
                Ok(Some(secret))
            }
            Err(keyring::Error::NoEntry) => Ok(None),
            Err(e) => Err(VaultError::KeychainUnavailable(e.to_string())),
        }
    }

    fn set(&self, service: &str, account: &str, secret: &[u8]) -> Result<(), VaultError> {
        use base64::Engine;
        let encoded = base64::engine::general_purpose::STANDARD.encode(secret);
        entry(service, account)?
            .set_password(&encoded)
            .map_err(|e| VaultError::KeychainUnavailable(e.to_string()))
    }

    fn delete(&self, service: &str, account: &str) -> Result<(), VaultError> {
        match entry(service, account)?.delete_credential() {
            Ok(()) | Err(keyring::Error::NoEntry) => Ok(()),
            Err(e) => Err(VaultError::KeychainUnavailable(e.to_string())),
        }
    }

    fn is_available(&self) -> bool {
        // Uma leitura de uma conta que não existe distingue "não há keychain"
        // de "há keychain e não há entrada".
        entry(SERVICE, "__probe__").is_ok()
    }
}

fn entry(service: &str, account: &str) -> Result<keyring::Entry, VaultError> {
    keyring::Entry::new(service, account)
        .map_err(|e| VaultError::KeychainUnavailable(e.to_string()))
}

/// Guarda em memória, **para testes**. Não usar em produção.
#[derive(Debug, Default)]
pub struct InMemoryStore {
    entries: std::sync::Mutex<std::collections::HashMap<(String, String), Vec<u8>>>,
}

impl InMemoryStore {
    pub fn new() -> Self {
        Self::default()
    }

    /// Quantas entradas existem — para os testes confirmarem que nada ficou para trás.
    pub fn len(&self) -> usize {
        self.entries.lock().unwrap_or_else(|e| e.into_inner()).len()
    }

    pub fn is_empty(&self) -> bool {
        self.len() == 0
    }

    fn key(service: &str, account: &str) -> (String, String) {
        (service.to_string(), account.to_string())
    }
}

impl SecretStore for InMemoryStore {
    fn get(&self, service: &str, account: &str) -> Result<Option<Vec<u8>>, VaultError> {
        Ok(self
            .entries
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .get(&Self::key(service, account))
            .cloned())
    }

    fn set(&self, service: &str, account: &str, secret: &[u8]) -> Result<(), VaultError> {
        self.entries
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .insert(Self::key(service, account), secret.to_vec());
        Ok(())
    }

    fn delete(&self, service: &str, account: &str) -> Result<(), VaultError> {
        self.entries
            .lock()
            .unwrap_or_else(|e| e.into_inner())
            .remove(&Self::key(service, account));
        Ok(())
    }
}
