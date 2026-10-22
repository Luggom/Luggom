# Validadores — testnet Luggom (por convite)

> **Testnet.** Sem valor monetário. A chain pode ser resetada.
> **Por convite:** entra no grupo https://t.me/+XRWgwPwvz4hkOTJh e posta o teu endereço
> `luggom1…` + moniker. O operador financia **200 LUG** de testnet. O p2p `26656` está
> **aberto** no seed `1530baa7…@62.146.183.122:26656`; validadores continuam por convite.

## Pré-requisitos

- Linux 64-bit (Ubuntu 22.04/24.04), 2+ vCPU, 4 GB RAM, 50 GB SSD, IP fixo.
- Porta **26656/tcp** aberta (p2p). RPC/LCD **não** expostos à internet.
- `luggomd` e `genesis.json` da release da testnet.

## 1. Instalar

```sh
# verifica o checksum antes de instalar
sha256sum -c SHA256SUMS

sudo install -m 0755 luggomd /usr/local/bin/luggomd
luggomd version
```

## 2. Inicializar

```sh
luggomd init "<moniker>" --chain-id luggom-test --home "$HOME/.luggom"
cp genesis.json "$HOME/.luggom/config/genesis.json"
```

## 3. Seed e gás

```sh
# seed público da testnet (p2p aberto; validadores por convite)
sed -i 's|^seeds = .*|seeds = "1530baa7b3a07e0cab8dc81c9c08fccc51a47471@62.146.183.122:26656"|' \
  "$HOME/.luggom/config/config.toml"
sed -i 's|^minimum-gas-prices = .*|minimum-gas-prices = "1uluggom"|' \
  "$HOME/.luggom/config/app.toml"
```

## 4. Arrancar

```sh
luggomd start --home "$HOME/.luggom" --minimum-gas-prices 1uluggom
```

Em produção, usa systemd (exemplo mínimo):

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

## 5. Sincronizar

Espera até `catching_up=false` e a altura acompanhar a rede:

```sh
curl -s http://127.0.0.1:26657/status | python3 -m json.tool | grep -E "latest_block_height|catching_up"
```

## 6. Chave e funding

```sh
luggomd keys add validator --keyring-backend test --home "$HOME/.luggom"
luggomd keys show validator -a --keyring-backend test --home "$HOME/.luggom"
```

Posta o endereço no grupo. O operador envia **200 LUG + margem para a taxa**.
Confirma o saldo:

```sh
luggomd query bank balances <luggom1…> --node tcp://127.0.0.1:26657 --home "$HOME/.luggom"
```

## 7. Criar o validador

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

`200 LUG` é o **padrão da testnet** (o mesmo bond dos 4 validadores atuais). Fica em stake;
só sai com unbonding de **21 dias**.

## 8. Verificar

```sh
luggomd query staking validators --node tcp://127.0.0.1:26657 --home "$HOME/.luggom" | grep -A3 "<moniker>"
```

O validador deve aparecer como `BOND_STATUS_BONDED` e estar a assinar blocos.
Se ficares offline, perdes blocos e podes ser jailed (slashing de testnet).

## 9. Backup e segurança

- Guarda **offline** `~/.luggom/config/priv_validator_key.json`, `node_key.json` e o keyring.
- Nunca partilhes a seed nem a `priv_validator_key`.
- Firewall: só **26656** (p2p) pública; RPC/LCD fechados ou restritos.
- Monitoriza altura, `catching_up` e blocos assinados; alerta se parar.

## 10. Notas

- Testnet: sem valor; a chain pode ser resetada sem aviso.
- O operador pode remover/jailed validadores em caso de abuso ou má configuração.
- Dúvidas e convites: grupo https://t.me/+XRWgwPwvz4hkOTJh (inglês).
