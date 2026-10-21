//! Assinatura local da carteira Luggom (Cosmos SDK, secp256k1).
//!
//! Implementa o lado do cliente da "OPÇÃO 1" (§13.97 / F2 §4): a API devolve a
//! transação NÃO assinada (sign-doc + body_bytes + auth_info_bytes) e o aparelho
//! assina aqui. A seed/mnemonic nunca sai do dispositivo — só a chave pública e
//! a assinatura são enviadas.
//!
//! Formato Cosmos:
//!   - endereço   = bech32(ripemd160(sha256(pubkey_comprimida)), hrp="luggom")
//!   - assinatura = ECDSA secp256k1 (RFC6979, low-S) sobre SHA-256(sign_doc)
//!   - TxRaw      = { body_bytes, auth_info_bytes, signatures: [sig] }

use k256::ecdsa::signature::hazmat::PrehashSigner;
use k256::ecdsa::SigningKey;
use ripemd::Ripemd160;
use sha2::{Digest, Sha256};

use crate::TlsnError;

/// Carteira derivada de um mnemonic.
#[derive(Debug, Clone, uniffi::Record)]
pub struct WalletInfo {
    /// Endereço bech32 (luggom1...).
    pub address: String,
    /// Chave pública secp256k1 comprimida (33 bytes).
    pub public_key: Vec<u8>,
}

/// Deriva a carteira (endereço + chave pública) a partir do mnemonic.
///
/// `hd_path` é o caminho BIP44 (padrão Cosmos: `m/44'/118'/0'/0/0`). A seed usa
/// passphrase vazia, como no Cosmos SDK.
#[uniffi::export]
pub fn derive_wallet(mnemonic: String, hd_path: String) -> Result<WalletInfo, TlsnError> {
    let signing_key = derive_signing_key(&mnemonic, &hd_path)?;
    let public_key = compressed_public_key(&signing_key);

    Ok(WalletInfo {
        address: address_from_public_key(&public_key)?,
        public_key,
    })
}

/// Assina o sign-doc e devolve o `TxRaw` (protobuf) pronto a transmitir.
///
/// `sign_doc` é o que a API devolveu em `unsigned.sign_doc_bytes` (decodificado
/// de base64). `body_bytes` e `auth_info_bytes` são os componentes da mesma tx.
/// A assinatura é colocada no campo `signatures` do TxRaw.
#[uniffi::export]
pub fn sign_tx(
    mnemonic: String,
    hd_path: String,
    sign_doc: Vec<u8>,
    body_bytes: Vec<u8>,
    auth_info_bytes: Vec<u8>,
) -> Result<Vec<u8>, TlsnError> {
    let signing_key = derive_signing_key(&mnemonic, &hd_path)?;
    let signature = sign_sign_doc(&signing_key, &sign_doc)?;
    Ok(encode_tx_raw(&body_bytes, &auth_info_bytes, &signature))
}

// ---------------------------------------------------------------------------
// Interno
// ---------------------------------------------------------------------------

/// Deriva a chave de assinatura pelo caminho BIP44/BIP32.
fn derive_signing_key(mnemonic: &str, hd_path: &str) -> Result<SigningKey, TlsnError> {
    let mnemonic = bip39::Mnemonic::parse(mnemonic.trim())
        .map_err(|e| TlsnError::InvalidConfig(format!("mnemonic inválido: {e}")))?;
    let seed = mnemonic.to_seed("");

    let path: bip32::DerivationPath = hd_path
        .trim()
        .parse()
        .map_err(|e| TlsnError::InvalidConfig(format!("hd_path inválido ({hd_path}): {e}")))?;

    let xprv = bip32::XPrv::derive_from_path(seed, &path)
        .map_err(|e| TlsnError::InvalidConfig(format!("derivação BIP32 falhou: {e}")))?;

    // Com a feature `secp256k1` do bip32, `private_key()` já devolve a chave de
    // assinatura do k256 — não é preciso reconstruí-la a partir dos bytes.
    Ok(xprv.private_key().clone())
}

/// Chave pública comprimida (33 bytes: prefixo 0x02/0x03 + X).
fn compressed_public_key(signing_key: &SigningKey) -> Vec<u8> {
    let point = signing_key.verifying_key().to_encoded_point(true);
    point.as_bytes().to_vec()
}

/// Endereço Cosmos: bech32(ripemd160(sha256(pubkey))).
fn address_from_public_key(public_key: &[u8]) -> Result<String, TlsnError> {
    let sha = Sha256::digest(public_key);
    let hash = Ripemd160::digest(sha);

    let hrp = bech32::Hrp::parse("luggom")
        .map_err(|e| TlsnError::InvalidConfig(format!("prefixo bech32 inválido: {e}")))?;
    bech32::encode::<bech32::Bech32>(hrp, &hash)
        .map_err(|e| TlsnError::InvalidConfig(format!("bech32 encode falhou: {e}")))
}

/// Assina SHA-256(sign_doc) com ECDSA secp256k1 (RFC6979, low-S), devolvendo r||s.
fn sign_sign_doc(signing_key: &SigningKey, sign_doc: &[u8]) -> Result<Vec<u8>, TlsnError> {
    let digest = Sha256::digest(sign_doc);
    let signature: k256::ecdsa::Signature = signing_key
        .sign_prehash(&digest)
        .map_err(|e| TlsnError::ProofFailed(format!("assinatura falhou: {e}")))?;
    Ok(signature.to_bytes().to_vec())
}

