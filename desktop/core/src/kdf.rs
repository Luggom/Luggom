use serde::{Deserialize, Serialize};
use zeroize::Zeroizing;

use crate::error::VaultError;

/// Comprimento do sal do Argon2id (16 bytes é o recomendado pelo RFC 9106).
pub(crate) const SALT_LEN: usize = 16;
/// Comprimento da chave derivada (XChaCha20-Poly1305 usa 32 bytes).
pub(crate) const KEY_LEN: usize = 32;

/// Tectos defensivos: um cofre forjado não pode pedir gigabytes de RAM
/// nem uma eternidade de CPU no `unlock` (DoS local).
const MAX_M_COST: u32 = 1 << 21; // 2 GiB
const MAX_T_COST: u32 = 16;
const MAX_P_COST: u32 = 8;

/// Parâmetros do Argon2id gravados no ficheiro do cofre.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
pub struct KdfParams {
    /// Custo de memória em KiB.
    pub m_cost: u32,
    /// Nº de passagens.
    pub t_cost: u32,
    /// Grau de paralelismo.
    pub p_cost: u32,
}

impl KdfParams {
    /// Parâmetros de produção: 64 MiB, 3 passagens.
    ///
    /// Acima do mínimo OWASP (19 MiB, t=2) de propósito — aqui a única barreira
    /// entre um atacante com o ficheiro e a seed é o PIN.
    pub const PRODUCTION: Self = Self {
        m_cost: 65_536,
        t_cost: 3,
        p_cost: 1,
    };

    /// Parâmetros mínimos, **só para testes** (a suíte tem de ser rápida).
    pub const FAST: Self = Self {
        m_cost: 8,
        t_cost: 1,
        p_cost: 1,
    };

    pub(crate) fn validate(self) -> Result<(), VaultError> {
        if self.m_cost == 0 || self.m_cost > MAX_M_COST {
            return Err(VaultError::InvalidKdf(format!(
                "m_cost fora do intervalo (1..={MAX_M_COST})"
            )));
        }
        if self.t_cost == 0 || self.t_cost > MAX_T_COST {
            return Err(VaultError::InvalidKdf(format!(
                "t_cost fora do intervalo (1..={MAX_T_COST})"
            )));
        }
        if self.p_cost == 0 || self.p_cost > MAX_P_COST {
            return Err(VaultError::InvalidKdf(format!(
                "p_cost fora do intervalo (1..={MAX_P_COST})"
            )));
        }
        Ok(())
    }

    /// Deriva a chave AEAD a partir do PIN e do sal.
    pub(crate) fn derive_key(
        self,
        pin: &str,
        salt: &[u8; SALT_LEN],
    ) -> Result<Zeroizing<[u8; KEY_LEN]>, VaultError> {
        self.validate()?;

        let params = argon2::Params::new(self.m_cost, self.t_cost, self.p_cost, Some(KEY_LEN))
            .map_err(|e| VaultError::InvalidKdf(e.to_string()))?;
        let argon = argon2::Argon2::new(
            argon2::Algorithm::Argon2id,
            argon2::Version::V0x13,
            params,
        );

        let mut key = Zeroizing::new([0u8; KEY_LEN]);
        argon
            .hash_password_into(pin.as_bytes(), salt, key.as_mut())
            .map_err(|e| VaultError::Crypto(format!("argon2id: {e}")))?;
        Ok(key)
    }
}
