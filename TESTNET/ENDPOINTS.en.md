# Public testnet endpoints — Luggom

**chain-id:** `luggom-test`
**Status:** public testnet. It can be reset (balances and history may disappear). Nothing here has monetary value.

## Endpoints

| Service | URL | What it is for |
|---|---|---|
| RPC (CometBFT) | `https://rpc.luggom.com` | status, blocks, transaction search |
| LCD / REST | `https://lcd.luggom.com` | Cosmos queries, balances, broadcast |
| API | `https://api.luggom.com` | mining, bank whitelist, tokenomics |
| TLSNotary notary | `https://notary.luggom.com` | proof sessions; WebSocket at `wss://notary.luggom.com/session` |

Direct IP (fallback): `62.146.183.122` on ports `26657` (RPC), `1317` (LCD), `8082` (API) and `7047` (notary).

p2p (`26656`) is **closed** to the public at this stage. External validators join by invitation.

## Quick check

```sh
curl https://rpc.luggom.com/status
curl https://lcd.luggom.com/cosmos/base/tendermint/v1beta1/blocks/latest
curl https://lcd.luggom.com/cosmos/base/tendermint/v1beta1/validatorsets/latest
curl https://api.luggom.com/api/v1/chain/info
curl https://notary.luggom.com/health
```

Expected: `chain_id = luggom-test`, height rising, **4 validators**, `ok` from the notary.

## First mining (on-chain)

- tx: `75EEF27E00190D2117E2DBA4388B694C34962467F56F77A57CC4114A61E5141D`
- height: `1284`
- result: `code 0`
- R$ 1,291.49 → 21.09167035214527237 LUG

Direct query:

```sh
curl https://lcd.luggom.com/cosmos/tx/v1beta1/txs/75EEF27E00190D2117E2DBA4388B694C34962467F56F77A57CC4114A61E5141D
```

## Configure the desktop app

| Field | Value |
|---|---|
| Network → Node (LCD) | `https://lcd.luggom.com` |
| Mine → API | `https://api.luggom.com` |
| Mine → Notary | `https://notary.luggom.com` |

## Limits and warnings

- The API rate-limits mining requests to **5 per hour per IP**.
- One operator, four validators on the same host: **no uptime SLA**.
- Do not use these endpoints as a production backend.
- The notary **key** is not public; only the endpoint is.
