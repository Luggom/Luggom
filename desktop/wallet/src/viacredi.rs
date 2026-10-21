//! Construtor do pedido do banco (Viacredi) no **momento de provar**.
//!
//! É o porte do `coletor-browser/make_prover_request.py` para Rust. A razão de
//! existir é o **tempo**: o `processJSON` da Viacredi é assinado com um
//! `Access-Time` que o banco só aceita **fresco** (poucos segundos); um pedido
//! pré-construído chega com 401. Aqui o pedido é montado **depois** do MPC setup
//! e enviado logo a seguir, portanto o banco vê-o com 1–2 s.
//!
//! O que NÃO muda: a prova continua a ser de uma **sessão TLS real** que o
//! notário verifica. Isto só constrói o pedido; não inventa comprovantes.

use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use p256::ecdsa::{signature::Signer, Signature, SigningKey};
use p256::pkcs8::EncodePublicKey;
use rand::rngs::OsRng;
use rsa::pkcs8::DecodePublicKey;
use rsa::{Oaep, RsaPublicKey};
use serde::Serialize;
use serde_json::Value;

use crate::prover::{ProveHeader, ProveRequest};
use crate::WalletError;

const BASE: &str = "https://contaonline.viacredi.coop.br";
const PATH: &str = "/servcorews/processJSON";
const FALLBACK_URI: &str = "https://contaonline.viacredi.coop.br/ib/auth";
const UA: &str =
    "Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0";

/// Cookies de analytics/marketing: não têm papel na sessão e estouram o teto do
/// MPC (o pedido inteiro é commitado e o record layer corta acima de 4096+32).
const COOKIES_IGNORADOS: &[&str] = &[
    "_gcl_au",
    "_pk_ref",
    "_pk_id",
    "_pk_ses",
    "OptanonConsent",
    "rxVisitor",
];

/// Cabeçalhos que não entram na assinatura mas que o antifraude (DataDome) e o
/// tracing do banco exigem.
const EXTRA_HEADERS: &[&str] = &["tracestate", "traceparent", "x-dtpc"];

/// Pedido a construir, tal como sai da captura do coletor.
#[derive(Debug, Clone, Serialize)]
pub struct ReceiptCall {
    pub trn: u32,
    pub seq_flow: Option<u32>,
    pub data: Value,
}

/// Captura do coletor (`captura-*.json`), reduzida ao que serve para assinar.
#[derive(Debug, Clone)]
pub struct Capture {
    pub token: String,
    pub client_id: String,
    pub public_rsa_b64: String,
    pub cookie: String,
    pub uri: String,
    pub extras: Vec<(String, String)>,
}

impl Capture {
    /// Lê a captura e extrai o que é preciso. Fail-closed: sem token, sem cookies
    /// ou sem chave RSA, não se constrói nada.
    pub fn parse(raw: &str) -> Result<Self, WalletError> {
        let doc: Value = serde_json::from_str(raw)
            .map_err(|e| WalletError::Network(format!("captura inválida: {e}")))?;
        let reqs = doc["reqs"]
            .as_array()
            .ok_or_else(|| WalletError::Network("captura sem `reqs`".into()))?;

        let (token, client_id) = reqs
            .iter()
            .rev()
            .find_map(|q| {
                let h = q.get("headers")?;
                let auth = h.get("Authorization")?.as_str()?;
                let cid = h.get("Client-Id")?.as_str()?;
                Some((auth.split(' ').nth(1)?.to_string(), cid.to_string()))
            })
            .ok_or_else(|| WalletError::Network("captura sem Authorization/Client-Id".into()))?;

        let public_rsa_b64 = pick_key(reqs, &token)?;
        let cookie = pick_cookie(reqs, &doc)
            .ok_or_else(|| WalletError::Network("captura sem cookies de sessão".into()))?;
        let uri = reqs
            .iter()
            .rev()
            .find_map(|q| {
                let h = q.get("headers")?;
                h.get("Access-Signature")?;
                Some(h.get("Access-Uri")?.as_str()?.to_string())
            })
            .unwrap_or_else(|| FALLBACK_URI.to_string());

        let extras = EXTRA_HEADERS
            .iter()
            .filter_map(|name| {
                reqs.iter().rev().find_map(|q| {
                    q.get("headers")?
                        .get(*name)?
                        .as_str()
                        .map(|v| (name.to_string(), v.to_string()))
                })
            })
            .collect();

        Ok(Self {
            token,
            client_id,
            public_rsa_b64,
            cookie,
            uri,
            extras,
        })
    }

