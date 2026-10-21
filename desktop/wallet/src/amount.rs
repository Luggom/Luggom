//! Conversão entre LUG (o que o utilizador escreve) e `uluggom` (a unidade base).
//!
//! 1 LUG = 10^18 uluggom. Tudo é feito com inteiros — nunca com vírgula
//! flutuante: `f64` não representa 10^18 com exatidão e um arredondamento aqui
//! é dinheiro perdido.

use crate::WalletError;

/// Casas decimais do LUG.
pub const DECIMALS: u32 = 18;

/// Denom da unidade base na chain.
pub const DENOM: &str = "uluggom";

/// Nome de exibição.
pub const DISPLAY_DENOM: &str = "LUG";

const SCALE: u128 = 10u128.pow(DECIMALS);

/// Converte um valor escrito em LUG (ex.: `"1,5"` ou `"0.005"`) em uluggom.
///
/// Aceita vírgula ou ponto como separador decimal. Rejeita mais de 18 casas
/// (não há como representar) e valores negativos.
pub fn lug_to_uluggom(input: &str) -> Result<u128, WalletError> {
    let text = input.trim().replace(',', ".");
    let text = text.replace('_', "");

    if text.is_empty() {
        return Err(WalletError::InvalidAmount("valor vazio".into()));
    }
    if text.starts_with('-') {
        return Err(WalletError::InvalidAmount(
            "o valor não pode ser negativo".into(),
        ));
    }

    let (whole, fraction) = match text.split_once('.') {
        Some((whole, fraction)) => (whole, fraction),
        None => (text.as_str(), ""),
    };

    if whole.is_empty() && fraction.is_empty() {
        return Err(WalletError::InvalidAmount("valor vazio".into()));
    }
    if !whole.chars().all(|c| c.is_ascii_digit())
        || !fraction.chars().all(|c| c.is_ascii_digit())
    {
        return Err(WalletError::InvalidAmount(format!("valor inválido: {input}")));
    }
    if fraction.len() > DECIMALS as usize {
        return Err(WalletError::InvalidAmount(format!(
            "demasiadas casas decimais (máximo {DECIMALS}): {input}"
        )));
    }

    let whole: u128 = if whole.is_empty() {
        0
    } else {
        whole
            .parse()
            .map_err(|_| WalletError::InvalidAmount(format!("valor demasiado grande: {input}")))?
    };

    let mut padded = fraction.to_string();
    while padded.len() < DECIMALS as usize {
        padded.push('0');
    }
    let fraction: u128 = if padded.is_empty() {
        0
    } else {
        padded
            .parse()
            .map_err(|_| WalletError::InvalidAmount(format!("valor inválido: {input}")))?
    };

    whole
        .checked_mul(SCALE)
        .and_then(|v| v.checked_add(fraction))
        .ok_or_else(|| WalletError::InvalidAmount(format!("valor demasiado grande: {input}")))
}

/// Converte uluggom para o texto em LUG, sem zeros à direita desnecessários.
pub fn uluggom_to_lug(amount: u128) -> String {
    let whole = amount / SCALE;
    let fraction = amount % SCALE;
    if fraction == 0 {
        return whole.to_string();
    }

    let mut text = format!("{:0width$}", fraction, width = DECIMALS as usize);
    while text.ends_with('0') {
        text.pop();
    }
    format!("{whole}.{text}")
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn converts_whole_and_fractional_values() {
        assert_eq!(lug_to_uluggom("1").unwrap(), 1_000_000_000_000_000_000);
        assert_eq!(lug_to_uluggom("0").unwrap(), 0);
        assert_eq!(lug_to_uluggom("0.5").unwrap(), 500_000_000_000_000_000);
        assert_eq!(lug_to_uluggom("0,5").unwrap(), 500_000_000_000_000_000);
        assert_eq!(lug_to_uluggom("1.5").unwrap(), 1_500_000_000_000_000_000);
    }

    #[test]
    fn converts_the_flat_fee_constant() {
        // 0,005 LUG é a taxa fixa imposta pela chain (app/ante_lug_move_fee.go).
        assert_eq!(lug_to_uluggom("0.005").unwrap(), 5_000_000_000_000_000);
    }

    #[test]
    fn handles_the_smallest_unit() {
        assert_eq!(lug_to_uluggom("0.000000000000000001").unwrap(), 1);
    }

    #[test]
    fn rejects_too_many_decimals() {
        assert!(lug_to_uluggom("0.0000000000000000001").is_err());
    }

    #[test]
    fn rejects_garbage_and_negatives() {
        assert!(lug_to_uluggom("").is_err());
        assert!(lug_to_uluggom("abc").is_err());
        assert!(lug_to_uluggom("-1").is_err());
        assert!(lug_to_uluggom("1.2.3").is_err());
        assert!(lug_to_uluggom(".5").is_ok(), "'.5' é meio LUG");
    }

    #[test]
    fn formats_back_without_trailing_zeros() {
        assert_eq!(uluggom_to_lug(1_000_000_000_000_000_000), "1");
        assert_eq!(uluggom_to_lug(5_000_000_000_000_000), "0.005");
        assert_eq!(uluggom_to_lug(1), "0.000000000000000001");
        assert_eq!(uluggom_to_lug(0), "0");
        assert_eq!(uluggom_to_lug(1_500_000_000_000_000_000), "1.5");
    }

    #[test]
    fn round_trips() {
        for text in ["1", "0.005", "123.456", "0.000000000000000001", "7"] {
            let amount = lug_to_uluggom(text).unwrap();
            assert_eq!(uluggom_to_lug(amount), text, "ida e volta de {text}");
        }
    }

    #[test]
    fn big_amounts_do_not_overflow() {
        // Supply máximo do projeto: 7 mil milhões de LUG.
        let total = lug_to_uluggom("7000000000").unwrap();
        assert_eq!(total, 7_000_000_000_000_000_000_000_000_000);
        assert!(lug_to_uluggom("999999999999999999999999999999999999999").is_err());
    }
}
