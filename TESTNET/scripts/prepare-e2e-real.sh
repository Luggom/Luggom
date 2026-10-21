#!/usr/bin/env bash
# Preparação do ambiente E2E local — Marco da Mineração Real (Viacredi, §13.58).
#
# Regenera o genesis com os params MVP 2.0 completos (whitelist 740 + bancos 22
# + blacklist governos), seta o founder_address (multisig ainda não montado —
# usa a chave local val-0/GGOM como placeholder), cria o validador com bond
# 200 LUG, e deixa o nó + API prontos para o APK real.
#
# Uso: ./scripts/prepare-e2e-real.sh [--home DIR] [--chain-id ID] [--moniker NAME]
#   --home      home do nó (default: $HOME/.luggom)
#   --chain-id  chain id (default: luggom-test)
#   --moniker   moniker (default: lug-local)
#   --keyring   keyring-backend (default: test)
#   --keyname   nome da chave do validador/founder (default: GGOM)

set -euo pipefail
cd "$(dirname "$0")/../.."
ROOT="$PWD"

BIN="$ROOT/build/luggomd"
HOME_DIR="${HOME_DIR:-$HOME/.luggom}"
CHAIN_ID="${CHAIN_ID:-luggom-test}"
MONIKER="${MONIKER:-lug-local}"
KEYRING="${KEYRING:-test}"
KEYNAME="${KEYNAME:-GGOM}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --home) HOME_DIR="$2"; shift 2;;
    --chain-id) CHAIN_ID="$2"; shift 2;;
    --moniker) MONIKER="$2"; shift 2;;
    --keyring) KEYRING="$2"; shift 2;;
    --keyname) KEYNAME="$2"; shift 2;;
    *) echo "Argumento desconhecido: $1" >&2; exit 1;;
  esac
done

LUG="000000000000000000"          # 10^18
BOND_200_LUG="200${LUG}"          # 200 LUG
# Genesis = bootstrap dos validadores + reserva de pó:
#   4.000 LUG    = §13.59 (fundador 3×200 + 17 comunitários×200).
#   0,000204 LUG = reserva para ativar 20.000.000 carteiras novas (pó 1e-11 +
#                  gas 2e-13 por carteira) — decisões DUST D1–D9.
# Só é viável porque a conta da reserva está ISENTA da taxa fixa de 0,005 LUG
# (§13.87): sem isenção, 20M carteiras exigiriam 100.000 LUG, não 0,000204.
# A reserva fica na MESMA conta (decisão D2): a API envia o pó com a chave GGOM,
# e uma conta de módulo não serve porque não tem chave privada.
TOTAL_GENESIS_LUG="4000000204000000000000"  # 4.000,000204 LUG em uluggom

# §13.139 (D3) — conta do FEEDER do oráculo. A chain só aceita a tx do oráculo
# vinda da authority OU do endereço em `usd_oracle_updater_address` (oracle_update.go);
# sem ele o feeder seria RECUSADO e a mineração pausaria (fail-closed).
#
# O gas sai do FUNDADOR, não de supply nova: o total do genesis NÃO muda (as 50
# LUG passam de uma conta para a outra). 50 LUG a 0,005/tx diária ≈ 27 anos.
# Override: ORACLE_FEEDER_ADDR=... ./scripts/prepare-e2e-real.sh
ORACLE_FEEDER_ADDR="${ORACLE_FEEDER_ADDR:-luggom1kppws3a2c4v4y3xnngzv3jk42m24sgu4qn7a0n}"
ORACLE_FEEDER_GAS_LUG="50000000000000000000"  # 50 LUG em uluggom
# bash não faz aritmética de 64 bits com estes valores (4e21 estoura) → python.
FOUNDER_GENESIS_LUG=$(python3 -c "print($TOTAL_GENESIS_LUG - $ORACLE_FEEDER_GAS_LUG)")

echo "=== Luggom — Preparação E2E Real (Marco Viacredi §13.58) ==="
echo "Binário: $BIN"
echo "Home:    $HOME_DIR"
echo "Chain:   $CHAIN_ID"

if [[ ! -x "$BIN" ]]; then
  echo "❌ $BIN não encontrado. Rode primeiro o build." >&2
  exit 1
fi

# 1. Init do nó (recria genesis do zero)
echo "=== Init do nó (genesis do zero com params MVP 2.0) ==="
rm -rf "$HOME_DIR/config" "$HOME_DIR/data"
"$BIN" init "$MONIKER" --chain-id "$CHAIN_ID" --home "$HOME_DIR"
GENESIS="$HOME_DIR/config/genesis.json"

