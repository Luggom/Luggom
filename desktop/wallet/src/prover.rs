//! Prover TLSNotary embutido — **porte do `prover-ws-poc`** (o CLI provado).
//!
//! Porquê um porte e não o `tlsn-mobile`: exercitado contra o notário, o prover
//! do `tlsn-mobile` **deixa o mux prover↔notário cair** (`io error: not
//! connected`) e o notário **nunca conclui a verificação** — logo não guarda a
//! atestação e a mineração é recusada. O CLI, com a **mesma** `tlsn-sdk-core`,
//! o **mesmo** adaptador e o **mesmo** reveal, conclui sempre. Este módulo
//! replica o fluxo do CLI (linear, num runtime próprio) e acrescenta o **portão
//! de aprovação** entre `compute_reveal` e `reveal`.
//!
//! Não se reimplementa criptografia: usa-se a `tlsn-sdk-core` tal como o CLI.

use std::pin::Pin;
use std::task::{Context, Poll};
use std::time::Duration;

use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use futures::{AsyncRead, AsyncWrite, Sink, SinkExt, Stream, StreamExt};
use serde::Deserialize;
use tlsn_sdk_core::handler::{ComputeRevealOutput, RangeWithHandler};
use tlsn_sdk_core::{
    compute_reveal, config::ProverMode, Handler, HandlerAction, HandlerPart, HandlerType,
    HttpRequest as SdkHttpRequest, Method, ProverConfig, SdkProver,
};
use tokio::net::TcpStream;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::{connect_async, MaybeTlsStream, WebSocketStream};
use tokio_util::compat::TokioAsyncReadCompatExt;

use crate::WalletError;

/// Orçamento do MPC — o CLI documenta que o mux só fecha com `max_sent_data=4096`.
const MAX_SENT_DATA: usize = 4096;
const MAX_RECV_DATA: usize = 65_536;

/// Pedido HTTP a provar — o comprovante do banco, com os cookies da sessão.
///
/// É o formato do `luggom-request.json` do coletor: `url`, `method`, `headers`
/// (inclui o `Cookie`) e `body`.
#[derive(Debug, Clone, Deserialize)]
pub struct ProveRequest {
    pub url: String,
    #[serde(default = "default_method")]
    pub method: String,
    #[serde(default)]
    pub headers: Vec<ProveHeader>,
    #[serde(default)]
    pub body: Option<String>,
}

fn default_method() -> String {
    "GET".to_string()
}

/// Um cabeçalho do pedido.
#[derive(Debug, Clone, Deserialize)]
pub struct ProveHeader {
    pub name: String,
    pub value: String,
}

impl ProveRequest {
    /// `server_name` atestado: host (com porta, se não for a padrão do esquema).
    pub fn server_name(&self) -> Result<String, WalletError> {
        let url = url::Url::parse(&self.url)
            .map_err(|e| WalletError::Network(format!("URL do comprovante inválida: {e}")))?;
        let host = url
            .host_str()
            .ok_or_else(|| WalletError::Network("a URL do comprovante não tem host".into()))?;
        Ok(match url.port() {
            Some(port) => format!("{host}:{port}"),
            None => host.to_string(),
        })
    }

    fn into_sdk(self) -> SdkHttpRequest {
        let method = match self.method.to_uppercase().as_str() {
            "POST" => Method::POST,
            "PUT" => Method::PUT,
            "DELETE" => Method::DELETE,
            _ => Method::GET,
        };
        let headers = self
            .headers
            .into_iter()
            .map(|h| (h.name, h.value.into_bytes()))
            .collect();

        SdkHttpRequest {
            uri: self.url,
            method,
            headers,
            body: self
                .body
                .map(|b| tlsn_sdk_core::Body::Raw(b.into_bytes())),
        }
    }

    /// Host (e porta) a que o MPC-TLS se liga.
    fn host_and_port(&self) -> Result<(String, u16), WalletError> {
        let url = url::Url::parse(&self.url)
            .map_err(|e| WalletError::Network(format!("URL do comprovante inválida: {e}")))?;
        let host = url
            .host_str()
            .ok_or_else(|| WalletError::Network("a URL do comprovante não tem host".into()))?
            .to_string();
        let port = url.port().unwrap_or(if url.scheme() == "https" { 443 } else { 80 });
        Ok((host, port))
    }
}

