//! Cliente da chain (endpoints REST/LCD do Cosmos SDK).
//!
//! Fala diretamente com um nó — **não** passa pela API do Luggom. O saldo e o
//! envio são operações da carteira, não do portão anti-fraude; e assim a
//! carteira continua a funcionar mesmo com a API em baixo.

use serde::Deserialize;

use crate::amount::{uluggom_to_lug, DENOM};
use crate::WalletError;

/// Cliente de um nó (REST/LCD).
#[derive(Debug, Clone)]
pub struct ChainClient {
    base: String,
    http: reqwest::Client,
}

/// Saldo e sequência de uma conta.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct AccountInfo {
    pub address: String,
    /// Saldo em uluggom (0 se a conta ainda não existir na chain).
    pub balance_uluggom: u128,
    pub account_number: u64,
    pub sequence: u64,
    /// `false` quando a conta ainda não tem estado na chain (nunca recebeu nada).
    pub exists: bool,
}

/// Sentido de uma transferência, do ponto de vista da carteira.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Direction {
    In,
    Out,
}

/// Uma linha do extrato.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Transfer {
    pub tx_hash: String,
    pub height: i64,
    pub timestamp: String,
    pub direction: Direction,
    pub counterparty: String,
    pub amount_uluggom: u128,
}

impl Transfer {
    /// Valor em LUG, já formatado para mostrar.
    pub fn amount_lug(&self) -> String {
        uluggom_to_lug(self.amount_uluggom)
    }
}

/// Resultado de um broadcast.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct BroadcastResult {
    pub tx_hash: String,
    pub code: u32,
    pub raw_log: String,
}

impl BroadcastResult {
    pub fn is_ok(&self) -> bool {
        self.code == 0
    }
}

#[derive(Debug, Deserialize)]
struct BalancesResponse {
    #[serde(default)]
    balances: Vec<CoinJson>,
}

#[derive(Debug, Deserialize)]
struct CoinJson {
    denom: String,
    amount: String,
}

#[derive(Debug, Deserialize)]
struct AccountResponse {
    account: Option<serde_json::Value>,
}

#[derive(Debug, Deserialize)]
struct TxsResponse {
    #[serde(default)]
    txs: Vec<serde_json::Value>,
    #[serde(default)]
    tx_responses: Vec<TxResponseJson>,
}

#[derive(Debug, Deserialize)]
struct TxResponseJson {
    txhash: String,
    #[serde(default)]
    height: String,
    #[serde(default)]
    timestamp: String,
    #[serde(default)]
    code: u32,
    #[serde(default)]
    events: Vec<EventJson>,
}

#[derive(Debug, Deserialize)]
struct EventJson {
    #[serde(rename = "type", default)]
    kind: String,
    #[serde(default)]
    attributes: Vec<AttributeJson>,
}

#[derive(Debug, Deserialize)]
struct AttributeJson {
    #[serde(default)]
    key: String,
    #[serde(default)]
    value: String,
}

/// Uma mineração vista na chain: o que se pagou e o que entrou em LUG.
///
/// ⚠️ O campo da chain chama-se `payment_amount`, mas traz o valor **na moeda do
/// recibo** (ex.: BRL) — `currency` diz qual. É tudo **lido da chain**: a app não
/// recalcula a tokenomics (quem a decide é a chain).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct MiningRecord {
    pub tx_hash: String,
    pub height: i64,
    pub timestamp: String,
    pub tax_entity: String,
    pub amount: String,
    pub currency: String,
    pub lug_uluggom: u128,
}

impl MiningRecord {
    /// LUG recebido, formatado para mostrar.
    pub fn lug(&self) -> String {
        uluggom_to_lug(self.lug_uluggom)
    }

    /// O valor pago, com duas casas (é moeda, não uma fracção de 18 dígitos).
    pub fn amount_short(&self) -> String {
        format_amount(&self.amount)
    }
}

/// "1267.820000000000000000" → "1267.82".
fn format_amount(raw: &str) -> String {
    let (whole, decimals) = raw.split_once('.').unwrap_or((raw, ""));
    let cents: String = decimals.chars().take(2).collect();
    format!("{whole}.{cents:0<2}")
}

#[derive(Debug, Deserialize)]
struct BroadcastResponse {
    tx_response: Option<TxResponseJson>,
}

