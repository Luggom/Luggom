# Luggom

**PT** — O Luggom é uma blockchain **Cosmos SDK** (`x/luggom`) onde se **minera LUG pagando imposto real**, provado por **TLSNotary** — sem KYC, sem terceiros.

**EN** — Luggom is a **Cosmos SDK** blockchain (`x/luggom`) where you **mine LUG by paying real taxes**, proven by **TLSNotary** — no KYC, no third parties.

## Links · Ligações

- Website / Site: [EN](site/index.html) · [PT](site/pt/index.html)
- Whitepaper: [EN](site/whitepaper.html) · [PT](site/pt/whitepaper.html)
- Developers: [EN](site/developers.html) · [PT](site/pt/developers.html) · [Endpoints](TESTNET/ENDPOINTS.en.md)
- Testnet: [`TESTNET/README.md`](TESTNET/README.md) · [EN](TESTNET/README.en.md)
- Roadmap / Roteiro: [`TESTNET/ROADMAP.md`](TESTNET/ROADMAP.md) · [EN](TESTNET/ROADMAP.en.md)
- Add a bank / Adicionar um banco: [`TESTNET/DEVELOPER_GUIDE.md`](TESTNET/DEVELOPER_GUIDE.md) · [EN](TESTNET/DEVELOPER_GUIDE.en.md)

## Status

**PT** — Hoje há **um banco** comprovado de ponta a ponta — o **Viacredi**, no Brasil.

**EN** — Today there is **one bank** proven end to end — **Viacredi**, in Brazil.

## Repo structure · Estrutura do repo

- `x/luggom/` — the core Cosmos SDK module / o módulo Cosmos SDK core.
- `api/` — the anti-fraud gate (receipt → attestation) / o portão anti-fraude.
- `desktop/` — the Tauri miner + wallet / o minerador + carteira Tauri.
- `coletor-browser/` — the Firefox collector extension / a extensão coletora do Firefox.
- `apk_luggom/native/tlsn-mobile/` — the Rust signer/prover crate used by the desktop wallet / o crate Rust de assinatura usado pela carteira desktop.
- `deps/cosmos-sdk/` — the vendored Cosmos SDK fork (export tally) / o fork do Cosmos SDK (export tally).
- `stubs/` — local Go stubs needed by the proto tooling / stubs Go locais do tooling de proto.
- `site/` — the public website (EN/PT) / o website público (EN/PT).
- `TESTNET/` — the public testnet package and endpoints (PT/EN) / o pacote público e endpoints da testnet (PT/EN).

## Build · Compilar

```sh
# Go 1.23.8
go build -ldflags="-checklinkname=0" -o build/luggomd ./cmd/luggomd

# Desktop (needs webkit2gtk and the Rust toolchain)
cd desktop && cargo build -p luggom-desktop
```

The repository vendors `deps/cosmos-sdk` and `stubs/`, so the Go build works as-is.
O repositório inclui `deps/cosmos-sdk` e `stubs/`, por isso o build Go funciona tal como está.

See [`TESTNET/README.md`](TESTNET/README.md) · [EN](TESTNET/README.en.md) for the full
run/validate/notarize/mine instructions.
Veja [`TESTNET/README.md`](TESTNET/README.md) · [EN](TESTNET/README.en.md) para as
instruções completas de rodar/validar/notariar/minerar.
