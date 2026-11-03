//! Construtor do pedido do comprovante do **Banco do Brasil** no momento de provar.
//!
//! Porte do `coletor-browser/make_prover_request_bb.py` para Rust. O BB não
//! assina o pedido como a Viacredi: o comprovante é uma página JSP autenticada
//! só por cookies, por isso o pedido pode ser montado a partir da captura.
//!
//! O que limita é o **tamanho**: o prover commita o pedido inteiro em MPC e o
//! record layer corta acima de 4096+32 bytes. O jar do BB (~46 cookies,
//! ~3,9 KB só de `Cookie`) estourava; aqui corta-se a telemetria e, se ainda
//! não chegar, o último recurso (`gas.gcs.sign`), até caber.
//!
//! Validado numa sessão real: 24 de 47 cookies, 4018 B, comprovante devolvido
//! (`sessaofinalizada` ausente).

use serde_json::Value;

use crate::prover::{ProveHeader, ProveRequest};
use crate::WalletError;

const UA: &str = "Mozilla/5.0 (X11; Linux x86_64; rv:154.0) Gecko/20100101 Firefox/154.0";

/// Tecto do record layer do TLSNotary (4096 + 32).
const TECTO: usize = 4128;

/// Cookies sem papel na sessão (telemetria/marketing) — cortados sempre.
const COOKIES_IGNORADOS: &[&str] = &[
    "ttcsid", "_pk_", "ADRUM", "_ga", "_gid", "_gat", "_gcl_", "_gali", "_fbp", "_clck", "_clsk",
    "_ttp", "_tt_", "_twpid", "_twsid", "_dc_gtm_", "_sfid_", "_evga_",
];

/// Último recurso, por ordem, se mesmo assim estourar. Provado na sessão real:
/// o comprovante responde sem o `gas.gcs.sign`.
const COOKIES_ULTIMO_RECURSO: &[&str] = &["gas.gcs.sign"];

/// Pedido do comprovante, tal como sai da captura.
#[derive(Debug, Clone, serde::Serialize)]
pub struct ReceiptCall {
    pub url: String,
    pub method: String,
    pub body: String,
}

/// Captura do coletor reduzida ao que serve para provar.
#[derive(Debug, Clone)]
pub struct Capture {
    pub cookie: String,
}

impl Capture {
    /// Fail-closed: sem o jar de cookies não se constrói nada.
    pub fn parse(raw: &str) -> Result<Self, WalletError> {
        let doc: Value = serde_json::from_str(raw)
            .map_err(|e| WalletError::Network(format!("captura inválida: {e}")))?;
        let cookie = doc
            .get("cookies")
            .and_then(|v| v.as_str())
            .filter(|c| !c.trim().is_empty())
            .ok_or_else(|| {
                WalletError::Network(
                    "a captura não traz cookies (precisa da permissão `cookies` no coletor)".into(),
                )
            })?;
        Ok(Self {
            cookie: cookie.to_string(),
        })
    }

    /// O pedido do comprovante: o último `same-origin` cujo URL tem
    /// `comprovante`. Fail-closed se não houver.
    pub fn receipt_call(raw: &str) -> Result<ReceiptCall, WalletError> {
        let doc: Value = serde_json::from_str(raw)
            .map_err(|e| WalletError::Network(format!("captura inválida: {e}")))?;
        let reqs = doc
            .get("reqs")
            .and_then(|v| v.as_array())
            .ok_or_else(|| WalletError::Network("captura sem `reqs`".into()))?;
        let origin = origin_of(&doc, reqs)?;

        // Preferir a rota clássica do comprovante (`/aapf/comprovante/…`), que é
        // a que devolve o recibo em HTML. Só se não existir é que se aceita a
        // "Central de Comprovantes" (`cfe-sol-central-comprovantes/…`).
        let requisito = |needle: &str| {
            reqs.iter()
                .rev()
                .find(|q| {
                    q.get("url")
                        .and_then(|v| v.as_str())
                        .map(|u| u.to_ascii_lowercase().contains(needle))
                        .unwrap_or(false)
                })
                .cloned()
        };
        let q = requisito("aapf/comprovante")
            .or_else(|| requisito("comprovante"))
            .ok_or_else(|| {
                WalletError::Network(
                    "a captura não tem o pedido do comprovante — abre o comprovante no banco".into(),
                )
            })?;

        let q = &q;

        let raw_url = q.get("url").and_then(|v| v.as_str()).unwrap_or("");
        Ok(ReceiptCall {
            url: absolute(&origin, raw_url)?,
            method: q
                .get("method")
                .and_then(|v| v.as_str())
                .unwrap_or("POST")
                .to_uppercase(),
            body: q.get("body").and_then(|v| v.as_str()).unwrap_or("").to_string(),
        })
    }

