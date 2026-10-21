use std::fs;
use std::path::{Path, PathBuf};
use std::time::{SystemTime, UNIX_EPOCH};

use base64::engine::general_purpose::STANDARD as B64;
use base64::Engine;
use bip39::{Language, Mnemonic};
use chacha20poly1305::aead::{Aead, KeyInit, Payload as AeadPayload};
use chacha20poly1305::{Key, XChaCha20Poly1305, XNonce};
use rand::RngCore;
use serde::de::DeserializeOwned;
use serde::{Deserialize, Serialize};
use zeroize::Zeroizing;

use crate::error::VaultError;
use crate::kdf::{KdfParams, SALT_LEN};
use crate::lockout::LockoutState;
use crate::secret::SecretPhrase;
use crate::store::{SecretStore, ACCOUNT, SERVICE};
use crate::WORDS_DEFAULT;

/// Comprimento mínimo da senha que cifra o cofre.
pub const MIN_PIN_LEN: usize = 8;

/// Comprimento da chave aleatória guardada no keychain (só para o atalho).
const KEYCHAIN_KEY_LEN: usize = 32;

/// Caminho BIP44 padrão do Cosmos SDK (o mesmo usado pelo assinador do APK).
pub const DEFAULT_HD_PATH: &str = "m/44'/118'/0'/0/0";

const VAULT_MAGIC: &str = "LUGGOM-VAULT";
const BACKUP_MAGIC: &str = "LUGGOM-VAULT-BACKUP";
const FORMAT_VERSION: u32 = 1;
const KDF_ALGORITHM: &str = "argon2id";
/// Versão do Argon2 (v1.3 = 0x13).
const KDF_VERSION: u32 = 0x13;
const CIPHER: &str = "xchacha20poly1305";
const NONCE_LEN: usize = 24;

/// Configuração usada ao escrever um cofre.
#[derive(Debug, Clone, Copy)]
pub struct VaultConfig {
    pub kdf: KdfParams,
}

impl Default for VaultConfig {
    fn default() -> Self {
        Self {
            kdf: KdfParams::PRODUCTION,
        }
    }
}

impl VaultConfig {
    /// Parâmetros baratos, **só para testes**.
    pub fn fast() -> Self {
        Self {
            kdf: KdfParams::FAST,
        }
    }
}

// ---------------------------------------------------------------------------
// Formato em disco
// ---------------------------------------------------------------------------

/// Envelope em claro. Só metadados — a seed vive dentro de `ciphertext`.
#[derive(Debug, Serialize, Deserialize)]
struct Envelope {
    magic: String,
    version: u32,
    kdf_algorithm: String,
    kdf_version: u32,
    kdf: KdfParams,
    salt: String,
    cipher: String,
    nonce: String,
    ciphertext: String,
    #[serde(default)]
    lockout: LockoutState,
    /// Segunda cópia do conteúdo, fechada com a chave do keychain (opt-in).
    #[serde(default, skip_serializing_if = "Option::is_none")]
    keychain: Option<KeychainSeal>,
}

/// Cópia do conteúdo fechada com a chave que vive no keychain do sistema.
#[derive(Debug, Serialize, Deserialize)]
struct KeychainSeal {
    cipher: String,
    nonce: String,
    ciphertext: String,
}

/// Conteúdo cifrado. **Sem `Debug`** de propósito: nunca deve ser impresso.
#[derive(Serialize, Deserialize)]
struct Payload {
    mnemonic: String,
    created_at_unix: u64,
    #[serde(default)]
    label: Option<String>,
}

/// Monta os dados autenticados-mas-visíveis (AAD).
///
/// Amarrar o cabeçalho ao texto cifrado impede que um atacante troque os
/// parâmetros do KDF (por exemplo, `m_cost = 8`) e continue a abrir o mesmo
/// cofre — a troca invalida a etiqueta do AEAD.
fn associated_data(magic: &str, kdf: KdfParams, salt_b64: &str) -> Vec<u8> {
    format!(
        "{magic}|{FORMAT_VERSION}|{KDF_ALGORITHM}|{KDF_VERSION}|{}|{}|{}|{salt_b64}",
        kdf.m_cost, kdf.t_cost, kdf.p_cost
    )
    .into_bytes()
}

