//! Fluxo de mineração do desktop (A4): prova TLSNotary → tx NÃO assinada →
//! assinatura LOCAL → broadcast.
//!
//! Espelha o caminho do APK (§13.58 passo 2 / OPÇÃO 1 §13.97): a **API** é o
//! portão anti-fraude — valida a prova, vai buscar a atestação ao **notário** e
//! monta a tx com o `miner_address` do utilizador — e o **aparelho** assina
//! localmente. A seed nunca sai deste processo, e a API nunca vê a chave privada.
//!
//! O que este módulo NÃO faz: correr o prover (isso é `crate::prover`) nem
//! verificar a prova (isso é o notário + a chain). Aqui só se fala com a API.

use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use serde::Deserialize;

use crate::{UnsignedTx, Wallet, WalletError};

/// Prova TLSNotary + identidade do minerador, pronta a submeter à API.
#[derive(Debug, Clone)]
pub struct MineBundle {
    /// Banco atestado (`"viacredi"`, `"caixa"`, …) — a chain verifica-o contra a
    /// whitelist on-chain.
    pub provider_id: String,
    /// Carteira que vai receber o LUG (tem de ser a do cofre aberto).
    pub dest_address: String,
    /// Chave pública secp256k1 comprimida (33 bytes) do minerador.
    pub public_key: Vec<u8>,
    /// `session_id` do prover (comprova que a prova é a que o notário viu).
    pub session_id: String,
    /// `server_name` atestado (host do banco).
    pub server_name: String,
    /// Código HTTP do comprovante.
    pub status: u16,
    /// Corpo revelado do comprovante.
    pub response_body: String,
    /// Transcript atestado, em base64.
    pub transcript_sent_b64: String,
    pub transcript_recv_b64: String,
}

/// Tx NÃO assinada + o que a API extraiu da prova.
#[derive(Debug, Clone)]
pub struct MinePreparation {
    pub payment_id: String,
    pub payment_amount: Option<String>,
    pub tax_entity: Option<String>,
    pub unsigned: UnsignedTx,
}

/// Resultado do broadcast.
#[derive(Debug, Clone)]
pub struct MineReceipt {
    pub tx_hash: String,
    pub code: i32,
}

/// O que a mineração produziu, para a UI mostrar.
#[derive(Debug, Clone)]
pub struct MinedResult {
    pub tx_hash: String,
    pub payment_amount: Option<String>,
    pub tax_entity: Option<String>,
}

/// Banco aceite pela whitelist on-chain (servido pela API).
#[derive(Debug, Clone, Deserialize)]
pub struct ProviderInfo {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub name: String,
}

#[derive(Debug, Deserialize)]
struct ProviderListReply {
    #[serde(default)]
    providers: Vec<ProviderInfo>,
}

#[derive(Debug, Deserialize)]
struct MineTaxReply {
    #[serde(default)]
    tx_hash: Option<String>,
    #[serde(default)]
    code: i32,
    #[serde(default)]
    error: Option<String>,
    #[serde(default)]
    payment_id: String,
    #[serde(default)]
    payment_amount: Option<String>,
    #[serde(default)]
    tax_entity: Option<String>,
    #[serde(default)]
    unsigned: Option<UnsignedTx>,
}

/// Cliente da API do Luggom (o portão anti-fraude).
#[derive(Debug, Clone)]
pub struct MineClient {
    base: String,
    http: reqwest::Client,
}

