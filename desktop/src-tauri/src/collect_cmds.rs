//! O banco, pelo Firefox do utilizador — e o app fica à escuta da captura.
//!
//! **Porquê assim** (e não uma janela dentro do app): o anti-fraude do banco
//! (DataDome) recusa o webview do sistema — medido a 2026-09-23, com e sem
//! instrumentação nossa. O **Firefox real** passa. Por isso o app **orquestra**:
//! abre o login no Firefox (onde corre a extensão coletora) e recebe a captura
//! no `127.0.0.1:8099` — o mesmo endereço que o `coletor-browser/background.js`
//! já usa. Nada é injetado na página do banco.

use std::io::{BufRead, BufReader, Read, Write};
use std::net::{TcpListener, TcpStream};

use tauri::{AppHandle, Manager, State};

use crate::error::CommandError;
use crate::mine_cmds::{capture_reply, CaptureReply};
use crate::state::AppState;

/// Onde a extensão entrega a captura (é o que o `background.js` usa).
const LISTEN_ADDR: &str = "127.0.0.1:8099";

/// Teto do que se aceita (a captura real anda nos ~centenas de KB).
const MAX_CAPTURE_BYTES: usize = 8 * 1024 * 1024;

/// Página de entrada por banco. Fail-closed: sem entrada conhecida não se abre nada.
const BANK_LOGIN: &[(&str, &str)] = &[(
    "viacredi",
    "https://contaonline.viacredi.coop.br/ib/auth",
)];

fn login_url(provider_id: &str) -> Result<&'static str, CommandError> {
    BANK_LOGIN
        .iter()
        .find(|(id, _)| *id == provider_id)
        .map(|(_, url)| *url)
        .ok_or_else(|| {
            CommandError::new(
                "bank_unsupported",
                format!("o banco `{provider_id}` ainda não tem login conhecido"),
            )
        })
}

/// O que o app fez ao abrir o banco — a UI usa isto para guiar o utilizador.
#[derive(serde::Serialize)]
pub struct BankOpenReply {
    /// Perfil de Firefox do Luggom (separado do perfil pessoal).
    pub profile: String,
    /// A extensão coletora já está instalada nesse perfil?
    pub extension: bool,
}

/// Ficheiro da extensão — é o `gecko.id` do `coletor-browser/manifest.json`.
const EXTENSION_FILE: &str = "coletor@luggom.io.xpi";

/// Abre o login do banco no Firefox **num perfil do app**, onde a extensão
/// coletora vive instalada de forma permanente.
///
/// Porquê um perfil próprio: o Firefox só instala extensões de forma permanente
/// se estiverem **assinadas** (política dele). Com o `.xpi` assinado colocado em
/// `~/.luggom/`, o app instala-o no perfil e o utilizador nunca tem de mexer em
/// `about:debugging` — nem a extensão cai quando o Firefox reinicia.
#[tauri::command]
pub fn bank_open_firefox(app: AppHandle, provider_id: String) -> Result<BankOpenReply, CommandError> {
    let url = login_url(&provider_id)?;
    let profile = profile_dir()?;
    // O `.xpi` viaja como *resource* do app (bundle.resources no tauri.conf.json).
    let mut extra = Vec::new();
    if let Ok(res) = app.path().resource_dir() {
        extra.push(res);
    }
    let extension = install_extension(&profile, &extra);

    // Com a extensão instalada no perfil do app, tudo é automático. **Sem** ela,
    // abre-se o Firefox normal do utilizador — é lá que ele já tem a sessão do
    // banco (e, tipicamente, a extensão em modo temporário).
    open_firefox(extension.then_some(profile.as_path()), url)?;

    Ok(BankOpenReply {
        profile: profile.display().to_string(),
        extension,
    })
}

/// O comprovante que a captura recebida já identifica — ou `None` enquanto não
/// houver (a extensão ainda não enviou nada).
#[tauri::command]
pub fn bank_status(state: State<'_, AppState>) -> Result<Option<CaptureReply>, CommandError> {
    match state.bank_capture() {
        None => Ok(None),
        Some(raw) => Ok(capture_reply(&raw).ok()),
    }
}

/// Esquece a captura (para começar de novo, com outra sessão).
#[tauri::command]
pub fn bank_forget(state: State<'_, AppState>) -> Result<(), CommandError> {
    state.clear_bank_capture();
    Ok(())
}

