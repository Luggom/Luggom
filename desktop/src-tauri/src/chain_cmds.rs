//! Comandos do A3: saldo, extrato e envio de LUG.
//!
//! Falam diretamente com o nó (REST/LCD) — a API do Luggom é do portão
//! anti-fraude, não da carteira. Assim a carteira funciona com a API em baixo.

use serde::Serialize;
use tauri::State;

use luggom_wallet::{
    build_send_tx, lug_to_uluggom, uluggom_to_lug, ChainClient, Direction, SendPlan,
};

use crate::error::CommandError;
use crate::settings;
use crate::state::AppState;

fn client() -> Result<ChainClient, CommandError> {
    ChainClient::new(settings::node_url()).map_err(CommandError::from)
}

#[derive(Debug, Serialize)]
pub struct ChainInfoReply {
    pub node_url: String,
    pub reachable: bool,
    pub chain_id: Option<String>,
    pub height: Option<i64>,
    pub error: Option<String>,
}

#[derive(Debug, Serialize)]
pub struct BalanceReply {
    pub address: String,
    /// Em uluggom, como **texto**: o JS não representa 10^18 com exatidão.
    pub balance_uluggom: String,
    pub balance_lug: String,
    /// `false` enquanto a conta não tiver estado na chain.
    pub exists: bool,
}

#[derive(Debug, Serialize)]
pub struct SendPlanReply {
    pub amount_lug: String,
    pub gas_fee_lug: String,
    pub flat_fee_lug: String,
    pub total_lug: String,
}

#[derive(Debug, Serialize)]
pub struct TransferReply {
    pub tx_hash: String,
    pub height: i64,
    pub timestamp: String,
    /// `"in"` ou `"out"`.
    pub direction: &'static str,
    pub counterparty: String,
    pub amount_lug: String,
}

#[derive(Debug, Serialize)]
pub struct SendReceipt {
    pub tx_hash: String,
    pub code: u32,
}

/// Nó configurado.
#[tauri::command]
pub fn chain_node_url() -> String {
    settings::node_url()
}

/// Muda o nó e grava a escolha.
#[tauri::command]
pub fn chain_set_node_url(url: String) -> Result<(), CommandError> {
    // Validar aqui evita gravar algo que só falharia no primeiro pedido.
    ChainClient::new(url.trim()).map_err(CommandError::from)?;
    settings::save_node_url(&url).map_err(|e| CommandError::internal(e.to_string()))
}

/// Estado do nó: alcançável? que chain? que altura?
#[tauri::command]
pub async fn chain_info() -> Result<ChainInfoReply, CommandError> {
    let node_url = settings::node_url();

    let reply = match client() {
        Ok(client) => match client.chain_id().await {
            Ok(chain_id) => ChainInfoReply {
                node_url,
                reachable: true,
                chain_id: Some(chain_id),
                height: client.height().await.ok(),
                error: None,
            },
            Err(err) => ChainInfoReply {
                node_url,
                reachable: false,
                chain_id: None,
                height: None,
                error: Some(err.to_string()),
            },
        },
        Err(err) => ChainInfoReply {
            node_url,
            reachable: false,
            chain_id: None,
            height: None,
            error: Some(err.message),
        },
    };

    Ok(reply)
}

/// Saldo da carteira aberta.
#[tauri::command]
pub async fn wallet_balance(state: State<'_, AppState>) -> Result<BalanceReply, CommandError> {
    let address = state
        .address()
        .ok_or_else(|| CommandError::new("locked", "A carteira está fechada."))?;

    let account = client()?.account(&address).await?;
    Ok(BalanceReply {
        address,
        balance_uluggom: account.balance_uluggom.to_string(),
        balance_lug: uluggom_to_lug(account.balance_uluggom),
        exists: account.exists,
    })
}

/// Extrato: transferências LUG de entrada e saída.
#[tauri::command]
pub async fn wallet_history(
    limit: u32,
    state: State<'_, AppState>,
) -> Result<Vec<TransferReply>, CommandError> {
    let address = state
        .address()
        .ok_or_else(|| CommandError::new("locked", "A carteira está fechada."))?;

    let transfers = client()?.history(&address, limit.clamp(1, 100)).await?;

    Ok(transfers
        .into_iter()
        .map(|transfer| {
            let amount_lug = transfer.amount_lug();
            TransferReply {
                tx_hash: transfer.tx_hash,
                height: transfer.height,
                timestamp: transfer.timestamp,
                direction: match transfer.direction {
                    Direction::In => "in",
                    Direction::Out => "out",
                },
                counterparty: transfer.counterparty,
                amount_lug,
            }
        })
        .collect())
}

