#!/usr/bin/env bash
# package-vps.sh — empacota o deploy da testnet para a VPS.
#
# Produz um tarball com: genesis + keyrings/consensus dos 4 validadores +
# binários + scripts + templates systemd/nginx/firewall + DEPLOY.md.
#
# ⚠️ A CHAVE PRIVADA DO NOTÁRIO (LUGGOM_NOTARY_KEY_HEX) NÃO é empacotada:
#    está em cold storage. O operador tem de a exportar NA VPS antes de subir
#    o notário (ver DEPLOY.md dentro do tarball).
#
# Uso: ./scripts/package-vps.sh [--out DIR] [--home DIR]
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$PWD"

HOME_DIR="${HOME_DIR:-$HOME/.luggom}"
OUT_DIR="${OUT_DIR:-$ROOT/checkpoints/backups}"
STAMP="$(date +%Y%m%d-%H%M%S)"
PKG="luggom-vps-$STAMP"
STAGE="$ROOT/build/vps-package"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out) OUT_DIR="$2"; shift 2;;
    --home) HOME_DIR="$2"; shift 2;;
    *) echo "opção desconhecida: $1" >&2; exit 1;;
  esac
done

echo "=== Luggom — empacotamento VPS ($PKG) ==="

# 1. Pré-requisitos
for b in build/luggomd build/luggom-api build/luggom-extract build/luggom-oracle-feeder \
         tlsn-verifier/servers/target/release/tlsn-verifier-server; do
  [[ -x "$b" ]] || { echo "❌ binário em falta: $b (faz o build antes)"; exit 1; }
done
GEN="$HOME_DIR/config/genesis.json"
[[ -f "$GEN" ]] || { echo "❌ genesis em falta: $GEN"; exit 1; }

rm -rf "$STAGE"
mkdir -p "$STAGE/$PKG/build" "$STAGE/$PKG/scripts" "$STAGE/$PKG/deploy/systemd" \
         "$STAGE/$PKG/deploy/nginx" "$STAGE/$PKG/notary" "$STAGE/$PKG/home"

# 2. Binários
cp build/luggomd build/luggom-api build/luggom-extract build/luggom-oracle-feeder "$STAGE/$PKG/build/"
cp tlsn-verifier/servers/target/release/tlsn-verifier-server "$STAGE/$PKG/build/"

# 3. Scripts
cp scripts/run-local-testnet.sh scripts/run-api.sh scripts/oracle-local-proxy.py \
   scripts/oracle-local-sources.json "$STAGE/$PKG/scripts/"
chmod +x "$STAGE/$PKG/scripts/"*.sh

# 4. Config do notário (webhooks vazios; o servidor lê config.yaml do CWD)
printf 'webhooks: {}\n' > "$STAGE/$PKG/notary/config.yaml"

# 5. Validadores — fundador = home/, val-0/1/2 = home/validators/val-*
#    NÃO copia data/ (a VPS arranca limpa do genesis) nem logs/meta.
copy_val() {
  local name="$1" src="$2" rel="$3"
  local dst="$STAGE/$PKG/home/$rel"
  mkdir -p "$dst/keyring-test" "$dst/config"
  cp -a "$src/keyring-test/." "$dst/keyring-test/"
  for f in priv_validator_key.json node_key.json config.toml app.toml client.toml; do
    [[ -f "$src/config/$f" ]] && cp "$src/config/$f" "$dst/config/"
  done
  cp "$GEN" "$dst/config/genesis.json"
  echo "  validador $name ✓"
}
copy_val fundador "$HOME_DIR" ""
copy_val val-0 "$HOME_DIR/validators/val-0" "validators/val-0"
copy_val val-1 "$HOME_DIR/validators/val-1" "validators/val-1"
copy_val val-2 "$HOME_DIR/validators/val-2" "validators/val-2"

# 6. Site (webpage) — nginx serve esta pasta
if [[ -d "$ROOT/site" ]]; then
  mkdir -p "$STAGE/$PKG/site"
  cp -a "$ROOT/site/." "$STAGE/$PKG/site/"
  echo "  site ✓"
fi

# 7. Instaladores desktop (se já tiverem sido buildados)
mkdir -p "$STAGE/$PKG/downloads"
found_installer=0
while IFS= read -r f; do
  cp "$f" "$STAGE/$PKG/downloads/"
  found_installer=1
done < <(find "$ROOT/desktop" \( -name '*.deb' -o -name '*.AppImage' -o -name 'SHA256SUMS' \) -type f 2>/dev/null || true)
[[ "$found_installer" == "1" ]] && echo "  instaladores ✓" || echo "  ⚠️  sem .deb/.AppImage/SHA256SUMS — buildar antes (tauri build)"

