# Item 153 — Bank domain on-chain (self-service)

> Goal: adding a bank = on-chain governance. After **ONE** `.xpi` re-sign, you never
> re-sign/rebuild again to include a bank.

## Current state (3 hardcoded places)

1. `x/luggom/types/operational_lists.go` → `DefaultBankProviders()` = `[]string` (IDs/slugs only).
2. `api/reclaimverify/providers.go` → `KnownProviders()` = slug→domain map, hardcoded in Go.
3. `coletor-browser/manifest.json` → `host_permissions` = Caixa/Viacredi/BB only.

## A. Proto (domain becomes on-chain data)

- `proto/luggom/luggom/v1/tx.proto` (lines 344-350): `MsgAddBankProvider` gains `string domain`
  (and optional `name`, `country`).
- `proto/luggom/luggom/v1/params.proto`: `bank_providers` stops being `repeated string` and becomes
  `repeated BankProvider { string provider_id; string domain; string name; string country; }`.
- Regenerate: `./scripts/regenerate_protos.sh` (protoc 25.3).

## B. Chain (keeper + types)

- `x/luggom/types/operational_lists.go`: `DefaultBankProviders()` returns the structured list
  (mirroring what is currently in `KnownProviders()`).
- `x/luggom/keeper/msg_operational_lists.go` (`AddBankProvider`): validate non-empty `domain`
  (fail-closed) and store id+domain. `RemoveBankProvider` stays the same.

## C. API + extractor (read on-chain)

- `api/reclaimverify/providers.go`: `KnownProviders()` stops being the source; domain resolution
  reads the on-chain list (LCD REST `:1317`) with a short cache.
- `cmd/luggom-extract/main.go` (`resolveProvider`): uses the on-chain list instead of `KnownProviders()`.
- ⚠️ **Design decision:** the extractor is currently a pure binary (doesn't talk to the chain).
  It will need a chain client (REST query) or to receive the list from the notary/API as input.
  Recommended: the extractor queries the LCD with a cache.

## D. Collector (dynamic whitelist)

- `coletor-browser/manifest.json`: `host_permissions` → `["<all_urls>"]`; `content_scripts.matches` → `<all_urls>`.
- `coletor-browser/background.js`: fetch the on-chain whitelist (via API) and **only** collect/send
  for domains that are on it.
- `coletor-browser/content.js`: the hook is already generic; confirm dynamic injection
  (`scripting.executeScript` on the whitelist).

## E. Security (important)

- The **domain becomes governable** ⇒ a bank must enter **by evidence**
  (sanitized real receipt + canonical field + validated extractor), not by a loose vote.
  This must become a rule in the governance flow.
- `<all_urls>` **attracts AMO scrutiny** — document the justification in the submission
  (which is why the re-sign is done **once**, deliberately).

## Suggested sequence

```
A (proto + regen) → B (chain) → C (API/extractor) → D (collector, 1 re-sign) → tests + evidence
```

## Expected result

After **D**: adding a US/Canada bank = devs deliver evidence → governance approves
(`AddBankProvider` with id+domain) → the domain goes on-chain → the collector (already `<all_urls>`)
and the extractor accept it. **Zero re-sign, zero rebuild, zero dependence on the Founder.**