/// Uma mineração, para o extrato de mineração: o que se pagou (na moeda do
/// recibo) e o LUG que entrou.
#[derive(Debug, Serialize)]
pub struct MiningRow {
    pub tx_hash: String,
    pub height: i64,
    pub timestamp: String,
    pub tax_entity: String,
    /// Valor pago (ex.: `"1267.82"`), na moeda de `currency`.
    pub amount: String,
    pub currency: String,
    /// LUG que entrou na carteira, formatado.
    pub lug: String,
}

/// Extrato de **mineração**: o imposto pago (FIAT) e quanto virou em LUG.
///
/// É só **leitura** da chain — os valores que a app mostra são os que a chain
/// registou, não uma reconstrução.
#[tauri::command]
pub async fn mining_history(
    limit: u32,
    state: State<'_, AppState>,
) -> Result<Vec<MiningRow>, CommandError> {
    let address = state
        .address()
        .ok_or_else(|| CommandError::new("locked", "A carteira está fechada."))?;

    let records = client()?.mining_history(&address, limit.clamp(1, 50)).await?;

    Ok(records
        .into_iter()
        .map(|record| {
            // Calcular antes de mover os campos (o `MiningRow` fica com Strings).
            let amount = record.amount_short();
            let lug = record.lug();
            MiningRow {
                tx_hash: record.tx_hash,
                height: record.height,
                timestamp: record.timestamp,
                tax_entity: record.tax_entity,
                amount,
                currency: record.currency,
                lug,
            }
        })
        .collect())
}

/// Quanto custa enviar este valor (valor + gás + taxa fixa), sem tocar na rede.
#[tauri::command]
pub fn wallet_send_plan(amount_lug: String) -> Result<SendPlanReply, CommandError> {
    let amount = lug_to_uluggom(&amount_lug)?;
    let plan = SendPlan::new(amount);
    Ok(SendPlanReply {
        amount_lug: uluggom_to_lug(plan.amount_uluggom),
        gas_fee_lug: uluggom_to_lug(plan.gas_fee_uluggom),
        flat_fee_lug: uluggom_to_lug(plan.flat_fee_uluggom),
        total_lug: uluggom_to_lug(plan.total_uluggom()),
    })
}

/// Envia LUG: monta, assina localmente e transmite.
///
/// A frase é copiada para um `Zeroizing` (é o único ponto em que existe uma
/// cópia fora do `SecretPhrase`) porque a operação atravessa `await`s e não se
/// pode segurar o trinco do estado durante uma ida à rede.
#[tauri::command]
pub async fn wallet_send(
    to: String,
    amount_lug: String,
    state: State<'_, AppState>,
) -> Result<SendReceipt, CommandError> {
    let (mnemonic, address) = state
        .mnemonic_copy()
        .ok_or_else(|| CommandError::new("locked", "A carteira está fechada."))?;

    let amount = lug_to_uluggom(&amount_lug)?;
    let client = client()?;

    // O chain_id e a sequência vêm do nó: assinar com valores errados dá uma
    // assinatura que a chain recusa (ou pior, um replay).
    let chain_id = client.chain_id().await?;
    let account = client.account(&address).await?;

    let plan = SendPlan::new(amount);
    if account.balance_uluggom < plan.total_uluggom() {
        return Err(CommandError::new(
            "insufficient_funds",
            "O saldo não chega para o valor mais as taxas.",
        ));
    }

    let tx_raw = build_send_tx(
        &mnemonic,
        &to,
        amount,
        &chain_id,
        account.account_number,
        account.sequence,
    )?;

    let sent = client.broadcast_tx_raw(&tx_raw).await?;
    if !sent.is_ok() {
        return Err(CommandError::new(
            "tx_rejected",
            format!("A chain recusou a transação (código {}).", sent.code),
        ));
    }

    let confirmed = match client.wait_for_tx(&sent.tx_hash, 20).await {
        Ok(confirmed) => confirmed,
        // Ficou só em mempool: devolvemos o hash (a tx pode entrar depois).
        Err(_) => sent,
    };

    if !confirmed.is_ok() {
        return Err(CommandError::new(
            "tx_rejected",
            format!("A transação falhou (código {}).", confirmed.code),
        ));
    }

    Ok(SendReceipt {
        tx_hash: confirmed.tx_hash,
        code: confirmed.code,
    })
}