    /// Pedido do **comprovante** visto na captura.
    ///
    /// Identifica-se por trazer o `ID_TRANSACAO` no `data` — é o pedido que o
    /// ecrã do comprovante faz. É daqui que sai o que a app vai pedir, para o
    /// utilizador só ter de abrir o comprovante no browser.
    pub fn receipt_call(raw: &str) -> Result<ReceiptCall, WalletError> {
        let doc: Value = serde_json::from_str(raw)
            .map_err(|e| WalletError::Network(format!("captura inválida: {e}")))?;
        doc["reqs"]
            .as_array()
            .and_then(|reqs| {
                reqs.iter().rev().find_map(|q| {
                    let body = q.get("body")?.as_str()?;
                    let json: Value = serde_json::from_str(body).ok()?;
                    let data = json.get("data")?;
                    // Só o comprovante traz `ID_TRANSACAO`.
                    data.get("ID_TRANSACAO")?;
                    Some(ReceiptCall {
                        trn: json.get("trn")?.as_u64()? as u32,
                        seq_flow: json.get("seqFlow").and_then(|v| v.as_u64()).map(|v| v as u32),
                        data: data.clone(),
                    })
                })
            })
            .ok_or_else(|| {
                WalletError::Network(
                    "a captura não tem o pedido do comprovante — abre o comprovante no banco"
                        .into(),
                )
            })
    }

    /// Constrói o pedido assinado, com o carimbo de tempo **agora**.
    pub async fn build(&self, call: &ReceiptCall) -> Result<ProveRequest, WalletError> {
        let mut body = serde_json::Map::new();
        body.insert("fi".into(), Value::from(7));
        body.insert("chn".into(), Value::from("3"));
        body.insert("trn".into(), Value::from(call.trn));
        if let Some(seq) = call.seq_flow {
            body.insert("seqFlow".into(), Value::from(seq));
        }
        body.insert("data".into(), call.data.clone());
        let body_s = serde_json::to_string(&Value::Object(body))
            .map_err(|e| WalletError::Network(format!("corpo do pedido: {e}")))?;

        let access_id = uuid::Uuid::new_v4().to_string();
        let access_time = self.system_timestamp().await?;

        // Chave P-256 efémera: a assinatura é feita com ela e a pública vai
        // cifrada no X-Request (é assim que o banco a conhece).
        let signing_key = SigningKey::random(&mut OsRng);
        let spki_b64 = B64.encode(
            signing_key
                .verifying_key()
                .to_public_key_der()
                .map_err(|e| WalletError::Network(format!("chave pública: {e}")))?
                .as_bytes(),
        );

        let canon = format!(
            "{access_id}:{access_time}:{BASE}:{}:{body_s}:Bearer {}",
            self.uri, self.token
        );
        let signature: Signature = signing_key.sign(canon.as_bytes());
        let access_signature = B64.encode(signature.to_der().as_bytes());

        let rsa_der = B64
            .decode(&self.public_rsa_b64)
            .map_err(|e| WalletError::Network(format!("chave RSA do banco: {e}")))?;
        let rsa = RsaPublicKey::from_public_key_der(&rsa_der)
            .map_err(|e| WalletError::Network(format!("chave RSA inválida: {e}")))?;
        // OAEP com MGF1-SHA1 e SHA-256 — é o que o browser do banco usa.
        let oaep = Oaep::new_with_mgf_hash::<sha2::Sha256, sha1::Sha1>();
        let x_request = B64.encode(
            rsa.encrypt(
                &mut OsRng,
                oaep,
                serde_json::to_string(&spki_b64)
                    .map_err(|e| WalletError::Network(e.to_string()))?
                    .as_bytes(),
            )
            .map_err(|e| WalletError::Network(format!("X-Request: {e}")))?,
        );

        let authorization = format!("Bearer {}", self.token);
        let referer = format!("{BASE}/ib/dashboard");
        let mut headers = vec![
            header("Host", "contaonline.viacredi.coop.br"),
            header("User-Agent", UA),
            header("Accept", "application/json, text/plain, */*"),
            header("Accept-Language", "pt-BR,pt;q=0.8,en-US;q=0.5,en;q=0.3"),
            header("Content-Type", "application/json"),
            header("Authorization", &authorization),
            header("Client-Id", &self.client_id),
            header("Origin", BASE),
            header("Referer", &referer),
            header("Sec-Fetch-Dest", "empty"),
            header("Sec-Fetch-Mode", "cors"),
            header("Sec-Fetch-Site", "same-origin"),
            header("Cookie", &self.cookie),
            header("Connection", "close"),
        ];
        for (name, value) in &self.extras {
            headers.push(header(name, value));
        }
        headers.push(header("Access-Id", &access_id));
        headers.push(header("Access-Time", &access_time));
        headers.push(header("Access-Origin", BASE));
        headers.push(header("Access-Uri", &self.uri));
        headers.push(header("Access-Signature", &access_signature));
        headers.push(header("X-Request", &x_request));

        Ok(ProveRequest {
            url: format!("{BASE}{PATH}"),
            method: "POST".into(),
            headers,
            body: Some(body_s),
        })
    }