/// Perfil de Firefox do app.
///
/// Em sistemas onde o Firefox é um **snap** (Ubuntu), o confinamento do snap
/// **não** deixa usar `~/.luggom/...` — o perfil tem de viver dentro de
/// `~/snap/firefox/common/`. Deteta-se o snap e escolhe-se o caminho certo.
fn profile_dir() -> Result<std::path::PathBuf, CommandError> {
    let home = std::env::var("HOME").map_err(|_| CommandError::internal("sem HOME"))?;
    let snap = std::path::Path::new("/snap/bin/firefox").exists()
        || std::path::Path::new("/snap/firefox/current").exists();
    let profile = if snap {
        std::path::Path::new(&home)
            .join("snap")
            .join("firefox")
            .join("common")
            .join("luggom-profile")
    } else {
        std::path::Path::new(&home).join(".luggom").join("firefox-profile")
    };
    std::fs::create_dir_all(profile.join("extensions"))
        .map_err(|e| CommandError::internal(format!("não consegui criar o perfil: {e}")))?;
    Ok(profile)
}

/// Instala a extensão coletora no perfil, se houver um `.xpi` para instalar.
/// Devolve `true` se a extensão ficou lá.
///
/// ⚠️ O Firefox **só aceita extensões assinadas** de forma permanente. O `.xpi`
/// tem de ser assinado uma vez (AMO, «unlisted») e colocado em `~/.luggom/` ou ao
/// lado do executável — com o nome `coletor@luggom.io.xpi`.
fn install_extension(profile: &std::path::Path, extra_dirs: &[std::path::PathBuf]) -> bool {
    let target = profile.join("extensions").join(EXTENSION_FILE);
    if target.exists() {
        return true;
    }

    let mut candidates: Vec<std::path::PathBuf> = Vec::new();
    // 1) Recursos empacotados com o app (o .xpi viaja no instalador).
    for dir in extra_dirs {
        candidates.push(dir.join(EXTENSION_FILE));
    }
    // 2) Override manual (útil em desenvolvimento).
    if let Ok(home) = std::env::var("HOME") {
        candidates.push(std::path::Path::new(&home).join(".luggom").join(EXTENSION_FILE));
    }
    // 3) Ao lado do executável.
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            candidates.push(dir.join(EXTENSION_FILE));
        }
    }

    for source in candidates {
        if source.exists() && std::fs::copy(&source, &target).is_ok() {
            println!(
                "[coletor] extensão instalada no perfil a partir de {}",
                source.display()
            );
            return true;
        }
    }
    false
}

#[cfg(target_os = "linux")]
fn open_firefox(profile: Option<&std::path::Path>, url: &str) -> Result<(), CommandError> {
    for browser in ["firefox", "firefox-esr"] {
        let mut command = std::process::Command::new(browser);
        if let Some(profile) = profile {
            // Perfil do app — isolado do Firefox pessoal do utilizador.
            command
                .arg(format!("-profile={}", profile.display()))
                .arg("-no-remote");
        }
        if command.arg(url).spawn().is_ok() {
            return Ok(());
        }
    }
    std::process::Command::new("xdg-open")
        .arg(url)
        .spawn()
        .map(|_| ())
        .map_err(|e| CommandError::internal(format!("não consegui abrir o browser: {e}")))
}

#[cfg(not(target_os = "linux"))]
fn open_firefox(_profile: Option<&std::path::Path>, _url: &str) -> Result<(), CommandError> {
    Err(CommandError::internal(
        "abrir o browser ainda só está implementado no Linux",
    ))
}

/// Fica à escuta da captura da extensão. Chamado **uma vez**, no arranque.
pub fn spawn_listener(app: AppHandle) {
    std::thread::spawn(move || {
        let listener = match TcpListener::bind(LISTEN_ADDR) {
            Ok(listener) => listener,
            Err(e) => {
                println!("[coletor] não consegui ouvir em {LISTEN_ADDR}: {e}");
                return;
            }
        };
        println!("[coletor] à espera da captura da extensão em http://{LISTEN_ADDR}/collect");
        for stream in listener.incoming() {
            let Ok(stream) = stream else { continue };
            let app = app.clone();
            std::thread::spawn(move || handle(stream, &app));
        }
    });
}

