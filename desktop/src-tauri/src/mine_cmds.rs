//! Comandos do A4: mineração E2E (prova → tx não assinada → assinatura local →
//! broadcast).
//!
//! A app recebe a **sessão do banco** — da janela do banco (A6, em memória) ou,
//! no modo avançado, da captura do coletor (ficheiro) — e **constrói o pedido na
//! hora**: o `processJSON` da Viacredi exige um `Access-Time` fresco, por isso a
//! assinatura tem de nascer dentro do prover (ver `viacredi.rs`).
//!
//! O prover corre numa **thread própria** (faz `block_on`). A aprovação é
//! entregue à UI por **evento** (`mine-preview`): a thread fica bloqueada à
//! espera de [`mine_decide`], mantendo a sessão do notário viva.

use std::sync::Mutex;

use serde::Serialize;
use tauri::{Emitter, State};

use luggom_wallet::prover::{self, ProveSource};
use luggom_wallet::viacredi::Capture;
use luggom_wallet::{MineBundle, MineClient, Wallet};

use crate::error::CommandError;
use crate::state::AppState;

/// Canal para a decisão de aprovação (uma mineração de cada vez).
static PENDING: Mutex<Option<std::sync::mpsc::Sender<bool>>> = Mutex::new(None);

#[derive(Debug, Serialize, Clone)]
pub struct ProviderReply {
    pub id: String,
    pub name: String,
}

/// Resumo do que a captura traz — para a UI confirmar antes de minerar.
#[derive(Debug, Serialize)]
pub struct CaptureReply {
    pub trn: u32,
    pub seq_flow: Option<u32>,
    /// O comprovante que vai ser pedido (ex.: `{"ID_TRANSACAO":"…"}`).
    pub receipt: String,
}

#[derive(Debug, Serialize)]
pub struct MineResultReply {
    pub tx_hash: String,
    pub payment_amount: Option<String>,
    pub tax_entity: Option<String>,
    /// `true` quando o utilizador recusou os dados revelados.
    pub cancelled: bool,
}

/// Valida uma captura e devolve o comprovante que ela identifica.
///
/// É o **único** sítio que decide se uma captura serve — usado pelo ficheiro
/// (`mine_load_capture`), pela janela do banco (`bank_status`) e pela mineração.
/// Fail-closed: sem token, sem cookies ou sem comprovante, é erro.
pub(crate) fn capture_reply(raw: &str) -> Result<CaptureReply, CommandError> {
    // Cada rail tem o seu formato de captura. O discriminador é o PEDIDO do
    // comprovante (não os headers): a API nova do BB também traz
    // `Authorization`/`Client-Id`, por isso `Capture::parse` sozinho confundia-a
    // com a Viacredi.
    if let Ok(call) = Capture::receipt_call(raw) {
        if Capture::parse(raw).is_ok() {
            return Ok(CaptureReply {
                trn: call.trn,
                seq_flow: call.seq_flow,
                receipt: serde_json::to_string(&call.data).unwrap_or_default(),
            });
        }
    }

    let call = luggom_wallet::bb::Capture::receipt_call(raw).map_err(capture_error)?;
    Ok(CaptureReply {
        trn: 0,
        seq_flow: None,
        receipt: call.url,
    })
}

/// Ordem de apresentação na lista de bancos: Brasil, EUA, Canadá, resto.
fn country_rank(country: &str) -> u8 {
    match country {
        "BR" => 0,
        "US" => 1,
        "CA" => 2,
        _ => 3,
    }
}

/// Bancos aceites (whitelist on-chain, servida pela API).
#[tauri::command]
pub async fn mine_providers(api_url: String) -> Result<Vec<ProviderReply>, CommandError> {
    let client = MineClient::new(api_url)?;
    let mut providers = client.providers().await?;
    // Estável: dentro de cada país mantém-se a ordem que a API devolveu.
    providers.sort_by_key(|p| country_rank(&p.country));
    Ok(providers
        .into_iter()
        .map(|p| ProviderReply { id: p.id, name: p.name })
        .collect())
}

/// Lê a captura (ficheiro, modo avançado) e devolve o comprovante que ela identifica.
#[tauri::command]
pub fn mine_load_capture(path: String) -> Result<CaptureReply, CommandError> {
    let raw = std::fs::read_to_string(path.trim()).map_err(|e| {
        CommandError::new("not_found", format!("não consegui ler a captura: {e}"))
    })?;
    capture_reply(&raw)
}

/// Um problema da captura (sessão do banco) — não é uma falha de rede, e a UI
/// tem texto próprio para isto.
fn capture_error(e: luggom_wallet::WalletError) -> CommandError {
    CommandError::new("capture", e.to_string())
}