#[derive(Debug, Deserialize)]
struct NodeInfoResponse {
    /// O LCD usa `snake_case`; aceitamos também o camelCase por segurança.
    #[serde(default, alias = "defaultNodeInfo")]
    default_node_info: Option<NodeInfo>,
}

#[derive(Debug, Deserialize)]
struct NodeInfo {
    #[serde(default)]
    network: String,
}

#[derive(Debug, Deserialize)]
struct LatestBlockResponse {
    #[serde(default)]
    block: Option<BlockJson>,
}

#[derive(Debug, Deserialize)]
struct BlockJson {
    #[serde(default)]
    header: Option<HeaderJson>,
}

#[derive(Debug, Deserialize)]
struct HeaderJson {
    #[serde(default)]
    height: String,
}

impl ChainClient {
    /// `base_url` é a raiz do REST do nó, ex.: `http://127.0.0.1:1317`.
    pub fn new(base_url: impl Into<String>) -> Result<Self, WalletError> {
        let base = base_url.into().trim_end_matches('/').to_string();
        if !base.starts_with("http://") && !base.starts_with("https://") {
            return Err(WalletError::Network(format!(
                "endereço do nó inválido: {base}"
            )));
        }
        let http = reqwest::Client::builder()
            .timeout(std::time::Duration::from_secs(20))
            .build()
            .map_err(|e| WalletError::Network(e.to_string()))?;
        Ok(Self { base, http })
    }

    pub fn base_url(&self) -> &str {
        &self.base
    }

    /// `chain_id` do nó — é preciso para assinar (entra no `SignDoc`).
    pub async fn chain_id(&self) -> Result<String, WalletError> {
        let info: NodeInfoResponse = self
            .get_json("/cosmos/base/tendermint/v1beta1/node_info")
            .await?;

        let network = info
            .default_node_info
            .map(|node| node.network)
            .filter(|network| !network.is_empty())
            .ok_or_else(|| {
                WalletError::UnexpectedResponse("o nó não devolveu chain_id".into())
            })?;
        Ok(network)
    }

    /// Altura do bloco mais recente.
    pub async fn height(&self) -> Result<i64, WalletError> {
        let response: LatestBlockResponse = self
            .get_json("/cosmos/base/tendermint/v1beta1/blocks/latest")
            .await?;

        let raw = response
            .block
            .and_then(|block| block.header)
            .map(|header| header.height)
            .unwrap_or_default();

        raw.parse()
            .map_err(|_| WalletError::UnexpectedResponse(format!("altura inválida: {raw}")))
    }

    /// Saldo + `account_number`/`sequence` (necessários para assinar).
    pub async fn account(&self, address: &str) -> Result<AccountInfo, WalletError> {
        let balances: BalancesResponse = self
            .get_json(&format!("/cosmos/bank/v1beta1/balances/{address}"))
            .await?;

        let balance_uluggom = balances
            .balances
            .iter()
            .find(|coin| coin.denom == DENOM)
            .and_then(|coin| coin.amount.parse::<u128>().ok())
            .unwrap_or(0);

        // Conta por criar: o LCD pode responder 404 ou `account: null`. Nos dois
        // casos é "conta sem estado" — account_number/sequence ficam a zero.
        let raw: Option<AccountResponse> = self
            .get_json_optional(&format!("/cosmos/auth/v1beta1/accounts/{address}"))
            .await?;

        let (account_number, sequence, exists) = match raw.and_then(|body| body.account) {
            Some(account) => (
                read_uint(&account, "account_number")?,
                read_uint(&account, "sequence")?,
                true,
            ),
            None => (0, 0, false),
        };

        Ok(AccountInfo {
            address: address.to_string(),
            balance_uluggom,
            account_number,
            sequence,
            exists,
        })
    }

    /// Só o saldo — mais leve para refrescar o ecrã.
    pub async fn balance(&self, address: &str) -> Result<u128, WalletError> {
        let balances: BalancesResponse = self
            .get_json(&format!("/cosmos/bank/v1beta1/balances/{address}"))
            .await?;
        Ok(balances
            .balances
            .iter()
            .find(|coin| coin.denom == DENOM)
            .and_then(|coin| coin.amount.parse::<u128>().ok())
            .unwrap_or(0))
    }