    /// Constrói o pedido com o `Cookie` encaixado no tecto do prover.
    pub fn build(&self, call: &ReceiptCall) -> Result<ProveRequest, WalletError> {
        let parsed = url::Url::parse(&call.url)
            .map_err(|e| WalletError::Network(format!("URL do comprovante inválida: {e}")))?;
        let host = parsed
            .host_str()
            .ok_or_else(|| WalletError::Network("URL do comprovante sem host".into()))?;
        let scheme = parsed.scheme();
        let origin = format!("{scheme}://{host}");
        let host_header = match parsed.port() {
            Some(port) => format!("{host}:{port}"),
            None => host.to_string(),
        };

        let mut headers = vec![
            header("Host", &host_header),
            header("User-Agent", UA),
            header("Accept", "application/json, text/plain, */*"),
            header("Accept-Language", "pt-BR,pt;q=0.8,en-US;q=0.5,en;q=0.3"),
            header("X-Requested-With", "XMLHttpRequest"),
            // Sec-Fetch-* de uma chamada same-origin da página. NÃO se manda
            // Accept-Encoding: comprimiria a resposta e o extractor deixaria de
            // ver o corpo.
            header("Sec-Fetch-Dest", "empty"),
            header("Sec-Fetch-Mode", "cors"),
            header("Sec-Fetch-Site", "same-origin"),
            header("Referer", &format!("{origin}/")),
            header("Origin", &origin),
            header("Connection", "close"),
            header("Cookie", ""),
        ];
        if !call.body.is_empty() {
            headers.insert(
                5,
                header(
                    "Content-Type",
                    "application/x-www-form-urlencoded; charset=UTF-8",
                ),
            );
        }

        let outros = headers
            .iter()
            .map(|h| h.name.len() + h.value.len() + 4)
            .sum::<usize>()
            + call.body.len()
            + 4;

        let cookie = fit_cookie(&self.cookie, outros);
        for h in &mut headers {
            if h.name == "Cookie" {
                h.value = cookie.clone();
            }
        }

        Ok(ProveRequest {
            url: call.url.clone(),
            method: call.method.clone(),
            headers,
            body: if call.body.is_empty() {
                None
            } else {
                Some(call.body.clone())
            },
        })
    }
}

fn header(name: &str, value: &str) -> ProveHeader {
    ProveHeader {
        name: name.to_string(),
        value: value.to_string(),
    }
}

/// Origem da captura: o `pageUrl` (o background acrescenta-o), senão a primeira
/// resposta/pedido com um URL absoluto.
fn origin_of(doc: &Value, reqs: &[Value]) -> Result<String, WalletError> {
    if let Some(page) = doc.get("pageUrl").and_then(|v| v.as_str()) {
        if let Ok(parsed) = url::Url::parse(page) {
            if let Some(host) = parsed.host_str() {
                return Ok(format!("{}://{host}", parsed.scheme()));
            }
        }
    }
    for q in reqs.iter().rev() {
        if let Some(u) = q.get("url").and_then(|v| v.as_str()) {
            if let Ok(parsed) = url::Url::parse(u) {
                if let Some(host) = parsed.host_str() {
                    return Ok(format!("{}://{host}", parsed.scheme()));
                }
            }
        }
    }
    Err(WalletError::Network(
        "não consegui a origem do comprovante (captura sem `pageUrl`)".into(),
    ))
}

/// URL absoluto do pedido, sem o cache-buster `nocache=`.
fn absolute(origin: &str, raw: &str) -> Result<String, WalletError> {
    if raw.starts_with("http") {
        return Ok(raw.to_string());
    }
    let (path, query) = match raw.split_once('?') {
        Some((p, q)) => (p, Some(q)),
        None => (raw, None),
    };
    let params: Vec<&str> = match query {
        Some(q) => q.split('&').filter(|p| !p.starts_with("nocache=")).collect(),
        None => Vec::new(),
    };
    if path.is_empty() {
        return Err(WalletError::Network("pedido do comprovante sem caminho".into()));
    }
    let mut out = format!("{origin}{path}");
    if !params.is_empty() {
        out.push('?');
        out.push_str(&params.join("&"));
    }
    Ok(out)
}

/// Corta a telemetria e, se ainda não chegar, o último recurso — até caber.
fn fit_cookie(raw: &str, outros_bytes: usize) -> String {
    for i in 0..=COOKIES_ULTIMO_RECURSO.len() {
        let candidate = prune_cookie(raw, &COOKIES_ULTIMO_RECURSO[..i]);
        if outros_bytes + candidate.len() <= TECTO {
            return candidate;
        }
    }
    prune_cookie(raw, COOKIES_ULTIMO_RECURSO)
}

