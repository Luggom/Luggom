//! Prova do prover embutido (A4) contra o **notário real**.
//!
//! Corre o MPC-TLS ponta-a-ponta: o prover liga-se ao notário (`:7047`) e ao
//! servidor TLS alvo, e devolve o transcript atestado. É a peça que o A4
//! embute no desktop — este teste prova que ela funciona, sem banco real.
//!
//! Fica desligado por omissão (fala com a rede e com o notário):
//!
//! ```text
//! cd desktop
//! LUGGOM_LIVE_NOTARY=1 cargo test -p luggom-wallet --test prover_live -- --ignored --nocapture
//! ```
//!
//! Variáveis: `LUGGOM_NOTARY_URL` (default `http://127.0.0.1:7047`) e
//! `LUGGOM_PROVE_TARGET` (um URL HTTPS com certificado válido).

use luggom_wallet::prover::{prove, ProveDecision, ProveHeader, ProveRequest, ProveSource};

fn enabled() -> bool {
    matches!(std::env::var("LUGGOM_LIVE_NOTARY").as_deref(), Ok("1"))
}

#[test]
#[ignore = "fala com o notário e com um servidor TLS real; requer LUGGOM_LIVE_NOTARY=1"]
fn proves_a_real_https_request_end_to_end() {
    if !enabled() {
        eprintln!("LUGGOM_LIVE_NOTARY != 1 — teste saltado");
        return;
    }

    let notary =
        std::env::var("LUGGOM_NOTARY_URL").unwrap_or_else(|_| "http://127.0.0.1:7047".to_string());
    let target = std::env::var("LUGGOM_PROVE_TARGET")
        .unwrap_or_else(|_| "https://example.com/".to_string());

    let request = ProveRequest {
        url: target,
        method: "GET".to_string(),
        headers: vec![
            ProveHeader {
                name: "Accept".to_string(),
                value: "text/html".to_string(),
            },
            ProveHeader {
                name: "User-Agent".to_string(),
                value: "luggom-desktop-prover/0.1".to_string(),
            },
        ],
        body: None,
    };

    // O portão de aprovação: aqui aprova-se sempre; o que se verifica é que o
    // notário **conclui a verificação** (é isso que guarda a atestação).
    let decision = prove(ProveSource::Ready(request), notary, |preview| {
        println!(
            "fase 1: HTTP {} · {} descritores de revelação",
            preview.status,
            preview.descriptors.len()
        );
        // O status não é fixado: o `sdk-core` emite a request-line em forma
        // absoluta, que CDNs recusam com 400 — mas a sessão TLS completa.
        assert!(preview.status > 0, "não houve resposta HTTP");
        assert!(
            !preview.descriptors.is_empty(),
            "o prover não produziu descritores"
        );
        true
    })
    .expect("prove falhou");

    match decision {
        ProveDecision::Approved(outcome) => {
            println!(
                "fase 2: transcript sent/recv = {}/{} bytes (base64)",
                outcome.transcript_sent_b64.len(),
                outcome.transcript_recv_b64.len()
            );
            assert!(!outcome.response_body.is_empty(), "corpo revelado vazio");
            assert!(
                !outcome.transcript_recv_b64.is_empty(),
                "transcript vazio — a prova não avançou"
            );
        }
        ProveDecision::Rejected => panic!("o portão recusou sem razão"),
    }
}