    /// Extrato: transferências em que o endereço é destinatário ou remetente.
    pub async fn history(&self, address: &str, limit: u32) -> Result<Vec<Transfer>, WalletError> {
        let mut all = Vec::new();

        for event in [
            format!("transfer.recipient='{address}'"),
            format!("transfer.sender='{address}'"),
        ] {
            let path = format!(
                "/cosmos/tx/v1beta1/txs?query={}&order_by=ORDER_BY_DESC&pagination.limit={limit}",
                urlencode(&event)
            );
            let response: TxsResponse = match self.get_json(&path).await {
                Ok(response) => response,
                // Um nó sem histórico devolve 400/500 em vez de lista vazia.
                Err(WalletError::Network(message)) if message.contains("400") => continue,
                Err(other) => return Err(other),
            };

            for (index, response_entry) in response.tx_responses.iter().enumerate() {
                if response_entry.code != 0 {
                    continue; // tx falhada não mexeu no saldo
                }
                let Some(tx) = response.txs.get(index) else {
                    continue;
                };
                let Some((counterparty, direction, amount)) =
                    read_send_message(tx, address)
                else {
                    continue;
                };
                all.push(Transfer {
                    tx_hash: response_entry.txhash.clone(),
                    height: response_entry.height.parse().unwrap_or(0),
                    timestamp: response_entry.timestamp.clone(),
                    direction,
                    counterparty,
                    amount_uluggom: amount,
                });
            }
        }

        // As duas consultas podem trazer a mesma tx (envio para si próprio).
        all.sort_by(|a, b| b.height.cmp(&a.height).then(b.tx_hash.cmp(&a.tx_hash)));
        all.dedup_by(|a, b| a.tx_hash == b.tx_hash);
        all.truncate(limit as usize);
        Ok(all)
    }

    /// Transmite um `TxRaw` já assinado.
    pub async fn broadcast_tx_raw(&self, tx_raw: &[u8]) -> Result<BroadcastResult, WalletError> {
        use base64::engine::general_purpose::STANDARD as B64;
        use base64::Engine;
        self.broadcast(&B64.encode(tx_raw)).await
    }