/// Uma linha do que vai ser revelado, para o ecrã de aprovação.
#[derive(Debug, Clone, serde::Serialize)]
pub struct RevealDescriptor {
    pub direction: String,
    pub label: String,
    pub action: String,
    pub preview: String,
}

/// Fase 1: prova até ao reveal; é preciso aprovar antes de finalizar.
#[derive(Debug, Clone, serde::Serialize)]
pub struct ProvePreview {
    pub session_id: String,
    pub status: u16,
    pub response_body: String,
    pub descriptors: Vec<RevealDescriptor>,
}

/// Fase 2: transcript atestado, pronto a virar `MineBundle`.
#[derive(Debug, Clone)]
pub struct ProveOutcome {
    pub session_id: String,
    /// `server_name` atestado (o host que o notário verificou).
    pub server_name: String,
    pub status: u16,
    pub response_body: String,
    pub transcript_sent_b64: String,
    pub transcript_recv_b64: String,
}

/// O que o portão de aprovação decidiu.
pub enum ProveDecision {
    Approved(ProveOutcome),
    Rejected,
}

/// O que se revela por omissão: a **resposta inteira** do banco (status line +
/// cabeçalhos + corpo).
///
/// O extrator do notário reconstrói a resposta HTTP e precisa do separador
/// `\r\n\r\n` — revelar só a status line e o corpo faz falhar a atestação
/// ("no header/body separator"). O **pedido** (com os cookies e o token) fica
/// escondido: é isso que protege a sessão do utilizador.
fn default_handlers() -> Vec<Handler> {
    vec![Handler {
        handler_type: HandlerType::Recv,
        part: HandlerPart::All,
        action: HandlerAction::Reveal,
        params: None,
    }]
}

/// Corre a prova completa. **Bloqueia** a thread; `approve` é chamado entre o
/// reveal e a finalização, e devolve `true` para revelar.
///
/// É o fluxo do CLI (`prover-ws-poc`): registar sessão → MPC setup → pedido →
/// `compute_reveal` → [aprovação] → `reveal` → `reveal_config` → esperar o
/// notário → transcript.
/// De onde vem o pedido a provar.
pub enum ProveSource {
    /// Já construído (ex.: um pedido capturado, ou um teste).
    Ready(ProveRequest),
    /// Construído **agora**, depois do MPC setup: o banco (Viacredi) só aceita
    /// um `Access-Time` fresco, por isso a assinatura tem de nascer aqui.
    Viacredi {
        capture: crate::viacredi::Capture,
        call: crate::viacredi::ReceiptCall,
    },
}

impl ProveSource {
    fn host_and_port(&self) -> Result<(String, u16), WalletError> {
        match self {
            ProveSource::Ready(request) => request.host_and_port(),
            ProveSource::Viacredi { .. } => {
                Ok(("contaonline.viacredi.coop.br".to_string(), 443))
            }
        }
    }
}

pub fn prove(
    source: ProveSource,
    notary_url: String,
    approve: impl FnOnce(ProvePreview) -> bool + Send,
) -> Result<ProveDecision, WalletError> {
    let runtime = tokio::runtime::Builder::new_multi_thread()
        .enable_all()
        .build()
        .map_err(|e| WalletError::Network(format!("runtime do prover: {e}")))?;

    runtime.block_on(prove_async(source, notary_url, approve))
}

