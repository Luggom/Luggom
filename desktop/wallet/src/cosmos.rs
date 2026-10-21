//! Codificação protobuf mínima para uma transação Cosmos assinada localmente.
//!
//! Segue a mesma linha do assinador (`apk_luggom/native/tlsn-mobile/src/signer.rs`):
//! escreve-se apenas o que é preciso, sem trazer o `prost`/`cosmrs` inteiro para
//! o binário do desktop. O que sai daqui é **SIGN_MODE_DIRECT**.
//!
//! ⚠️ **Regra proto3: campos com o valor por defeito NÃO são escritos.** Isto já
//! mordeu o projeto uma vez (§13.125: `account_number = 0` omitido e a conta #0
//! não conseguia minerar) — por isso [`write_uint`] salta o zero por omissão.

pub const TYPE_URL_MSG_SEND: &str = "/cosmos.bank.v1beta1.MsgSend";
pub const TYPE_URL_PUBLIC_KEY: &str = "/cosmos.crypto.secp256k1.PubKey";

/// `SIGN_MODE_DIRECT`.
const SIGN_MODE_DIRECT: u64 = 1;

const WIRE_VARINT: u8 = 0;
const WIRE_LEN: u8 = 2;

// ---------------------------------------------------------------------------
// Primitivas
// ---------------------------------------------------------------------------

fn write_varint(out: &mut Vec<u8>, mut value: u64) {
    while value >= 0x80 {
        out.push((value as u8) | 0x80);
        value >>= 7;
    }
    out.push(value as u8);
}

fn write_tag(out: &mut Vec<u8>, field: u32, wire: u8) {
    write_varint(out, ((field as u64) << 3) | wire as u64);
}

fn write_len_field(out: &mut Vec<u8>, field: u32, value: &[u8]) {
    write_tag(out, field, WIRE_LEN);
    write_varint(out, value.len() as u64);
    out.extend_from_slice(value);
}

fn write_string(out: &mut Vec<u8>, field: u32, value: &str) {
    if value.is_empty() {
        return; // proto3: string vazia é o valor por defeito
    }
    write_len_field(out, field, value.as_bytes());
}

/// Escreve um campo varint. **Zero não é escrito** (valor por defeito do proto3).
fn write_uint(out: &mut Vec<u8>, field: u32, value: u64) {
    if value == 0 {
        return;
    }
    write_tag(out, field, WIRE_VARINT);
    write_varint(out, value);
}

// ---------------------------------------------------------------------------
// Mensagens
// ---------------------------------------------------------------------------

/// `cosmos.base.v1beta1.Coin`.
pub fn coin(denom: &str, amount: u128) -> Vec<u8> {
    let mut out = Vec::new();
    write_string(&mut out, 1, denom);
    // O montante é uma **string** no proto: "0" é um valor válido e é escrito.
    write_string(&mut out, 2, &amount.to_string());
    out
}

/// `cosmos.bank.v1beta1.MsgSend`.
pub fn msg_send(from_address: &str, to_address: &str, amount: &[Vec<u8>]) -> Vec<u8> {
    let mut out = Vec::new();
    write_string(&mut out, 1, from_address);
    write_string(&mut out, 2, to_address);
    for one in amount {
        write_len_field(&mut out, 3, one);
    }
    out
}

/// `google.protobuf.Any`.
pub fn any(type_url: &str, value: &[u8]) -> Vec<u8> {
    let mut out = Vec::new();
    write_string(&mut out, 1, type_url);
    write_len_field(&mut out, 2, value);
    out
}

/// `cosmos.crypto.secp256k1.PubKey` — chave comprimida de 33 bytes.
pub fn public_key(key: &[u8]) -> Vec<u8> {
    let mut out = Vec::new();
    write_len_field(&mut out, 1, key);
    out
}

/// `cosmos.tx.v1beta1.SignerInfo` com `ModeInfo.single` = `SIGN_MODE_DIRECT`.
pub fn signer_info(public_key_any: &[u8], sequence: u64) -> Vec<u8> {
    // ModeInfo.Single { mode: 1 }  →  [0x08, 0x01]
    let mut single = Vec::new();
    write_uint(&mut single, 1, SIGN_MODE_DIRECT);

    // ModeInfo { single: 1 }  →  [0x0A, len, single]
    let mut mode_info = Vec::new();
    write_len_field(&mut mode_info, 1, &single);

    let mut out = Vec::new();
    write_len_field(&mut out, 1, public_key_any);
    write_len_field(&mut out, 2, &mode_info);
    write_uint(&mut out, 3, sequence);
    out
}

/// `cosmos.tx.v1beta1.Fee`.
pub fn fee(amount: &[Vec<u8>], gas_limit: u64) -> Vec<u8> {
    let mut out = Vec::new();
    for one in amount {
        write_len_field(&mut out, 1, one);
    }
    write_uint(&mut out, 2, gas_limit);
    out
}

/// `cosmos.tx.v1beta1.TxBody`.
pub fn tx_body(messages: &[Vec<u8>], memo: &str) -> Vec<u8> {
    let mut out = Vec::new();
    for message in messages {
        write_len_field(&mut out, 1, message);
    }
    write_string(&mut out, 2, memo);
    out
}

