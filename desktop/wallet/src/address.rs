//! Endereços bech32 do Luggom.

/// Prefixo legível dos endereços (`luggom1…`).
pub const HRP: &str = "luggom";

/// Verifica se `address` é um endereço Luggom válido.
///
/// Serve para travar um envio para um endereço mal escrito antes de ele ser
/// assinado — uma transferência para o sítio errado não se desfaz.
pub fn is_valid_address(address: &str) -> bool {
    match bech32::decode(address) {
        Ok((hrp, data)) => hrp.as_str() == HRP && data.len() == 20,
        Err(_) => false,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn accepts_the_known_address() {
        assert!(is_valid_address(
            "luggom19rl4cm2hmr8afy4kldpxz3fka4jguq0amgkgeu"
        ));
    }

    #[test]
    fn rejects_a_wrong_prefix_or_broken_checksum() {
        // Mesmos dados, prefixo de outra rede.
        assert!(!is_valid_address(
            "cosmos19rl4cm2hmr8afy4kldpxz3fka4jguq0an8tlhp"
        ));
        assert!(!is_valid_address(
            "luggom19rl4cm2hmr8afy4kldpxz3fka4jguq0amgkgeu"
                .replace('9', "8")
                .as_str()
        ));
        assert!(!is_valid_address("luggom1"));
        assert!(!is_valid_address(""));
        assert!(!is_valid_address("não é um endereço"));
    }
}