async fn prove_async(
    source: ProveSource,
    notary_url: String,
    approve: impl FnOnce(ProvePreview) -> bool,
) -> Result<ProveDecision, WalletError> {
    let (hostname, port) = source.host_and_port()?;

    // 1. Registar a sessão no notário (é ESTE socket que leva o reveal_config).
    let (mut session_ws, session_id) = register_session(&notary_url).await?;
    tracing::info!("sessão registada: {session_id}");

    // 2. Prover + MPC setup (igual ao CLI).
    let config = ProverConfig::builder(hostname.clone())
        .mode(ProverMode::Mpc)
        .max_sent_data(MAX_SENT_DATA)
        .max_recv_data(MAX_RECV_DATA)
        .build()
        .map_err(|e| WalletError::Network(format!("config do prover: {e}")))?;

    let mut prover =
        SdkProver::new(config).map_err(|e| WalletError::Network(format!("prover: {e}")))?;

    let verifier_url = format!("{}/verifier?sessionId={session_id}", ws_base(&notary_url)?);
    let (verifier_ws, _) = connect_async(&verifier_url)
        .await
        .map_err(|e| WalletError::Network(format!("não liguei ao notário: {e}")))?;
    prover
        .setup(WsIoAdapter::new(verifier_ws))
        .await
        .map_err(|e| WalletError::Network(format!("MPC setup: {e}")))?;

    // 3. Construir o pedido **agora** — depois do MPC setup e imediatamente
    //    antes de o enviar. É isto que faz o banco aceitar o `Access-Time`.
    let sdk_request = match source {
        ProveSource::Ready(request) => request.into_sdk(),
        ProveSource::Viacredi { capture, call } => capture.build(&call).await?.into_sdk(),
    };

    // 4. Ligar ao servidor do banco e enviar o pedido dentro do MPC-TLS.
    let server_addr = format!("{hostname}:{port}");
    let tcp = TcpStream::connect(&server_addr)
        .await
        .map_err(|e| WalletError::Network(format!("não liguei a {server_addr}: {e}")))?;

    let response = prover
        .send_request_mpc(tcp.compat(), sdk_request)
        .await
        .map_err(|e| WalletError::Network(format!("pedido MPC: {e}")))?;

    // 4. Calcular o que se revela.
    let transcript = prover
        .transcript()
        .map_err(|e| WalletError::Network(format!("transcript: {e}")))?;
    let computed = compute_reveal(&transcript.sent, &transcript.recv, &default_handlers())
        .map_err(|e| WalletError::Network(format!("compute_reveal: {e}")))?;

    let response_body = response
        .body
        .as_ref()
        .map(|b| String::from_utf8_lossy(b).into_owned())
        .unwrap_or_default();

    let preview = ProvePreview {
        session_id: session_id.clone(),
        status: response.status,
        response_body: response_body.clone(),
        descriptors: build_descriptors(&transcript.sent, &transcript.recv, &computed),
    };

    // 5. Portão de aprovação (bloqueia esta thread; o runtime segue a correr).
    if !approve(preview) {
        tracing::info!("reveal recusado — sessão descartada");
        let _ = session_ws.close(None).await;
        return Ok(ProveDecision::Rejected);
    }

    // 6. Revelar e finalizar.
    prover
        .reveal(computed.reveal.clone(), computed.commit.clone())
        .await
        .map_err(|e| WalletError::Network(format!("reveal: {e}")))?;

    // 7. Dizer ao notário que ranges revelar e **esperar pela conclusão**.
    //    Sem esta espera o notário não guarda a atestação (mineração → 404).
    let reveal_config = build_reveal_config(&computed);
    session_ws
        .send(Message::Text(reveal_config.to_string()))
        .await
        .map_err(|e| WalletError::Network(format!("reveal_config: {e}")))?;

    match tokio::time::timeout(Duration::from_secs(30), session_ws.next()).await {
        Ok(Some(Ok(Message::Text(text)))) => tracing::info!("notário: {text}"),
        Ok(Some(Ok(_))) => {}
        Ok(Some(Err(e))) => tracing::warn!("sessão: {e}"),
        Ok(None) => tracing::info!("sessão fechada pelo notário"),
        Err(_) => tracing::warn!("timeout à espera do notário"),
    }

    Ok(ProveDecision::Approved(ProveOutcome {
        session_id,
        server_name: hostname,
        status: response.status,
        response_body,
        transcript_sent_b64: B64.encode(&transcript.sent),
        transcript_recv_b64: B64.encode(&transcript.recv),
    }))
}

fn ws_base(notary_url: &str) -> Result<String, WalletError> {
    let parsed = url::Url::parse(notary_url)
        .map_err(|e| WalletError::Network(format!("notário inválido: {e}")))?;
    let scheme = if parsed.scheme() == "https" { "wss" } else { "ws" };
    Ok(format!(
        "{scheme}://{}{}",
        parsed.host_str().unwrap_or("127.0.0.1"),
        parsed.port().map(|p| format!(":{p}")).unwrap_or_default()
    ))
}

