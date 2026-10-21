#!/usr/bin/env bash
# bootstrap-validators.sh — cria o CONJUNTO DE VALIDADORES do genesis.
#
# ⚠️ O SUPPLY TOTAL NÃO MUDA: a dotação de cada validador sai do SALDO DO
# FUNDADOR no genesis (o script subtrai-a no fim). Decisão: o bootstrap de 4.000
# LUG é para os validadores; antes, estava todo numa chave.
#
# DOIS MODOS — e a diferença importa:
#
#   local N        DESCARTAVEL (para o hard test / FASE 8). Gera N chaves AQUI,
#                  imprime os mnemonics e produz os gentx. NUNCA usar na testnet
#                  nem na mainnet: quem gera a chave vê o mnemonic.
#
#   external F     TESTNET/MAINNET — **os validadores criam as SUAS contas**. F é
#                  um ficheiro com um endereço por linha ("luggom1… [200]"); este
#                  script FUNDA-os no genesis e recolhe os **gentx que ELES
#                  enviarem** (um gentx é assinado pela chave do validador, logo
#                  tem de ser produzido do lado deles).
#
# Uso:
#   ./scripts/bootstrap-validators.sh local 3
#   ./scripts/bootstrap-validators.sh external validators.txt --gentx-dir ./gentxs
#
# Opções: --home DIR · --chain-id ID · --keyring BACKEND · --keyname FUNDADOR
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$PWD"

BIN="$ROOT/build/luggomd"
HOME_DIR="${HOME_DIR:-$HOME/.luggom}"
CHAIN_ID="${CHAIN_ID:-luggom-test}"
KEYRING="${KEYRING:-test}"
KEYNAME="${KEYNAME:-GGOM}"
GENTX_DIR=""

LUG="000000000000000000"          # 10^18
DENOM="uluggom"
BOND_LUG="200"                    # 200 LUG por validador (desenho do bootstrap)
BOND_COIN="${BOND_LUG}${LUG}${DENOM}"   # a string COMPLETA da moeda (com denom)

# Supply canónico do bootstrap — TEM de ser o mesmo de prepare-e2e-real.sh
# (4.000 LUG dos validadores + 0,000204 da reserva de pó). O `add-genesis-account`
# ACRESCENTA ao supply; no fim repomo-lo, senão cada validador "cunhava" LUG.
TOTAL_GENESIS_LUG="4000000204000000000000"

MODE="${1:-}"
shift || true

# Parsing: as flags podem vir ANTES ou DEPOIS dos posicionais (N / ficheiro).
POS=()
while [[ $# -gt 0 ]]; do
  case "$1" in
    --home) HOME_DIR="$2"; shift 2;;
    --chain-id) CHAIN_ID="$2"; shift 2;;
    --keyring) KEYRING="$2"; shift 2;;
    --keyname) KEYNAME="$2"; shift 2;;
    --gentx-dir) GENTX_DIR="$2"; shift 2;;
    -*) echo "opção desconhecida: $1" >&2; exit 1;;
    *) POS+=("$1"); shift;;
  esac
done

if [[ ! -x "$BIN" ]]; then echo "❌ $BIN não encontrado (corre o build)." >&2; exit 1; fi
GENESIS="$HOME_DIR/config/genesis.json"
if [[ ! -f "$GENESIS" ]]; then
  echo "❌ $GENESIS não existe — corre primeiro ./scripts/prepare-e2e-real.sh" >&2; exit 1
fi

