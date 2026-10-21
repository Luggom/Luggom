# Endpoints públicos da testnet — Luggom

**chain-id:** `luggom-test`
**Estado:** testnet pública. Pode ser resetada (saldos e histórico podem desaparecer). Nada aqui tem valor monetário.

## Endpoints

| Serviço | URL | Para que serve |
|---|---|---|
| RPC (CometBFT) | `https://rpc.luggom.com` | status, blocos, pesquisa de transações |
| LCD / REST | `https://lcd.luggom.com` | consultas Cosmos, saldos, broadcast |
| API | `https://api.luggom.com` | mineração, whitelist de bancos, tokenomics |
| Notário TLSNotary | `https://notary.luggom.com` | sessões de prova; WebSocket em `wss://notary.luggom.com/session` |

IP direto (fallback): `62.146.183.122` nas portas `26657` (RPC), `1317` (LCD), `8082` (API) e `7047` (notário).

O p2p (`26656`) está **fechado** ao público nesta fase. Validadores externos entram por convite.

## Verificação rápida

```sh
curl https://rpc.luggom.com/status
curl https://lcd.luggom.com/cosmos/base/tendermint/v1beta1/blocks/latest
curl https://lcd.luggom.com/cosmos/base/tendermint/v1beta1/validatorsets/latest
curl https://api.luggom.com/api/v1/chain/info
curl https://notary.luggom.com/health
```

Respostas esperadas: `chain_id = luggom-test`, altura a subir, **4 validadores**, `ok` no notário.

## Primeira mineração (on-chain)

- tx: `75EEF27E00190D2117E2DBA4388B694C34962467F56F77A57CC4114A61E5141D`
- altura: `1284`
- resultado: `code 0`
- R$ 1.291,49 → 21,09167035214527237 LUG

Consulta direta:

```sh
curl https://lcd.luggom.com/cosmos/tx/v1beta1/txs/75EEF27E00190D2117E2DBA4388B694C34962467F56F77A57CC4114A61E5141D
```

## Configurar a aplicação desktop

| Campo | Valor |
|---|---|
| Network → Node (LCD) | `https://lcd.luggom.com` |
| Minerar → API | `https://api.luggom.com` |
| Minerar → Notário | `https://notary.luggom.com` |

## Limites e avisos

- A API limita pedidos de mineração a **5 por hora por IP**.
- Um operador, quatro validadores no mesmo servidor: **sem SLA de uptime**.
- Não uses estes endpoints como backend de produção.
- A chave do notário **não** é pública; só o endpoint é.
