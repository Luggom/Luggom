//! Prova do A3 contra uma chain **a correr**: saldo, envio e extrato.
//!
//! Este teste mexe em estado real (envia LUG), por isso fica desligado: só
//! corre com `LUGGOM_LIVE_CHAIN=1` e `--ignored`.
//!
//! ```text
//! cd desktop
//! LUGGOM_LIVE_CHAIN=1 cargo test -p luggom-wallet --test chain_live -- --ignored --nocapture
//! ```
//!
//! Pressupõe uma chain local com o endereço abaixo financiado:
//! `luggomd tx bank send GGOM luggom19rl4cm2hmr8afy4kldpxz3fka4jguq0amgkgeu 1000uluggom…`
//! (o mnemónico é o vector público do BIP39 — não é segredo nenhum).

use luggom_wallet::{
    build_send_tx, lug_to_uluggom, uluggom_to_lug, ChainClient, Direction, SendPlan, Wallet,
};

/// Vector público do BIP39: só serve para a chain local.
const TEST_MNEMONIC: &str =
    "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";

const NODE: &str = "http://127.0.0.1:1317";

fn chain_is_enabled() -> bool {
    match std::env::var("LUGGOM_LIVE_CHAIN") {
        Ok(value) if value == "1" => true,
        _ => {
            eprintln!("LUGGOM_LIVE_CHAIN != 1 — teste saltado (mexe em estado real)");
            false
        }
    }
}

#[tokio::test]
#[ignore = "envia LUG a sério; requer LUGGOM_LIVE_CHAIN=1 e uma chain local"]
async fn balance_send_and_history_against_a_live_chain() {
    if !chain_is_enabled() {
        return;
    }

    let client = ChainClient::new(NODE).expect("cliente");
    let chain_id = client.chain_id().await.expect("chain_id");

    let wallet = Wallet::from_mnemonic(TEST_MNEMONIC).expect("carteira");
    let sender = wallet.address().to_string();

    // Um segundo endereço válido, derivado do mesmo mnemónico noutro caminho.
    let receiver = Wallet::from_mnemonic_at(TEST_MNEMONIC, "m/44'/118'/0'/0/1")
        .expect("carteira destino")
        .address()
        .to_string();
    assert_ne!(sender, receiver);

    let before = client.account(&sender).await.expect("conta do remetente");
    let receiver_before = client
        .account(&receiver)
        .await
        .expect("conta do destinatário");
    println!(
        "remetente: {} LUG · destinatário: {} LUG",
        uluggom_to_lug(before.balance_uluggom),
        uluggom_to_lug(receiver_before.balance_uluggom)
    );
    assert!(
        before.balance_uluggom > 0,
        "financia primeiro {sender} antes de correr este teste"
    );

    let amount = lug_to_uluggom("10").unwrap();
    let plan = SendPlan::new(amount);
    assert!(
        before.balance_uluggom >= plan.total_uluggom(),
        "saldo insuficiente: {} < {}",
        before.balance_uluggom,
        plan.total_uluggom()
    );

    // 1) Montar + assinar (a seed entra, a assinatura sai).
    let tx_raw = build_send_tx(
        TEST_MNEMONIC,
        &receiver,
        amount,
        &chain_id,
        before.account_number,
        before.sequence,
    )
    .expect("assinatura");

    // 2) Transmitir e esperar pela inclusão no bloco.
    let sent = client.broadcast_tx_raw(&tx_raw).await.expect("broadcast");
    assert!(sent.is_ok(), "a chain recusou: código {}", sent.code);
    println!("tx {}", sent.tx_hash);

    let confirmed = client
        .wait_for_tx(&sent.tx_hash, 30)
        .await
        .expect("a tx não apareceu na chain");
    assert_eq!(confirmed.code, 0, "a tx falhou dentro do bloco");

    // 3) O saldo tem de descer exatamente pelo previsto — é isto que prova que
    //    o gás e a taxa fixa de 0,005 LUG estão bem modelados.
    let after = client.account(&sender).await.expect("conta do remetente");
    assert_eq!(
        after.balance_uluggom,
        before.balance_uluggom - plan.total_uluggom(),
        "o custo real ({}) não bateu com o previsto ({})",
        before.balance_uluggom - after.balance_uluggom,
        plan.total_uluggom()
    );

    // 4) E o destinatário recebeu exatamente o valor enviado.
    let receiver_after = client.account(&receiver).await.expect("destinatário");
    assert_eq!(
        receiver_after.balance_uluggom,
        receiver_before.balance_uluggom + amount,
        "o destinatário não recebeu o valor certo"
    );

    // 5) O extrato mostra a transferência de saída.
    let history = client.history(&sender, 10).await.expect("extrato");
    let entry = history
        .iter()
        .find(|transfer| transfer.tx_hash == sent.tx_hash)
        .expect("a transferência não apareceu no extrato");
    assert_eq!(entry.direction, Direction::Out);
    assert_eq!(entry.counterparty, receiver);
    assert_eq!(entry.amount_uluggom, amount);

    println!(
        "✅ enviado {} LUG · custo total {} LUG (valor {} + gás {} + taxa {})",
        uluggom_to_lug(amount),
        uluggom_to_lug(plan.total_uluggom()),
        uluggom_to_lug(plan.amount_uluggom),
        uluggom_to_lug(plan.gas_fee_uluggom),
        uluggom_to_lug(plan.flat_fee_uluggom),
    );
}

#[tokio::test]
#[ignore = "requer uma chain local a correr; só lê, não mexe"]
async fn reads_a_live_chain() {
    if !chain_is_enabled() {
        return;
    }

    let client = ChainClient::new(NODE).expect("cliente");
    let chain_id = client.chain_id().await.expect("chain_id");
    let height = client.height().await.expect("altura");
    let wallet = Wallet::from_mnemonic(TEST_MNEMONIC).expect("carteira");
    let account = client.account(wallet.address()).await.expect("conta");

    println!(
        "chain {} · altura {} · {} tem {} LUG (conta {})",
        chain_id,
        height,
        wallet.address(),
        uluggom_to_lug(account.balance_uluggom),
        account.account_number
    );
    assert!(height > 0, "a chain não está a produzir blocos");
}
