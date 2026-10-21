package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Maximum annual mining cap (MVP 2.0 §13.116, que substitui §13.49/§13.52):
//
//	year 0: 7,000,000,000 (7B) LUG mineable
//	following years: −5% a.a. (7B → 6.65B → 6.32B → ...) down to ~0 (no floor/carry/flat)
//	measurement: 365-day sliding window (sum of daily buckets)
//	immutable code: protocol rule since genesis (strict filter rejects changes)
//
// The implementation uses DAILY buckets (key "mining_cap_" + YYYYMMDD) to avoid
// scanning thousands of records per transaction: the 365d window sums at most 365 keys.
const (
	// miningCapYear0 is 7B LUG in uluggom (7 × 10^9 × 10^18).
	miningCapYear0 = "7000000000000000000000000000"
	// miningCapDecay is the annual −5% factor.
	miningCapDecay = "0.95"
	// miningCapWindowDays is the sliding window (365 days).
	miningCapWindowDays = 365
)

// MiningCapDailyBuckets tracks uluggom mined per UTC day (key: YYYYMMDD).
type MiningCapDailyBuckets struct {
	// day -> total uluggom mined that day
	Map collections.Map[string, sdkmath.Int]
}

// recordMinedAmount adds amount (uluggom) to today's bucket and enforces the cap.
// Called by the mining keepers (Mine, MineUSD, MineTaxPayment) BEFORE minting.
// Returns an error if the cap would be exceeded (rejects the mining).
func (k Keeper) recordMinedAmount(ctx context.Context, amount sdkmath.Int) error {
	if amount.IsNil() || amount.IsNegative() {
		return fmt.Errorf("mining amount must be non-negative")
	}
	if amount.IsZero() {
		return nil
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	dayKey := sdkCtx.BlockTime().UTC().Format("20060102")

	// Sum the sliding window (last 365 days, including today).
	windowStart := sdkCtx.BlockTime().UTC().AddDate(0, 0, -(miningCapWindowDays - 1))
	startKey := windowStart.Format("20060102")

	var windowTotal sdkmath.Int = sdkmath.ZeroInt()
	// Bound the iteration to at most 365 buckets.
	// Walking the whole map would be expensive; we iterate with a lexicographic prefix
	// [startKey, ...] and filter keys >= startKey.
	err := k.MiningCapDailyBuckets.Walk(sdkCtx, nil, func(key string, val sdkmath.Int) (bool, error) {
		if key >= startKey {
			windowTotal = windowTotal.Add(val)
		}
		return false, nil
	})
	if err != nil {
		return fmt.Errorf("failed to walk mining cap buckets: %w", err)
	}

	capVigente, err := k.miningCapVigente(sdkCtx)
	if err != nil {
		return err
	}

	// Gate: mined_365d + amount <= current_cap; otherwise REJECT (decision §13.41 item 7).
	novoTotal := windowTotal.Add(amount)
	if novoTotal.GT(capVigente) {
		return fmt.Errorf(
			"mining cap exceeded: window=%s + new=%s > cap=%s (annual maximum cap)",
			windowTotal.String(), amount.String(), capVigente.String(),
		)
	}

	// Update today's bucket.
	cur, err := k.MiningCapDailyBuckets.Get(sdkCtx, dayKey)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			return fmt.Errorf("failed to read daily bucket: %w", err)
		}
		cur = sdkmath.ZeroInt()
	}
	if err := k.MiningCapDailyBuckets.Set(sdkCtx, dayKey, cur.Add(amount)); err != nil {
		return fmt.Errorf("failed to set daily bucket: %w", err)
	}

	return nil
}

// miningCapVigente returns the current annual cap: 15B × 0.95^years_since_genesis (uluggom).
// O epoch usa a MESMA âncora do multiplicador da cesta: o bloco 0 (§13.170
// Grupo D — derivada, não um param).
func (k Keeper) miningCapVigente(sdkCtx sdk.Context) (sdkmath.Int, error) {
	blockTime := sdkCtx.BlockTime()
	startTime := k.chainStartTime(sdkCtx)

	years := 0
	if blockTime.After(startTime) {
		elapsed := blockTime.Sub(startTime)
		years = int(elapsed.Hours() / (365 * 24))
	}

	capDec := sdkmath.LegacyMustNewDecFromStr(miningCapYear0)
	decay := sdkmath.LegacyMustNewDecFromStr(miningCapDecay)
	for i := 0; i < years; i++ {
		capDec = capDec.Mul(decay)
	}

	// Truncate to Int (uluggom). The cap decays toward ~0 (asymptotic).
	return capDec.TruncateInt(), nil
}

// --- Wrappers exportados para testes (package keeper_test) ---

// MiningCapVigenteForTest exposes miningCapVigente to external tests.
func (k Keeper) MiningCapVigenteForTest(sdkCtx sdk.Context) (sdkmath.Int, error) {
	return k.miningCapVigente(sdkCtx)
}

// RecordMinedAmountForTest exposes recordMinedAmount to external tests.
func (k Keeper) RecordMinedAmountForTest(ctx context.Context, amount sdkmath.Int) error {
	return k.recordMinedAmount(ctx, amount)
}