fn encrypt_envelope<T: Serialize>(
    magic: &str,
    pin: &str,
    payload: &T,
    config: VaultConfig,
    lockout: LockoutState,
) -> Result<Envelope, VaultError> {
    let kdf = config.kdf;
    kdf.validate()?;

    let mut salt = [0u8; SALT_LEN];
    rand::rngs::OsRng.fill_bytes(&mut salt);
    let salt_b64 = B64.encode(salt);

    let mut nonce = [0u8; NONCE_LEN];
    rand::rngs::OsRng.fill_bytes(&mut nonce);

    let key = kdf.derive_key(pin, &salt)?;
    let cipher = XChaCha20Poly1305::new(Key::from_slice(&key[..]));

    let mut plaintext = Zeroizing::new(serde_json::to_vec(payload)?);
    let ciphertext = cipher
        .encrypt(
            XNonce::from_slice(&nonce),
            AeadPayload {
                msg: plaintext.as_mut(),
                aad: &associated_data(magic, kdf, &salt_b64),
            },
        )
        .map_err(|e| VaultError::Crypto(format!("cifra: {e}")))?;

    Ok(Envelope {
        magic: magic.to_string(),
        version: FORMAT_VERSION,
        kdf_algorithm: KDF_ALGORITHM.to_string(),
        kdf_version: KDF_VERSION,
        kdf,
        salt: salt_b64,
        cipher: CIPHER.to_string(),
        nonce: B64.encode(nonce),
        ciphertext: B64.encode(ciphertext),
        lockout,
        keychain: None,
    })
}

/// AAD da cópia do keychain: liga-a ao ficheiro e ao par serviço/conta, para
/// que um selo não possa ser transplantado para outro cofre nem para outra
/// entrada do keychain.
fn keychain_aad(magic: &str, service: &str, account: &str) -> Vec<u8> {
    format!("{magic}|{FORMAT_VERSION}|keychain|{service}|{account}").into_bytes()
}

/// Fecha o conteúdo com uma chave já existente (o keychain não precisa de KDF:
/// a chave é aleatória de 32 bytes).
fn seal_with_key(aad: &[u8], key: &[u8], payload: &Payload) -> Result<KeychainSeal, VaultError> {
    let mut nonce = [0u8; NONCE_LEN];
    rand::rngs::OsRng.fill_bytes(&mut nonce);

    let cipher = XChaCha20Poly1305::new(Key::from_slice(key));
    let mut plaintext = Zeroizing::new(serde_json::to_vec(payload)?);
    let ciphertext = cipher
        .encrypt(
            XNonce::from_slice(&nonce),
            AeadPayload {
                msg: plaintext.as_mut(),
                aad,
            },
        )
        .map_err(|e| VaultError::Crypto(format!("cifra (keychain): {e}")))?;

    Ok(KeychainSeal {
        cipher: CIPHER.to_string(),
        nonce: B64.encode(nonce),
        ciphertext: B64.encode(ciphertext),
    })
}

fn open_with_key(seal: &KeychainSeal, aad: &[u8], key: &[u8]) -> Result<Payload, VaultError> {
    if seal.cipher != CIPHER {
        return Err(VaultError::Format(format!(
            "cifra não suportada: {}",
            seal.cipher
        )));
    }

    let nonce = B64
        .decode(&seal.nonce)
        .map_err(|e| VaultError::Format(format!("nonce inválido: {e}")))?;
    if nonce.len() != NONCE_LEN {
        return Err(VaultError::Format("nonce com comprimento errado".into()));
    }
    let ciphertext = B64
        .decode(&seal.ciphertext)
        .map_err(|e| VaultError::Format(format!("cifra inválida: {e}")))?;

    let cipher = XChaCha20Poly1305::new(Key::from_slice(key));
    let mut plaintext = cipher
        .decrypt(
            XNonce::from_slice(&nonce),
            AeadPayload {
                msg: &ciphertext,
                aad,
            },
        )
        .map_err(|_| {
            VaultError::KeychainUnavailable("a chave do keychain não abre este cofre".into())
        })?;

    let payload = serde_json::from_slice::<Payload>(&plaintext)
        .map_err(|e| VaultError::Format(format!("conteúdo inválido: {e}")))?;
    zeroize::Zeroize::zeroize(&mut plaintext);
    Ok(payload)
}

fn parse_envelope(raw: &str, expected_magic: &str) -> Result<Envelope, VaultError> {
    let envelope: Envelope =
        serde_json::from_str(raw).map_err(|e| VaultError::Format(e.to_string()))?;

    if envelope.magic != expected_magic {
        return Err(VaultError::NotABackup);
    }
    if envelope.version != FORMAT_VERSION {
        return Err(VaultError::Format(format!(
            "versão de formato não suportada: {}",
            envelope.version
        )));
    }
    if envelope.kdf_algorithm != KDF_ALGORITHM || envelope.kdf_version != KDF_VERSION {
        return Err(VaultError::Format(format!(
            "KDF não suportado: {} v{}",
            envelope.kdf_algorithm, envelope.kdf_version
        )));
    }
    if envelope.cipher != CIPHER {
        return Err(VaultError::Format(format!(
            "cifra não suportada: {}",
            envelope.cipher
        )));
    }
    envelope.kdf.validate()?;
    Ok(envelope)
}

