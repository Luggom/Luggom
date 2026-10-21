# Contributing to Luggom

Thanks for helping. Luggom is a Cosmos SDK chain where you mine LUG by paying real taxes, proven end to end with TLSNotary.

The public testnet is live (`luggom-test`). Nothing on it has monetary value, and it can be reset at any time.

## Where to talk

- **Bugs:** open a GitHub issue with the **bug report** template.
- **Ideas:** open a GitHub issue with the **feature request** template.
- **Security:** do **not** open a public issue — follow [`SECURITY.md`](SECURITY.md).
- **Chat:** [Telegram group](https://t.me/+XRWgwPwvz4hkOTJh) (English).

## Repository layout

- `x/luggom/` — the Cosmos SDK module (mining, split, reserves, governance rules).
- `api/` — the anti-fraud gate (receipt → attestation).
- `desktop/` — the Tauri miner and wallet.
- `coletor-browser/` — the Firefox collector extension.
- `apk_luggom/native/tlsn-mobile/` — the Rust signer/prover crate used by the wallet.
- `deps/cosmos-sdk/` — the vendored Cosmos SDK fork (export tally).
- `stubs/` — local Go stubs used by the proto tooling.
- `site/` — the website (EN at the root, PT in `/pt/`).
- `TESTNET/` — the operator guide, endpoints and developer guide.

## Build

Go 1.23.8:

```sh
go build -ldflags="-checklinkname=0" -o build/luggomd ./cmd/luggomd
go build -o build/luggom-api ./cmd/luggom-api
go build -o build/luggom-extract ./cmd/luggom-extract
go build -o build/luggom-oracle-feeder ./cmd/luggom-oracle-feeder
go build -o build/sandbox-bank ./cmd/sandbox-bank
```

`go build ./...` also walks `cosmos_sdk_fork_patches/` (standalone patch files, not packages); build the commands explicitly.

Desktop (needs `webkit2gtk` and the Rust toolchain):

```sh
cd desktop && cargo build -p luggom-desktop
```

The website is static; serve `site/` with any HTTP server.

## Tests

```sh
go test ./x/luggom/... ./api/... ./cmd/luggom-api/... ./app/... ./cmd/sandbox-bank/...
cargo test -p luggom-core -p luggom-wallet
cd desktop/ui && npm run build
```

Integration tests that need a real bank bundle skip when the bundle is absent.

## Reporting a bug

Use the bug template and include:

- the commit or version;
- the component (chain, API, notary, desktop, collector, site);
- the environment (OS, Go/Rust/Node versions, Firefox version);
- `chain-id` and block height;
- the endpoint or UI path;
- exact steps to reproduce;
- expected vs actual;
- sanitised logs.

**Never paste** seeds, private keys, cookies, full bank receipts or personal data. For a bank receipt, redact the payer and keep only the canonical field needed to reproduce the issue.

## Proposing a bank or rail

A new bank goes live by evidence, not by vote. Read [`TESTNET/DEVELOPER_GUIDE.en.md`](TESTNET/DEVELOPER_GUIDE.en.md) and deliver:

- a **sanitised real receipt** from that bank;
- the **canonical payment field** (e.g. PIX E2E in Brazil);
- the extractor rules and a test fixture.

## Pull requests

- Keep PRs small and focused; one concern per PR.
- Run the tests above before opening.
- Do not commit keys, `.env` files, keyrings, cookies, real receipts or build artifacts.
- By contributing you agree that your contribution is licensed under **Apache-2.0**.

## Rules

1. English in issues and PRs.
2. No price talk, speculation or financial advice. Testnet LUG has no value.
3. No spam, referrals or unsolicited DMs.
4. Be civil. This is an anti-censorship project.