# 8. Templates de deploy
cat > "$STAGE/$PKG/deploy/DEPLOY.md" <<'MD'
# Deploy VPS — Testnet Luggom

## 0. Pré-requisitos

- VPS com IP público fixo e acesso SSH.
- **A chave privada do notário (`LUGGOM_NOTARY_KEY_HEX`) NÃO vem neste pacote** —
  está em cold storage. Exportá-la manualmente na VPS (passo 3).
- Confirmar o `chain-id` (o genesis atual usa `luggom-test`).

## 1. Extrair e instalar

```sh
sudo mkdir -p /opt/luggom
sudo tar -xzf luggom-vps-*.tar.gz -C /opt/luggom --strip-components=1
cd /opt/luggom
```

## 2. Ligar os endpoints públicos (RPC/LCD/gRPC)

O runner local liga RPC/LCD/gRPC a `127.0.0.1`. Na VPS, tornar público:

```sh
./deploy/patch-public-bind.sh
```

## 3. Configurar a chave do notário (cold storage)

```sh
sudo tee /opt/luggom/.env >/dev/null <<'ENV'
# A hex cujo endereço é luggom1ylugc8zzgtcltpd6a2a90ldnnjl828eznwdtu9
LUGGOM_NOTARY_KEY_HEX=<COLE-A-HEX-AQUI>
ENV
sudo chmod 600 /opt/luggom/.env
```

## 4. Firewall

```sh
sudo ./deploy/setup-firewall.sh
```

## 5. Subir os serviços

```sh
sudo cp deploy/systemd/*.service /etc/systemd/system/
sudo cp deploy/systemd/*.timer /etc/systemd/system/ 2>/dev/null || true
sudo systemctl daemon-reload
sudo systemctl enable --now luggom-chain luggom-notary luggom-api luggom-oracle-proxy
sudo systemctl enable --now luggom-oracle-feeder.timer
```

## 6. Validar (de fora)

```sh
curl -s http://<IP>:26657/status | python3 -m json.tool | head
curl -s http://<IP>:1317/cosmos/base/tendermint/v1beta1/node_info
curl -s http://<IP>:8082/api/v1/chain/info
curl -s http://<IP>:7047/health
```

A altura tem de subir nos 4 nós (`run-local-testnet.sh status`).

## 7. Webpage + binários (nginx)

```sh
sudo cp deploy/nginx/luggom.com.conf /etc/nginx/sites-available/
sudo ln -sf ../sites-available/luggom.com.conf /etc/nginx/sites-enabled/
sudo nginx -t && sudo systemctl reload nginx
```

O site fica em `/opt/luggom/site`; os instaladores em `/opt/luggom/downloads`.
TLS: `certbot --nginx -d luggom.com -d www.luggom.com`.

## Pendências a confirmar

- `LUGGOM_DATA_PROVIDER` — o hard test usou `mock`; confirmar o valor para testnet.
- `chain-id` público (hoje `luggom-test`).
- p2p `:26656` para validadores externos (o runner usa `0.0.0.0` no p2p; abrir se
  a testnet aceitar validadores de fora).
MD

cat > "$STAGE/$PKG/deploy/patch-public-bind.sh" <<'SH'
#!/usr/bin/env bash
# Liga RPC/LCD/gRPC do runner da chain a 0.0.0.0 (públicos), em vez de 127.0.0.1.
set -euo pipefail
SCRIPT="/opt/luggom/scripts/run-local-testnet.sh"
[[ -f "$SCRIPT" ]] || { echo "❌ $SCRIPT não existe"; exit 1; }
sed -i \
  -e 's|tcp://127\.0\.0\.1:%s|tcp://0.0.0.0:%s|g' \
  -e 's|"127\.0\.0\.1:%s"|"0.0.0.0:%s"|g' \
  "$SCRIPT"
echo "✅ $SCRIPT → RPC/LCD/gRPC em 0.0.0.0"
SH

cat > "$STAGE/$PKG/deploy/setup-firewall.sh" <<'SH'
#!/usr/bin/env bash
# Firewall da VPS — testnet Luggom (ufw).
set -euo pipefail
sudo ufw allow OpenSSH
sudo ufw allow 26657/tcp comment 'Luggom RPC'
sudo ufw allow 1317/tcp  comment 'Luggom LCD'
sudo ufw allow 8082/tcp  comment 'Luggom API'
sudo ufw allow 7047/tcp  comment 'Luggom notary TLSNotary'
sudo ufw allow 80/tcp    comment 'webpage HTTP'
sudo ufw allow 443/tcp   comment 'webpage HTTPS'
# Validadores externos: descomentar quando a testnet aceitar terceiros.
# sudo ufw allow 26656/tcp comment 'Luggom p2p'
sudo ufw --force enable
sudo ufw status verbose
SH