fn decrypt_envelope<T: DeserializeOwned>(
    envelope: &Envelope,
    pin: &str,
) -> Result<T, VaultError> {
    let salt = B64
        .decode(&envelope.salt)
        .map_err(|e| VaultError::Format(format!("sal inválido: {e}")))?;
    let salt: [u8; SALT_LEN] = salt
        .as_slice()
        .try_into()
        .map_err(|_| VaultError::Format("sal com comprimento errado".into()))?;

    let nonce = B64
        .decode(&envelope.nonce)
        .map_err(|e| VaultError::Format(format!("nonce inválido: {e}")))?;
    if nonce.len() != NONCE_LEN {
        return Err(VaultError::Format("nonce com comprimento errado".into()));
    }
    let ciphertext = B64
        .decode(&envelope.ciphertext)
        .map_err(|e| VaultError::Format(format!("cifra inválida: {e}")))?;

    let key = envelope.kdf.derive_key(pin, &salt)?;
    let cipher = XChaCha20Poly1305::new(Key::from_slice(&key[..]));

    let mut plaintext = cipher
        .decrypt(
            XNonce::from_slice(&nonce),
            AeadPayload {
                msg: &ciphertext,
                aad: &associated_data(&envelope.magic, envelope.kdf, &envelope.salt),
            },
        )
        // Falha de autenticação = PIN errado (ou ficheiro adulterado). O AEAD
        // não permite distinguir os dois casos.
        .map_err(|_| VaultError::WrongPin)?;

    let payload = serde_json::from_slice::<T>(&plaintext)
        .map_err(|e| VaultError::Format(format!("conteúdo inválido: {e}")))?;
    zeroize::Zeroize::zeroize(&mut plaintext);
    Ok(payload)
}

// ---------------------------------------------------------------------------
// Cofre
// ---------------------------------------------------------------------------

/// Uma carteira cifrada em disco.
pub struct Vault {
    path: PathBuf,
}

impl Vault {
    pub fn at(path: impl Into<PathBuf>) -> Self {
        Self { path: path.into() }
    }

    pub fn path(&self) -> &Path {
        &self.path
    }

    pub fn exists(&self) -> bool {
        self.path.is_file()
    }

    /// Cria uma carteira nova e devolve a frase, para o utilizador a guardar.
    ///
    /// A frase devolvida é a **única** ocasião em que a seed pode ser mostrada;
    /// a partir daqui só com o PIN se volta a vê-la.
    pub fn create(&self, pin: &str, config: VaultConfig) -> Result<SecretPhrase, VaultError> {
        ensure_pin(pin)?;
        self.refuse_if_present()?;

        let mnemonic = Mnemonic::generate_in(Language::English, WORDS_DEFAULT)
            .map_err(|e| VaultError::InvalidMnemonic(e.to_string()))?;
        let phrase = normalize(&mnemonic.to_string());

        self.write_new(&phrase, pin, config)?;
        Ok(SecretPhrase::new(phrase))
    }

    /// Importa uma frase existente.
    pub fn import(
        &self,
        pin: &str,
        phrase: &str,
        config: VaultConfig,
    ) -> Result<SecretPhrase, VaultError> {
        ensure_pin(pin)?;
        self.refuse_if_present()?;

        let phrase = normalize(phrase);
        validate_mnemonic(&phrase)?;

        self.write_new(&phrase, pin, config)?;
        Ok(SecretPhrase::new(phrase))
    }

    /// Abre o cofre. Aplica o travão de tentativas.
    pub fn unlock(&self, pin: &str) -> Result<SecretPhrase, VaultError> {
        let (payload, _) = self.open(pin)?;
        Ok(SecretPhrase::new(payload.mnemonic))
    }

    /// Abre o cofre e devolve o conteúdo com o envelope (para reescrever metadados).
    fn open(&self, pin: &str) -> Result<(Payload, Envelope), VaultError> {
        let mut envelope = self.read()?;
        let now = now_unix();

        if let Some(secs) = envelope.lockout.seconds_remaining(now) {
            return Err(VaultError::LockedOut(secs));
        }

        match decrypt_envelope::<Payload>(&envelope, pin) {
            Ok(payload) => {
                validate_mnemonic(&payload.mnemonic)?;
                // Repor o contador é higiene, não segurança: se o disco estiver
                // só-de-leitura, abrir continua a valer a pena.
                if envelope.lockout != LockoutState::default() {
                    envelope.lockout.register_success();
                    let _ = self.write_envelope(&envelope);
                }
                Ok((payload, envelope))
            }
            Err(err) => {
                envelope.lockout.register_failure(now);
                // Aqui o erro **tem** de propagar: sem persistir, o travão não trava.
                self.write_envelope(&envelope)?;
                Err(err)
            }
        }
    }

    /// Troca o PIN, re-cifrando com sal novo.
    pub fn change_pin(&self, old_pin: &str, new_pin: &str) -> Result<(), VaultError> {
        ensure_pin(new_pin)?;
        let (payload, source) = self.open(old_pin)?;

        let envelope = encrypt_envelope(
            VAULT_MAGIC,
            new_pin,
            &payload,
            VaultConfig { kdf: source.kdf },
            LockoutState::default(),
        )?;
        self.write_envelope(&envelope)
    }