    /// `Access-Time` do banco (`getSystemTS`) — não é o relógio local.
    async fn system_timestamp(&self) -> Result<String, WalletError> {
        let client = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(20))
            .build()
            .map_err(|e| WalletError::Network(e.to_string()))?;
        let response = client
            .get(format!("{BASE}/servcorews/getSystemTS"))
            .header("Accept", "application/json, text/plain, */*")
            .header("Cookie", &self.cookie)
            .header("User-Agent", UA)
            .send()
            .await
            .map_err(|e| WalletError::Network(format!("getSystemTS: {e}")))?;
        let text = response
            .text()
            .await
            .map_err(|e| WalletError::Network(format!("getSystemTS: {e}")))?;
        let value = text.trim();
        if value.is_empty() || !value.chars().all(|c| c.is_ascii_digit()) {
            return Err(WalletError::Network(format!(
                "getSystemTS devolveu algo inesperado: {}",
                &value[..value.len().min(60)]
            )));
        }
        Ok(value.to_string())
    }
}

fn header(name: &str, value: &str) -> ProveHeader {
    ProveHeader {
        name: name.to_string(),
        value: value.to_string(),
    }
}

/// A chave RSA em uso: a do pedido (mais fidedigna), senão a do JWT.
fn pick_key(reqs: &[Value], token: &str) -> Result<String, WalletError> {
    for q in reqs.iter().rev() {
        if let Some(key) = q
            .get("encKeyAtRequest")
            .and_then(|e| e.get("encKey"))
            .and_then(|v| v.as_str())
        {
            if !key.is_empty() {
                return Ok(key.to_string());
            }
        }
    }
    jwt_public_rsa(token)
        .ok_or_else(|| WalletError::Network("não encontrei a chave RSA na captura".into()))
}