fn prune_cookie(cookie: &str, extra: &[&str]) -> String {
    cookie
        .split(';')
        .map(str::trim)
        .filter(|part| !part.is_empty())
        .filter(|part| {
            let name = part.split('=').next().unwrap_or("");
            !extra.contains(&name) && !COOKIES_IGNORADOS.iter().any(|prefix| name.starts_with(prefix))
        })
        .collect::<Vec<_>>()
        .join("; ")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn prunes_telemetry_and_keeps_the_session() {
        let pruned = prune_cookie(
            "JSESSIONID=abc; ttcsid=1; _pk_ref.16.dae0=x; gas.gcs.sign=sig; TS018652b6=waf",
            &[],
        );
        assert_eq!(pruned, "JSESSIONID=abc; gas.gcs.sign=sig; TS018652b6=waf");
    }

    #[test]
    fn drops_the_last_resort_cookie_only_when_needed() {
        // Cabe com tudo: não toca no gas.gcs.sign.
        let raw = "JSESSIONID=abc; gas.gcs.sign=sig";
        assert!(fit_cookie(raw, 3000).contains("gas.gcs.sign"));
        // Não cabe: corta o último recurso.
        assert!(!fit_cookie(raw, 4128 - 10).contains("gas.gcs.sign"));
    }

    #[test]
    fn picks_the_receipt_request_and_resolves_the_url() {
        let raw = r#"{"pageUrl":"https://www2.bb.com.br/aapf/principal.jsp",
            "cookies":"JSESSIONID=abc",
            "reqs":[
              {"url":"/aapf/servico","method":"POST","body":"a=1"},
              {"url":"/aapf/comprovante/869-CO.jsp?time=1&nocache=2","method":"POST","body":"b=2"}
            ]}"#;
        let call = Capture::receipt_call(raw).unwrap();
        assert_eq!(
            call.url,
            "https://www2.bb.com.br/aapf/comprovante/869-CO.jsp?time=1"
        );
        assert_eq!(call.method, "POST");
        assert_eq!(call.body, "b=2");
    }

    #[test]
    fn prefers_the_classic_route_over_the_central_one() {
        let raw = r#"{"pageUrl":"https://www2.bb.com.br/aapf/principal.jsp",
            "cookies":"JSESSIONID=abc",
            "reqs":[
              {"url":"/aapf/comprovante/869-CO.jsp","method":"POST","body":"a=1"},
              {"url":"https://www2.bb.com.br/cfe-sol-central-comprovantes/api/v1/gerador/comprovante/emitir","method":"POST","body":"b=2"}
            ]}"#;
        let call = Capture::receipt_call(raw).unwrap();
        assert!(call.url.contains("/aapf/comprovante/"));
    }

    #[test]
    fn falls_back_to_the_central_comprovante_api() {
        let raw = r#"{"pageUrl":"https://www2.bb.com.br/aapf/principal.jsp",
            "cookies":"JSESSIONID=abc",
            "reqs":[{"url":"https://www2.bb.com.br/cfe-sol-central-comprovantes/api/v1/gerador/comprovante/emitir","method":"POST","body":"b=2"}]}"#;
        let call = Capture::receipt_call(raw).unwrap();
        assert!(call.url.contains("cfe-sol-central-comprovantes"));
    }

    #[test]
    fn refuses_a_capture_without_cookies_or_receipt() {
        assert!(Capture::parse(r#"{"reqs":[]}"#).is_err());
        assert!(Capture::receipt_call(r#"{"reqs":[]}"#).is_err());
    }

    #[test]
    fn builds_a_request_that_fits_the_ceiling() {
        let capture = Capture {
            cookie: (0..46)
                .map(|i| format!("c{i}=0123456789"))
                .chain(std::iter::once("JSESSIONID=abc".to_string()))
                .chain(std::iter::once("ttcsid=xyz".to_string()))
                .collect::<Vec<_>>()
                .join("; "),
        };
        let call = ReceiptCall {
            url: "https://www2.bb.com.br/aapf/comprovante/869-CO.jsp?time=1".into(),
            method: "POST".into(),
            body: "tipoComprovante=91".into(),
        };
        let request = capture.build(&call).unwrap();
        let size = request
            .headers
            .iter()
            .map(|h| h.name.len() + h.value.len() + 4)
            .sum::<usize>()
            + request.body.as_deref().unwrap_or("").len()
            + 4;
        assert!(size <= TECTO, "pedido com {size} B excede {TECTO}");
        assert!(request.headers.iter().any(|h| h.name == "Cookie"
            && !h.value.contains("ttcsid")));
    }
}