    /// Estado do travão de tentativas (para a UI mostrar "restam N tentativas").
    pub fn lockout_state(&self) -> Result<LockoutState, VaultError> {
        Ok(self.read()?.lockout)
    }

    /// Apaga o cofre. Exige o PIN: evita apagar a carteira por engano.
    ///
    /// Não há "apagamento seguro" — em memória flash e sistemas copy-on-write
    /// não é garantido. A proteção real é a cifra: sem o PIN, o que fica no
    /// disco não serve para nada.
    pub fn delete(&self, pin: &str) -> Result<(), VaultError> {
        let _ = self.unlock(pin)?;
        match fs::remove_file(&self.path) {
            Ok(()) => Ok(()),
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
            Err(e) => Err(VaultError::Io(e)),
        }
    }

    /// Exporta um backup cifrado para outro ficheiro.
    ///
    /// O backup usa **sal novo** (nunca reutiliza o do cofre) e pode ser
    /// restaurado numa máquina limpa com [`restore_from_backup`].
    pub fn export_backup(
        &self,
        pin: &str,
        dest: impl AsRef<Path>,
        config: VaultConfig,
    ) -> Result<(), VaultError> {
        let dest = dest.as_ref();
        if dest.exists() {
            return Err(VaultError::AlreadyExists(dest.to_path_buf()));
        }

        let (payload, _) = self.open(pin)?;
        let envelope =
            encrypt_envelope(BACKUP_MAGIC, pin, &payload, config, LockoutState::default())?;
        write_secret_file(dest, serde_json::to_vec_pretty(&envelope)?.as_slice())
    }

    /// O atalho do keychain está ativado nesta carteira?
    pub fn has_keychain_unlock(&self) -> Result<bool, VaultError> {
        Ok(self.read()?.keychain.is_some())
    }

    /// Ativa o "abrir sem PIN".
    ///
    /// Guarda no keychain uma **chave aleatória** (nunca a seed) que fecha uma
    /// segunda cópia do mesmo conteúdo. Exige o PIN, para que ninguém ligue o
    /// atalho numa carteira que não é sua.
    pub fn enable_keychain_unlock(
        &self,
        pin: &str,
        store: &dyn SecretStore,
    ) -> Result<(), VaultError> {
        if !store.is_available() {
            return Err(VaultError::KeychainUnavailable(
                "esta máquina não tem keychain utilizável".into(),
            ));
        }

        let (payload, mut envelope) = self.open(pin)?;
        let aad = keychain_aad(&envelope.magic, SERVICE, ACCOUNT);

        let mut key = Zeroizing::new([0u8; KEYCHAIN_KEY_LEN]);
        rand::rngs::OsRng.fill_bytes(key.as_mut());
        let seal = seal_with_key(&aad, key.as_ref(), &payload)?;

        // A chave primeiro: se a escrita do cofre falhar, desfazemos a entrada
        // em vez de deixar um selo sem chave (que pareceria "ligado").
        store.set(SERVICE, ACCOUNT, key.as_ref())?;
        envelope.keychain = Some(seal);
        if let Err(err) = self.write_envelope(&envelope) {
            let _ = store.delete(SERVICE, ACCOUNT);
            return Err(err);
        }
        Ok(())
    }

    /// Desliga o atalho e remove a chave do keychain.
    pub fn disable_keychain_unlock(
        &self,
        pin: &str,
        store: &dyn SecretStore,
    ) -> Result<(), VaultError> {
        let (_, mut envelope) = self.open(pin)?;
        envelope.keychain = None;
        self.write_envelope(&envelope)?;

        // Se isto falhar, a chave fica órfã — já não existe selo que ela abra,
        // portanto não há nada a proteger. Não vale a pena falhar a operação.
        let _ = store.delete(SERVICE, ACCOUNT);
        Ok(())
    }

    /// Abre o cofre sem PIN, pela chave do keychain.
    ///
    /// Não passa pelo travão de tentativas: não há PIN a adivinhar aqui, e quem
    /// chegou até aqui já tem a sessão do sistema aberta.
    pub fn unlock_with_keychain(
        &self,
        store: &dyn SecretStore,
    ) -> Result<SecretPhrase, VaultError> {
        let envelope = self.read()?;
        let seal = envelope
            .keychain
            .as_ref()
            .ok_or(VaultError::KeychainNotEnabled)?;

        let key = store
            .get(SERVICE, ACCOUNT)?
            .ok_or(VaultError::KeychainNotEnabled)?;
        if key.len() != KEYCHAIN_KEY_LEN {
            return Err(VaultError::KeychainUnavailable(
                "chave guardada com comprimento errado".into(),
            ));
        }

        let aad = keychain_aad(&envelope.magic, SERVICE, ACCOUNT);
        let payload = open_with_key(seal, &aad, &key)?;
        validate_mnemonic(&payload.mnemonic)?;
        Ok(SecretPhrase::new(payload.mnemonic))
    }

    // -- interno ------------------------------------------------------------

