use serde::{Deserialize, Serialize};

/// Tentativas gratuitas antes do primeiro bloqueio.
pub const MAX_FREE_ATTEMPTS: u32 = 5;
/// Duração do primeiro bloqueio, em segundos.
pub const BASE_LOCKOUT_SECS: u64 = 30;
/// Tecto do bloqueio.
pub const MAX_LOCKOUT_SECS: u64 = 3600;

/// Estado de bloqueio por PIN errado.
///
/// ⚠️ **Limite assumido:** este estado vive no ficheiro do cofre, que o atacante
/// local controla (pode restaurar uma cópia antiga e zerar o contador). É um
/// travão de UX, **não** uma fronteira de segurança — a fronteira é o custo do
/// Argon2id e a entropia do PIN.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct LockoutState {
    pub failed_attempts: u32,
    /// Instante Unix até ao qual a abertura está recusada (0 = livre).
    pub locked_until_unix: u64,
}

impl LockoutState {
    /// Segundos que faltam para poder tentar de novo, se estiver bloqueado.
    pub fn seconds_remaining(&self, now_unix: u64) -> Option<u64> {
        (self.locked_until_unix > now_unix).then(|| self.locked_until_unix - now_unix)
    }

    /// Tentativas gratuitas que ainda restam antes do próximo bloqueio.
    pub fn attempts_left(&self) -> u32 {
        MAX_FREE_ATTEMPTS.saturating_sub(self.failed_attempts)
    }

    /// Regista uma falha e aplica o bloqueio exponencial a partir da 5.ª.
    pub fn register_failure(&mut self, now_unix: u64) {
        self.failed_attempts = self.failed_attempts.saturating_add(1);
        if self.failed_attempts >= MAX_FREE_ATTEMPTS {
            let excess = self.failed_attempts - MAX_FREE_ATTEMPTS;
            let secs = BASE_LOCKOUT_SECS
                .saturating_mul(1u64 << excess.min(20))
                .min(MAX_LOCKOUT_SECS);
            self.locked_until_unix = now_unix.saturating_add(secs);
        }
    }

    /// Zera o estado após uma abertura bem-sucedida.
    pub fn register_success(&mut self) {
        *self = Self::default();
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn first_four_failures_do_not_lock() {
        let mut state = LockoutState::default();
        for _ in 0..(MAX_FREE_ATTEMPTS - 1) {
            state.register_failure(1_000);
        }
        assert_eq!(state.seconds_remaining(1_000), None);
        assert_eq!(state.attempts_left(), 1);
    }

    #[test]
    fn fifth_failure_locks_for_base_duration() {
        let mut state = LockoutState::default();
        for _ in 0..MAX_FREE_ATTEMPTS {
            state.register_failure(1_000);
        }
        assert_eq!(state.seconds_remaining(1_000), Some(BASE_LOCKOUT_SECS));
        assert_eq!(state.seconds_remaining(1_000 + BASE_LOCKOUT_SECS - 1), Some(1));
        assert_eq!(state.seconds_remaining(1_000 + BASE_LOCKOUT_SECS), None);
    }

    #[test]
    fn lockout_doubles_and_saturates_at_the_cap() {
        let mut state = LockoutState::default();
        for _ in 0..MAX_FREE_ATTEMPTS {
            state.register_failure(0);
        }
        state.register_failure(0);
        assert_eq!(state.seconds_remaining(0), Some(BASE_LOCKOUT_SECS * 2));

        for _ in 0..40 {
            state.register_failure(0);
        }
        assert_eq!(state.seconds_remaining(0), Some(MAX_LOCKOUT_SECS));
    }

    #[test]
    fn success_clears_the_state() {
        let mut state = LockoutState::default();
        for _ in 0..MAX_FREE_ATTEMPTS {
            state.register_failure(1_000);
        }
        state.register_success();
        assert_eq!(state, LockoutState::default());
    }
}
