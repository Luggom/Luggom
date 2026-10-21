# Roadmap — Luggom Testnet

What is left to open the testnet, who does what, and what the community can take
on **now**. It is public: it works as the **launch checklist** for the Founder and
the **contribution map** for newcomers.

> **Updated:** 2026-10-08. Code state in `docs/CURRENT_STATE.md` and
> `checkpoint04.md` (internal).

---

## 1. The goal of the testnet

A **public** network where the community **validates, notarises, mines and adds
banks** without depending on the Founder. The code is ready and validated locally
(T20 with a real session → `code 0`); what is missing is **launching and
unblocking self-service**.

---

## 2. Launch checklist (Founder — in this order)

> ⚠️ Order matters. The notary key must be **in the genesis** — if you start the
> nodes first, you will have to reset again.

1. **Fix the notary key** (`LUGGOM_NOTARY_KEY_HEX`) — decide the real key.
2. **Generate the genesis** with the notary address:
   `./scripts/prepare-e2e-real.sh --chain-id luggom-test` (with the variable above).
3. **Start the validators** in `external` mode (never `local`):
   `./scripts/bootstrap-validators.sh external validators.txt --gentx-dir ./gentxs`.
4. **Start the notary** (`tlsn-verifier-server`, README §2).
5. **Start the API** (`./scripts/run-api.sh`) and the **oracle** (feeder + proxy).
6. **Expose on the VPS** — public endpoints (not `127.0.0.1`): RPC `:26657`,
   LCD `:1317`, API `:8082`, notary `:7047`.
7. **Sign the `.xpi` on AMO** (`./scripts/build-collector-xpi.sh --sign`).
8. **Host the installers** (`.deb` + `.AppImage` + `SHA256SUMS`) on the webpage.
9. **Publish on GitHub** — including the `docs/` that today live only in the
   tarball (`adapter-spec.md`, `CHECKLIST_FECHO_FASE7.md`, `matriz-vetores-T3.md`).
10. **Windows/macOS installers** (today Linux only; `tauri.windows.conf.json` ready).

---

## 3. What the community can take on now

| Contribution | How | Doc |
|---|---|---|
| **Run a validator** | create the account, produce the gentx, start the node | `README.md` §1 |
| **Be a notary** | run `tlsn-verifier-server` with the key | `README.md` §2 |
| **Mine** | install the desktop app and prove a payment | `README.md` §3 |
| **Add a bank** | deliver an evidence package (sanitised receipt + canonical field) | `DEVELOPER_GUIDE.md` |
| **Test Windows/macOS** | keychain + installers (only Linux validated) | — |
| **Teach mining (GoldHunters)** | onboarding people/companies | site/whitepaper |

**The highest-leverage contribution right now:** the **evidence packages** for
**US/CA** (and new BR banks) — the US/CA rails are off on purpose waiting for the
**per-rail canonical field**, and that is **real data**, not code.

---

## 4. What is NOT up to the community (needs code/decisions)

- **Item 153** — add a bank **without re-signing the app** (on-chain domain + dynamic collector). The biggest autonomy unlock.
- **API auth (A-2)** — no route has authentication; decision for the testnet.
- **Gov Reserve governance** — quorum (51 vs 67), withdrawal cap, `min_deposit` (proposal-spam risk).
- **Block 2** — t-of-n notaries + `dustReserveAddress` (distribute notarisation).

---

## 5. Current state (summary)

- **Code:** FASE 8 essentially closed (local hard test Blocks 1–4; A-8 and T-GOV-1
  closed). Chain reset done (2026-10-07).
- **Mineable E2E:** Viacredi (`code 0`). Caixa: rail **FAIL** (anti-bot). BB:
  pending session. US/CA: missing canonical field.
- **Left to open:** the §2 checklist.