/// `PUBLIC_RSA` do JWT (o `sub` é um JSON em base64url com um campo `values`).
fn jwt_public_rsa(token: &str) -> Option<String> {
    let payload = token.split('.').nth(1)?;
    let decoded = b64url(payload)?;
    let sub = serde_json::from_slice::<Value>(&decoded).ok()?;
    let sub = sub.get("sub")?.as_str()?;
    let decoded = b64url(sub)?;
    serde_json::from_slice::<Value>(&decoded)
        .ok()?
        .get("values")?
        .get("PUBLIC_RSA")?
        .as_str()
        .map(|s| s.to_string())
}

fn b64url(text: &str) -> Option<Vec<u8>> {
    use base64::engine::general_purpose::URL_SAFE;
    // O JWT pode vir sem padding — o motor exige-o, por isso acrescenta-se.
    let padding = "=".repeat((4 - text.len() % 4) % 4);
    URL_SAFE
        .decode(format!("{text}{padding}").as_bytes())
        .ok()
}

/// Cookies da sessão, sem os de analytics.
fn pick_cookie(reqs: &[Value], doc: &Value) -> Option<String> {
    for q in reqs.iter().rev() {
        for field in ["encKeyAtRequest", "encKeyAtSend"] {
            if let Some(c) = q.get(field).and_then(|e| e.get("cookie")).and_then(|v| v.as_str()) {
                if !c.is_empty() {
                    return Some(prune_cookie(c));
                }
            }
        }
    }
    for q in reqs.iter().rev() {
        if let Some(c) = q
            .get("headers")
            .and_then(|h| h.get("Cookie"))
            .and_then(|v| v.as_str())
        {
            if !c.is_empty() {
                return Some(prune_cookie(c));
            }
        }
    }
    doc.get("cookie").and_then(|v| v.as_str()).map(prune_cookie)
}

fn prune_cookie(cookie: &str) -> String {
    cookie
        .split(';')
        .map(str::trim)
        .filter(|part| !part.is_empty())
        .filter(|part| {
            let name = part.split('=').next().unwrap_or("");
            !COOKIES_IGNORADOS.contains(&name)
        })
        .collect::<Vec<_>>()
        .join("; ")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn prunes_analytics_cookies() {
        let pruned = prune_cookie("_gcl_au=1; cck=abc; OptanonConsent=x; tga=def");
        assert_eq!(pruned, "cck=abc; tga=def");
    }

    #[test]
    fn reads_the_public_key_from_the_jwt() {
        use base64::engine::general_purpose::URL_SAFE_NO_PAD;
        let sub = URL_SAFE_NO_PAD.encode(br#"{"values":{"PUBLIC_RSA":"chave-do-banco"}}"#);
        let payload = URL_SAFE_NO_PAD.encode(format!(r#"{{"sub":"{sub}"}}"#));
        let token = format!("header.{payload}.sig");
        assert_eq!(jwt_public_rsa(&token).as_deref(), Some("chave-do-banco"));
    }

    #[test]
    fn finds_the_receipt_call_by_its_transaction_id() {
        let raw = r#"{"reqs":[
            {"headers":{"Authorization":"Bearer a.b.c","Client-Id":"c1"}},
            {"body":"{\"fi\":7,\"chn\":\"3\",\"trn\":605,\"seqFlow\":15,\"data\":{\"CODIGO_TIPO_TRANSACAO\":\"1\"}}"},
            {"body":"{\"fi\":7,\"chn\":\"3\",\"trn\":605,\"seqFlow\":11,\"data\":{\"ID_TRANSACAO\":\"867598663\"}}"}
        ]}"#;
        let call = Capture::receipt_call(raw).unwrap();
        assert_eq!(call.trn, 605);
        assert_eq!(call.seq_flow, Some(11));
        assert_eq!(call.data["ID_TRANSACAO"], "867598663");

        // Sem comprovante na captura → erro (fail-closed).
        assert!(Capture::receipt_call(r#"{"reqs":[]}"#).is_err());
    }

    #[test]
    fn refuses_a_capture_without_a_token() {
        assert!(Capture::parse(r#"{"reqs":[]}"#).is_err());
    }
}
