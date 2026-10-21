use std::sync::Mutex;

use luggom_core::SecretPhrase;
use luggom_wallet::Wallet;
use zeroize::Zeroizing;

/// Carteira aberta em memória enquanto a sessão durar.
pub struct Unlocked {
    pub phrase: SecretPhrase,
    pub wallet: Wallet,
}

impl Unlocked {
    pub fn address(&self) -> &str {
        self.wallet.address()
    }

    /// A frase em claro, para assinar. Cada chamada é um ponto de auditoria.
    pub fn mnemonic(&self) -> &str {
        self.phrase.expose()
    }
}

/// Estado da aplicação.
#[derive(Default)]
pub struct AppState {
    unlocked: Mutex<Option<Unlocked>>,
    /// Captura da sessão do banco, enviada pela extensão coletora (Firefox).
    /// Vive só em memória — são cookies de sessão, não vão para o disco.
    bank_capture: Mutex<Option<String>>,
}

impl AppState {
    /// Corre `f` com a carteira aberta (ou `None`).
    ///
    /// Recupera de um mutex envenenado: um pânico noutra thread não pode deixar
    /// a aplicação sem acesso à carteira.
    pub fn with<T>(&self, f: impl FnOnce(&mut Option<Unlocked>) -> T) -> T {
        let mut guard = self
            .unlocked
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        f(&mut guard)
    }

    /// Endereço da carteira aberta (não é segredo).
    pub fn address(&self) -> Option<String> {
        self.with(|slot| slot.as_ref().map(|open| open.address().to_string()))
    }

    /// Cópia da frase para uma operação assíncrona.
    ///
    /// ⚠️ É o **único** ponto em que existe uma cópia da seed fora do
    /// `SecretPhrase`. Existe porque uma ida à rede atravessa `await`s e não se
    /// pode segurar o trinco do estado todo esse tempo. Vai dentro de um
    /// [`Zeroizing`], que a limpa ao sair.
    pub fn mnemonic_copy(&self) -> Option<(Zeroizing<String>, String)> {
        self.with(|slot| {
            slot.as_ref()
                .map(|open| (Zeroizing::new(open.mnemonic().to_string()), open.address().to_string()))
        })
    }

    /// Guarda a captura que a extensão enviou.
    pub fn set_bank_capture(&self, raw: String) {
        let mut guard = self
            .bank_capture
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        *guard = Some(raw);
    }

    /// A captura em memória (se já houver alguma).
    pub fn bank_capture(&self) -> Option<String> {
        self.bank_capture
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner())
            .clone()
    }

    /// Esquece a captura (para começar de novo).
    pub fn clear_bank_capture(&self) {
        let mut guard = self
            .bank_capture
            .lock()
            .unwrap_or_else(|poisoned| poisoned.into_inner());
        *guard = None;
    }
}
