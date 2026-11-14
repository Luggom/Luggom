#!/usr/bin/env bash
# Regenera o código Go dos protos via protoc standalone (procedimento validado no E2E §13.29/§13.30).
#
# POR QUE protoc e não buf generate:
#   O buf generate (v1.47) escreve os .pb.go em <out>/<module-path>/ (ex: x/luggom/types/luggom/luggom/v1/)
#   mesmo com paths=source_relative no gocosmos — o path fica duplicado. O protoc com
#   --gocosmos_opt=paths=source_relative gera direto em x/luggom/types/ (comportamento canónico).
#   O buf build/export continua sendo usado para resolver as deps (buf export).
#
# Requer: Go 1.23.8 (ou ./.go1.23.8), protoc 25.3 em /tmp/protoc-bin, plugins em ~/go/bin.
# Uso: ./scripts/regenerate_protos.sh

set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$PWD"

# --- Go 1.23.8 ---
GO_BIN=""
if [[ -x "$ROOT/.go1.23.8/bin/go" ]]; then
  GO_BIN="$ROOT/.go1.23.8/bin/go"
elif command -v go &>/dev/null && go version 2>/dev/null | grep -q "1.23"; then
  GO_BIN="$(command -v go)"
else
  echo "❌ Go 1.23.8 não encontrado (extraia para $ROOT/.go1.23.8)" >&2
  exit 1
fi
export GOROOT="$(cd "$(dirname "$(dirname "$GO_BIN")")" && pwd)"
export PATH="$(dirname "$GO_BIN"):$PATH"
export GOTOOLCHAIN=local
export GOMODCACHE="$ROOT/.gomodcache"
export GOCACHE="$ROOT/.gocache"
export GOTMPDIR="$ROOT/.gotmp"
mkdir -p "$GOMODCACHE" "$GOCACHE" "$GOTMPDIR"
echo "📦 Go: $("$GO_BIN" version)"

# --- buf (para export das deps) ---
BUF=""
if command -v buf &>/dev/null && buf --version 2>/dev/null | awk -F. '{exit !($1>=1 && $2>=47)}'; then
  BUF="$(command -v buf)"
elif [[ -x /tmp/buf-1.47.2-bin ]]; then
  BUF="/tmp/buf-1.47.2-bin"
else
  echo "📦 Baixando buf 1.47.2 ..."
  curl -sL --retry 3 "https://github.com/bufbuild/buf/releases/download/v1.47.2/buf-Linux-x86_64" -o /tmp/buf-1.47.2-bin
  chmod +x /tmp/buf-1.47.2-bin
  BUF="/tmp/buf-1.47.2-bin"
fi

# --- protoc 25.3 ---
PROTOC="/tmp/protoc-bin/bin/protoc"
if [[ ! -x "$PROTOC" ]]; then
  echo "❌ protoc 25.3 não encontrado em /tmp/protoc-bin (veja §13.29 do checkpoint)" >&2
  exit 1
fi

# --- plugins Go ---
export GOBIN="$HOME/go/bin"
mkdir -p "$GOBIN"
if [[ ! -x "$GOBIN/protoc-gen-gocosmos" ]]; then
  "$GO_BIN" install github.com/cosmos/gogoproto/protoc-gen-gocosmos@v1.7.0
fi
if [[ ! -x "$GOBIN/protoc-gen-grpc-gateway" ]]; then
  "$GO_BIN" install github.com/grpc-ecosystem/grpc-gateway/protoc-gen-grpc-gateway@v1.16.0
fi

# --- export deps (inclui os .proto do luggom) ---
IMPORTS="/tmp/proto-imports"
rm -rf "$IMPORTS"
mkdir -p "$IMPORTS"
echo "📦 Exportando deps (buf export) ..."
(cd "$ROOT/proto" && "$BUF" export --output "$IMPORTS")

# --- gerar com protoc (paths=source_relative → direto em x/luggom/types) ---
GEN="/tmp/gen-protoc"
rm -rf "$GEN"
mkdir -p "$GEN"
echo "🔧 Gerando com protoc ..."
"$PROTOC" \
  --proto_path="$IMPORTS" \
  --proto_path=/tmp/protoc-bin/include \
  --plugin=protoc-gen-gocosmos="$GOBIN/protoc-gen-gocosmos" \
  --gocosmos_out="$GEN" \
  --gocosmos_opt=plugins=grpc,paths=source_relative,Mgoogle/protobuf/any.proto=github.com/cosmos/gogoproto/types/any,Mcosmos/orm/v1/orm.proto=cosmossdk.io/orm,Mcosmos/app/v1alpha1/module.proto=cosmossdk.io/api/cosmos/app/v1alpha1 \
  "$IMPORTS/luggom/luggom/v1/tx.proto" \
  "$IMPORTS/luggom/luggom/v1/params.proto" \
  "$IMPORTS/luggom/luggom/v1/query.proto" \
  "$IMPORTS/luggom/luggom/v1/genesis.proto" \
  "$IMPORTS/luggom/luggom/module/v1/module.proto" 2>&1 | grep -v "unused" || true

# --- gateway (v1.16 gera em path com o prefixo do módulo; copiamos do path real) ---
"$PROTOC" \
  --proto_path="$IMPORTS" \
  --proto_path=/tmp/protoc-bin/include \
  --plugin=protoc-gen-grpc-gateway="$GOBIN/protoc-gen-grpc-gateway" \
  --grpc-gateway_out="$GEN" \
  --grpc-gateway_opt=logtostderr=true,allow_colon_final_segments=true \
  "$IMPORTS/luggom/luggom/v1/query.proto" 2>&1 | grep -v "unused" || true

# --- copiar para x/luggom/types ---
cp "$GEN/luggom/luggom/v1/"*.go "$ROOT/x/luggom/types/"
GW="$(find "$GEN" -name "query.pb.gw.go" | head -1)"
if [[ -n "$GW" ]]; then
  cp "$GW" "$ROOT/x/luggom/types/query.pb.gw.go"
fi
echo "✅ Código regenerado em x/luggom/types/."
echo "📝 Build: go build -ldflags=\"-checklinkname=0\" -o build/luggomd ./cmd/luggomd"
