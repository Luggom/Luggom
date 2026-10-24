#!/usr/bin/env bash
# upgrade-preflight.sh — verifica se este nó está pronto para uma altura de halt.
#
# Porquê: com validadores externos, um upgrade deixa de ser "paro os meus quatro
# servidores". A regra passa a ser uma altura anunciada com antecedência, e cada
# operador tem de chegar lá com o binário certo e com `--halt-height` ligado.
# Este script responde a três perguntas: o binário é o certo? o nó está
# sincronizado? quanto tempo falta para a altura?
#
# Uso:
#   ./scripts/upgrade-preflight.sh \
#     --bin /opt/luggom/build/luggomd \
#     --home /opt/luggom/home \
#     --node tcp://127.0.0.1:26657 \
#     --sha256 <hash-esperado> \
#     --halt-height <altura>
#
# Sai com 0 se estiver tudo certo, 1 se houver um problema bloqueante.

set -euo pipefail

BIN="./build/luggomd"
HOME_DIR="${LUGGOM_HOME:-$HOME/.luggom}"
NODE="tcp://127.0.0.1:26657"
EXPECTED_SHA=""
HALT_HEIGHT=""
BLOCK_TIME=5

while [[ $# -gt 0 ]]; do
  case "$1" in
    --bin) BIN="$2"; shift 2;;
    --home) HOME_DIR="$2"; shift 2;;
    --node) NODE="$2"; shift 2;;
    --sha256) EXPECTED_SHA="$2"; shift 2;;
    --halt-height) HALT_HEIGHT="$2"; shift 2;;
    --block-time) BLOCK_TIME="$2"; shift 2;;
    -h|--help) sed -n '2,20p' "$0"; exit 0;;
    *) echo "opção desconhecida: $1" >&2; exit 1;;
  esac
done

fail=0
ok()   { printf "  ok    %s\n" "$*"; }
warn() { printf "  aviso %s\n" "$*"; }
bad()  { printf "  FALHA %s\n" "$*"; fail=1; }

echo "== binário =="
if [[ ! -x "$BIN" ]]; then
  bad "não encontrei $BIN (ou não é executável)"
else
  got=$(sha256sum "$BIN" | cut -d' ' -f1)
  ok "$BIN  sha256=$got"
  if [[ -n "$EXPECTED_SHA" ]]; then
    if [[ "$got" == "$EXPECTED_SHA" ]]; then
      ok "hash confere com o anunciado"
    else
      bad "hash NÃO confere: esperado $EXPECTED_SHA"
    fi
  else
    warn "sem --sha256; não é possível confirmar que é o binário do upgrade"
  fi
fi

echo "== estado do nó =="
# o endpoint é JSON-RPC sobre HTTP; tcp:// não serve para curl
NODE_HTTP="${NODE/tcp:/http:}"
status=$(curl -sf --max-time 10 "$NODE_HTTP/status" 2>/dev/null || true)
if [[ -z "$status" ]]; then
  bad "não consegui ler $NODE/status"
else
  eval "$(echo "$status" | python3 -c '
import sys, json
r = json.load(sys.stdin)["result"]
i = r["sync_info"]
print("height=%s" % i["latest_block_height"])
print("catching=%s" % ("true" if i["catching_up"] else "false"))
print("network=%s" % r["node_info"]["network"])
')"

  ok "rede: ${network:-?}  altura: $height"
  if [[ "$catching" == "false" ]]; then
    ok "sincronizado (catching_up=false)"
  else
    bad "ainda a sincronizar (catching_up=true) — não bondes nem reinicies agora"
  fi

  if [[ -n "$HALT_HEIGHT" ]]; then
    if (( HALT_HEIGHT <= height )); then
      bad "a altura de halt ($HALT_HEIGHT) já passou (altura $height) — o nó devia ter parado"
    else
      left=$(( HALT_HEIGHT - height ))
      secs=$(( left * BLOCK_TIME ))
      ok "faltam $left blocos (~$(( secs / 60 )) min a ${BLOCK_TIME}s/bloco)"
      if (( left > 5000 )); then
        warn "ainda falta muito; liga --halt-height agora e deixa correr"
      fi
    fi
  else
    warn "sem --halt-height; não é possível calcular a distância à altura anunciada"
  fi
fi

echo "== halt ligado? =="
if systemctl cat luggom-chain.service 2>/dev/null | grep -qE -- "--halt-height|LUGGOM_HALT_HEIGHT=[1-9]"; then
  ok "halt armado na unit (flag direta ou LUGGOM_HALT_HEIGHT)"
else
  warn "a unit não tem --halt-height; acrescenta um drop-in, por exemplo:"
  echo '         [Service]'
  echo '         ExecStart='
  echo '         ExecStart=/opt/luggom/build/luggomd start --home /opt/luggom/home --halt-height <ALTURA>'
  echo '         (depois: systemctl daemon-reload && systemctl restart luggom-chain)'
fi

echo
if (( fail == 0 )); then
  echo "PRONTO para a altura de halt."
else
  echo "NÃO PRONTO — resolve os pontos acima antes da altura."
  exit 1
fi
