//! Notary-side attestation (§13.121, Fase B).
//!
//! O notário é a única parte que **verificou** o transcript. Aqui ele:
//!   1. corre o **extrator partilhado** (Go, o mesmo código da API) sobre o
//!      transcript que verificou → campos canónicos + payload a assinar;
//!   2. assina `sha256(payload)` com a chave **secp256k1** dele (compacta,
//!      65 bytes: recovery||r||s — o mesmo formato que o keeper recupera);
//!   3. guarda `{session_id → fields + assinatura}` para a API buscar.
//!
//! Assim a derivação (extração do valor) fica **dentro do domínio de confiança
//! do notário** — uma API comprometida deixa de poder inflacionar: só relaya.

use std::process::{Command, Stdio};
use std::time::{Duration, Instant};

use base64::Engine;
use k256::ecdsa::SigningKey;
use serde_json::Value;
use sha2::{Digest, Sha256};

/// TTL de uma atestação guardada (a API busca logo após a prova).
pub const ATTESTATION_TTL: Duration = Duration::from_secs(15 * 60);

/// Atestação de uma sessão verificada.
#[derive(Debug, Clone)]
pub struct StoredAttestation {
    pub server_name: String,
    pub fields: Value,
    pub signature: String,
    pub created: Instant,
}

impl StoredAttestation {
    pub fn is_fresh(&self) -> bool {
        self.created.elapsed() < ATTESTATION_TTL
    }

    /// JSON devolvido à API.
    pub fn to_json(&self) -> Value {
        serde_json::json!({
            "server_name": self.server_name,
            "fields": self.fields,
            "signature": self.signature,
        })
    }
}

/// Corre o extrator partilhado sobre o transcript verificado.
///
/// `recv` são os bytes recebidos da sessão TLS (o mesmo `received_unsafe()`
/// que o extrator analisa). Devolve `(fields, payload)`.
///
/// `miner_address` vem do registo da sessão (quem recebe a cunhagem) e
/// `chain_id` do ambiente do notário — ambos entram no payload v4, pelo que
/// ficam cobertos pela assinatura.
pub fn extract(
    extractor_bin: &str,
    server_name: &str,
    miner_address: &str,
    chain_id: &str,
    recv: &[u8],
) -> Result<(Value, String), String> {
    let input = serde_json::json!({
        "server_name": server_name,
        "miner_address": miner_address,
        "chain_id": chain_id,
        "transcript_recv_b64": base64::engine::general_purpose::STANDARD.encode(recv),
    });

    let output = Command::new(extractor_bin)
        .stdin(Stdio::piped())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .and_then(|mut child| {
            use std::io::Write;
            child
                .stdin
                .take()
                .expect("stdin piped")
                .write_all(input.to_string().as_bytes())?;
            child.wait_with_output()
        })
        .map_err(|e| format!("failed to run extractor {extractor_bin}: {e}"))?;

    let stdout = String::from_utf8_lossy(&output.stdout).to_string();
    let parsed: Value = serde_json::from_str(stdout.trim()).map_err(|e| {
        format!(
            "extractor returned invalid json: {e} — stdout={:?} stderr={:?}",
            stdout,
            String::from_utf8_lossy(&output.stderr)
        )
    })?;

    if let Some(err) = parsed.get("error").and_then(|e| e.as_str()) {
        return Err(format!("extractor rejected: {err}"));
    }
    let fields = parsed
        .get("fields")
        .cloned()
        .ok_or_else(|| "extractor returned no fields".to_string())?;
    let payload = parsed
        .get("payload")
        .and_then(|p| p.as_str())
        .ok_or_else(|| "extractor returned no payload".to_string())?
        .to_string();

    Ok((fields, payload))
}

/// Assina `sha256(payload)` com a chave secp256k1 do notário.
/// Devolve 65 bytes em hex: `recovery_id || r || s` (formato do keeper).
pub fn sign(notary_key: &[u8], payload: &str) -> Result<String, String> {
    let signing_key =
        SigningKey::from_slice(notary_key).map_err(|_| "invalid notary key".to_string())?;
    let digest = Sha256::digest(payload.as_bytes());
    let (signature, recid) = signing_key
        .sign_prehash_recoverable(&digest)
        .map_err(|e| format!("failed to sign attestation: {e}"))?;

    // Formato compacto do decred/Bitcoin: header = 27 + recid + 4 (chave comprimida).
    // É o que o keeper (RecoverCompact) espera — validado pelo teste de interop.
    let mut out = Vec::with_capacity(65);
    out.push(27 + 4 + recid.to_byte());
    out.extend_from_slice(&signature.to_bytes());
    Ok(hex::encode(out))
}

/// Chave pública comprimida (33 bytes) do notário, em hex — dá para conferir o
/// endereço que vai em `params.api_verifier_address`.
pub fn verifying_key_hex(notary_key: &[u8]) -> Result<String, String> {
    let signing_key =
        SigningKey::from_slice(notary_key).map_err(|_| "invalid notary key".to_string())?;
    let compressed = signing_key.verifying_key().to_encoded_point(true);
    Ok(hex::encode(compressed.as_bytes()))
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Vetor de interoperabilidade Rust→Go: a assinatura produzida aqui tem de
    /// ser RECUPERÁVEL pelo keeper Go (formato `recid||r||s`, 65 bytes).
    /// O teste Go correspondente está em `x/luggom/types/attestation_test.go`.
    #[test]
    fn interop_vector() {
        let key = vec![0x11u8; 32];
        // NOTA: o valor é o que o extrator GO produz (LegacyDec.String() → 18 casas).
        // O notário NÃO canonicaliza — assina o payload que o extrator lhe dá.
        // v4: domínio novo + `miner_address` e `chain_id` logo a seguir (a ordem
        // do BuildAttestationPayload). O pagador é vazio (daí o "\n\n" antes do
        // banco).
        let payload = "luggom-attestation/v4\nluggom1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5n0k6\nluggom-test\ncommit-abc\npay-1\n240.500000000000000000\nbr:receita_federal\nBRL\n1770000000\nnature-hash-abc\n\nviacredi";
        let sig = sign(&key, payload).expect("sign");
        println!("INTEROP_RUST_PAYLOAD={payload}");
        println!("INTEROP_RUST_SIG={sig}");
        assert_eq!(sig.len(), 130, "assinatura compacta = 65 bytes (130 hex)");
    }
}
