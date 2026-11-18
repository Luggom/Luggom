# Gentx para `luggom-test01`

Checklist curta para os validadores externos (9 estão no genesis final;
NODESYNC, Sr20de e BonyNode entram depois). O guia completo do reset é
[RESET-2026-11-19.md](RESET-2026-11-19.md). Release:
<https://github.com/Luggom/Luggom/releases/tag/testnet-2026-11-19>

## 1. Verificar os artefactos

```sh
sha256sum luggomd genesis-draft.json
# luggomd            3daae139402537890967ec6c67a3b9ab43b8d0e1d2a3911feaf4fd5f41eda7b2
# genesis-draft.json 4926bdacc60f48d3511f622b72ff25be976f3cb6cc75ebdc0fa325c4ad84ce86
luggomd version --long --output json   # testnet-2026-11-19 / 7fa7dfc
```

## 2. Usar o genesis draft

```sh
cp ~/.luggom/config/genesis.json ~/.luggom/config/genesis.luggom-test.json.bak
cp genesis-draft.json ~/.luggom/config/genesis.json
```

A tua conta já está fundada com **201 LUG** (200 bond + 1 fee) no draft.

## 3. Criar o gentx

```sh
luggomd genesis gentx <a-tua-chave> 200000000000000000000uluggom \
  --chain-id luggom-test01 \
  --home ~/.luggom \
  --keyring-backend <test|file|os> \
  --commission-rate 0.10 --commission-max-rate 0.20 --commission-max-change-rate 0.01
```

Usa o **mesmo `priv_validator_key.json`** com que vais correr. Não o regeneres.
O gentx tem de usar a chave de consenso do nó.

## 4. Confirmar a chave

```sh
luggomd comet show-validator --home ~/.luggom
```

Compara a pubkey de consenso com o campo `pubkey` em
`~/.luggom/config/gentx/gentx-*.json`. Têm de coincidir.

## 5. Enviar o ficheiro

Envia `~/.luggom/config/gentx/gentx-*.json` ao coordenador até
**2026-11-18 02:00 UTC** (grupo do Telegram ou endereço da release).

**Não** corras `collect-gentxs`. **Não** arranques a chain com o genesis draft. O
genesis final (13 gentxs) está publicado na release. NODESYNC, Sr20de e BonyNode
ficam fundados mas não estão no genesis; depois do arranque, sincronizam e usam
`MsgCreateValidator` com 200 LUG + gas.
