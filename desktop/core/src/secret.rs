use std::fmt;

use zeroize::Zeroizing;

/// Frase mnemónica em memória.
///
/// Zera-se ao ser largada (`Zeroizing`) e **não** implementa `Display`; o
/// `Debug` imprime `<redacted>` — o vector `T-APP-5` proíbe que a seed apareça
/// em ecrãs de diagnóstico, logs ou crash reports.
///
/// O acesso ao conteúdo é deliberadamente explícito — [`SecretPhrase::expose`] —
/// para que qualquer leitura da seed seja visível numa revisão de código.
pub struct SecretPhrase(Zeroizing<String>);

impl SecretPhrase {
    pub(crate) fn new(phrase: String) -> Self {
        Self(Zeroizing::new(phrase))
    }

    /// Devolve a frase em claro. Cada chamada é um ponto de auditoria.
    pub fn expose(&self) -> &str {
        self.0.as_str()
    }

    /// Nº de palavras (12/15/18/21/24 para BIP39 válido).
    pub fn word_count(&self) -> usize {
        self.0.split_whitespace().count()
    }
}

impl fmt::Debug for SecretPhrase {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        // NUNCA imprimir o conteúdo.
        write!(f, "SecretPhrase(<redacted>, {} palavras)", self.word_count())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn debug_does_not_leak_the_phrase() {
        let secret = SecretPhrase::new("abandon abandon abandon about".to_string());
        let rendered = format!("{secret:?}");
        assert!(!rendered.contains("abandon"), "Debug vazou a seed: {rendered}");
        assert_eq!(secret.word_count(), 4);
    }
}
