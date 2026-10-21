//! Cofre da seed e lógica sem GUI do desktop Luggom (Bloco 5, FASE A2).
//!
//! Este crate **não** depende de Tauri nem do `tlsn-mobile`: é Rust puro, para
//! poder ser testado sem `webkit2gtk`. Toda a criptografia da seed vive aqui.
//!
//! Regras de ouro (vector `T-APP-5`):
//! - a frase mnemónica **nunca** é escrita em claro no disco;
//! - **nunca** aparece em `Debug`/`Display` nem em logs;
//! - em memória vive dentro de [`SecretPhrase`], que se zera ao ser largada.

mod error;
mod kdf;
mod lockout;
mod secret;
mod store;
mod vault;

pub use error::VaultError;
pub use kdf::KdfParams;
pub use lockout::LockoutState;
pub use secret::SecretPhrase;
pub use store::{InMemoryStore, OsKeychain, SecretStore, ACCOUNT, SERVICE};
pub use vault::{restore_from_backup, Vault, VaultConfig, DEFAULT_HD_PATH, MIN_PIN_LEN};

/// Nº de palavras da carteira nova (24 = 256 bits de entropia).
pub const WORDS_DEFAULT: usize = 24;