impl MineClient {
    /// `base_url` é a raiz da API, ex.: `http://127.0.0.1:8082`.
    pub fn new(base_url: impl Into<String>) -> Result<Self, WalletError> {
        let base = base_url.into().trim_end_matches('/').to_string();
        if !base.starts_with("http://") && !base.starts_with("https://") {
            return Err(WalletError::Network(format!("endereço da API inválido: {base}")));
        }
        let http = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(60))
            .build()
            .map_err(|e| WalletError::Network(e.to_string()))?;
        Ok(Self { base, http })
    }

    pub fn base_url(&self) -> &str {
        &self.base
    }

    /// Bancos aceites — a whitelist on-chain, servida pela API.
    pub async fn providers(&self) -> Result<Vec<ProviderInfo>, WalletError> {
        let response = self
            .http
            .get(format!("{}/api/v1/reclaim/providers", self.base))
            .send()
            .await
            .map_err(|e| WalletError::Network(e.to_string()))?;

        let status = response.status();
        let text = response
            .text()
            .await
            .map_err(|e| WalletError::Network(e.to_string()))?;

        if !status.is_success() {
            return Err(WalletError::Network(format!(
                "HTTP {}: {}",
                status.as_u16(),
                truncate(&text)
            )));
        }

        let reply: ProviderListReply = serde_json::from_str(&text)
            .map_err(|e| WalletError::UnexpectedResponse(format!("{e}: {}", truncate(&text))))?;
        Ok(reply.providers)
    }

    /// Passo 1: envia a prova e recebe a tx não assinada.
    pub async fn prepare(&self, bundle: &MineBundle) -> Result<MinePreparation, WalletError> {
        let body = serde_json::json!({
            "provider_id": bundle.provider_id,
            "dest_address": bundle.dest_address,
            "public_key": B64.encode(&bundle.public_key),
            "tlsn_proof": {
                "session_id": bundle.session_id,
                "server_name": bundle.server_name,
                "status": bundle.status,
                "response_body": bundle.response_body,
                "transcript": {
                    "sent": bundle.transcript_sent_b64,
                    "recv": bundle.transcript_recv_b64,
                },
            },
        });

        let reply: MineTaxReply = self.post_json("/api/v1/mine/tax", &body).await?;
        let unsigned = reply.unsigned.ok_or_else(|| WalletError::MineRejected {
            code: reply.code,
            message: reply
                .error
                .clone()
                .unwrap_or_else(|| "a API não devolveu a transação a assinar".into()),
        })?;
        if reply.code != 0 {
            return Err(WalletError::MineRejected {
                code: reply.code,
                message: reply.error.unwrap_or_default(),
            });
        }

        Ok(MinePreparation {
            payment_id: reply.payment_id,
            payment_amount: reply.payment_amount,
            tax_entity: reply.tax_entity,
            unsigned,
        })
    }

    /// Passo 2: transmite a tx já assinada localmente.
    pub async fn broadcast(
        &self,
        tx_bytes_b64: &str,
        payment_id: &str,
    ) -> Result<MineReceipt, WalletError> {
        let body = serde_json::json!({ "tx_bytes": tx_bytes_b64, "payment_id": payment_id });
        let reply: MineTaxReply = self.post_json("/api/v1/mine/tax/broadcast", &body).await?;
        if reply.code != 0 {
            return Err(WalletError::MineRejected {
                code: reply.code,
                message: reply.error.unwrap_or_else(|| "o broadcast falhou".into()),
            });
        }
        Ok(MineReceipt {
            tx_hash: reply.tx_hash.unwrap_or_default(),
            code: reply.code,
        })
    }

    /// Passos 1–2: preparar → **assinar localmente** → transmitir.
    ///
    /// É o caminho completo do A4. O `dest_address` do bundle tem de ser o da
    /// carteira aberta — a API recusaria de outro modo, e falhar aqui dá uma
    /// mensagem melhor do que um erro de rede a meio.
    pub async fn mine(
        &self,
        wallet: &Wallet,
        mnemonic: &str,
        bundle: &MineBundle,
    ) -> Result<MinedResult, WalletError> {
        if bundle.dest_address != wallet.address() {
            return Err(WalletError::InvalidAddress(bundle.dest_address.clone()));
        }

        let prep = self.prepare(bundle).await?;

        // A seed entra aqui e só aqui: assina-se no aparelho, com o assinador
        // real do APK. O que sai é um `TxRaw` — a chave privada nunca sai.
        let tx_raw = wallet.sign(mnemonic, &prep.unsigned)?;

        let receipt = self.broadcast(&B64.encode(&tx_raw), &prep.payment_id).await?;
        Ok(MinedResult {
            tx_hash: receipt.tx_hash,
            payment_amount: prep.payment_amount,
            tax_entity: prep.tax_entity,
        })
    }

    async fn post_json<T: for<'de> Deserialize<'de>>(
        &self,
        path: &str,
        body: &serde_json::Value,
    ) -> Result<T, WalletError> {
        let response = self
            .http
            .post(format!("{}{}", self.base, path))
            .json(body)
            .send()
            .await
            .map_err(|e| WalletError::Network(e.to_string()))?;

        let status = response.status();
        let text = response
            .text()
            .await
            .map_err(|e| WalletError::Network(e.to_string()))?;

        // A API devolve os erros de domínio como JSON (`error` + `code`) mesmo em
        // 4xx — por isso se tenta desserializar primeiro e só se cai para o erro
        // de rede quando o corpo não é JSON (ex.: `http.Error` em texto).
        serde_json::from_str::<T>(&text).map_err(|e| {
            if !status.is_success() {
                WalletError::Network(format!(
                    "HTTP {}: {}",
                    status.as_u16(),
                    truncate(&text)
                ))
            } else {
                WalletError::UnexpectedResponse(format!("{e}: {}", truncate(&text)))
            }
        })
    }
}