# 2. Ajustes via python: staking 250, slashing 70%, gas 1uluggom,
#    founder_address = chave local, whitelist/bancos/blacklist via default do NewParams.
echo "=== Ajustando genesis (staking/slashing/founder) ==="
python3 - "$GENESIS" <<'PY'
import json, sys
path = sys.argv[1]
with open(path) as f:
    g = json.load(f)

# Staking: MaxValidators = 180 (§13.116 — antes 250, §13.61)
g["app_state"]["staking"]["params"]["max_validators"] = 180
# Slashing: 70% uptime (§13.62)
sl = g["app_state"]["slashing"]["params"]
sl["signed_blocks_window"] = "10000"
sl["min_signed_per_window"] = "0.700000000000000000"
sl["downtime_jail_duration"] = "600s"

# Governança MVP 2.0 (§13.57/§13.84): fundador autônomo até 50M (IBC) →
# 100%/0% (LLC extinta). Os BPS antigos (9000/1000) eram do MVP 1.5.
lp = g["app_state"]["luggom"]["params"]
lp["gov_founder_vote_weight_bps"] = 10000
lp["gov_validators_vote_weight_bps"] = 0
lp["gov_llc_vote_weight_bps"] = 0
lp["gov_validator_min_for_weight_shift"] = 0

with open(path, "w") as f:
    json.dump(g, f, indent=2)
print("  max_validators:", g["app_state"]["staking"]["params"]["max_validators"])
print("  min_signed_per_window:", sl["min_signed_per_window"])
PY

# 3. Minimum gas prices: 1uluggom (§13.87)
APP_TOML="$HOME_DIR/config/app.toml"
if [[ -f "$APP_TOML" ]]; then
  sed -i 's/^minimum-gas-prices = .*/minimum-gas-prices = "1uluggom"/' "$APP_TOML" \
    || echo "  (app.toml sem minimum-gas-prices — ajuste manual)"
  echo "minimum-gas-prices: $(grep '^minimum-gas-prices' "$APP_TOML")"
fi

# 4. Chave do validador/founder
if ! "$BIN" keys show "$KEYNAME" --keyring-backend "$KEYRING" --home "$HOME_DIR" >/dev/null 2>&1; then
  echo "=== Criando chave $KEYNAME ==="
  "$BIN" keys add "$KEYNAME" --keyring-backend "$KEYRING" --home "$HOME_DIR" 2>&1 | tail -3
fi
FOUNDER_ADDR=$("$BIN" keys show "$KEYNAME" -a --keyring-backend "$KEYRING" --home "$HOME_DIR")
echo "Founder/validador: $FOUNDER_ADDR"

# 5. Conta genesis: 4.000 LUG (bootstrap §13.59) + 0,000204 de reserva de pó.
#    Menos o gas do feeder do oráculo (§13.139 D3), que vai para a conta dele.
#    A reserva fica na mesma conta (a API usa a chave GGOM para enviar o pó; uma
#    conta de módulo não serve porque não tem chave privada — decisão D2).
"$BIN" genesis add-genesis-account "$FOUNDER_ADDR" "${FOUNDER_GENESIS_LUG}uluggom" \
  --keyring-backend "$KEYRING" --home "$HOME_DIR"

# 5b. Conta do FEEDER do oráculo, com o gas (o total do genesis NÃO muda).
"$BIN" genesis add-genesis-account "$ORACLE_FEEDER_ADDR" "${ORACLE_FEEDER_GAS_LUG}uluggom" \
  --keyring-backend "$KEYRING" --home "$HOME_DIR"
echo "Feeder do oráculo: $ORACLE_FEEDER_ADDR (${ORACLE_FEEDER_GAS_LUG} uluggom)"

# 6. Setar founder_address (recebe 8%/5% do mint — §13.84) e o
#    usd_oracle_updater_address (quem pode alimentar o oráculo — §13.139 D3).
python3 - "$GENESIS" "$FOUNDER_ADDR" "$ORACLE_FEEDER_ADDR" <<'PY'
import json, sys
path, addr, feeder = sys.argv[1], sys.argv[2], sys.argv[3]
with open(path) as f:
    g = json.load(f)
g["app_state"]["luggom"]["params"]["founder_address"] = addr
g["app_state"]["luggom"]["params"]["usd_oracle_updater_address"] = feeder
with open(path, "w") as f:
    json.dump(g, f, indent=2)