/// Uma ligação: lê o pedido, guarda a captura e responde. Deliberadamente
/// mínimo — é um POST local de uma extensão, não um servidor.
fn handle(mut stream: TcpStream, app: &AppHandle) {
    let _ = stream.set_read_timeout(Some(std::time::Duration::from_secs(20)));

    let Ok(peek) = stream.try_clone() else { return };
    let mut reader = BufReader::new(peek);

    let mut request_line = String::new();
    if reader.read_line(&mut request_line).unwrap_or(0) == 0 {
        return;
    }
    let mut parts = request_line.split_whitespace();
    let method = parts.next().unwrap_or("").to_uppercase();
    let path = parts.next().unwrap_or("").to_string();

    let mut content_length = 0usize;
    loop {
        let mut line = String::new();
        if reader.read_line(&mut line).unwrap_or(0) == 0 {
            break;
        }
        let trimmed = line.trim_end();
        if trimmed.is_empty() {
            break;
        }
        if let Some((name, value)) = trimmed.split_once(':') {
            if name.eq_ignore_ascii_case("content-length") {
                content_length = value.trim().parse().unwrap_or(0);
            }
        }
    }

    if method == "OPTIONS" {
        respond(&mut stream, "204 No Content", "");
        return;
    }
    if method != "POST" || path != "/collect" {
        respond(&mut stream, "404 Not Found", r#"{"error":"not found"}"#);
        return;
    }
    if content_length == 0 || content_length > MAX_CAPTURE_BYTES {
        respond(&mut stream, "413 Payload Too Large", r#"{"error":"tamanho"}"#);
        return;
    }

    let mut body = vec![0u8; content_length];
    if reader.read_exact(&mut body).is_err() {
        respond(&mut stream, "400 Bad Request", r#"{"error":"corpo truncado"}"#);
        return;
    }

    match String::from_utf8(body) {
        Ok(raw) if store(app, &raw) => respond(&mut stream, "200 OK", r#"{"ok":true}"#),
        _ => respond(&mut stream, "400 Bad Request", r#"{"error":"captura inválida"}"#),
    }
}

/// Valida e guarda a captura. Fail-closed: sem `reqs` não serve para nada.
fn store(app: &AppHandle, raw: &str) -> bool {
    let Ok(parsed) = serde_json::from_str::<serde_json::Value>(raw) else {
        return false;
    };
    if parsed.get("reqs").and_then(|v| v.as_array()).is_none() {
        return false;
    }
    write_trace(&parsed);
    let state = app.state::<AppState>();
    state.set_bank_capture(raw.to_string());
    println!("[coletor] captura recebida ({} bytes)", raw.len());
    true
}

fn respond(stream: &mut TcpStream, status: &str, body: &str) {
    let _ = write!(
        stream,
        "HTTP/1.1 {status}\r\n\
         Access-Control-Allow-Origin: *\r\n\
         Access-Control-Allow-Headers: *\r\n\
         Access-Control-Allow-Methods: POST, OPTIONS\r\n\
         Content-Type: application/json\r\n\
         Content-Length: {}\r\n\
         Connection: close\r\n\r\n{body}",
        body.len()
    );
    let _ = stream.flush();
}

/// Diagnóstico: resumo do que o banco pediu e do que recebeu — **só** método,
/// URL (sem query) e estado, nunca cookies nem corpos.
fn write_trace(doc: &serde_json::Value) {
    use std::fmt::Write as _;

    let clean = |raw: &str| -> String {
        let no_query = raw.split('?').next().unwrap_or(raw);
        if no_query.len() > 140 {
            format!("{}…", &no_query[..140])
        } else {
            no_query.to_string()
        }
    };

    let mut out = String::new();
    if let Some(reqs) = doc.get("reqs").and_then(|v| v.as_array()) {
        let _ = writeln!(out, "== PEDIDOS ({}) ==", reqs.len());
        for r in reqs {
            let method = r.get("method").and_then(|v| v.as_str()).unwrap_or("?");
            let url = r.get("url").and_then(|v| v.as_str()).unwrap_or("?");
            let _ = writeln!(out, "{method} {}", clean(url));
        }
    }
    if let Some(resps) = doc.get("resps").and_then(|v| v.as_array()) {
        let _ = writeln!(out, "\n== RESPOSTAS ({}) ==", resps.len());
        for r in resps {
            let status = r.get("status").and_then(|v| v.as_u64()).unwrap_or(0);
            let url = r.get("url").and_then(|v| v.as_str()).unwrap_or("?");
            let bytes = r.get("body").and_then(|v| v.as_str()).map(str::len).unwrap_or(0);
            let _ = writeln!(out, "{status} ({bytes} B) {}", clean(url));
        }
    }
    let _ = std::fs::write("/tmp/luggom-bank-trace.txt", out);
}

#[cfg(test)]
mod tests {
    use super::*;

    // T-APP / empacotamento: o .xpi viaja como *resource* do app e é copiado para
    // o perfil do Firefox na primeira abertura do banco.
    #[test]
    fn install_extension_copies_from_a_resource_dir() {
        let base = std::env::temp_dir().join(format!("luggom-collector-test-{}", std::process::id()));
        let res = base.join("resources");
        let prof = base.join("profile");
        std::fs::create_dir_all(&res).unwrap();
        std::fs::create_dir_all(prof.join("extensions")).unwrap();
        std::fs::write(res.join(EXTENSION_FILE), b"xpi-de-teste").unwrap();

        assert!(install_extension(&prof, &[res.clone()]), "devia instalar a partir do resource dir");
        assert!(prof.join("extensions").join(EXTENSION_FILE).exists());
        // Idempotente: a segunda chamada não falha nem duplica.
        assert!(install_extension(&prof, &[res]));
        // Sem candidato nenhum, não instala (fail-closed).
        let empty = base.join("vazio");
        assert!(!install_extension(&base.join("outro-perfil"), &[empty]));

        std::fs::remove_dir_all(&base).ok();
    }
}
