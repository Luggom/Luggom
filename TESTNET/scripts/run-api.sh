#!/usr/bin/env bash
# Sobe a API HTTP para o APK (Etapa 8). Chain deve estar rodando (luggomd start).
# Uso: cd <repo> && ./scripts/run-api.sh

set -e
cd "$(dirname "$0")/../.."
export LUGGOM_HOME="${LUGGOM_HOME:-$HOME/.luggom}"
export LUGGOMD_BIN="${LUGGOMD_BIN:-$PWD/build/luggomd}"
export LUGGOM_API_PORT="${LUGGOM_API_PORT:-8082}"
# Etapa G2 (opcional): escrow P2P em memória na API — export LUGGOM_ESCROW_LAB=1

if [ ! -x "$LUGGOMD_BIN" ]; then
  echo "Erro: binário não encontrado: $LUGGOMD_BIN"
  echo "Rode: go build -o build/luggomd ./cmd/luggomd"
  exit 1
fi

API_BIN="./build/luggom-api"
if [ ! -x "$API_BIN" ]; then
  echo "Construindo luggom-api..."
  go build -o build/luggom-api ./cmd/luggom-api
fi

# §13.145 (A5/A10): a API NÃO mina sem notário. Sem LUGGOM_NOTARY_URL, o
# /api/v1/mine/tax responde 503 de propósito (fail-closed) — antes ela assinava
# sozinha o que extraíra de um transcript que o cliente enviou, e a mineração era
# forjável se a mainnet subisse mal configurada.
if [ -z "${LUGGOM_NOTARY_URL:-}" ]; then
  echo "⚠️  LUGGOM_NOTARY_URL não definido → a mineração vai responder 503."
  echo "    Suba o notário (:7047) e exporte LUGGOM_NOTARY_URL=http://127.0.0.1:7047"
fi

echo "Iniciando luggom-api em :$LUGGOM_API_PORT (home=$LUGGOM_HOME) notary=${LUGGOM_NOTARY_URL:-<nenhum>}"
exec "$API_BIN"
