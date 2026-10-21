//! Montagem de um envio de LUG (`cosmos.bank.v1beta1.MsgSend`).
//!
//! O custo de um envio tem **três** parcelas, e a UI tem de mostrar as três:
//!
//! 1. o valor enviado;
//! 2. o **gás** (`gas_limit × preço do gás`), que o ante handler normal cobra;
//! 3. a **taxa fixa de movimento** de 0,005 LUG — imposta pela chain a quem
//!    envia LUG (`app/ante_lug_move_fee.go`), cobrada **depois** do gás.

use crate::address::is_valid_address;
use crate::amount::DENOM;
use crate::cosmos::{self, TYPE_URL_MSG_SEND, TYPE_URL_PUBLIC_KEY};
use crate::{Wallet, WalletError, DEFAULT_HD_PATH};

/// Taxa fixa de movimento de LUG, em uluggom (0,005 LUG).
///
/// Espelha `lugMovementFlatFeeUluggom` da chain (`app/ante_lug_move_fee.go`).
/// Está aqui só para a UI **avisar** do custo — quem cobra é a chain.
pub const FLAT_MOVE_FEE_ULUGGOM: u128 = 5_000_000_000_000_000;

/// Gás que um envio consome.
pub const SEND_GAS_LIMIT: u64 = 200_000;

/// Preço de gás mínimo aceite pelo nó (`--minimum-gas-prices 1uluggom`).
pub const GAS_PRICE_ULUGGOM: u128 = 1;

/// O que o envio custa, para a UI mostrar antes de assinar.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct SendPlan {
    pub amount_uluggom: u128,
    pub gas_fee_uluggom: u128,
    pub flat_fee_uluggom: u128,
}

impl SendPlan {
    pub fn new(amount_uluggom: u128) -> Self {
        Self {
            amount_uluggom,
            gas_fee_uluggom: SEND_GAS_LIMIT as u128 * GAS_PRICE_ULUGGOM,
            flat_fee_uluggom: FLAT_MOVE_FEE_ULUGGOM,
        }
    }

    /// Total que sai do saldo: valor + gás + taxa fixa.
    pub fn total_uluggom(&self) -> u128 {
        self.amount_uluggom
            .saturating_add(self.gas_fee_uluggom)
            .saturating_add(self.flat_fee_uluggom)
    }
}

/// Monta e assina um envio, devolvendo o `TxRaw` protobuf pronto a transmitir.
///
/// A seed entra, a assinatura sai — nunca o contrário.
#[allow(clippy::too_many_arguments)]
pub fn build_send_tx(
    mnemonic: &str,
    to_address: &str,
    amount_uluggom: u128,
    chain_id: &str,
    account_number: u64,
    sequence: u64,
) -> Result<Vec<u8>, WalletError> {
    if amount_uluggom == 0 {
        return Err(WalletError::InvalidAmount(
            "o valor a enviar tem de ser maior que zero".into(),
        ));
    }
    if !is_valid_address(to_address) {
        return Err(WalletError::InvalidAddress(to_address.to_string()));
    }
    if chain_id.is_empty() {
        return Err(WalletError::Network("chain_id em falta".into()));
    }

    let wallet = Wallet::from_mnemonic(mnemonic)?;
    let from_address = wallet.address().to_string();

    let message = cosmos::any(
        TYPE_URL_MSG_SEND,
        &cosmos::msg_send(
            &from_address,
            to_address,
            &[cosmos::coin(DENOM, amount_uluggom)],
        ),
    );
    let body_bytes = cosmos::tx_body(&[message], "");

    let public_key = cosmos::any(TYPE_URL_PUBLIC_KEY, &cosmos::public_key(wallet.public_key()));
    let signer = cosmos::signer_info(&public_key, sequence);

    let plan = SendPlan::new(amount_uluggom);
    let fee = cosmos::fee(
        &[cosmos::coin(DENOM, plan.gas_fee_uluggom)],
        SEND_GAS_LIMIT,
    );
    let auth_info_bytes = cosmos::auth_info(&[signer], &fee);

    let sign_doc = cosmos::sign_doc(&body_bytes, &auth_info_bytes, chain_id, account_number);

    // O assinador real do APK: assina SHA-256(sign_doc) com secp256k1 e devolve
    // o TxRaw já montado.
    tlsn_mobile::signer::sign_tx(
        mnemonic.to_string(),
        DEFAULT_HD_PATH.to_string(),
        sign_doc,
        body_bytes,
        auth_info_bytes,
    )
    .map_err(WalletError::from)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::amount::lug_to_uluggom;

    const MNEMONIC: &str =
        "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";
    const DESTINATION: &str = "luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna";

    #[test]
    fn plan_totals_the_three_parcels() {
        let amount = lug_to_uluggom("10").unwrap();
        let plan = SendPlan::new(amount);
        assert_eq!(plan.gas_fee_uluggom, 200_000);
        assert_eq!(plan.flat_fee_uluggom, lug_to_uluggom("0.005").unwrap());
        assert_eq!(
            plan.total_uluggom(),
            amount + 200_000 + 5_000_000_000_000_000
        );
    }

    #[test]
    fn builds_a_signed_tx_raw() {
        let raw = build_send_tx(MNEMONIC, DESTINATION, 1_000_000_000_000_000_000, "luggom-test", 3, 1)
            .unwrap();
        // TxRaw = body_bytes(1) + auth_info_bytes(2) + signatures(3)
        assert_eq!(raw[0], 0x0a, "campo 1 (body_bytes)");
        assert!(
            raw.contains(&0x1a),
            "campo 3 (signatures) tem de estar presente"
        );
        assert!(raw.len() > 100, "tx demasiado pequena: {}", raw.len());
    }

    #[test]
    fn refuses_zero_and_invalid_destination() {
        assert!(matches!(
            build_send_tx(MNEMONIC, DESTINATION, 0, "luggom-test", 0, 0),
            Err(WalletError::InvalidAmount(_))
        ));
        assert!(matches!(
            build_send_tx(MNEMONIC, "luggom1inventado", 1, "luggom-test", 0, 0),
            Err(WalletError::InvalidAddress(_))
        ));
        assert!(matches!(
            build_send_tx(MNEMONIC, DESTINATION, 1, "", 0, 0),
            Err(WalletError::Network(_))
        ));
    }

    #[test]
    fn refuses_an_invalid_mnemonic() {
        assert!(build_send_tx("não é mnemónico", DESTINATION, 1, "luggom-test", 0, 0).is_err());
    }
}