    fn refuse_if_present(&self) -> Result<(), VaultError> {
        if self.exists() {
            return Err(VaultError::AlreadyExists(self.path.clone()));
        }
        Ok(())
    }

    fn write_new(
        &self,
        phrase: &str,
        pin: &str,
        config: VaultConfig,
    ) -> Result<(), VaultError> {
        let payload = Payload {
            mnemonic: phrase.to_string(),
            created_at_unix: now_unix(),
            label: None,
        };
        let envelope = encrypt_envelope(
            VAULT_MAGIC,
            pin,
            &payload,
            config,
            LockoutState::default(),
        )?;
        self.write_envelope(&envelope)
    }

    fn read(&self) -> Result<Envelope, VaultError> {
        let raw = match fs::read_to_string(&self.path) {
            Ok(raw) => raw,
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {
                return Err(VaultError::NotFound(self.path.clone()));
            }
            Err(e) => return Err(VaultError::Io(e)),
        };
        parse_envelope(&raw, VAULT_MAGIC)
    }

    fn write_envelope(&self, envelope: &Envelope) -> Result<(), VaultError> {
        write_secret_file(&self.path, serde_json::to_vec_pretty(envelope)?.as_slice())
    }
}

/// Restaura um cofre a partir de um backup.
///
/// A frase é lida com o PIN **do backup**; o cofre novo é criado com `new_pin`.
pub fn restore_from_backup(
    backup: impl AsRef<Path>,
    backup_pin: &str,
    vault_path: impl Into<PathBuf>,
    new_pin: &str,
    config: VaultConfig,
) -> Result<SecretPhrase, VaultError> {
    ensure_pin(new_pin)?;

    let raw = fs::read_to_string(backup.as_ref()).map_err(|e| match e.kind() {
        std::io::ErrorKind::NotFound => VaultError::NotFound(backup.as_ref().to_path_buf()),
        _ => VaultError::Io(e),
    })?;
    let envelope = parse_envelope(&raw, BACKUP_MAGIC)?;
    let payload: Payload = decrypt_envelope(&envelope, backup_pin)?;

    let vault = Vault::at(vault_path);
    vault.import(new_pin, &payload.mnemonic, config)?;
    Ok(SecretPhrase::new(payload.mnemonic))
}

// ---------------------------------------------------------------------------
// Auxiliares
// ---------------------------------------------------------------------------

fn ensure_pin(pin: &str) -> Result<(), VaultError> {
    if pin.chars().count() < MIN_PIN_LEN {
        return Err(VaultError::WeakPin(MIN_PIN_LEN));
    }
    Ok(())
}

/// Colapsa espaços e passa a minúsculas (BIP39 é insensível a maiúsculas).
fn normalize(phrase: &str) -> String {
    phrase
        .split_whitespace()
        .map(|w| w.to_lowercase())
        .collect::<Vec<_>>()
        .join(" ")
}

fn validate_mnemonic(phrase: &str) -> Result<(), VaultError> {
    Mnemonic::parse_in(Language::English, phrase)
        .map(|_| ())
        .map_err(|e| VaultError::InvalidMnemonic(e.to_string()))
}

fn now_unix() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

/// Escreve um ficheiro com permissões 0600, de forma atómica.
fn write_secret_file(path: &Path, contents: &[u8]) -> Result<(), VaultError> {
    use std::io::Write;

    if let Some(dir) = path.parent() {
        if !dir.as_os_str().is_empty() {
            fs::create_dir_all(dir)?;
        }
    }

    let mut suffix = [0u8; 8];
    rand::rngs::OsRng.fill_bytes(&mut suffix);
    let tmp = path.with_extension(format!("tmp-{}", hex(&suffix)));

    let write_result = (|| -> Result<(), VaultError> {
        let mut file = fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&tmp)?;
        #[cfg(unix)]
        {
            use std::os::unix::fs::PermissionsExt;
            file.set_permissions(fs::Permissions::from_mode(0o600))?;
        }
        file.write_all(contents)?;
        file.sync_all()?;
        Ok(())
    })();

    if let Err(err) = write_result {
        let _ = fs::remove_file(&tmp);
        return Err(err);
    }

    fs::rename(&tmp, path)?;
    Ok(())
}

