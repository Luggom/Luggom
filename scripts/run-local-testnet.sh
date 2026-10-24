#!/usr/bin/env bash
# run-local-testnet.sh — arranca/para o conjunto de validadores LOCAL (hard test).
#
# Porquê: com N validadores o quórum é 2/3+1 — com 3 validadores, **um só nó não
# produz blocos**. O hard test exige os N nós a correr, com portas distintas e
# `persistent_peers` ligados entre si.
#
# Pré-requisito: `./scripts/bootstrap-validators.sh local N` (que cria
# `$HOME_DIR/validators/val-i` com o seu home, gentx e o genesis final).
# ⚠️ N é o número de validadores ADICIONAIS ao fundador — para **3 no total**
# (o mínimo do hard test) usa `local 2`. O runner arranca o FUNDADOR também (o
# nó dele vive no home do coordenador).
#
# Uso:
#   ./scripts/run-local-testnet.sh start     # arranca todos (em background)
#   ./scripts/run-local-testnet.sh stop
#   ./scripts/run-local-testnet.sh status
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$PWD"

BIN="$ROOT/build/luggomd"
HOME_DIR="${HOME_DIR:-$HOME/.luggom}"
CHAIN_ID="${CHAIN_ID:-luggom-test}"
VALROOT="$HOME_DIR/validators"
ACTION="${1:-status}"
PIDDIR="$HOME_DIR/testnet-pids"

