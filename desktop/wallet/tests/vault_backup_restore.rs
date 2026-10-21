//! Critério de aceite do A2 (vector `T-APP-3`), no nível que já corre sem GUI:
//!
//! **exportar → apagar → importar devolve o mesmo endereço** (e como o saldo
//! vive na chain, o mesmo endereço implica o mesmo saldo).

use luggom_core::{restore_from_backup, Vault, VaultConfig};
use luggom_wallet::Wallet;

const PIN: &str = "12345678";

#[test]
fn export_delete_import_keeps_the_same_address() {
    let dir = tempfile::TempDir::new().unwrap();
    let vault = Vault::at(dir.path().join("vault.json"));
    let backup = dir.path().join("backup.luggom");

    let phrase = vault.create(PIN, VaultConfig::fast()).unwrap();
    let address = Wallet::from_mnemonic(phrase.expose())
        .unwrap()
        .address()
        .to_string();

    vault.export_backup(PIN, &backup, VaultConfig::fast()).unwrap();
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

    let restored_address = Wallet::from_mnemonic(restored.expose())
        .unwrap()
        .address()
        .to_string();
    assert_eq!(restored_address, address, "o endereço mudou depois do restauro");
}

#[test]
fn an_imported_phrase_gives_a_stable_address() {
    let dir = tempfile::TempDir::new().unwrap();
    let vault = Vault::at(dir.path().join("vault.json"));

    // Frase conhecida: o endereço tem de bater certo com o `luggomd`.
    vault
        .import(
            PIN,
            "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about",
            VaultConfig::fast(),
        )
        .unwrap();

    let unlocked = vault.unlock(PIN).unwrap();
    let wallet = Wallet::from_mnemonic(unlocked.expose()).unwrap();
    assert_eq!(
        wallet.address(),
        "luggom19rl4cm2hmr8afy4kldpxz3fka4jguq0amgkgeu"
    );
}
