package keeper

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

var (
	// MVP 2.0 basket (§13.132, 2026-09-17; ritmo revisto no §13.173):
	// multiplicador 3 desde o ano 0, +10% a.a. composto, sem tecto. Substitui o 6
	// do §13.116 (que por sua vez substituiu o 7 do §13.52). HISTÓRICO:
	// 2 → 7 → 6 → 3 — a decisão do §13.130 (×5) nunca chegou ao código, por isso
	// este salto é 6 → 3; o ritmo de valorização subiu de +5% (§13.132) a +10%
	// (§13.173).
	// LUG = (EUR/USD + GBP/USD + CHF/USD) / 3 × multiplier.
	// É CONSTANTE COMPILADA (não é param) ⇒ a governança não a pode tocar; é
	// isto que torna a regra de venda do cofre (preço = cesta) imutável de facto.
	defaultCHFUSD = sdkmath.LegacyMustNewDecFromStr("1.10")
	defaultEURUSD = sdkmath.LegacyMustNewDecFromStr("1.18")
	defaultGBPUSD = sdkmath.LegacyMustNewDecFromStr("1.37")
	basketMult    = sdkmath.LegacyMustNewDecFromStr("3")

	// Guardrails (§13.48/§13.52): proportional floor 0.75 × multiplier (anti-crash sanity check).
	// maxUSDPerLUG removed (no cap — §13.11/§13.48/§13.51).

	// Per-leg FX sanity (quote currency USD per 1 unit CHF/EUR/GBP).
	minFxLegUSD = sdkmath.LegacyMustNewDecFromStr("0.05")
	maxFxLegUSD = sdkmath.LegacyMustNewDecFromStr("50.00")

	// Annual basket epoch: +10% compound every 365 days (§13.173).
	basketAnnualGrowth = sdkmath.LegacyMustNewDecFromStr("1.10")
	basketYearDuration = 365 * 24 * time.Hour
)

const (
	defaultUsdOracleMaxStaleBlocks = uint64(60)
	// Default TIME-based staleness (§13.56): 36h. Replaces block-counting.
	defaultUsdOracleMaxStaleSeconds = uint64(36 * 60 * 60)
)

// basketMultiplierAt returns the current basket multiplier applying the +10% a.a. compound
// since the chain start (annual epoch, deterministic by timestamp).
// Immutable tokenomics (§13.173): starts at 3, grows 10% compound per year, no cap.
func (k Keeper) basketMultiplierAt(ctx context.Context) (sdkmath.LegacyDec, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	blockTime := sdkCtx.BlockTime()
	// §13.170 Grupo D — a âncora do epoch é derivada (bloco 0), não um param.
	startTime := k.chainStartTime(sdkCtx)

	years := 0
	if blockTime.After(startTime) {
		elapsed := blockTime.Sub(startTime)
		years = int(math.Floor(float64(elapsed) / float64(basketYearDuration)))
	}

	// mult = 3 × 1.10^years (integer power, deterministic)
	mult := basketMult
	for i := 0; i < years; i++ {
		mult = mult.Mul(basketAnnualGrowth)
	}
	return mult, nil
}

// usdParityFromBasketRates computes LUG = (CHF+EUR+GBP)/3 × current multiplier.
// The floor is proportional: 0.75 × multiplier (§13.48/§13.52). No upper cap.
func (k Keeper) usdParityFromBasketRates(ctx context.Context, chfUSD, eurUSD, gbpUSD sdkmath.LegacyDec) (sdkmath.LegacyDec, error) {
	if chfUSD.LTE(sdkmath.LegacyZeroDec()) || eurUSD.LTE(sdkmath.LegacyZeroDec()) || gbpUSD.LTE(sdkmath.LegacyZeroDec()) {
		return sdkmath.LegacyZeroDec(), fmt.Errorf("oracle fx legs must be positive")
	}
	mult, err := k.basketMultiplierAt(ctx)
	if err != nil {
		return sdkmath.LegacyZeroDec(), err
	}
	avg := chfUSD.Add(eurUSD).Add(gbpUSD).QuoInt64(3)
	parity := avg.Mul(mult)

	// Anti-crash sanity check: floor 0.75 × multiplier (tracks the multiplier).
	minParity := sdkmath.LegacyMustNewDecFromStr("0.75").Mul(mult)
	if parity.LT(minParity) {
		return sdkmath.LegacyZeroDec(), fmt.Errorf(
			"computed usd parity below floor: got=%s min=%s",
			parity.String(),
			minParity.String(),
		)
	}
	return parity, nil
}