    /// Transmite uma transação já assinada (`TxRaw` em base64).
    pub async fn broadcast(&self, tx_bytes_base64: &str) -> Result<BroadcastResult, WalletError> {
        let body = serde_json::json!({
            "tx_bytes": tx_bytes_base64,
            "mode": "BROADCAST_MODE_SYNC",
        });

        let response = self
            .http
            .post(format!("{}/cosmos/tx/v1beta1/txs", self.base))
            .json(&body)
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
                "o nó recusou a transação (HTTP {}): {}",
                status.as_u16(),
                truncate(&text)
            )));
        }

        let parsed: BroadcastResponse = serde_json::from_str(&text)
            .map_err(|e| WalletError::UnexpectedResponse(format!("{e}: {}", truncate(&text))))?;

        let tx_response = parsed.tx_response.ok_or_else(|| {
            WalletError::UnexpectedResponse(format!("sem tx_response: {}", truncate(&text)))
        })?;

        Ok(BroadcastResult {
            tx_hash: tx_response.txhash,
            code: tx_response.code,
            raw_log: String::new(),
        })
    }

    /// Espera que a chain inclua a transação (o broadcast é `sync`).
    pub async fn wait_for_tx(
        &self,
        tx_hash: &str,
        attempts: u32,
    ) -> Result<BroadcastResult, WalletError> {
        for _ in 0..attempts {
            let response = self
                .http
                .get(format!("{}/cosmos/tx/v1beta1/txs/{tx_hash}", self.base))
                .send()
                .await
                .map_err(|e| WalletError::Network(e.to_string()))?;

            if response.status().is_success() {
                let text = response
                    .text()
                    .await
                    .map_err(|e| WalletError::Network(e.to_string()))?;
                let parsed: BroadcastResponse = serde_json::from_str(&text)
                    .map_err(|e| WalletError::UnexpectedResponse(e.to_string()))?;
                if let Some(tx_response) = parsed.tx_response {
                    return Ok(BroadcastResult {
                        tx_hash: tx_response.txhash,
                        code: tx_response.code,
                        raw_log: String::new(),
                    });
                }
            }

            tokio::time::sleep(std::time::Duration::from_millis(500)).await;
        }

        Err(WalletError::Network(format!(
            "a transação {tx_hash} não apareceu na chain a tempo"
        )))
    }

    /// Minerações desta carteira: para cada `MsgMineTaxPayment` **bem-sucedida**,
    /// o que se pagou (na moeda do recibo) e o LUG que entrou.
    ///
    /// Tudo **lido da chain** — a app não recalcula a tokenomics.
    pub async fn mining_history(
        &self,
        address: &str,
        limit: u32,
    ) -> Result<Vec<MiningRecord>, WalletError> {
        let event = format!("message.sender='{address}'");
        let path = format!(
            "/cosmos/tx/v1beta1/txs?query={}&order_by=ORDER_BY_DESC&pagination.limit={}",
            urlencode(&event),
            limit.clamp(1, 100)
        );

        let response: TxsResponse = match self.get_json(&path).await {
            Ok(response) => response,
            // Um nó sem histórico devolve 400/500 em vez de lista vazia.
            Err(WalletError::Network(message)) if message.contains("400") => return Ok(Vec::new()),
            Err(other) => return Err(other),
        };

        let mut records = Vec::new();
        for (index, entry) in response.tx_responses.iter().enumerate() {
            if entry.code != 0 {
                continue; // tx falhada não cunhou nada
            }
            let Some(tx) = response.txs.get(index) else {
                continue;
            };
            let Some((tax_entity, amount, currency)) = Self::read_mine_message(tx) else {
                continue;
            };
            records.push(MiningRecord {
                tx_hash: entry.txhash.clone(),
                height: entry.height.parse().unwrap_or(0),
                timestamp: entry.timestamp.clone(),
                tax_entity,
                amount,
                currency,
                lug_uluggom: Self::received_uluggom(&entry.events, address),
            });
        }

        records.sort_by_key(|record| std::cmp::Reverse(record.height));
        Ok(records)
    }

    /// `(ente, valor pago, moeda)` de uma `MsgMineTaxPayment` — se a tx for uma.
    fn read_mine_message(tx: &serde_json::Value) -> Option<(String, String, String)> {
        let messages = tx.get("body")?.get("messages")?.as_array()?;
        for message in messages {
            let kind = message.get("@type")?.as_str()?;
            if !kind.ends_with("MsgMineTaxPayment") {
                continue;
            }
            let text = |key: &str| {
                message
                    .get(key)
                    .and_then(|value| value.as_str())
                    .unwrap_or_default()
                    .to_string()
            };
            return Some((text("tax_entity"), text("payment_amount"), text("currency_code")));
        }
        None
    }

    /// Soma o que a carteira **recebeu** nesta tx (eventos `coin_received`).
    fn received_uluggom(events: &[EventJson], address: &str) -> u128 {
        let mut total = 0u128;
        for event in events {
            if event.kind != "coin_received" {
                continue;
            }
            let mut receiver = None;
            let mut amount = None;
            for attribute in &event.attributes {
                match attribute.key.as_str() {
                    "receiver" => receiver = Some(attribute.value.as_str()),
                    "amount" => amount = Some(attribute.value.as_str()),
                    _ => {}
                }
            }
            if receiver != Some(address) {
                continue;
            }
            if let Some(value) = amount {
                if let Ok(parsed) = value.trim_end_matches(DENOM).parse::<u128>() {
                    total += parsed;
                }
            }
        }
        total
    }

    async fn get_json<T: for<'de> Deserialize<'de>>(&self, path: &str) -> Result<T, WalletError> {
        self.get_json_impl(path).await?.ok_or_else(|| {
            WalletError::UnexpectedResponse(format!("recurso inexistente: {path}"))
        })
    }

    /// Igual a [`Self::get_json`], mas um 404 devolve `None` em vez de erro.
    async fn get_json_optional<T: for<'de> Deserialize<'de>>(
        &self,
        path: &str,
    ) -> Result<Option<T>, WalletError> {
        self.get_json_impl(path).await
    }

    async fn get_json_impl<T: for<'de> Deserialize<'de>>(
        &self,
        path: &str,
    ) -> Result<Option<T>, WalletError> {
        let response = self
            .http
            .get(format!("{}{}", self.base, path))
            .send()
            .await
            .map_err(|e| WalletError::Network(e.to_string()))?;

        let status = response.status();
        if status == reqwest::StatusCode::NOT_FOUND {
            return Ok(None);
        }

        let text = response
            .text()
            .await
            .map_err(|e| WalletError::Network(e.to_string()))?;

        if !status.is_success() {
            return Err(WalletError::Network(format!(
                "HTTP {} em {path}: {}",
                status.as_u16(),
                truncate(&text)
            )));
        }

        serde_json::from_str(&text)
            .map(Some)
            .map_err(|e| WalletError::UnexpectedResponse(format!("{e}: {}", truncate(&text))))
    }
}