/// `cosmos.tx.v1beta1.AuthInfo`.
pub fn auth_info(signer_infos: &[Vec<u8>], fee_message: &[u8]) -> Vec<u8> {
    let mut out = Vec::new();
    for info in signer_infos {
        write_len_field(&mut out, 1, info);
    }
    write_len_field(&mut out, 2, fee_message);
    out
}

/// `cosmos.tx.v1beta1.SignDoc` — é o SHA-256 disto que se assina.
pub fn sign_doc(
    body_bytes: &[u8],
    auth_info_bytes: &[u8],
    chain_id: &str,
    account_number: u64,
) -> Vec<u8> {
    let mut out = Vec::new();
    write_len_field(&mut out, 1, body_bytes);
    write_len_field(&mut out, 2, auth_info_bytes);
    write_string(&mut out, 3, chain_id);
    write_uint(&mut out, 4, account_number);
    out
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hex(bytes: &[u8]) -> String {
        bytes.iter().map(|b| format!("{b:02x}")).collect()
    }

    #[test]
    fn coin_matches_the_expected_wire_format() {
        // field1 (string "uluggom") + field2 (string "1000")
        let expected = [
            &[0x0a, 0x07][..],
            b"uluggom",
            &[0x12, 0x04][..],
            b"1000",
        ]
        .concat();
        assert_eq!(coin("uluggom", 1000), expected);
    }

    #[test]
    fn coin_keeps_a_zero_amount() {
        // "0" é uma string válida (não é o valor por defeito do proto3).
        assert!(coin("uluggom", 0).ends_with(&[0x12, 0x01, b'0']));
    }

    #[test]
    fn msg_send_matches_the_expected_wire_format() {
        let amount = vec![coin("uluggom", 1)];
        let encoded = msg_send("A", "B", &amount);

        let mut expected = Vec::new();
        expected.extend_from_slice(&[0x0a, 0x01, b'A']); // from_address
        expected.extend_from_slice(&[0x12, 0x01, b'B']); // to_address
        expected.push(0x1a); // amount (repeated Coin)
        expected.push(amount[0].len() as u8);
        expected.extend_from_slice(&amount[0]);

        assert_eq!(encoded, expected);
    }

    #[test]
    fn public_key_wraps_the_compressed_key() {
        assert_eq!(
            hex(&public_key(&[0x02, 0xaa, 0xbb])),
            "0a0302aabb"
        );
    }

    #[test]
    fn signer_info_sets_sign_mode_direct() {
        let encoded = signer_info(&public_key(&[0x02, 0xaa]), 0);
        // campo 1 = PubKey{ key: 02aa } → 0a040a0202aa
        // campo 2 = ModeInfo{ single: Single{ mode: 1 } } → 12040a020801
        // sequence = 0 ⇒ campo 3 OMITIDO (proto3)
        assert_eq!(hex(&encoded), "0a040a0202aa12040a020801");
    }

    #[test]
    fn signer_info_includes_a_non_zero_sequence() {
        let encoded = signer_info(&public_key(&[0x02, 0xaa]), 7);
        assert!(hex(&encoded).ends_with("1807"));
    }

    #[test]
    fn sign_doc_omits_account_number_zero() {
        // Este é o caso que partiu a conta #0 (§13.125): se o campo fosse
        // escrito como 0, o digest deixaria de bater com o da chain.
        let body = vec![0x0a, 0x00];
        let auth = vec![0x12, 0x00];
        let encoded = sign_doc(&body, &auth, "luggom-test", 0);

        let expected = [
            &[0x0a, 0x02][..],
            &body,
            &[0x12, 0x02][..],
            &auth,
            &[0x1a, 0x0b][..],
            b"luggom-test",
        ]
        .concat();
        // Comparação exata: não há campo 4 (account_number) nenhures.
        assert_eq!(encoded, expected);
    }

    #[test]
    fn sign_doc_writes_account_number_when_non_zero() {
        let encoded = sign_doc(&[1], &[2], "c", 42);
        // field4 varint: tag 0x20, valor 0x2a
        assert!(encoded.ends_with(&[0x20, 0x2a]));
    }

    #[test]
    fn tx_body_omits_an_empty_memo() {
        let encoded = tx_body(&[vec![0x01]], "");
        assert_eq!(hex(&encoded), "0a0101");
    }

    #[test]
    fn tx_body_writes_a_non_empty_memo() {
        let encoded = tx_body(&[], "oi");
        assert_eq!(hex(&encoded), "12026f69");
    }

    #[test]
    fn fee_omits_a_zero_gas_limit() {
        assert_eq!(hex(&fee(&[], 0)), "");
        // 200000 = varint c0 9a 0c, no campo 2 (tag 0x10)
        assert_eq!(hex(&fee(&[], 200_000)), "10c09a0c");
    }

    #[test]
    fn varint_encodes_multi_byte_values() {
        let mut out = Vec::new();
        write_varint(&mut out, 300);
        assert_eq!(out, vec![0xac, 0x02]);
    }
}