/// Codifica o `TxRaw` do Cosmos (protobuf):
///   1: body_bytes, 2: auth_info_bytes, 3: repeated signatures.
fn encode_tx_raw(body_bytes: &[u8], auth_info_bytes: &[u8], signature: &[u8]) -> Vec<u8> {
    let mut out = Vec::with_capacity(body_bytes.len() + auth_info_bytes.len() + signature.len() + 16);
    write_bytes_field(&mut out, 1, body_bytes);
    write_bytes_field(&mut out, 2, auth_info_bytes);
    write_bytes_field(&mut out, 3, signature);
    out
}

/// Escreve um campo protobuf length-delimited (wire type 2).
fn write_bytes_field(out: &mut Vec<u8>, field_number: u32, value: &[u8]) {
    write_varint(out, ((field_number as u64) << 3) | 2);
    write_varint(out, value.len() as u64);
    out.extend_from_slice(value);
}

/// Escreve um varint protobuf.
fn write_varint(out: &mut Vec<u8>, mut value: u64) {
    while value >= 0x80 {
        out.push((value as u8) | 0x80);
        value >>= 7;
    }
    out.push(value as u8);
}

#[cfg(test)]
mod tests {
    use super::*;

    // Mnemonic BIP39 padrão de teste.
    const MNEMONIC: &str =
        "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";
    const HD_PATH: &str = "m/44'/118'/0'/0/0";

    #[test]
    fn derives_compressed_public_key() {
        let info = derive_wallet(MNEMONIC.to_string(), HD_PATH.to_string()).unwrap();
        assert_eq!(info.public_key.len(), 33);
        assert!(info.public_key[0] == 0x02 || info.public_key[0] == 0x03);
        assert!(info.address.starts_with("luggom1"), "got {}", info.address);
    }

    #[test]
    fn derivation_is_deterministic() {
        let a = derive_wallet(MNEMONIC.to_string(), HD_PATH.to_string()).unwrap();
        let b = derive_wallet(MNEMONIC.to_string(), HD_PATH.to_string()).unwrap();
        assert_eq!(a.address, b.address);
        assert_eq!(a.public_key, b.public_key);
    }

    #[test]
    fn different_paths_give_different_keys() {
        let a = derive_wallet(MNEMONIC.to_string(), HD_PATH.to_string()).unwrap();
        let b = derive_wallet(MNEMONIC.to_string(), "m/44'/118'/0'/0/1".to_string()).unwrap();
        assert_ne!(a.address, b.address);
    }

    #[test]
    fn signature_verifies_against_sign_doc_hash() {
        use k256::ecdsa::signature::hazmat::PrehashVerifier;

        let key = derive_signing_key(MNEMONIC, HD_PATH).unwrap();
        let sign_doc = b"sign-doc-de-teste";
        let sig_bytes = sign_sign_doc(&key, sign_doc).unwrap();
        assert_eq!(sig_bytes.len(), 64);

        // A chain verifica ECDSA sobre SHA-256(sign_doc) como PREHASH.
        let digest = Sha256::digest(sign_doc);
        let sig = k256::ecdsa::Signature::from_slice(&sig_bytes).unwrap();
        assert!(key.verifying_key().verify_prehash(&digest, &sig).is_ok());

        // E uma assinatura de outro sign-doc NÃO pode validar.
        let other = Sha256::digest(b"outro-sign-doc");
        assert!(key.verifying_key().verify_prehash(&other, &sig).is_err());
    }

    #[test]
    fn rejects_invalid_mnemonic() {
        assert!(derive_wallet("não é um mnemonic".to_string(), HD_PATH.to_string()).is_err());
    }

    /// KAT contra o Cosmos SDK real: os valores abaixo foram produzidos por
    /// `luggomd keys add --recover` com o mesmo mnemonic e o mesmo hd_path.
    /// Se este teste falhar, a derivação do aparelho NÃO coincide com a chain
    /// e o LUG minerado iria para um endereço diferente do esperado.
    #[test]
    fn matches_luggomd_key_derivation() {
        const EXPECTED_ADDRESS: &str = "luggom19rl4cm2hmr8afy4kldpxz3fka4jguq0amgkgeu";
        const EXPECTED_PUBKEY_HEX: &str =
            "024f4e2ad99c34d60b9ba6283c9431a8418af8673212961f97a77b6377fcd05b62";

        let info = derive_wallet(MNEMONIC.to_string(), HD_PATH.to_string()).unwrap();
        assert_eq!(info.address, EXPECTED_ADDRESS);
        assert_eq!(hex_encode(&info.public_key), EXPECTED_PUBKEY_HEX);
    }

    fn hex_encode(bytes: &[u8]) -> String {
        bytes.iter().map(|b| format!("{b:02x}")).collect()
    }

    #[test]
    fn tx_raw_has_expected_wire_format() {
        let body = vec![0xAA, 0xBB];
        let auth = vec![0xCC];
        let sig = vec![0x01, 0x02, 0x03];
        let raw = encode_tx_raw(&body, &auth, &sig);

        // field1 (0x0a) len2 body, field2 (0x12) len1 auth, field3 (0x1a) len3 sig
        assert_eq!(
            raw,
            vec![0x0a, 0x02, 0xAA, 0xBB, 0x12, 0x01, 0xCC, 0x1a, 0x03, 0x01, 0x02, 0x03]
        );
    }

    #[test]
    fn varint_encodes_multi_byte() {
        let mut out = Vec::new();
        write_varint(&mut out, 300); // 300 = 0b1_0010_1100 → [0xAC, 0x02]
        assert_eq!(out, vec![0xAC, 0x02]);
    }
}
