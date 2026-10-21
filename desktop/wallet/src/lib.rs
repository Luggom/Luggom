//! Camada de carteira do desktop Luggom.
//!
//! **Não reimplementa criptografia.** Chama o assinador Rust real
//! (`apk_luggom/native/tlsn-mobile/src/signer.rs`), o mesmo código que assinou
//! as minerações on-chain do APK. Um desvio na derivação mandaria o LUG
//! minerado para um endereço diferente do esperado — há um KAT contra o
//! `luggomd` a guardar isso.
//!
//! Depende do `tlsn-mobile`, mas **não** do Tauri: assim pode ser testado sem
//! `webkit2gtk` (o `webkit2gtk` só é preciso para compilar o crate `tauri`).

mod address;
pub mod amount;
pub mod chain;
pub mod cosmos;
pub mod mine;
pub mod prover;
pub mod send;
pub mod viacredi;

use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;

pub use address::{is_valid_address, HRP};
pub use amount::{lug_to_uluggom, uluggom_to_lug, DECIMALS, DENOM, DISPLAY_DENOM};
pub use chain::{AccountInfo, BroadcastResult, ChainClient, Direction, Transfer};
pub use mine::{
    MineBundle, MineClient, MinePreparation, MineReceipt, MinedResult, ProviderInfo,
};
pub use prover::{ProveDecision, ProveOutcome, ProvePreview, ProveRequest, RevealDescriptor};
pub use send::{build_send_tx, SendPlan, FLAT_MOVE_FEE_ULUGGOM, SEND_GAS_LIMIT};
pub use tlsn_mobile::signer::WalletInfo;

/// Caminho BIP44 padrão do Cosmos SDK.
pub const DEFAULT_HD_PATH: &str = "m/44'/118'/0'/0/0";

#[derive(Debug, thiserror::Error)]
pub enum WalletError {
    #[error("chave/mnemónico inválido: {0}")]
    InvalidKey(String),

    #[error("campo de transação inválido ({field}): {reason}")]
    InvalidTxField { field: &'static str, reason: String },

    #[error("valor inválido: {0}")]
    InvalidAmount(String),

    #[error("endereço inválido: {0}")]
    InvalidAddress(String),

    #[error("a rede não respondeu: {0}")]
    Network(String),

    #[error("resposta inesperada da rede: {0}")]
    UnexpectedResponse(String),

    #[error("mineração recusada ({code}): {message}")]
    MineRejected { code: i32, message: String },
}

impl From<tlsn_mobile::TlsnError> for WalletError {
    fn from(e: tlsn_mobile::TlsnError) -> Self {
        WalletError::InvalidKey(e.to_string())
    }
}

/// Transação não assinada, tal como a API a devolve (Opção 1, §13.97:
/// a API monta com `--generate-only`, o aparelho assina localmente).
#[derive(Debug, Clone, serde::Deserialize)]
pub struct UnsignedTx {
    /// O documento a assinar, em base64 (`sign_doc_bytes` na API).
    pub sign_doc_bytes: String,
    /// `TxBody` serializado, em base64.
    pub body_bytes: String,
    /// `AuthInfo` serializado, em base64.
    pub auth_info_bytes: String,
    /// `TxRaw` sem assinaturas, em base64 (opcional).
    #[serde(default)]
    pub tx_bytes: Option<String>,
    #[serde(default)]
    pub chain_id: Option<String>,
    #[serde(default)]
    pub account_number: Option<u64>,
    #[serde(default)]
    pub sequence: Option<u64>,
}

/// Carteira pronta a usar, derivada de uma frase.
///
/// A frase **não** é guardada aqui: quem a tem é o cofre (`luggom-core`).
/// Esta estrutura só carrega a chave pública e o endereço, que não são segredo.
pub struct Wallet {
    info: WalletInfo,
}

impl Wallet {
    /// Deriva a carteira a partir do mnemónico (o caminho padrão do Cosmos SDK).
    pub fn from_mnemonic(mnemonic: &str) -> Result<Self, WalletError> {
        Self::from_mnemonic_at(mnemonic, DEFAULT_HD_PATH)
    }

    pub fn from_mnemonic_at(mnemonic: &str, hd_path: &str) -> Result<Self, WalletError> {
        let info = tlsn_mobile::signer::derive_wallet(mnemonic.to_string(), hd_path.to_string())?;
        Ok(Self { info })
    }

    /// Endereço bech32 (`luggom1…`).
    pub fn address(&self) -> &str {
        &self.info.address
    }

    /// Chave pública secp256k1 comprimida (33 bytes).
    pub fn public_key(&self) -> &[u8] {
        &self.info.public_key
    }

    /// Assina a transação devolvida pela API e devolve o `TxRaw` (protobuf).
    ///
    /// A seed nunca sai do processo: só a assinatura é devolvida.
    pub fn sign(&self, mnemonic: &str, tx: &UnsignedTx) -> Result<Vec<u8>, WalletError> {
        let sign_doc = decode_field(&tx.sign_doc_bytes, "sign_doc_bytes")?;
        let body_bytes = decode_field(&tx.body_bytes, "body_bytes")?;
        let auth_info_bytes = decode_field(&tx.auth_info_bytes, "auth_info_bytes")?;

        let raw = tlsn_mobile::signer::sign_tx(
            mnemonic.to_string(),
            DEFAULT_HD_PATH.to_string(),
            sign_doc,
            body_bytes,
            auth_info_bytes,
        )?;
        Ok(raw)
    }
}

fn decode_field(value: &str, field: &'static str) -> Result<Vec<u8>, WalletError> {
    B64.decode(value).map_err(|e| WalletError::InvalidTxField {
        field,
        reason: e.to_string(),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    const MNEMONIC: &str =
        "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";

    /// KAT contra o Cosmos SDK real (`luggomd keys add --recover`).
    #[test]
    fn derivation_matches_luggomd() {
        let wallet = Wallet::from_mnemonic(MNEMONIC).unwrap();
        assert_eq!(
            wallet.address(),
            "luggom19rl4cm2hmr8afy4kldpxz3fka4jguq0amgkgeu"
        );
        assert_eq!(wallet.public_key().len(), 33);
    }

    #[test]
    fn invalid_mnemonic_is_rejected() {
        assert!(Wallet::from_mnemonic("não é um mnemónico").is_err());
    }
}
