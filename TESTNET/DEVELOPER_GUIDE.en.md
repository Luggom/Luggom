# Developer Guide — Luggom Testnet

How the community turns a **bank / tax portal** into a **mineable rail**. This is
the public version of what currently lives in `docs/adapter-spec.md`.

> **Security:** this folder is public. **Never** commit keys, seeds, keyrings,
> session cookies or real receipts (PII) here. Receipts enter **masked**.

---

## 1. Who does what

| Part | Who does it | Is it code? |
|---|---|---|
| **Evidence** — sanitised real receipt + the rail's canonical field | **community** | no |
| **Probe** — does the rail answer an independent TLS client? | **community** | no (a *property* of the bank's infra) |
| **Wire it to the protocol** — whitelist, host check, request builder, plugin | **core/governance** | yes |

**Practical consequence:** a bank does **not** need to run our code. It needs to
**serve the receipt in a provable way** — which is what this guide validates.

---

## 2. Rule #1 — anti-bot (the Caixa lesson)

The receipt endpoint **must answer an independent TLS client** that presents the
session cookie — it **must not require a device token minted by JavaScript and
bound to the browser**. TLSNotary terminates TLS **outside** the browser, so
imitating a browser is not enough: it has to be *the* browser.

The measurement that closed Caixa (same cookie, same session, exact headers):

| Client | Result |
|---|---|
| Firefox **154** (the real browser) | **200** |
| Firefox 147/148 (impersonation) | 302 |
| Chrome 150 (current) | 302 |
| Safari 260 | 302 |

Such a portal **is not adapter-able**. **Probe before any code** (Step 1).

---

## 3. Full flow (summary)

```
0. Evidence  →  1. Probe  →  2. Code  →  3. Governance + release
```

---

## Step 0 — Evidence package (prerequisite)

This is what **the person who actually mined** delivers. Without it, no code.

1. **Capture a real session** — with the extension (`coletor-browser/`), on the
   target bank, make the payment as usual. The extension records requests/responses
   + the cookie jar (including `httpOnly`).
2. **Answer the 6 questions**:
   1. the **beneficiary** nickname (the tax entity);
   2. is the endpoint **reflective**? (replays the query without side effects);
   3. is the attested state **"paid"**?;
   4. what is the **canonical `payment_id`** (unique per payment)?
   5. is the **amount paid** **labelled**? (exact format);
   6. is the **date** (with time) labelled?
3. **Sanitise the receipt** — mask PII (e.g. `**.392.130/0001-**`).
   **Hard rule: never commit a real receipt.**

### The canonical field (what works and what does NOT)

The `payment_id` must be: **unique per payment**, **only one** in the receipt, and
in a **labelled field** (not in prose). A `BN`/`EIN`/`APN`/`RT` belongs to the
**account** and repeats between payments — it would block the 2nd payment as a
replay, or give a selectable id. Same mistake as a PIX key (§13.133).

| Rail | Canonical field (what to reveal) | Does NOT work |
|---|---|---|
| **BR — PIX** | **PIX E2E** (32 chars: `E`+ISPB+AAAAMMDD+HHMM+11) | PIX key (belongs to the holder) |
| **US — IRS** (Direct Pay / EFTPS) | **confirmation number** | SSN/EIN, `tax year`, form number |
| **US — county/state** | portal **receipt/confirmation #** | APN/parcel, lot, `tax year` |
| **CA — CRA** | **confirmation number** | BN (9 digits), RT0001 |
| **CA — municipal** | **receipt #** | roll/assessment, tax year |

> **US/CA are off on purpose** until this field exists per rail. The code is ready
> to receive it; what is missing is the **real evidence**.

### How the recipient is identified (BR → US/CA)

Brazil is the **tested and closed** model: on the collection PIX the receipt
carries the recipient's **CNPJ** (the PIX key is bound to the document), so the
extractor reads the CNPJ and confirms it is on the on-chain whitelist. It is a
strong identifier — there is no ambiguity.

In the US and Canada receipts usually do **not** carry the government's EIN/BN:
the "account number" shown belongs to the taxpayer (parcel/APN/roll/folio). The
recipient is therefore identified two ways, strongest first.

> **Goal (spec): get as close to the BR model as possible.** In Brazil the receipt
> carries the **government's CNPJ** — a strong identifier, tested and hardened
> against fraud. When bringing a real US/CA receipt, **choose the portal/rail that
> exposes the recipient's EIN/BN**. The closer the rail is to BR (tax ID on the
> receipt), the safer it is and the less it depends on the name guards. The name
> fallback is acceptable, but it is the **weakest** path — use it only when no
> portal on the rail shows the ID.

**1. Tax ID, when the receipt shows it (always prefer it):**
- **US** — the recipient's **EIN** (property tax / IRS).
- **Canada — CRA (federal)** — **BN (9 digits) + RT0001**: here it is **like
  Brazil** — the receipt carries the tax authority's identifier, no ambiguity.
- **Canada — municipal** — the recipient's **BN**, when present.

**2. NAME resolution, when the ID is absent (fallback):**
- **US** — county property tax: "Yavapai County Treasurer".
- **Canada — municipal** — "City of Vancouver".
- The **beneficiary** name (never the body prose) resolves against the whitelist.

**Anti-fraud guards (already in code, mandatory so the BR model is not inverted):**

- the name/EIN comes only from the **beneficiary** field
  (`Recipient`/`RecipientName`), never from the body text — otherwise a page that
  merely *mentions* the tax authority would mine without a real payment;
- resolves **only when unambiguous** — "Orange County" with no state does **not**
  resolve;
