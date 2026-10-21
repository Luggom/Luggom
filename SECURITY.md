# Security policy

## Supported versions

The project is in **public testnet** (`chain-id: luggom-test`). There is no mainnet and no funds at risk.

Security fixes are applied to the `main` branch of this repository.

## Reporting a vulnerability

**Do not open a public issue, pull request or social-media post for a vulnerability.**

Send a private report to **luggom@proton.me** with:

- a description of the issue and its impact;
- the affected component (chain, API, notary, desktop app, collector, site);
- the commit or version;
- reproduction steps, proof of concept or logs (sanitised);
- any suggested fix.

Please give us reasonable time to investigate and ship a fix before public disclosure.

## Scope

In scope:

- `x/luggom/` — mining rules, split, anti-replay, governance guards, oracle;
- `api/` and `cmd/luggom-api/` — proof verification, attestation, anti-fraud gate;
- the TLSNotary notary integration;
- `desktop/` — vault, signing, proof request, local key handling;
- `coletor-browser/` — receipt capture and the local collector;
- the website and downloads.

Out of scope:

- third-party services (banks, Cloudflare, VPS provider, Telegram, X);
- social engineering;
- denial of service against the shared testnet VPS (report it, but it is not a critical finding);
- issues that require a compromised device or a leaked seed.

## Testing rules

- Test only with your own accounts, receipts and wallets.
- Do not access, modify or exfiltrate other users' data.
- Do not publish real receipts, personal data or session cookies.
- Do not use the testnet to attack third parties.

## Our commitment

- We will acknowledge a report as soon as possible.
- We will keep you informed while the fix is developed.
- We will credit you in the fix, unless you prefer to stay anonymous.
- There is no bug bounty: the testnet has no monetary value.
