# Notary source snapshot

These two files are the Luggom-specific notary, taken from our fork of
[`tlsnotary/tlsn-extension`](https://github.com/tlsnotary/tlsn-extension)
(`Luggom/tlsn-verifier`, commit `825983b`).

- `attestation.rs` — extracts the canonical payload and signs it with the
  notary's secp256k1 key (compact 65-byte signature, `recid||r||s`).
- `main.rs` — the verifier server: session registration, `/verifier`,
  `/proxy` and `/attestation`.

They are published **for review**, not as a buildable crate: `main.rs`
imports `crate::verifier` and `crate::ws`, which are upstream files not
included here. The rest of the fork is upstream code.

## What is signed

The domain is `luggom-attestation/v4`. The payload is one field per line:

```
domain
miner_address
chain_id
proof_commitment
payment_id
payment_amount
tax_entity
currency_code
payment_date
tax_nature_hash
payer_tax_id
provider_id
```

`attestation.rs` does **not** build this payload — the shared Go extractor
(`cmd/luggom-extract` in this repository) does, and the chain rebuilds it
from the message (`x/luggom/keeper/attestation_verify.go`). The notary only
signs the string it is given.

`miner_address` comes from the wallet at session registration
(`sessionData.miner_address`); `chain_id` comes from `LUGGOM_CHAIN_ID`. If
either is missing, no attestation is stored and the API refuses the mining.

## License

The upstream `tlsn-extension` is MIT-licensed; these modifications are
released under the same terms. Copyright (c) the TLSNotary contributors.
Luggom modifications by Lugo Vane.