- **cross-state blocked** — "Suffolk County NY" cannot credit `suffolk_ma`;
- **utilities/services rejected** — "… Water District" is not tax;
- requires **specific fiscal evidence** (property tax / parcel / APN / tax year /
  IRS) — the words "county" or "tax" alone are not enough;
- the **EIN is never the payer's** (only the recipient's);
- **US states have no single EIN** → they are identified **by name**.

> **Golden rule for whoever brings the real receipt:** use the **tax ID (EIN/BN)
> whenever the bank shows it**. The name fallback exists only because many portals
> do not show the ID — and, for that very reason, it is the path that needs the
> guards above the most.

---

## Step 1 — Probe the rail

**In this order — Caixa taught us that reversing it is expensive** (the adapter was
built before knowing whether the rail passed; it did not).

```sh
python3 coletor-browser/verify_rail.py --banco <id>
```

| Result | Meaning | Action |
|---|---|---|
| **PASS** (200/400) | the rail answers an independent client | proceed to Step 2 |
| **FAIL** (302 `sessaoEncerradaNovo`) | the WAF requires the real browser | **STOP** — do not build the adapter |
| other | read the body | retry with a fresh capture before deciding |

> If the capture is **older than 5 min**, a FAIL may just be a dead session — retry
> with a fresh one before giving up.

---

## Step 2 — Code checklist (9 points + 1 test)

| # | File | What |
|---|---|---|
| 1 | `api/reclaimverify/providers.go` (`KnownProviders`) | `{ID, Name, Country, Domain}`. **Without `Domain`, the host check is fail-closed** |
| 2 | `x/luggom/types/operational_lists.go` (`DefaultBankProviders`) | the ID in the **on-chain** whitelist (gov 67%). **Without it, `/mine/tax` returns 403/1208** |
| 3 | `api/taxnature/taxnature.go` (`Country`) | the ID in the country `switch`. **Without it, `VerifyTlsnProof` aborts** |
| 4 | `coletor-browser/make_prover_request_<bank>.py` | the **request builder** (template: **Caixa** if cookie-only; **Viacredi** if ECDSA/RSA signature) |
| 5 | `coletor-browser/manifest.json` | the domain in `host_permissions` **and** `content_scripts.matches` |
| 6 | `tlsn-verifier/packages/plugins/src/luggom_tax_<bank>.plugin.ts` + `registry.ts` | the desktop miner plugin (today each bank is a plugin) |
| 7 | `api/taxproof/taxproof.go` | **only if** the receipt does not match the generic BR heuristics — here goes the **per-rail canonical regex** (US/CA) |
| 8 | `api/taxproof/taxproof_test.go` | receipt-format test (template: Viacredi/Caixa) |
| 9 | `cmd/sandbox-bank` | the sandbox format, to develop **without a real account** |

**Extra:** `api/taxproof/providers_sync_test.go` forces (1) and (2) not to diverge.

---

## Step 3 — Governance + release

- **On-chain whitelist:** the slug enters via `MsgAddBankProvider` (**governance**),
  not by an individual dev. In the testnet cohort, it enters via `DefaultBankProviders`.
- **Re-sign the `.xpi`** (the `manifest.json` changed) and **re-release the app**
  (`.deb`/`.AppImage` + `SHA256SUMS`).
- **Rebuild the API** (`providers.go`/`taxproof.go`/`taxnature.go` changed).

> **Future work (testnet item):** make the domain **on-chain data** instead of code
> (`bank_providers` stores the domain; the API reads it on-chain; the collector uses
> broad `host_permissions` + dynamic `scripting.executeScript`). This removes the
> **re-sign** per new bank — but does **not** remove the evidence or the extractor.

---

## 5. Technical limits

| Limit | Value | Why |
|---|---|---|
| Request size | ≤ **4096 bytes** | TLSNotary prover cap (`maxSentData`) |
| Response compression | **no** | the extractor reads the body in clear |
| Endpoint | **same-origin**, JSON preferred | that is what the host check validates |
| Response | **2xx** | any other code is never a receipt |

## 6. What a bank does NOT solve

- **Direct debit / auto-pay is NOT mineable.** The receipt attests the
  **intermediary** (the bank's collection), not the **entity** (the Treasury). The
  tax must be **paid manually** (form/boleto, or PIX-arrecadação with the form QR).
- **Paying from an account that is not the holder's** does not help: the receipt
  must come from the user's own session.

## 7. Privacy (the two phases)

- **Production:** *selective disclosure* — only the receipt fields (including the
  canonical id) are revealed; the rest is **committed**, not revealed.
- **Development:** the capture **contains PII**. Before it becomes a fixture/spec
  it must be **masked**. **Never** publish a real receipt.

## 8. How to validate (before closing)

```sh
export PATH="$PWD/.go1.23.8/bin:$PATH"

# API + chain
go test ./api/taxproof/... ./api/taxnature/... ./api/reclaimverify/... ./x/luggom/...

# desktop (prover + wallet)
cargo test -p luggom-core -p luggom-wallet && cargo clippy --all-targets

# probe the new rail
python3 coletor-browser/verify_rail.py --banco <id>
```

## 9. Bank status (today)

| Bank | Whitelist | Request builder | Plugin | Rail | Real E2E |
|---|---|---|---|---|---|
| **Viacredi/Ailos** | ✅ | ✅ | ✅ | ✅ | ✅ **mined `code 0`** |
| **Caixa** | ✅ | ✅ | ✅ | ❌ **FAIL (anti-bot)** | ❌ |
| **BB** | ✅ | 🟡 sandbox | 🟡 | pending | ❌ |
| Sandbox (`localhost:9000`) | — | ✅ | ✅ | ✅ | ✅ |
| **US/CA** (chase, rbc, …) | ✅ on-chain | — | — | — | ❌ missing canonical field |

