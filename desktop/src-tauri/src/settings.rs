//! Preferências do desktop (onde fica o nó a que se liga).
//!
//! Nada aqui é secreto — não tem a mesma proteção do cofre.

use std::fs;
use std::path::PathBuf;

use serde::{Deserialize, Serialize};

use crate::paths::data_dir;

/// Nó REST por omissão: o validador fundador da chain local.
pub const DEFAULT_NODE_URL: &str = "http://127.0.0.1:1317";

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Settings {
    #[serde(default)]
    pub node_url: Option<String>,
}

fn settings_path() -> PathBuf {
    data_dir().join("config.json")
}

/// Lê as preferências; um ficheiro ausente ou ilegível vale os valores por omissão.
pub fn load() -> Settings {
    fs::read_to_string(settings_path())
        .ok()
        .and_then(|text| serde_json::from_str(&text).ok())
        .unwrap_or_default()
}

pub fn node_url() -> String {
    load()
        .node_url
        .filter(|url| !url.trim().is_empty())
        .unwrap_or_else(|| DEFAULT_NODE_URL.to_string())
}

pub fn save_node_url(url: &str) -> std::io::Result<()> {
    let path = settings_path();
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir)?;
    }

    let settings = Settings {
        node_url: Some(url.trim().to_string()),
    };
    let text = serde_json::to_string_pretty(&settings)?;
    fs::write(path, text)
}