/// Regista a sessão e devolve o socket (que leva depois o `reveal_config`).
async fn register_session(
    notary_url: &str,
) -> Result<(WebSocketStream<MaybeTlsStream<TcpStream>>, String), WalletError> {
    let (mut ws, _) = connect_async(format!("{}/session", ws_base(notary_url)?))
        .await
        .map_err(|e| WalletError::Network(format!("sessão: {e}")))?;

    let register = serde_json::json!({
        "type": "register",
        "maxRecvData": MAX_RECV_DATA,
        "maxSentData": MAX_SENT_DATA,
        "sessionData": { "purpose": "luggom_desktop" },
    });
    ws.send(Message::Text(register.to_string()))
        .await
        .map_err(|e| WalletError::Network(format!("registo: {e}")))?;

    while let Some(message) = ws.next().await {
        let text = match message {
            Ok(Message::Text(text)) => text,
            Ok(_) => continue,
            Err(e) => return Err(WalletError::Network(format!("sessão: {e}"))),
        };
        let value: serde_json::Value = serde_json::from_str(&text)
            .map_err(|e| WalletError::Network(format!("resposta do notário: {e}")))?;
        match value["type"].as_str() {
            Some("session_registered") => {
                let id = value["sessionId"]
                    .as_str()
                    .ok_or_else(|| WalletError::Network("sem sessionId".into()))?;
                return Ok((ws, id.to_string()));
            }
            Some("error") => {
                return Err(WalletError::Network(format!(
                    "notário recusou a sessão: {}",
                    value["message"].as_str().unwrap_or("?")
                )))
            }
            _ => continue,
        }
    }
    Err(WalletError::Network("a sessão fechou sem registo".into()))
}

// ---------------------------------------------------------------------------
// Adaptador WebSocket → AsyncRead/AsyncWrite (cópia do CLI)
// ---------------------------------------------------------------------------

/// Adapta um `WebSocketStream` (mensagens) a um fluxo de bytes contínuo, para
/// satisfazer o trait `Io` da `tlsn-sdk-core`. É o mesmo código do CLI.
pub struct WsIoAdapter {
    inner: WebSocketStream<MaybeTlsStream<TcpStream>>,
    read_buf: Vec<u8>,
    read_pos: usize,
}

impl WsIoAdapter {
    pub fn new(ws: WebSocketStream<MaybeTlsStream<TcpStream>>) -> Self {
        Self {
            inner: ws,
            read_buf: Vec::new(),
            read_pos: 0,
        }
    }
}

impl AsyncRead for WsIoAdapter {
    fn poll_read(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &mut [u8],
    ) -> Poll<std::io::Result<usize>> {
        if self.read_pos < self.read_buf.len() {
            let remaining = &self.read_buf[self.read_pos..];
            let n = remaining.len().min(buf.len());
            buf[..n].copy_from_slice(&remaining[..n]);
            self.read_pos += n;
            if self.read_pos >= self.read_buf.len() {
                self.read_buf.clear();
                self.read_pos = 0;
            }
            return Poll::Ready(Ok(n));
        }

        match Pin::new(&mut self.inner).poll_next(cx) {
            Poll::Ready(Some(Ok(Message::Binary(data)))) => {
                let n = data.len().min(buf.len());
                buf[..n].copy_from_slice(&data[..n]);
                if n < data.len() {
                    self.read_buf = data[n..].to_vec();
                    self.read_pos = 0;
                }
                Poll::Ready(Ok(n))
            }
            Poll::Ready(Some(Ok(Message::Close(_)))) | Poll::Ready(None) => Poll::Ready(Ok(0)),
            Poll::Ready(Some(Ok(_))) => {
                cx.waker().wake_by_ref();
                Poll::Pending
            }
            Poll::Ready(Some(Err(e))) => {
                Poll::Ready(Err(std::io::Error::other(e)))
            }
            Poll::Pending => Poll::Pending,
        }
    }
}

impl AsyncWrite for WsIoAdapter {
    fn poll_write(
        mut self: Pin<&mut Self>,
        cx: &mut Context<'_>,
        buf: &[u8],
    ) -> Poll<std::io::Result<usize>> {
        match Pin::new(&mut self.inner).poll_ready(cx) {
            Poll::Ready(Ok(())) => {
                let msg = Message::Binary(buf.to_vec());
                match Pin::new(&mut self.inner).start_send(msg) {
                    Ok(()) => Poll::Ready(Ok(buf.len())),
                    Err(e) => Poll::Ready(Err(std::io::Error::other(e))),
                }
            }
            Poll::Ready(Err(e)) => {
                Poll::Ready(Err(std::io::Error::other(e)))
            }
            Poll::Pending => Poll::Pending,
        }
    }

