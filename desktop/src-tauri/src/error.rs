use luggom_core::VaultError;
use luggom_wallet::WalletError;
use serde::Serialize;

/// Erro devolvido à UI.
///
/// Tem `kind` legível por máquina: a UI escolhe a mensagem **e o idioma** a
/// partir dele, e usa `message` só como último recurso (`internal`).
#[derive(Debug, Serialize)]
pub struct CommandError {
    pub kind: &'static str,
    pub message: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub retry_after_seconds: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub min_pin_length: Option<usize>,
    /// Código de recusa da mineração (1202 anti-replay, 1206 gov, 1207 natureza, 1208 banco).
    #[serde(skip_serializing_if = "Option::is_none")]
    pub mine_code: Option<i32>,
}

impl CommandError {
    pub fn new(kind: &'static str, message: impl Into<String>) -> Self {
        Self {
            kind,
            message: message.into(),
            retry_after_seconds: None,
            min_pin_length: None,
            mine_code: None,
        }
    }

    pub fn internal(message: impl Into<String>) -> Self {
        Self::new("internal", message)
    }
}

impl From<VaultError> for CommandError {
    fn from(e: VaultError) -> Self {
        match e {
            VaultError::WrongPin => Self::new("wrong_pin", "PIN incorreto."),
            VaultError::LockedOut(seconds) => {
                let mut err = Self::new(
                    "locked_out",
                    format!("Bloqueado por tentativas erradas. Tente daqui a {seconds} s."),
                );
                err.retry_after_seconds = Some(seconds);
                err
            }
            VaultError::WeakPin(min) => {
                let mut err = Self::new(
                    "weak_pin",
                    format!("O PIN tem de ter pelo menos {min} caracteres."),
                );
                err.min_pin_length = Some(min);
                err
            }
            VaultError::AlreadyExists(_) => {
                Self::new("already_exists", "Já existe uma carteira neste computador.")
            }
            VaultError::NotFound(_) => {
                Self::new("not_found", "Não existe carteira neste computador.")
            }
            VaultError::InvalidMnemonic(_) => Self::new(
                "invalid_mnemonic",
                "A frase não é um mnemónico BIP39 válido.",
            ),
            VaultError::NotABackup => Self::new(
                "not_a_backup",
                "O ficheiro escolhido não é um backup do Luggom.",
            ),
            VaultError::KeychainUnavailable(_) => Self::new(
                "keychain_unavailable",
                "O keychain do sistema não está disponível.",
            ),
            VaultError::KeychainNotEnabled => Self::new(
                "keychain_not_enabled",
                "O desbloqueio pelo keychain não está ativado.",
            ),
            other => Self::internal(other.to_string()),
        }
    }
}

impl From<WalletError> for CommandError {
    fn from(e: WalletError) -> Self {
        match e {
            WalletError::InvalidAmount(message) => Self::new("invalid_amount", message),
            WalletError::InvalidAddress(address) => Self::new(
                "invalid_address",
                format!("Endereço inválido: {address}"),
            ),
            WalletError::Network(message) => Self::new("network", message),
            WalletError::UnexpectedResponse(message) => Self::new("network", message),
            WalletError::InvalidKey(message) => Self::new("invalid_key", message),
            WalletError::MineRejected { code, message } => {
                let mut err = Self::new("mine_rejected", message);
                err.mine_code = Some(code);
                err
            }
            WalletError::InvalidTxField { field, reason } => {
                Self::internal(format!("campo {field}: {reason}"))
            }
        }
    }
}
