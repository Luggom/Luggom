use std::path::PathBuf;
use thiserror::Error;

/// Erros do cofre.
#[derive(Debug, Error)]
pub enum VaultError {
    #[error("erro de I/O: {0}")]
    Io(#[from] std::io::Error),

    #[error("formato do cofre inválido: {0}")]
    Format(String),

    #[error("já existe um cofre em {0}")]
    AlreadyExists(PathBuf),

    #[error("não existe cofre em {0}")]
    NotFound(PathBuf),

    /// Falha de autenticação na abertura. Na prática é PIN errado; um cofre
    /// corrompido produz o mesmo erro (o AEAD não distingue os dois casos).
    #[error("PIN incorreto (ou cofre corrompido)")]
    WrongPin,

    #[error("cofre bloqueado; tente novamente em {0} s")]
    LockedOut(u64),

    #[error("mnemónico inválido: {0}")]
    InvalidMnemonic(String),

    #[error("PIN demasiado curto: mínimo {0} caracteres")]
    WeakPin(usize),

    #[error("parâmetros de KDF inválidos: {0}")]
    InvalidKdf(String),

    #[error("falha criptográfica: {0}")]
    Crypto(String),

    #[error("ficheiro não é um backup do Luggom")]
    NotABackup,

    #[error("keychain do sistema indisponível: {0}")]
    KeychainUnavailable(String),

    #[error("o desbloqueio pelo keychain não está ativado nesta carteira")]
    KeychainNotEnabled,
}

impl From<serde_json::Error> for VaultError {
    fn from(e: serde_json::Error) -> Self {
        VaultError::Format(format!("JSON: {e}"))
    }
}