# O FUNDADOR também é validador e tem o seu nó no home do COORDENADOR — se não
# subir, o quórum (2/3+1) não fecha. Lista de nós: [fundador, val-0, val-1, …].
mapfile -t VD < <(ls -d "$VALROOT"/val-* 2>/dev/null || true)
if [[ ${#VD[@]} -eq 0 ]]; then
  echo "❌ sem validadores em $VALROOT — corre: ./scripts/bootstrap-validators.sh local N" >&2
  exit 1
fi
DIRS=("$HOME_DIR" "${VD[@]}")
NAMES=("fundador")
for i in "${!VD[@]}"; do NAMES+=("val-$i"); done

# Nó FULL (não-validador), se existir: serve para testar SYNC/CATCH-UP sem
# derrubar consenso (§13.73). Cria-se com `$0 prep-full`.
FULLDIR="$VALROOT/full"
if [[ -d "$FULLDIR" ]]; then
  DIRS+=("$FULLDIR"); NAMES+=("full")
fi
N=${#DIRS[@]}

port_p2p() { echo $((26656 + 10 * $1)); }
port_rpc() { echo $((26657 + 10 * $1)); }

node_id() { "$BIN" comet show-node-id --home "$1" 2>/dev/null || "$BIN" tendermint show-node-id --home "$1"; }

# O pacote da VPS não copia data/ (arranca limpo do genesis), mas o CometBFT
# exige data/priv_validator_state.json. Criar só num home virgem; se já há
# blocos, falhar em vez de escrever altura 0 (evita double-sign).
ensure_pv_state() { # dir
  local dir="$1" data="$1/data" st="$1/data/priv_validator_state.json"
  [[ -f "$st" ]] && return 0
  if [[ -e "$data/blockstore.db" || -e "$data/state.db" ]]; then
    echo "❌ $dir tem blocos mas falta priv_validator_state.json — restaurar do backup (não criar estado 0)" >&2
    exit 1
  fi
  mkdir -p "$data"
  printf '{\n  "height": "0",\n  "round": 0,\n  "step": 0\n}\n' > "$st"
}

configure() { # idx dir
  local i="$1" dir="$2"
  local p2p rpc peers j
  p2p=$(port_p2p "$i"); rpc=$(port_rpc "$i")
  peers=""
  for j in "${!DIRS[@]}"; do
    [[ "$j" -eq "$i" ]] && continue
    peers+="$(node_id "${DIRS[$j]}")@127.0.0.1:$(port_p2p "$j"),"
  done
  peers="${peers%,}"

  python3 - "$dir/config/config.toml" "$dir/config/app.toml" "$p2p" "$rpc" "$peers" "$i" <<'PY'
import re, sys
cfg, app, p2p, rpc, peers, i = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4], sys.argv[5], sys.argv[6]
grpc = str(9090 + 10 * int(i)); api = str(1317 + 10 * int(i)); pprof = str(6060 + 10 * int(i))

def edit(path, fn):
    with open(path) as f:
        lines = f.read().split("\n")
    out = fn(lines)
    with open(path, "w") as f:
        f.write("\n".join(out))

def settoml(lines, section, key, val):
    sec = None
    for n, l in enumerate(lines):
        m = re.match(r"^\s*\[([^\]]+)\]", l)
        if m:
            sec = m.group(1).strip()
            continue
        if sec == section and re.match(r"^\s*%s\s*=" % re.escape(key), l):
            lines[n] = '%s = "%s"' % (key, val)
            return
    lines.append("[%s]" % section); lines.append('%s = "%s"' % (key, val))

# rpc/api/grpc escutam em 0.0.0.0 DE PROPÓSITO: é o fallback por IP documentado
# em TESTNET/ENDPOINTS.md e o que o nginx usa. A fronteira de segurança é a
# firewall (só 26656/26657/1317/8082 abertos), não o bind. A telemetria
# (prometheus/pprof) fica em 127.0.0.1.
edit(cfg, lambda L: (
    settoml(L, "rpc", "laddr", "tcp://0.0.0.0:%s" % rpc),
    settoml(L, "p2p", "laddr", "tcp://0.0.0.0:%s" % p2p),
    settoml(L, "p2p", "persistent_peers", peers),
    settoml(L, "p2p", "addr_book_strict", "false"),
    settoml(L, "p2p", "allow_duplicate_ip", "true"),
    settoml(L, "instrumentation", "prometheus_listen_addr", "tcp://127.0.0.1:%s" % pprof),
    L)[-1])

edit(app, lambda L: (
    # O desktop (carteira) fala com o nó por REST/LCD — sem isto ligado, a
    # carteira não tem como ler saldo nem transmitir nada (§13.150/A3).
    settoml(L, "api", "enable", "true"),
    settoml(L, "api", "address", "tcp://0.0.0.0:%s" % api),
    settoml(L, "grpc", "address", "0.0.0.0:%s" % grpc),
    L)[-1])
print("  nó %s → rpc :%s · p2p :%s · api :%s · grpc :%s" % (i, rpc, p2p, api, grpc))
PY
}

case "$ACTION" in
  prep-full)
    echo "=== Nó FULL (não-validador) — sync/catch-up (§13.73) ==="
    rm -rf "$FULLDIR"
    "$BIN" init "lug-full" --chain-id "$CHAIN_ID" --home "$FULLDIR" >/dev/null 2>&1
    # Mesmo genesis (é o que o faz seguir a MESMA chain); sem gentx — não é validador.
    cp "$HOME_DIR/config/genesis.json" "$FULLDIR/config/genesis.json"
    echo "  criado $FULLDIR (genesis copiado; portas e peers são postos no start)"
    echo "  ⚠️  na 1.ª vez ele SINCRONIZA desde o bloco 1 — é esse o teste de catch-up"
    ;;

  start)
    mkdir -p "$PIDDIR"
    # Upgrade por altura de halt (ver TESTNET/UPGRADES.md). 0/vazio = desligado.
    # A unit do pacote passa isto por Environment= quando há um upgrade anunciado.
    HALT_FLAG=""
    if [[ -n "${LUGGOM_HALT_HEIGHT:-}" && "${LUGGOM_HALT_HEIGHT}" != "0" ]]; then
      HALT_FLAG="--halt-height ${LUGGOM_HALT_HEIGHT}"
      echo "⚠️  --halt-height ${LUGGOM_HALT_HEIGHT} em TODOS os nós (upgrade)"
    fi
    echo "=== A configurar $N validador(es) ==="
    for i in "${!DIRS[@]}"; do configure "$i" "${DIRS[$i]}"; done
    echo "=== A arrancar $N nó(s) ==="
    for i in "${!DIRS[@]}"; do
      dir="${DIRS[$i]}"
      ensure_pv_state "$dir"
      nohup "$BIN" start --home "$dir" --minimum-gas-prices 1uluggom $HALT_FLAG \
        >"$dir/node.log" 2>&1 &
      echo $! > "$PIDDIR/${NAMES[$i]}.pid"
      echo "  ${NAMES[$i]} pid $(cat "$PIDDIR/${NAMES[$i]}.pid") → log $dir/node.log"
    done
    echo ""
    echo "Espera uns segundos e confirma com: $0 status"
    ;;

  stop)
    if [[ -d "$PIDDIR" ]]; then
      for f in "$PIDDIR"/*.pid; do
        [[ -f "$f" ]] || continue
        pid=$(cat "$f")
        if kill -0 "$pid" 2>/dev/null; then kill "$pid" && echo "  parado pid $pid"; fi
        rm -f "$f"
      done
    else
      echo "  (sem pids registados)"
    fi
    ;;

  status)
    echo "=== Estado dos $N nó(s) ==="
    for i in "${!DIRS[@]}"; do
      port=$(port_rpc "$i")
      h=$(curl -s -m 2 "http://127.0.0.1:$port/status" 2>/dev/null \
          | python3 -c "import sys,json;print(json.load(sys.stdin)['result']['sync_info']['latest_block_height'])" 2>/dev/null || echo "-")
      printf "  %-9s rpc :%s  altura=%s\n" "${NAMES[$i]}" "$port" "$h"
    done
    ;;

  *)
    echo "uso: $0 start | stop | status" >&2; exit 1;;
esac
