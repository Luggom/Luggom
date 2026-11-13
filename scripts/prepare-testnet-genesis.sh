#!/usr/bin/env bash
# Prepara o BINÁRIO e o GENESIS da testnet pública (8.200 LUG) SEM tocar na VPS.
# O reset/deploy é uma ação separada e destrutiva, só com aprovação explícita.
#
# Saída:
#   build/luggomd-testnet              — binário com o LBC compilado
#   $HOME_DIR/config/genesis.json      — genesis final (8.200 LUG, 4 validadores)
#
# Uso: ./scripts/prepare-testnet-genesis.sh
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$PWD"

BIN="$ROOT/build/luggomd"
TESTNET_BIN="$ROOT/build/luggomd-testnet"
HOME_DIR="${HOME_DIR:-/tmp/luggom-testnet-genesis}"
CHAIN_ID="${CHAIN_ID:-luggom-test01}"
TOTAL_GENESIS_LUG="${TOTAL_GENESIS_LUG:-8200000000000000000000}" # 8.200 LUG
BOND_LUG="${BOND_LUG:-200}"
VAL_COUNT="${VAL_COUNT:-3}" # validadores adicionais; + fundador = 4 no total

echo "=== build ==="
(cd "$ROOT" && PATH="$ROOT/.go1.23.8/bin:$PATH" go build -ldflags="-checklinkname=0" -o build/luggomd ./cmd/luggomd)

echo "=== genesis em $HOME_DIR ==="
rm -rf "$HOME_DIR"
export HOME_DIR CHAIN_ID TOTAL_GENESIS_LUG BOND_LUG
"$ROOT/scripts/prepare-e2e-real.sh" --home "$HOME_DIR" --chain-id "$CHAIN_ID"
"$ROOT/scripts/bootstrap-validators.sh" local "$VAL_COUNT" --home "$HOME_DIR" --chain-id "$CHAIN_ID"

# O bootstrap pode reescrever o genesis; fixa o x/gov canónico.
python3 - "$HOME_DIR" <<'PY'
import glob, json, os, sys
root = sys.argv[1]
files = [os.path.join(root, "config", "genesis.json")]
files += sorted(glob.glob(os.path.join(root, "validators", "val-*", "config", "genesis.json")))
for path in files:
    if not os.path.exists(path):
        continue
    with open(path) as f:
        g = json.load(f)
    gov = g["app_state"]["gov"]["params"]
    gov.update({
        "quorum": "0.200000000000000000",
        "threshold": "0.500000000000000000",
        "veto_threshold": "0.330000000000000000",
        "expedited_threshold": "0.667000000000000000",
        "min_deposit": [{"denom": "uluggom", "amount": "1000000000000000000000"}],
        "expedited_min_deposit": [{"denom": "uluggom", "amount": "2000000000000000000000"}],
        "max_deposit_period": "259200s",
        "voting_period": "432000s",
        "expedited_voting_period": "86400s",
        "min_initial_deposit_ratio": "0.100000000000000000",
    })
    with open(path, "w") as f:
        json.dump(g, f, indent=2)
print("gov canónico aplicado em", len(files), "genesis")
PY
"$BIN" genesis validate --home "$HOME_DIR"

FOUNDER=$("$BIN" keys show GGOM -a --keyring-backend test --home "$HOME_DIR")
echo "=== LBC placeholder (testnet): $FOUNDER ==="
echo "    A mainnet TEM de usar o endereço x/group 3/5."
(cd "$ROOT" && PATH="$ROOT/.go1.23.8/bin:$PATH" go build \
  -ldflags="-checklinkname=0 -X luggom/x/luggom/types.BusinessCoreAddress=$FOUNDER" \
  -o build/luggomd-testnet ./cmd/luggomd)

echo "✅ pronto (NÃO deployado): $TESTNET_BIN"
echo "   genesis: $HOME_DIR/config/genesis.json"
echo "   supply:  $TOTAL_GENESIS_LUG uluggom (8.200 LUG)"
echo "   falta:   LBC x/group 3/5 (mainnet), notário (LUGGOM_NOTARY_KEY_HEX) e denom IBC do BTC"
