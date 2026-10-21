use std::path::PathBuf;

/// Variável de ambiente que sobrepõe a pasta de dados (útil para testes e para
/// correr várias instâncias lado a lado).
pub const DATA_DIR_ENV: &str = "LUGGOM_DESKTOP_DATA";

/// Pasta de dados do desktop: `<casa>/.luggom-desktop`.
///
/// Não é para ficar em `~/Documentos` nem sincronizada para a nuvem: o cofre
/// cifrado pode ser copiado, mas quanto menos cópias andarem por aí, melhor.
pub fn data_dir() -> PathBuf {
    if let Some(dir) = std::env::var_os(DATA_DIR_ENV) {
        if !dir.is_empty() {
            return PathBuf::from(dir);
        }
    }

    let home = std::env::var_os("HOME")
        .or_else(|| std::env::var_os("USERPROFILE"))
        .unwrap_or_default();
    PathBuf::from(home).join(".luggom-desktop")
}

/// Caminho do cofre.
pub fn vault_path() -> PathBuf {
    data_dir().join("vault.json")
}
