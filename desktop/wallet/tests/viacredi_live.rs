//! Prova do **construtor do pedido** (Viacredi) contra o banco e o notário reais.
//!
//! É o que tira o coletor do caminho crítico: a captura dá a sessão, e o pedido
//! assinado nasce aqui — com carimbo de tempo fresco, que é o que o banco exige.
//!
//! ```text
//! cd desktop
//! LUGGOM_CAPTURE=../coletor-browser/runs/2026-09-21/captura-viacredi.json \
//!   LUGGOM_LIVE_NOTARY=1 cargo test -p luggom-wallet --test viacredi_live -- --ignored --nocapture
//! ```

use luggom_wallet::prover::{prove, ProveDecision, ProveSource};
use luggom_wallet::viacredi::Capture;

#[test]
#[ignore = "fala com o banco e com o notário reais; requer LUGGOM_CAPTURE e LUGGOM_LIVE_NOTARY=1"]
fn builds_the_request_and_proves_it() {
    if std::env::var("LUGGOM_LIVE_NOTARY").as_deref() != Ok("1") {
        eprintln!("LUGGOM_LIVE_NOTARY != 1 — teste saltado");
        return;
    }
    let path = match std::env::var("LUGGOM_CAPTURE") {
        Ok(path) if !path.trim().is_empty() => path,
        _ => {
            eprintln!("LUGGOM_CAPTURE em falta — teste saltado");
            return;
        }
    };

    let raw = std::fs::read_to_string(&path).expect("ler a captura");
    let capture = match Capture::parse(&raw) {
        Ok(capture) => capture,
        Err(e) => panic!("parse falhou: {e}"),
    };
    let call = Capture::receipt_call(&raw).expect("pedido do comprovante");
    println!(
        "comprovante: trn={} seqFlow={:?} data={}",
        call.trn, call.seq_flow, call.data
    );

    let notary =
        std::env::var("LUGGOM_NOTARY_URL").unwrap_or_else(|_| "http://127.0.0.1:7047".to_string());
    // v4: o notário assina o destinatário; o teste usa um endereço de referência.
    let miner = std::env::var("LUGGOM_MINER_ADDRESS")
        .unwrap_or_else(|_| "luggom1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5n0k6".to_string());

    let decision = prove(
        ProveSource::Viacredi { capture, call },
        miner,
        notary,
        |preview| {
            println!(
                "fase 1: HTTP {} · {} descritores",
                preview.status,
                preview.descriptors.len()
            );
            assert_eq!(preview.status, 200, "o banco não devolveu 200");
            true
        },
    )
    .expect("prove");

    match decision {
        ProveDecision::Approved(outcome) => {
            println!("fase 2: {} · recv {} bytes (b64)", outcome.server_name, outcome.transcript_recv_b64.len());
            assert!(!outcome.response_body.is_empty());
        }
        ProveDecision::Rejected => panic!("recusado sem razão"),
    }
}