print("  founder_address:", addr)
print("  usd_oracle_updater_address:", feeder)
PY

# 7. Gentx (bond 200 LUG) + collect
"$BIN" genesis gentx "$KEYNAME" "${BOND_200_LUG}uluggom" \
  --chain-id "$CHAIN_ID" --keyring-backend "$KEYRING" \
  --moniker "$MONIKER" --home "$HOME_DIR" \
  --commission-rate "0.10" --commission-max-rate "0.20" --commission-max-change-rate "0.01"
"$BIN" genesis collect-gentxs --home "$HOME_DIR"
"$BIN" genesis validate --home "$HOME_DIR"

# 7b. §13.121 — LIGAR A FASE 2 (o notário assina).
#     Se LUGGOM_NOTARY_KEY_HEX estiver definido, deriva o endereço do notário
#     (com o utilitário partilhado) e grava `api_verifier_address`
#     (§13.170: a fase é agora uma constante de código = 2).
#     `api_verifier_address` no genesis. Sem esta chave, a chain fica na fase 1
#     (trust-API) — que é FORJÁVEL e não deve ir para testnet.
if [[ -n "${LUGGOM_NOTARY_KEY_HEX:-}" ]]; then
  NOTARY_ADDR=$(./build/luggom-extract --print-address "$LUGGOM_NOTARY_KEY_HEX") || {
    echo "ERRO: LUGGOM_NOTARY_KEY_HEX inválida (não consegui derivar o endereço)"; exit 1; }
  python3 - "$GENESIS" "$NOTARY_ADDR" <<'PY'
import json, sys
path, addr = sys.argv[1], sys.argv[2]
with open(path) as f:
    g = json.load(f)
p = g["app_state"]["luggom"]["params"]
# §13.170 Grupo B: `tlsnotary_phase` deixou de ser param (constante de código = 2).
# Só o endereço do verificador (Grupo C, rotável) vai para o genesis.
p["api_verifier_address"] = addr
with open(path, "w") as f:
    json.dump(g, f, indent=2)
print("  api_verifier_address:", addr)
PY
else
  echo "  ⚠️  LUGGOM_NOTARY_KEY_HEX não definido → a chain fica SEM api_verifier_address."
  echo "      Efeito real (§13.145): a fase default é 2, mas sem verificador a"
  echo "      mineração é RECUSADA (fail-closed) — e a API também não mina (503)."
  echo "      Esta combinação só serve para inspecionar a chain. Para minar:"
  echo "      defina a chave do notário E exporte LUGGOM_NOTARY_URL."
fi

# 8. Verificação final dos params Luggom
echo ""
echo "=== Verificação dos params Luggom no genesis ==="
python3 - "$GENESIS" <<'PY'
import json, sys
g = json.load(open(sys.argv[1]))
p = g["app_state"]["luggom"]["params"]
print("  founder_address:", repr(p.get("founder_address")))
print("  usd_oracle_updater_address:", repr(p.get("usd_oracle_updater_address")))
print("  tax_entity_whitelist:", len(p.get("tax_entity_whitelist", [])), "entradas")
print("  bank_providers:", len(p.get("bank_providers", [])), "bancos")
print("  government_entity_blacklist:", len(p.get("government_entity_blacklist", [])), "entradas")
print("  pool_assets:", p.get("pool_assets"))
print("  dynamic_usd_oracle_enabled:", p.get("dynamic_usd_oracle_enabled"))
PY

echo ""
echo "=== Ambiente pronto ==="
echo "Para subir o nó:"
echo "  export LUGGOM_HOME=\"$HOME_DIR\""
echo "  ./build/luggomd start --home \"$HOME_DIR\""
echo ""
echo "Para subir a API (outra shell):"
echo "  export LUGGOM_HOME=\"$HOME_DIR\" LUGGOMD_BIN=\"$ROOT/build/luggomd\" LUGGOM_API_PORT=\"8082\" LUGGOM_DATA_PROVIDER=\"mock\" LUGGOM_KEYRING=\"$KEYNAME\""
echo "  ./build/luggom-api"
echo ""
echo "Marco real (Viacredi) — depois de subir nó + API:"
echo "  1. APK aponta para http://<IP-da-máquina>:8082 (ADB wifi: adb reverse tcp:8082 tcp:8082)"
echo "  2. Logar no Viacredi no celular, gerar prova do IPTU pago, minerar"
