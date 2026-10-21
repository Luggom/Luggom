# Desktop Luggom (Bloco 5 — Tauri)

Minerador + carteira com a seed. Workspace Rust, **fora** do APK.

## Estrutura

| Crate | Papel | Precisa de `webkit2gtk`? |
|---|---|---|
| `core/` — `luggom-core` | **Cofre da seed** (BIP39 + Argon2id + XChaCha20-Poly1305, backup/restauro, PIN). Rust puro. | **Não** |
| `wallet/` — `luggom-wallet` | Derivação e assinatura, a chamar o **assinador real** do APK (`apk_luggom/native/tlsn-mobile`). | **Não** |
| `src-tauri/` | Comandos Tauri (a ponte para a UI). | **Sim** |
| `ui/` | Frontend (Vite + TypeScript). | — |

Só o crate `tauri` precisa das bibliotecas de sistema. Por isso `core` e `wallet`
testam-se numa máquina sem elas.

## Testar (não precisa de nada de sistema)

```bash
cd desktop
cargo test -p luggom-core -p luggom-wallet
```

## Preparar o ambiente para o `src-tauri` (Linux)

```bash
sudo apt-get install -y pkg-config libwebkit2gtk-4.1-dev libgtk-3-dev \
    librsvg2-dev libayatana-appindicator3-dev
```

## Regras de ouro

- A **seed nunca** é escrita em claro no disco nem aparece em `Debug`/logs.
- **Não se reimplementa criptografia.** A derivação e a assinatura vêm do
  assinador Rust real do APK — há um KAT contra o `luggomd`
  (`luggom19rl4cm2hmr8afy4kldpxz3fka4jguq0amgkgeu`) a guardar isso.
- A seed nunca sai do processo: só a assinatura é enviada.
