#!/usr/bin/env bash
# Chain LOCAL descartável com 10M LUG no genesis — entra direto na DAO.
# Nada disto vai para a testnet pública.
#
# Uso:
#   ./scripts/run-dao-rehearsal.sh start   # cria o genesis e arranca os nós
#   ./scripts/run-dao-rehearsal.sh status
#   ./scripts/run-dao-rehearsal.sh stop
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$PWD"

BIN="$ROOT/build/luggomd"
HOME_DIR="${HOME_DIR:-/tmp/luggom-dao}"
CHAIN_ID="${CHAIN_ID:-luggom-dao}"
TOTAL_GENESIS_LUG="${TOTAL_GENESIS_LUG:-10000000000000000000000000}" # 10M × 1e18
BOND_LUG="${BOND_LUG:-200}"
ACTION="${1:-start}"

if [[ ! -x "$BIN" ]]; then
  echo "a compilar o luggomd..."
  (cd "$ROOT" && PATH="$ROOT/.go1.23.8/bin:$PATH" go build -ldflags="-checklinkname=0" -o build/luggomd ./cmd/luggomd)
fi

case "$ACTION" in
  start)
    export HOME_DIR CHAIN_ID TOTAL_GENESIS_LUG BOND_LUG
    "$ROOT/scripts/prepare-e2e-real.sh" --home "$HOME_DIR" --chain-id "$CHAIN_ID"
    "$ROOT/scripts/bootstrap-validators.sh" local 1 --home "$HOME_DIR" --chain-id "$CHAIN_ID"
    # O bootstrap pode reescrever o genesis; fixa os params canónicos do x/gov
    # no coordinator e nos validadores antes de arrancar.
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
    "$ROOT/scripts/run-local-testnet.sh" start
    "$ROOT/scripts/run-local-testnet.sh" status
    echo "DAO rehearsal: supply=$TOTAL_GENESIS_LUG uluggom (10M LUG); chain=$CHAIN_ID"
    ;;
  status)
    HOME_DIR="$HOME_DIR" CHAIN_ID="$CHAIN_ID" "$ROOT/scripts/run-local-testnet.sh" status
    ;;
  stop)
    HOME_DIR="$HOME_DIR" CHAIN_ID="$CHAIN_ID" "$ROOT/scripts/run-local-testnet.sh" stop
    ;;
  *)
    echo "uso: $0 start|status|stop" >&2; exit 1;;
esac