FOUNDER_ADDR=$("$BIN" keys show "$KEYNAME" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")

fund_one() { # addr coin_completa (ex.: 200000000000000000000uluggom)
  "$BIN" genesis add-genesis-account "$1" "$2" \
    --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null
}

case "$MODE" in
  local)
    N="${POS[0]:-3}"
    VALROOT="$HOME_DIR/validators"
    mkdir -p "$VALROOT"
    echo "=== Modo LOCAL (descartável) — $N validadores ==="
    echo "⚠️  Cada validador tem o SEU home: o gentx leva a chave de CONSENSO do nó"
    echo "    (priv_validator_key). Com um só home, os N partilhariam a mesma — inválido."
    echo "⚠️  Os mnemonics vão aparecer abaixo. NUNCA usar fora do hard test."
    for i in $(seq 0 $((N-1))); do
      NAME="val-$i"
      VH="$VALROOT/$NAME"
      rm -rf "$VH"
      "$BIN" init "lug-$NAME" --chain-id "$CHAIN_ID" --home "$VH" >/dev/null 2>&1
      if ! "$BIN" keys show "$NAME" --keyring-backend "$KEYRING" --home "$VH" >/dev/null 2>&1; then
        echo "--- chave $NAME ---"
        "$BIN" keys add "$NAME" --keyring-backend "$KEYRING" --home "$VH" 2>&1
      fi
      ADDR=$("$BIN" keys show "$NAME" -a --keyring-backend "$KEYRING" --home "$VH")
      fund_one "$ADDR" "$BOND_COIN"
      # O gentx lê o genesis DO SEU HOME: copia-se o do coordenador DEPOIS de
      # fundar (senão o validador não tem saldo e o gentx falha a validação).
      cp "$GENESIS" "$VH/config/genesis.json"
      "$BIN" genesis gentx "$NAME" "$BOND_COIN" --chain-id "$CHAIN_ID" \
        --keyring-backend "$KEYRING" --home "$VH" \
        --commission-rate 0.10 --commission-max-rate 0.20 --commission-max-change-rate 0.01 >/dev/null
      cp "$VH/config/gentx/"*.json "$HOME_DIR/config/gentx/" 2>/dev/null || true
      echo "  $NAME → $ADDR (bond ${BOND_LUG} LUG, home $VH)"
    done
    ;;

  external)
    FILE="${POS[0]:-}"
    if [[ -z "$FILE" || ! -f "$FILE" ]]; then
      echo "❌ modo external precisa de um ficheiro de endereços (um por linha)" >&2; exit 1
    fi
    echo "=== Modo EXTERNAL (testnet) — validadores com as SUAS contas ==="
    while read -r addr amount; do
      [[ -z "$addr" || "$addr" == \#* ]] && continue
      if [[ -z "${amount:-}" ]]; then
        amt="$BOND_COIN"
      elif [[ "$amount" =~ ^[0-9]+$ ]]; then
        amt="${amount}${LUG}${DENOM}"            # "200" ⇒ 200 LUG
      else
        amt="$amount"                            # já é "…uluggom"
      fi
      fund_one "$addr" "$amt"
      echo "  fundado $addr com $amt"
    done < "$FILE"
    if [[ -n "$GENTX_DIR" ]]; then
      echo "--- colocando os gentx enviados pelos validadores ---"
      cp "$GENTX_DIR"/*.json "$HOME_DIR/config/gentx/" 2>/dev/null || echo "  (nenhum .json em $GENTX_DIR)"
    else
      echo "⏳ FALTA: recolher os gentx que ELES produzem e pôr em $HOME_DIR/config/gentx/"
      echo "   (cada validador corre: luggomd genesis gentx <chave> 200…uluggom --chain-id $CHAIN_ID)"
    fi
    ;;

  *)
    echo "uso: $0 local N | external FICHEIRO [--gentx-dir DIR]" >&2; exit 1;;
esac

# ---- acerto do fundador + do supply (nada de LUG nova) ----------------------
# O fundador absorve o remanescente: o que os OUTROS têm (feeder + validadores)
# sai do saldo dele, e o supply volta ao canónico — senão cada validador
# acrescentado ao genesis inflacionava a oferta.
python3 - "$GENESIS" "$FOUNDER_ADDR" "$TOTAL_GENESIS_LUG" <<'PY'
import json, sys
path, founder, total = sys.argv[1], sys.argv[2], int(sys.argv[3])
with open(path) as f:
    g = json.load(f)
b = g["app_state"]["bank"]
bal = {x["address"]: int(x["coins"][0]["amount"]) for x in b["balances"]}
others = sum(v for k, v in bal.items() if k != founder)
novo = total - others
if novo < 0:
    print("❌ inconsistente: o saldo do fundador ficaria negativo "
          "(%s pedido por outros > %s do bootstrap)" % (others, total)); sys.exit(1)
antigo = bal.get(founder, 0)
for x in b["balances"]:
    if x["address"] == founder:
        x["coins"][0]["amount"] = str(novo)
b["supply"][0]["amount"] = str(total)
with open(path, "w") as f:
    json.dump(g, f, indent=2)
print("  fundador: %s → %s uluggom (−%s para os validadores)" % (antigo, novo, antigo - novo))
print("  supply  : reposto ao canónico %s" % total)
PY

if [[ -z "$GENTX_DIR" || "$MODE" == "local" ]]; then
  "$BIN" genesis collect-gentxs --home "$HOME_DIR" >/dev/null
fi
"$BIN" genesis validate --home "$HOME_DIR" >/dev/null && echo "✅ genesis válido"

# No modo local, o genesis FINAL (já com todos os gentx) vai para cada home,
# senão os nós arrancariam com genesis diferentes e não haveria consenso.
if [[ "$MODE" == "local" ]]; then
  for vh in "$HOME_DIR"/validators/val-*; do
    [[ -d "$vh" ]] && cp "$GENESIS" "$vh/config/genesis.json"
  done
  echo "genesis final distribuído por $(ls -d "$HOME_DIR"/validators/val-* 2>/dev/null | wc -l) home(s)"
fi

echo ""
echo "=== Verificação ==="
python3 - "$GENESIS" <<'PY'
import json, sys
g = json.load(open(sys.argv[1]))
b = g["app_state"]["bank"]
bal = {x["address"]: int(x["coins"][0]["amount"]) for x in b["balances"]}
supply = int(b["supply"][0]["amount"])
tot = sum(bal.values())
print("  supply            :", supply)
print("  soma dos saldos   :", tot, "(igual ao supply)" if tot == supply else "  ❌ DIFERENTE")
print("  contas            :", len(bal))
print("  gentx no genesis  :", len(g["app_state"]["genutil"]["gen_txs"]))
PY
