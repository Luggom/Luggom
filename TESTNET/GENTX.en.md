# Gentx for `luggom-test01`

This is the short checklist for the external validators (9 are in the final
genesis; NODESYNC, Sr20de and BonyNode join later). The full reset guide
is [RESET-2026-11-19.en.md](RESET-2026-11-19.en.md). Release:
<https://github.com/Luggom/Luggom/releases/tag/testnet-2026-11-19>

## 1. Verify the artifacts

```sh
sha256sum luggomd genesis-draft.json
# luggomd            3daae139402537890967ec6c67a3b9ab43b8d0e1d2a3911feaf4fd5f41eda7b2
# genesis-draft.json 4926bdacc60f48d3511f622b72ff25be976f3cb6cc75ebdc0fa325c4ad84ce86
luggomd version --long --output json   # testnet-2026-11-19 / 7fa7dfc
```

## 2. Use the draft genesis

```sh
cp ~/.luggom/config/genesis.json ~/.luggom/config/genesis.luggom-test.json.bak
cp genesis-draft.json ~/.luggom/config/genesis.json
```

Your account is already funded with **201 LUG** (200 bond + 1 fee) in the draft.

## 3. Create the gentx

```sh
luggomd genesis gentx <your-key> 200000000000000000000uluggom \
  --chain-id luggom-test01 \
  --home ~/.luggom \
  --keyring-backend <test|file|os> \
  --commission-rate 0.10 --commission-max-rate 0.20 --commission-max-change-rate 0.01
```

Use the **same `priv_validator_key.json`** you will run with. Do not regenerate it.
The gentx must use the consensus key of the node.

## 4. Check the key

```sh
luggomd comet show-validator --home ~/.luggom
```

Compare the consensus pubkey with the `pubkey` field in
`~/.luggom/config/gentx/gentx-*.json`. They must match.

## 5. Send the file

Send `~/.luggom/config/gentx/gentx-*.json` to the coordinator by
**2026-11-18 02:00 UTC** (Telegram group or the address in the release).

Do **not** run `collect-gentxs`. Do **not** start the chain with the draft genesis.
The final genesis (13 gentxs) is published in the release. NODESYNC, Sr20de and BonyNode are funded but
not in the genesis; after launch, sync and run `MsgCreateValidator` with 200 LUG +
gas.