// staticUSDParityFallback returns the C1 semi-static parity using current multiplier.
func (k Keeper) staticUSDParityFallback(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.usdParityFromBasketRates(ctx, defaultCHFUSD, defaultEURUSD, defaultGBPUSD)
}

// computeUSDParityFromBasket is the legacy C1 entrypoint (tests).
func (k Keeper) computeUSDParityFromBasket(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.staticUSDParityFallback(ctx)
}

// --- Wrappers exportados para testes (package keeper_test) ---

// ComputeUSDParityFromBasketForTest exposes computeUSDParityFromBasket to external tests.
func (k Keeper) ComputeUSDParityFromBasketForTest(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.computeUSDParityFromBasket(ctx)
}

// BasketMultiplierAtForTest exposes basketMultiplierAt to external tests.
func (k Keeper) BasketMultiplierAtForTest(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.basketMultiplierAt(ctx)
}

// UsdParityFromBasketRatesForTest exposes usdParityFromBasketRates to external tests.
func (k Keeper) UsdParityFromBasketRatesForTest(ctx context.Context, chfUSD, eurUSD, gbpUSD sdkmath.LegacyDec) (sdkmath.LegacyDec, error) {
	return k.usdParityFromBasketRates(ctx, chfUSD, eurUSD, gbpUSD)
}

func validateOracleFxLeg(label string, d sdkmath.LegacyDec) error {
	if d.IsNil() || d.LTE(sdkmath.LegacyZeroDec()) {
		return fmt.Errorf("%s must be positive", label)
	}
	if d.LT(minFxLegUSD) || d.GT(maxFxLegUSD) {
		return fmt.Errorf("%s out of bounds [%s,%s]", label, minFxLegUSD.String(), maxFxLegUSD.String())
	}
	return nil
}

// usdParityForMineUSD resolves USD/LUG parity for the mining path.
//
// §13.28 / §6b / §13.56: com o oráculo dinâmico LIGADO **não há fallback para a
// cesta fixa**. Snapshot ausente ou stale (>36 h) ⇒ **ERRO** ⇒ a mineração
// **PÁRA** (fail-closed) até o feeder repor o snapshot ou a governança intervir.
//
// O fallback estático C1 só existe com `DynamicUsdOracleEnabled=false` (dev/
// teste) — **nunca** é um caminho de produção. Antes desta mudança o código caía
// no C1 quando o snapshot faltava ou estava stale, contradizendo a decisão (e
// deixando a mineração correr sobre uma cesta hardcoded que ninguém podia parar).
func (k Keeper) usdParityForMineUSD(ctx context.Context) (sdkmath.LegacyDec, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	snap, err := k.UsdOracleSnapshot.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return sdkmath.LegacyZeroDec(), fmt.Errorf(
				"usd oracle has no snapshot: mining is PAUSED until the feeder commits one (§13.28/§6b)")
		}
		return sdkmath.LegacyZeroDec(), err
	}

	// §13.170 Grupo B — congelado em código (36 h).
	maxStaleSecs := types.UsdOracleMaxStaleSecondsFrozen

	// Staleness por tempo: compara BlockTime com UpdatedAt do snapshot.
	if snap.UpdatedAt > 0 {
		updatedAt := time.Unix(snap.UpdatedAt, 0)
		if sdkCtx.BlockTime().Sub(updatedAt) > time.Duration(maxStaleSecs)*time.Second {
			return sdkmath.LegacyZeroDec(), fmt.Errorf(
				"usd oracle snapshot is stale (older than %ds): mining is PAUSED until the feeder refreshes it (§13.56)",
				maxStaleSecs)
		}
	} else if sdkCtx.BlockHeight()-snap.BlockHeight > int64(defaultUsdOracleMaxStaleBlocks) {
		// Snapshots antigos sem UpdatedAt (antes da Fase 1).
		return sdkmath.LegacyZeroDec(), fmt.Errorf(
			"usd oracle snapshot is stale (older than %d blocks): mining is PAUSED (§13.56)",
			defaultUsdOracleMaxStaleBlocks)
	}

	return k.usdParityFromBasketRates(ctx, snap.ChfUsd, snap.EurUsd, snap.GbpUsd)
}
