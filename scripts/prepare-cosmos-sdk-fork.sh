#!/usr/bin/env bash
# Recria ./deps/cosmos-sdk a partir do module cache e aplica patches Luggom (tally export + bridge + tally ponderado).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$(go env GOMODCACHE)/github.com/cosmos/cosmos-sdk@v0.53.3"
DEST="$ROOT/deps/cosmos-sdk"
if [[ ! -d "$SRC" ]]; then
  echo "Cosmos SDK cache missing: $SRC (run go mod download)" >&2
  exit 1
fi
rm -rf "$DEST"
cp -a "$SRC" "$DEST"
TALLY="$DEST/x/gov/keeper/tally.go"
if ! grep -q 'DefaultCalculateVoteResultsAndVotingPower = defaultCalculateVoteResultsAndVotingPower' "$TALLY"; then
  printf '\n// Luggom fork: export default tally for weighted governance wrapper.\nvar DefaultCalculateVoteResultsAndVotingPower = defaultCalculateVoteResultsAndVotingPower\n' >> "$TALLY"
fi
cp "$ROOT/cosmos_sdk_fork_patches/x/gov/keeper/luggom_bridge.go" "$DEST/x/gov/keeper/"
cp "$ROOT/cosmos_sdk_fork_patches/x/gov/keeper/luggom_weighted_tally.go" "$DEST/x/gov/keeper/"
# FASE 3: limite 1% stake por validador (patch staking).
cp "$ROOT/cosmos_sdk_fork_patches/x/staking/keeper/luggom_stake_limit.go" "$DEST/x/staking/keeper/"
cp "$ROOT/cosmos_sdk_fork_patches/x/staking/types/luggom_errors.go" "$DEST/x/staking/types/"
echo "OK: $DEST"