    fn poll_flush(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        Pin::new(&mut self.inner).poll_flush(cx).map_err(std::io::Error::other)
    }

    fn poll_close(mut self: Pin<&mut Self>, cx: &mut Context<'_>) -> Poll<std::io::Result<()>> {
        Pin::new(&mut self.inner).poll_close(cx).map_err(std::io::Error::other)
    }
}

// ---------------------------------------------------------------------------
// Descritores e reveal_config
// ---------------------------------------------------------------------------

fn build_descriptors(
    sent: &[u8],
    recv: &[u8],
    computed: &ComputeRevealOutput,
) -> Vec<RevealDescriptor> {
    let mut out = Vec::new();
    for range in &computed.sent_ranges_with_handlers {
        out.push(descriptor_from(range, sent, "SENT"));
    }
    for range in &computed.recv_ranges_with_handlers {
        out.push(descriptor_from(range, recv, "RECV"));
    }
    out
}

fn descriptor_from(range: &RangeWithHandler, bytes: &[u8], direction: &str) -> RevealDescriptor {
    let slice = bytes.get(range.start..range.end).unwrap_or(&[]);
    let (action, label) = match &range.handler.action {
        HandlerAction::Reveal => ("REVEAL".to_string(), format!("{direction} · corpo")),
        HandlerAction::Hash { .. } => ("HASH".to_string(), format!("{direction} · hash")),
    };
    RevealDescriptor {
        direction: direction.to_string(),
        label,
        action,
        preview: String::from_utf8_lossy(slice).into_owned(),
    }
}

/// JSON do `reveal_config` no formato que o notário espera (`{kind: ...}`).
fn build_reveal_config(computed: &ComputeRevealOutput) -> serde_json::Value {
    let map = |range: &RangeWithHandler| {
        serde_json::json!({
            "start": range.start,
            "end": range.end,
            "handler": {
                "type": match range.handler.handler_type {
                    HandlerType::Sent => "SENT",
                    HandlerType::Recv => "RECV",
                },
                "part": "ALL",
                "action": { "kind": "REVEAL" },
            }
        })
    };
    let sent: Vec<_> = computed.sent_ranges_with_handlers.iter().map(map).collect();
    let recv: Vec<_> = computed.recv_ranges_with_handlers.iter().map(map).collect();
    serde_json::json!({ "type": "reveal_config", "sent": sent, "recv": recv })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn request(url: &str) -> ProveRequest {
        ProveRequest {
            url: url.into(),
            method: "GET".into(),
            headers: vec![],
            body: None,
        }
    }

    #[test]
    fn derives_the_server_name_from_the_receipt_url() {
        assert_eq!(
            request("https://contaonline.viacredi.coop.br/portal/receipt")
                .server_name()
                .unwrap(),
            "contaonline.viacredi.coop.br"
        );
        assert_eq!(
            request("https://banco.example.com:8443/extrato")
                .server_name()
                .unwrap(),
            "banco.example.com:8443"
        );
    }

    #[test]
    fn rejects_a_receipt_url_without_host() {
        assert!(request("não é uma url").server_name().is_err());
    }

    #[test]
    fn accepts_the_collector_request_format() {
        let parsed: ProveRequest = serde_json::from_str(
            r#"{
                "url": "https://contaonline.viacredi.coop.br/servcorews/processJSON",
                "method": "POST",
                "headers": [ { "name": "Cookie", "value": "JSESSIONID=abc" } ],
                "body": "{\"fi\":7}"
            }"#,
        )
        .unwrap();
        assert_eq!(parsed.method, "POST");
        assert_eq!(parsed.headers.len(), 1);
        assert_eq!(parsed.body.as_deref(), Some("{\"fi\":7}"));

        let (host, port) = parsed.host_and_port().unwrap();
        assert_eq!(host, "contaonline.viacredi.coop.br");
        assert_eq!(port, 443);
    }

    #[test]
    fn defaults_the_method_to_get() {
        let parsed: ProveRequest =
            serde_json::from_str(r#"{"url": "https://example.com/"}"#).unwrap();
        assert_eq!(parsed.method, "GET");
    }

    #[test]
    fn builds_the_ws_base_from_http_and_https() {
        assert_eq!(ws_base("http://127.0.0.1:7047").unwrap(), "ws://127.0.0.1:7047");
        assert_eq!(ws_base("https://notary.example").unwrap(), "wss://notary.example");
    }
}