fn truncate(text: &str) -> String {
    const LIMIT: usize = 300;
    if text.len() <= LIMIT {
        return text.to_string();
    }
    let mut end = LIMIT;
    while !text.is_char_boundary(end) {
        end -= 1;
    }
    format!("{}…", &text[..end])
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{BufRead, BufReader, Read, Write};
    use std::net::TcpListener;
    use std::sync::{Arc, Mutex};
    use std::thread::JoinHandle;

    const MNEMONIC: &str =
        "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";

    /// Servidor HTTP mínimo: responde `responses[i]` ao i-ésimo pedido e guarda
    /// os corpos recebidos. É deliberadamente burro — só o suficiente para o
    /// cliente falar HTTP/1.1 com `Content-Length`.
    struct MockApi {
        base: String,
        bodies: Arc<Mutex<Vec<String>>>,
        handle: Option<JoinHandle<()>>,
    }

    impl MockApi {
        fn start(responses: Vec<String>) -> Self {
            let listener = TcpListener::bind("127.0.0.1:0").unwrap();
            let base = format!("http://{}", listener.local_addr().unwrap());
            let bodies = Arc::new(Mutex::new(Vec::new()));
            let collected = Arc::clone(&bodies);

            let handle = std::thread::spawn(move || {
                for response in responses {
                    let (stream, _) = listener.accept().unwrap();
                    let mut reader = BufReader::new(stream.try_clone().unwrap());

                    let mut content_length = 0usize;
                    loop {
                        let mut line = String::new();
                        if reader.read_line(&mut line).unwrap() == 0 {
                            break;
                        }
                        if let Some(value) = line.to_ascii_lowercase().strip_prefix("content-length:") {
                            content_length = value.trim().parse().unwrap_or(0);
                        }
                        if line == "\r\n" {
                            break;
                        }
                    }
                    let mut body = vec![0u8; content_length];
                    reader.read_exact(&mut body).unwrap();
                    collected
                        .lock()
                        .unwrap()
                        .push(String::from_utf8_lossy(&body).into_owned());

                    let mut stream = stream;
                    let _ = stream.write_all(response.as_bytes());
                    let _ = stream.flush();
                }
            });

            Self { base, bodies, handle: Some(handle) }
        }

        fn bodies(&self) -> Vec<String> {
            self.bodies.lock().unwrap().clone()
        }
    }

    impl Drop for MockApi {
        fn drop(&mut self) {
            if let Some(handle) = self.handle.take() {
                let _ = handle.join();
            }
        }
    }

    fn http_ok(body: &str) -> String {
        format!(
            "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
            body.len(),
            body
        )
    }

    fn http_error(status: &str, body: &str) -> String {
        format!(
            "HTTP/1.1 {status}\r\nContent-Type: application/json\r\nContent-Length: {}\r\n\r\n{}",
            body.len(),
            body
        )
    }

    fn bundle_for(address: &str) -> MineBundle {
        MineBundle {
            provider_id: "viacredi".into(),
            dest_address: address.into(),
            public_key: vec![0x02; 33],
            session_id: "11111111-2222-3333-4444-555555555555".into(),
            server_name: "contaonline.viacredi.coop.br".into(),
            status: 200,
            response_body: "{\"recibo\":true}".into(),
            transcript_sent_b64: B64.encode(b"POST / HTTP/1.1\r\n"),
            transcript_recv_b64: B64.encode(b"HTTP/1.1 200 OK\r\n"),
        }
    }

    #[tokio::test]
    async fn mines_end_to_end_against_the_api() {
        let wallet = Wallet::from_mnemonic(MNEMONIC).unwrap();

        let unsigned = serde_json::json!({
            "code": 0,
            "payment_id": "E9999999",
            "payment_amount": "1291.49",
            "tax_entity": "br:iptu_sao_paulo",
            "unsigned": {
                "sign_doc_bytes": B64.encode(b"sign-doc"),
                "body_bytes": B64.encode(b"body"),
                "auth_info_bytes": B64.encode(b"auth-info"),
                "chain_id": "luggom-test",
                "account_number": 7,
                "sequence": 2
            }
        })
        .to_string();

        let api = MockApi::start(vec![
            http_ok(&unsigned),
            http_ok("{\"tx_hash\":\"ABC\",\"code\":0}"),
        ]);

        let client = MineClient::new(&api.base).unwrap();
        let mined = client
            .mine(&wallet, MNEMONIC, &bundle_for(wallet.address()))
            .await
            .unwrap();

        assert_eq!(mined.tx_hash, "ABC");
        assert_eq!(mined.payment_amount.as_deref(), Some("1291.49"));

        // O 2.º pedido leva a tx ASSINADA (TxRaw: campo 1 body, 2 auth_info, 3 assinaturas).
        let bodies = api.bodies();
        assert_eq!(bodies.len(), 2, "esperava /mine/tax e /mine/tax/broadcast");

        let first: serde_json::Value = serde_json::from_str(&bodies[0]).unwrap();
        assert_eq!(first["provider_id"], "viacredi");
        assert_eq!(first["tlsn_proof"]["session_id"], bundle_for("x").session_id);

        let second: serde_json::Value = serde_json::from_str(&bodies[1]).unwrap();
        assert_eq!(second["payment_id"], "E9999999");
        let tx_raw = B64.decode(second["tx_bytes"].as_str().unwrap()).unwrap();
        assert_eq!(tx_raw[0], 0x0a, "TxRaw começa com body_bytes (campo 1)");
        assert!(tx_raw.contains(&0x1a), "TxRaw tem campo 3 (assinaturas)");
    }

    #[tokio::test]
    async fn surfaces_the_api_rejection_code() {
        let wallet = Wallet::from_mnemonic(MNEMONIC).unwrap();

        // 409: comprovante já minado (anti-replay 1202).
        let api = MockApi::start(vec![http_error(
            "409 Conflict",
            "{\"error\":\"já utilizado\",\"code\":1202}",
        )]);

        let client = MineClient::new(&api.base).unwrap();
        let err = client
            .prepare(&bundle_for(wallet.address()))
            .await
            .unwrap_err();

        match err {
            WalletError::MineRejected { code, message } => {
                assert_eq!(code, 1202);
                assert_eq!(message, "já utilizado");
            }
            other => panic!("esperava MineRejected, veio {other:?}"),
        }
    }

    #[tokio::test]
    async fn refuses_a_bundle_for_another_wallet() {
        let wallet = Wallet::from_mnemonic(MNEMONIC).unwrap();
        let client = MineClient::new("http://127.0.0.1:1").unwrap();

        let mut bundle = bundle_for(wallet.address());
        bundle.dest_address = "luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna".into();

        let err = client.mine(&wallet, MNEMONIC, &bundle).await.unwrap_err();
        assert!(matches!(err, WalletError::InvalidAddress(_)));
    }

    #[test]
    fn rejects_a_base_url_without_scheme() {
        assert!(MineClient::new("127.0.0.1:8082").is_err());
        assert!(MineClient::new("http://127.0.0.1:8082/").is_ok());
    }
}
