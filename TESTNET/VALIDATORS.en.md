# Validators — Luggom testnet (by invitation)

> **Upgrades:** the chain is upgraded by a published **halt height**, not by an ad-hoc restart. Read `TESTNET/UPGRADES.en.md` before you bond — one missed height means downtime, and downtime means jail.

> **Testnet.** No monetary value. The chain can be reset.
> **By invitation:** join https://t.me/+XRWgwPwvz4hkOTJh and post your `luggom1…` address
> and moniker. The operator funds **200 testnet LUG**. p2p `26656` is **open** on the seed
> `1530baa7…@62.146.183.122:26656`; validators remain by invitation.

## Prerequisites

- Linux 64-bit (Ubuntu 22.04/24.04), 2+ vCPU, 4 GB RAM, 50 GB SSD, static IP.
- Port **26656/tcp** open (p2p). Do **not** expose RPC/LCD to the internet.
- `luggomd` and `genesis.json` from the testnet release.

## 1. Install

```sh
sha256sum -c SHA256SUMS

sudo install -m 0755 luggomd /usr/local/bin/luggomd
luggomd version
```

## 2. Initialise

```sh
luggomd init "<moniker>" --chain-id luggom-test --home "$HOME/.luggom"
cp genesis.json "$HOME/.luggom/config/genesis.json"
```

## 3. Seed and gas

```sh
# public testnet seed (p2p open; validators by invitation)
sed -i 's|^seeds = .*|seeds = "1530baa7b3a07e0cab8dc81c9c08fccc51a47471@62.146.183.122:26656"|' \
  "$HOME/.luggom/config/config.toml"
sed -i 's|^minimum-gas-prices = .*|minimum-gas-prices = "1uluggom"|' \
  "$HOME/.luggom/config/app.toml"
```

## 4. Start

```sh
luggomd start --home "$HOME/.luggom" --minimum-gas-prices 1uluggom
```

For a service, use systemd:

```ini
[Unit]
Description=Luggom testnet validator
After=network-online.target

[Service]
User=luggom
ExecStart=/usr/local/bin/luggomd start --home /home/luggom/.luggom --minimum-gas-prices 1uluggom
Restart=on-failure
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

## 5. Sync

Wait until `catching_up=false` and the height tracks the network:

```sh
curl -s http://127.0.0.1:26657/status | python3 -m json.tool | grep -E "latest_block_height|catching_up"
```

## 6. Key and funding

```sh
luggomd keys add validator --keyring-backend test --home "$HOME/.luggom"
luggomd keys show validator -a --keyring-backend test --home "$HOME/.luggom"
```

Post the address in the group. The operator sends **200 LUG + a fee margin**.
Check the balance:

```sh
luggomd query bank balances <luggom1…> --node tcp://127.0.0.1:26657 --home "$HOME/.luggom"
```

## 7. Create the validator

```sh
PUBKEY=$(luggomd comet show-validator --home "$HOME/.luggom")

luggomd tx staking create-validator \
  --amount 200000000000000000000uluggom \
  --pubkey "$PUBKEY" \
  --moniker "<moniker>" \
  --chain-id luggom-test \
  --commission-rate 0.05 \
  --commission-max-rate 0.20 \
  --commission-max-change-rate 0.01 \
  --min-self-delegation 1 \
  --from validator --keyring-backend test --home "$HOME/.luggom" \
  --gas auto --gas-adjustment 1.3 --gas-prices 1uluggom \
  --node tcp://127.0.0.1:26657 -y
```

`200 LUG` is the **testnet standard** (the same bond as the four current validators). It
stays bonded; it only leaves after **21 days** of unbonding.

## 8. Verify

```sh
luggomd query staking validators --node tcp://127.0.0.1:26657 --home "$HOME/.luggom" | grep -A3 "<moniker>"
```

The validator should be `BOND_STATUS_BONDED` and signing blocks. If you go offline you miss
blocks and can be jailed (testnet slashing).

## 9. Backup and security

- Keep `~/.luggom/config/priv_validator_key.json`, `node_key.json` and the keyring **offline**.
- Never share the seed or the `priv_validator_key`.
- Firewall: only **26656** (p2p) public; RPC/LCD closed or restricted.
- Monitor height, `catching_up` and signed blocks; alert when it stops.

## 10. Notes

- Testnet: no value; the chain can be reset without notice.
- The operator may remove/jail validators for abuse or misconfiguration.
- Questions and invitations: https://t.me/+XRWgwPwvz4hkOTJh (English).

## Building from source

The network runs the source at tag **`testnet-2026-10-27`** (commit `298b619`).
Build with Go **1.23.8**, the version in `go.mod`:

```sh
git clone --branch testnet-2026-10-27 --depth 1 https://github.com/Luggom/Luggom
cd Luggom
go version        # must be go1.23.8 (or a toolchain that honours go.mod)
make install      # go install, with the version/commit ldflags
luggomd version --long --output json
```

The JSON must report `"version":"testnet-2026-10-27"` and `"commit":"298b619"`.
If it does, your binary was built from exactly the source the network runs.

`make install` also passes `-checklinkname=0` (required by Go 1.23 with sonic).
If you prefer to build by hand, reproduce the same ldflags:

```sh
go build -ldflags "-checklinkname=0 \
  -X github.com/cosmos/cosmos-sdk/version.Name=luggom \
  -X github.com/cosmos/cosmos-sdk/version.AppName=luggomd \
  -X github.com/cosmos/cosmos-sdk/version.Version=testnet-2026-10-27 \
  -X github.com/cosmos/cosmos-sdk/version.Commit=298b619" -o luggomd ./cmd/luggomd
```

The Cosmos SDK fork lives in `deps/cosmos-sdk` and is wired by a `replace` in
`go.mod`; it is pinned by the tag, so there is nothing else to fetch and no
uncommitted source involved.