/// Decide a aprovação da revelação (desbloqueia a thread do prover).
#[tauri::command]
pub fn mine_decide(approved: bool) -> Result<(), CommandError> {
    if let Some(sender) = PENDING.lock().unwrap_or_else(|e| e.into_inner()).take() {
        let _ = sender.send(approved);
    }
    Ok(())
}

/// Corre a mineração: resolve a captura → prova (com aprovação) → `/mine/tax` →
/// assinatura local → broadcast. Só regressa quando termina (ou falha).
#[tauri::command]
pub async fn mine_run(
    capture_path: Option<String>,
    capture_inline: Option<String>,
    provider_id: String,
    notary_url: String,
    api_url: String,
    app: tauri::AppHandle,
    state: State<'_, AppState>,
) -> Result<MineResultReply, CommandError> {
    // A captura vem, por ordem: do argumento (testes), do ficheiro (coletor,
    // modo avançado) ou da **janela do banco** (o caminho normal). Sem nenhuma
    // delas, não há o que provar.
    let raw = if let Some(raw) = capture_inline.filter(|raw| !raw.trim().is_empty()) {
        raw
    } else if let Some(path) = capture_path.filter(|path| !path.trim().is_empty()) {
        std::fs::read_to_string(path.trim()).map_err(|e| {
            CommandError::new("not_found", format!("não consegui ler a captura: {e}"))
        })?
    } else if let Some(raw) = state.bank_capture() {
        raw
    } else {
        return Err(CommandError::new(
            "mine_capture",
            "indique a sessão do banco (abre o banco na app)",
        ));
    };

    // O pedido a provar depende do banco: cada rail tem o seu formato de captura.
    // O Brasil do BB não assina o pedido — o comprovante é uma JSP autenticada
    // só por cookies, por isso o pedido sai da própria captura.
    let source = match provider_id.as_str() {
        "banco_do_brasil" => {
            let capture = luggom_wallet::bb::Capture::parse(&raw).map_err(capture_error)?;
            let call = luggom_wallet::bb::Capture::receipt_call(&raw).map_err(capture_error)?;
            ProveSource::BancoDoBrasil { capture, call }
        }
        _ => {
            let capture = Capture::parse(&raw).map_err(capture_error)?;
            let call = Capture::receipt_call(&raw).map_err(capture_error)?;
            ProveSource::Viacredi { capture, call }
        }
    };

    let (mnemonic, address) = state
        .mnemonic_copy()
        .ok_or_else(|| CommandError::new("locked", "A carteira está fechada."))?;

    // Canal para a decisão da UI. Um envio repetido (duplo clique) cai no vazio
    // porque só há um `Sender` e ele é consumido no `take()`.
    let (decide_tx, decide_rx) = std::sync::mpsc::channel::<bool>();
    *PENDING.lock().unwrap_or_else(|e| e.into_inner()) = Some(decide_tx);

    // O prover faz `block_on` internamente → thread própria. A UI recebe a
    // pré-visualização por evento e responde com `mine_decide`.
    let prover_address = address.clone();
    let decision = tauri::async_runtime::spawn_blocking(move || {
        prover::prove(
            source,
            prover_address,
            notary_url,
            move |preview| {
                let _ = app.emit("mine-preview", &preview);
                decide_rx.recv().unwrap_or(false)
            },
        )
    })
    .await
    .map_err(|e| CommandError::internal(e.to_string()))??;

    *PENDING.lock().unwrap_or_else(|e| e.into_inner()) = None;

    let outcome = match decision {
        prover::ProveDecision::Rejected => {
            return Ok(MineResultReply {
                tx_hash: String::new(),
                payment_amount: None,
                tax_entity: None,
                cancelled: true,
            })
        }
        prover::ProveDecision::Approved(outcome) => outcome,
    };

    let wallet = Wallet::from_mnemonic(&mnemonic)?;
    let bundle = MineBundle {
        provider_id,
        dest_address: address,
        public_key: wallet.public_key().to_vec(),
        session_id: outcome.session_id,
        server_name: outcome.server_name,
        status: outcome.status,
        response_body: outcome.response_body,
        transcript_sent_b64: outcome.transcript_sent_b64,
        transcript_recv_b64: outcome.transcript_recv_b64,
    };

    let client = MineClient::new(api_url)?;
    let mined = client.mine(&wallet, &mnemonic, &bundle).await?;

    Ok(MineResultReply {
        tx_hash: mined.tx_hash,
        payment_amount: mined.payment_amount,
        tax_entity: mined.tax_entity,
        cancelled: false,
    })
}