/// Lê um campo numérico que o Cosmos devolve como **string**.
fn read_uint(account: &serde_json::Value, field: &str) -> Result<u64, WalletError> {
    let raw = account
        .get(field)
        .and_then(|value| value.as_str())
        .ok_or_else(|| {
            WalletError::UnexpectedResponse(format!("campo {field} em falta na conta"))
        })?;
    raw.parse()
        .map_err(|_| WalletError::UnexpectedResponse(format!("campo {field} inválido: {raw}")))
}

/// Extrai a transferência de uma `MsgSend` (o único tipo que o extrato mostra).
fn read_send_message(
    tx: &serde_json::Value,
    owner: &str,
) -> Option<(String, Direction, u128)> {
    let messages = tx.get("body")?.get("messages")?.as_array()?;

    for message in messages {
        let type_url = message.get("@type")?.as_str()?;
        if type_url != "/cosmos.bank.v1beta1.MsgSend" {
            continue;
        }

        let from = message.get("from_address")?.as_str()?;
        let to = message.get("to_address")?.as_str()?;
        let amount = message
            .get("amount")?
            .as_array()?
            .iter()
            .find(|coin| coin.get("denom").and_then(|d| d.as_str()) == Some(DENOM))
            .and_then(|coin| coin.get("amount").and_then(|a| a.as_str()))
            .and_then(|a| a.parse::<u128>().ok())?;

        if from == owner {
            return Some((to.to_string(), Direction::Out, amount));
        }
        if to == owner {
            return Some((from.to_string(), Direction::In, amount));
        }
    }
    None
}

fn urlencode(value: &str) -> String {
    value
        .bytes()
        .map(|byte| match byte {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                (byte as char).to_string()
            }
            other => format!("%{other:02X}"),
        })
        .collect()
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

    #[test]
    fn rejects_a_base_url_without_scheme() {
        assert!(ChainClient::new("127.0.0.1:1317").is_err());
        assert!(ChainClient::new("http://127.0.0.1:1317").is_ok());
        assert!(ChainClient::new("http://127.0.0.1:1317/").is_ok());
    }

    #[test]
    fn reads_a_send_from_the_owner_side() {
        let tx = serde_json::json!({
            "body": { "messages": [ {
                "@type": "/cosmos.bank.v1beta1.MsgSend",
                "from_address": "luggom1a",
                "to_address": "luggom1b",
                "amount": [ { "denom": "uluggom", "amount": "1500000000000000000" } ]
            } ] }
        });

        let (counterparty, direction, amount) = read_send_message(&tx, "luggom1a").unwrap();
        assert_eq!(counterparty, "luggom1b");
        assert_eq!(direction, Direction::Out);
        assert_eq!(amount, 1_500_000_000_000_000_000);

        let (counterparty, direction, _) = read_send_message(&tx, "luggom1b").unwrap();
        assert_eq!(counterparty, "luggom1a");
        assert_eq!(direction, Direction::In);
    }

    #[test]
    fn ignores_other_message_types_and_other_denoms() {
        let swap = serde_json::json!({
            "body": { "messages": [ { "@type": "/luggom.luggom.v1.MsgSwap" } ] }
        });
        assert!(read_send_message(&swap, "luggom1a").is_none());

        let other_denom = serde_json::json!({
            "body": { "messages": [ {
                "@type": "/cosmos.bank.v1beta1.MsgSend",
                "from_address": "luggom1a",
                "to_address": "luggom1b",
                "amount": [ { "denom": "uatom", "amount": "5" } ]
            } ] }
        });
        assert!(read_send_message(&other_denom, "luggom1a").is_none());
    }

    #[test]
    fn reads_account_fields_that_come_as_strings() {
        let account = serde_json::json!({ "account_number": "12", "sequence": "3" });
        assert_eq!(read_uint(&account, "account_number").unwrap(), 12);
        assert_eq!(read_uint(&account, "sequence").unwrap(), 3);
        assert!(read_uint(&serde_json::json!({}), "sequence").is_err());
    }

    #[test]
    fn urlencodes_the_event_query() {
        assert_eq!(
            urlencode("transfer.recipient='luggom1abc'"),
            "transfer.recipient%3D%27luggom1abc%27"
        );
    }
}