cat > "$STAGE/$PKG/deploy/systemd/luggom-chain.service" <<'INI'
[Unit]
Description=Luggom testnet chain (4 validadores)
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/luggom
Environment=HOME_DIR=/opt/luggom/home
ExecStart=/opt/luggom/scripts/run-local-testnet.sh start
ExecStop=/opt/luggom/scripts/run-local-testnet.sh stop
TimeoutStartSec=300

[Install]
WantedBy=multi-user.target
INI

cat > "$STAGE/$PKG/deploy/systemd/luggom-notary.service" <<'INI'
[Unit]
Description=Luggom TLSNotary notary
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/luggom/notary
EnvironmentFile=/opt/luggom/.env
Environment=LUGGOM_EXTRACTOR_BIN=/opt/luggom/build/luggom-extract
# v4: o chain-id entra no payload assinado (impede replay testnet → mainnet).
# Tem de coincidir com o chain-id da chain e com o da API.
Environment=LUGGOM_CHAIN_ID=luggom-test
Environment=PORT=7047
ExecStart=/opt/luggom/build/tlsn-verifier-server
Restart=on-failure

[Install]
WantedBy=multi-user.target
INI

cat > "$STAGE/$PKG/deploy/systemd/luggom-api.service" <<'INI'
[Unit]
Description=Luggom API
After=luggom-chain.service luggom-notary.service

[Service]
Type=simple
Environment=LUGGOM_HOME=/opt/luggom/home
Environment=LUGGOMD_BIN=/opt/luggom/build/luggomd
Environment=LUGGOM_API_PORT=8082
Environment=LUGGOM_CHAIN_ID=luggom-test
Environment=LUGGOM_KEYRING=GGOM
Environment=LUGGOM_NOTARY_URL=http://127.0.0.1:7047
Environment=LUGGOM_NODE=tcp://127.0.0.1:26657
Environment=LUGGOM_DATA_PROVIDER=mock
ExecStart=/opt/luggom/build/luggom-api
Restart=on-failure

[Install]
WantedBy=multi-user.target
INI

cat > "$STAGE/$PKG/deploy/systemd/luggom-oracle-proxy.service" <<'INI'
[Unit]
Description=Luggom oracle FX proxy (inverte as taxas)
After=network-online.target

[Service]
Type=simple
WorkingDirectory=/opt/luggom
ExecStart=/usr/bin/python3 /opt/luggom/scripts/oracle-local-proxy.py
Restart=on-failure

[Install]
WantedBy=multi-user.target
INI

cat > "$STAGE/$PKG/deploy/systemd/luggom-oracle-feeder.service" <<'INI'
[Unit]
Description=Luggom oracle feeder (1x/dia)
After=luggom-chain.service luggom-oracle-proxy.service

[Service]
Type=oneshot
WorkingDirectory=/opt/luggom
ExecStart=/opt/luggom/build/luggom-oracle-feeder -once \
  -sources-file /opt/luggom/scripts/oracle-local-sources.json \
  -bin /opt/luggom/build/luggomd \
  -home /opt/luggom/home \
  -chain-id luggom-test \
  -keyring test \
  -from oracle-feeder \
  -node tcp://127.0.0.1:26657

[Install]
WantedBy=multi-user.target
INI

cat > "$STAGE/$PKG/deploy/systemd/luggom-oracle-feeder.timer" <<'INI'
[Unit]
Description=Run luggom-oracle-feeder daily

[Timer]
OnCalendar=*-*-* 00:15:00
Persistent=true

[Install]
WantedBy=timers.target
INI

cat > "$STAGE/$PKG/deploy/nginx/luggom.com.conf" <<'NGINX'
server {
    listen 80;
    server_name luggom.com www.luggom.com;

    root /opt/luggom/site;
    index index.html;

    location / {
        try_files $uri $uri/ =404;
    }

    location /downloads/ {
        alias /opt/luggom/downloads/;
        autoindex on;
    }
}
NGINX

chmod +x "$STAGE/$PKG/deploy/"*.sh

# 9. Tarball
mkdir -p "$OUT_DIR"
tar -czf "$OUT_DIR/$PKG.tar.gz" -C "$STAGE" "$PKG"
rm -rf "$STAGE"
echo ""
echo "✅ $OUT_DIR/$PKG.tar.gz"
echo "   (próximo: transferir para a VPS e seguir deploy/DEPLOY.md)"
