# Gentx for `luggom-test01`

This is the short checklist for the 12 external validators. The full reset guide
is [RESET-2026-11-17.en.md](RESET-2026-11-17.en.md). Release:
<https://github.com/Luggom/Luggom/releases/tag/testnet-2026-11-17>

## 1. Verify the artifacts

```sh
sha256sum luggomd genesis-draft.json
# luggomd            1d1d4866e53c96690e842b8cf1d7440e0d46ee825f018c623c05281f34ed3e3e
# genesis-draft.json 4926bdacc60f48d3511f622b72ff25be976f3cb6cc75ebdc0fa325c4ad84ce86
luggomd version --long --output json   # testnet-2026-11-17 / 7fa7dfc
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
**2026-11-16 02:00 UTC** (Telegram group or the address in the release).

Do **not** run `collect-gentxs`. Do **not** start the chain with the draft genesis.
The final genesis (16 gentxs) is published after the deadline.
