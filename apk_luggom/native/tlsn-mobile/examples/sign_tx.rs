//! Ferramenta de desenvolvimento: assina uma transação Luggom com o MESMO
//! código que o APK usa (módulo `signer`, via UniFFI).
//!
//! Existe para fechar o E2E sem depender do aparelho: em vez de reimplementar a
//! assinatura em Go (o que não provaria nada sobre o cliente real), este binário
//! chama exactamente `derive_wallet` / `sign_tx`.
//!
//! Uso:
//!   echo '{"mnemonic":"...","sign_doc":"<b64>","body_bytes":"<b64>","auth_info_bytes":"<b64>"}' \
//!     | cargo run --example sign_tx
//!
//! Saída (sucesso): {"signed_tx":"<b64>","address":"luggom1...","public_key":"<b64>"}
//! Saída (erro):    {"error":"..."}   — com exit code 1

use std::io::Read;

use base64::Engine;
use serde::Deserialize;
use tlsn_mobile::signer::{derive_wallet, sign_tx};

#[derive(Deserialize)]
struct Input {
    mnemonic: String,
    #[serde(default = "default_hd_path")]
    hd_path: String,
    sign_doc: String,
    body_bytes: String,
    auth_info_bytes: String,
}

fn default_hd_path() -> String {
    "m/44'/118'/0'/0/0".to_string()
}

fn main() {
    if let Err(e) = run() {
        println!("{{\"error\":{}}}", json_string(&e));
        std::process::exit(1);
    }
}

fn run() -> Result<(), String> {
    let mut raw = String::new();
    std::io::stdin()
        .read_to_string(&mut raw)
        .map_err(|e| format!("stdin: {e}"))?;

    let input: Input = serde_json::from_str(&raw).map_err(|e| format!("invalid json: {e}"))?;

    let b64 = base64::engine::general_purpose::STANDARD;
    let sign_doc = b64
        .decode(input.sign_doc.trim())
        .map_err(|e| format!("sign_doc base64: {e}"))?;
    let body = b64
        .decode(input.body_bytes.trim())
        .map_err(|e| format!("body_bytes base64: {e}"))?;
    let auth_info = b64
        .decode(input.auth_info_bytes.trim())
        .map_err(|e| format!("auth_info_bytes base64: {e}"))?;

    // Carteira derivada da seed (a seed nunca sai do aparelho, em produção).
    let wallet = derive_wallet(input.mnemonic.clone(), input.hd_path.clone())
        .map_err(|e| format!("derive_wallet: {e:?}"))?;

    // Assinatura local do sign-doc → TxRaw assinado.
    let signed = sign_tx(
        input.mnemonic,
        input.hd_path,
        sign_doc,
        body,
        auth_info,
    )
    .map_err(|e| format!("sign_tx: {e:?}"))?;

    println!(
        "{{\"signed_tx\":{},\"address\":{},\"public_key\":{}}}",
        json_string(&b64.encode(&signed)),
        json_string(&wallet.address),
        json_string(&b64.encode(&wallet.public_key)),
    );
    Ok(())
}

/// Serializa uma string como literal JSON (suficiente para endereços/base64/erros).
fn json_string(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}