fn hex(bytes: &[u8]) -> String {
    bytes.iter().map(|b| format!("{b:02x}")).collect()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::store::{InMemoryStore, OsKeychain};
    use tempfile::TempDir;

    const PIN: &str = "12345678";
    const MNEMONIC: &str =
        "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about";

    fn vault_in(dir: &TempDir) -> Vault {
        Vault::at(dir.path().join("vault.json"))
    }

    /// Confirma que nada da seed aparece em claro no ficheiro.
    ///
    /// Não basta procurar palavras isoladas: o texto cifrado em base64 contém,
    /// por acaso, palavras curtas (aconteceu com "once"). Procuramos a frase
    /// inteira, pares de palavras consecutivas (o espaço não existe no alfabeto
    /// base64, logo um par nunca pode calhar por acidente) e a chave JSON.
    fn assert_no_plaintext(contents: &str, phrase: &str) {
        assert!(!contents.contains(phrase), "a frase completa está em claro");
        assert!(
            !contents.contains("\"mnemonic\""),
            "a chave do payload está em claro"
        );

        let words: Vec<&str> = phrase.split_whitespace().collect();
        for pair in words.windows(2) {
            let joined = format!("{} {}", pair[0], pair[1]);
            assert!(!contents.contains(&joined), "par de palavras em claro: {joined}");
        }
    }

    #[test]
    fn create_then_unlock_round_trips() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);

        let created = vault.create(PIN, VaultConfig::fast()).unwrap();
        let unlocked = vault.unlock(PIN).unwrap();

        assert_eq!(created.expose(), unlocked.expose());
        assert_eq!(unlocked.word_count(), 24);
    }

    #[test]
    fn the_file_never_contains_the_seed_in_clear() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);

        let phrase = vault.create(PIN, VaultConfig::fast()).unwrap();
        let on_disk = fs::read_to_string(vault.path()).unwrap();

        assert_no_plaintext(&on_disk, phrase.expose());
    }

    #[test]
    fn import_round_trips_and_normalizes_the_phrase() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);

        let messy = format!("  {}  ", MNEMONIC.to_uppercase().replace(' ', "   "));
        vault.import(PIN, &messy, VaultConfig::fast()).unwrap();

        assert_eq!(vault.unlock(PIN).unwrap().expose(), MNEMONIC);
    }

    #[test]
    fn import_rejects_an_invalid_phrase() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);

        let err = vault
            .import(PIN, "não é um mnemónico", VaultConfig::fast())
            .unwrap_err();
        assert!(matches!(err, VaultError::InvalidMnemonic(_)));
        assert!(!vault.exists());
    }

    #[test]
    fn import_rejects_a_phrase_with_a_bad_checksum() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);

        // Mesmas palavras do mnemónico válido, mas com a última trocada.
        let bad = MNEMONIC.replace("about", "abandon");
        assert!(matches!(
            vault.import(PIN, &bad, VaultConfig::fast()).unwrap_err(),
            VaultError::InvalidMnemonic(_)
        ));
    }

    #[test]
    fn refuses_a_short_pin() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        assert!(matches!(
            vault.create("12345", VaultConfig::fast()).unwrap_err(),
            VaultError::WeakPin(MIN_PIN_LEN)
        ));
    }

    #[test]
    fn wrong_pin_is_rejected_and_counted() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        assert!(matches!(
            vault.unlock("000000").unwrap_err(),
            VaultError::WrongPin
        ));
        assert_eq!(vault.lockout_state().unwrap().failed_attempts, 1);

        // E o PIN certo continua a abrir.
        assert!(vault.unlock(PIN).is_ok());
        assert_eq!(vault.lockout_state().unwrap().failed_attempts, 0);
    }

    #[test]
    fn repeated_failures_lock_the_vault() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        for _ in 0..5 {
            assert!(vault.unlock("000000").is_err());
        }

        // Bloqueado: nem o PIN certo passa, e o erro diz quanto falta esperar.
        match vault.unlock(PIN).unwrap_err() {
            VaultError::LockedOut(secs) => assert!(secs > 0),
            other => panic!("esperava LockedOut, veio {other:?}"),
        }
    }

    #[test]
    fn refuses_to_overwrite_an_existing_vault() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        assert!(matches!(
            vault.create(PIN, VaultConfig::fast()).unwrap_err(),
            VaultError::AlreadyExists(_)
        ));
        assert!(matches!(
            vault.import(PIN, MNEMONIC, VaultConfig::fast()).unwrap_err(),
            VaultError::AlreadyExists(_)
        ));
    }

    #[test]
    fn unlocking_a_missing_vault_reports_not_found() {
        let dir = TempDir::new().unwrap();
        assert!(matches!(
            vault_in(&dir).unlock(PIN).unwrap_err(),
            VaultError::NotFound(_)
        ));
    }

    #[test]
    fn backup_restore_recovers_the_same_seed() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let backup = dir.path().join("backup.luggom");

        vault.import(PIN, MNEMONIC, VaultConfig::fast()).unwrap();
        vault.export_backup(PIN, &backup, VaultConfig::fast()).unwrap();

        // É o caso T-APP-3: exportar → apagar → importar.
        vault.delete(PIN).unwrap();
        assert!(!vault.exists());

        let restored = restore_from_backup(
            &backup,
            PIN,
            vault.path(),
            "novo-pin-123",
            VaultConfig::fast(),
        )
        .unwrap();

        assert_eq!(restored.expose(), MNEMONIC);
        assert_eq!(vault.unlock("novo-pin-123").unwrap().expose(), MNEMONIC);
        // O PIN antigo já não abre: o cofre foi recriado com o novo.
        assert!(vault.unlock(PIN).is_err());
    }

    #[test]
    fn the_backup_file_never_contains_the_seed_in_clear() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let backup = dir.path().join("backup.luggom");

        let phrase = vault.create(PIN, VaultConfig::fast()).unwrap();
        vault.export_backup(PIN, &backup, VaultConfig::fast()).unwrap();

        let on_disk = fs::read_to_string(&backup).unwrap();
        assert_no_plaintext(&on_disk, phrase.expose());
    }

    #[test]
    fn backup_uses_its_own_salt() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let backup = dir.path().join("backup.luggom");

        vault.create(PIN, VaultConfig::fast()).unwrap();
        vault.export_backup(PIN, &backup, VaultConfig::fast()).unwrap();

        let vault_salt = parse_envelope(&fs::read_to_string(vault.path()).unwrap(), VAULT_MAGIC)
            .unwrap()
            .salt;
        let backup_salt = parse_envelope(&fs::read_to_string(&backup).unwrap(), BACKUP_MAGIC)
            .unwrap()
            .salt;
        assert_ne!(vault_salt, backup_salt, "o sal foi reutilizado");
    }

    #[test]
    fn refuses_to_clobber_an_existing_backup() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let backup = dir.path().join("backup.luggom");

        vault.create(PIN, VaultConfig::fast()).unwrap();
        vault.export_backup(PIN, &backup, VaultConfig::fast()).unwrap();

        assert!(matches!(
            vault.export_backup(PIN, &backup, VaultConfig::fast()).unwrap_err(),
            VaultError::AlreadyExists(_)
        ));
    }

    #[test]
    fn a_vault_file_is_not_a_backup() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        assert!(matches!(
            restore_from_backup(
                vault.path(),
                PIN,
                dir.path().join("outro.json"),
                PIN,
                VaultConfig::fast()
            )
            .unwrap_err(),
            VaultError::NotABackup
        ));
    }

    #[test]
    fn change_pin_rewraps_the_same_seed() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        let before = vault.unlock(PIN).unwrap().expose().to_string();
        vault.change_pin(PIN, "outro-pin-99").unwrap();

        assert!(vault.unlock(PIN).is_err());
        assert_eq!(vault.unlock("outro-pin-99").unwrap().expose(), before);
    }

    #[test]
    fn tampering_with_the_kdf_header_invalidates_the_file() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        // Baixar o custo de memória é o ataque óbvio: tem de invalidar a etiqueta.
        let raw = fs::read_to_string(vault.path()).unwrap();
        let tampered = raw.replace("\"m_cost\": 8", "\"m_cost\": 16");
        assert_ne!(raw, tampered, "o teste não alterou nada");
        fs::write(vault.path(), tampered).unwrap();

        assert!(matches!(vault.unlock(PIN).unwrap_err(), VaultError::WrongPin));
    }

    #[cfg(unix)]
    #[test]
    fn the_vault_file_is_owner_only() {
        use std::os::unix::fs::PermissionsExt;
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        let mode = fs::metadata(vault.path()).unwrap().permissions().mode();
        assert_eq!(mode & 0o777, 0o600, "permissões do cofre erradas");
    }

    #[test]
    fn the_vault_is_safe_against_a_truncated_file() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        fs::write(vault.path(), "{\"magic\":\"LUGGOM-VAULT\"").unwrap();
        assert!(vault.unlock(PIN).is_err());
    }

    // -- keychain ------------------------------------------------------------

    #[test]
    fn keychain_unlock_is_off_by_default() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let store = InMemoryStore::new();
        vault.create(PIN, VaultConfig::fast()).unwrap();

        assert!(!vault.has_keychain_unlock().unwrap());
        assert!(matches!(
            vault.unlock_with_keychain(&store).unwrap_err(),
            VaultError::KeychainNotEnabled
        ));
    }

    #[test]
    fn keychain_unlock_round_trips_the_same_seed() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let store = InMemoryStore::new();

        vault.import(PIN, MNEMONIC, VaultConfig::fast()).unwrap();
        vault.enable_keychain_unlock(PIN, &store).unwrap();

        assert!(vault.has_keychain_unlock().unwrap());
        assert_eq!(
            vault.unlock_with_keychain(&store).unwrap().expose(),
            MNEMONIC
        );
        // E o PIN continua a ser um caminho válido.
        assert_eq!(vault.unlock(PIN).unwrap().expose(), MNEMONIC);
    }

    #[test]
    fn the_keychain_holds_a_key_and_not_the_seed() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let store = InMemoryStore::new();

        vault.import(PIN, MNEMONIC, VaultConfig::fast()).unwrap();
        vault.enable_keychain_unlock(PIN, &store).unwrap();

        let stored = store.get(SERVICE, ACCOUNT).unwrap().unwrap();
        assert_eq!(stored.len(), 32, "devia ser uma chave, não uma frase");
        assert!(!String::from_utf8_lossy(&stored).contains("abandon"));
    }

    #[test]
    fn neither_file_contains_the_seed_with_the_keychain_enabled() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let store = InMemoryStore::new();

        let phrase = vault.create(PIN, VaultConfig::fast()).unwrap();
        vault.enable_keychain_unlock(PIN, &store).unwrap();

        let on_disk = fs::read_to_string(vault.path()).unwrap();
        assert_no_plaintext(&on_disk, phrase.expose());
    }

    #[test]
    fn disabling_the_keychain_removes_the_seal_and_the_entry() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let store = InMemoryStore::new();

        vault.create(PIN, VaultConfig::fast()).unwrap();
        vault.enable_keychain_unlock(PIN, &store).unwrap();
        assert_eq!(store.len(), 1);

        vault.disable_keychain_unlock(PIN, &store).unwrap();

        assert!(!vault.has_keychain_unlock().unwrap());
        assert!(store.is_empty(), "a chave ficou no keychain");
        assert!(matches!(
            vault.unlock_with_keychain(&store).unwrap_err(),
            VaultError::KeychainNotEnabled
        ));
        // O PIN continua a abrir: só o atalho desapareceu.
        assert!(vault.unlock(PIN).is_ok());
    }

    #[test]
    fn enabling_with_a_wrong_pin_leaves_nothing_behind() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let store = InMemoryStore::new();
        vault.create(PIN, VaultConfig::fast()).unwrap();

        assert!(matches!(
            vault.enable_keychain_unlock("000000", &store).unwrap_err(),
            VaultError::WrongPin
        ));
        assert!(store.is_empty(), "ficou uma chave no keychain");
        assert!(!vault.has_keychain_unlock().unwrap());
    }

    #[test]
    fn a_replaced_keychain_key_does_not_open_the_vault() {
        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        let store = InMemoryStore::new();
        vault.create(PIN, VaultConfig::fast()).unwrap();
        vault.enable_keychain_unlock(PIN, &store).unwrap();

        // O selo é autenticado: trocar a chave por lixo tem de falhar.
        let mut tampered = store.get(SERVICE, ACCOUNT).unwrap().unwrap();
        tampered[0] ^= 0xff;
        store.set(SERVICE, ACCOUNT, &tampered).unwrap();

        assert!(vault.unlock_with_keychain(&store).is_err());
    }

    #[test]
    fn a_machine_without_a_keychain_is_refused() {
        struct NoKeychain;

        impl SecretStore for NoKeychain {
            fn get(&self, _: &str, _: &str) -> Result<Option<Vec<u8>>, VaultError> {
                Ok(None)
            }
            fn set(&self, _: &str, _: &str, _: &[u8]) -> Result<(), VaultError> {
                unreachable!("não deve ser chamado")
            }
            fn delete(&self, _: &str, _: &str) -> Result<(), VaultError> {
                Ok(())
            }
            fn is_available(&self) -> bool {
                false
            }
        }

        let dir = TempDir::new().unwrap();
        let vault = vault_in(&dir);
        vault.create(PIN, VaultConfig::fast()).unwrap();

        assert!(matches!(
            vault.enable_keychain_unlock(PIN, &NoKeychain).unwrap_err(),
            VaultError::KeychainUnavailable(_)
        ));
    }

    /// Smoke do keychain **real** desta máquina. Não corre por omissão (escreve
    /// no keychain do utilizador): `cargo test -p luggom-core -- --ignored`.
    #[test]
    #[ignore]
    fn os_keychain_smoke() {
        let store = OsKeychain;
        if !store.is_available() {
            eprintln!("sem keychain utilizável nesta máquina — nada a testar");
            return;
        }

        let probe = "luggom-desktop-probe";
        store.set(SERVICE, probe, b"segredo-de-teste").unwrap();
        assert_eq!(
            store.get(SERVICE, probe).unwrap().unwrap(),
            b"segredo-de-teste"
        );
        store.delete(SERVICE, probe).unwrap();
        assert!(store.get(SERVICE, probe).unwrap().is_none());
    }

    // T-APP-4 (hard test, 2026-09-30): o travão de tentativas PERSISTE no
    // ficheiro — reabrir o cofre (reiniciar a app) NÃO zera o contador. Sem
    // isto, um atacante local contornava o lockout bastando reiniciar a app.
    #[test]
    fn lockout_survives_reopening_the_vault() {
        let dir = TempDir::new().unwrap();
        let path = dir.path().join("vault.json");

        {
            let vault = Vault::at(path.clone());
            vault.create(PIN, VaultConfig::fast()).unwrap();
            for _ in 0..5 {
                assert!(vault.unlock("000000").is_err());
            }
        }

        // Nova instância sobre o MESMO ficheiro (simula reiniciar a app).
        let reopened = Vault::at(path);
        match reopened.unlock(PIN).unwrap_err() {
            VaultError::LockedOut(secs) => assert!(secs > 0),
            other => panic!("esperava LockedOut depois de reabrir, veio {other:?}"),
        }
        assert_eq!(reopened.lockout_state().unwrap().failed_attempts, 5);
    }
}
