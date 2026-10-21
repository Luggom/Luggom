package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

// TestComputeUSDParityFromBasket verifies the MVP 2.0 basket parity at year 0.
// §13.132: multiplicador 3 ⇒ (1.10 + 1.18 + 1.37) / 3 = 1.216666... × 3 = 3.649999...
func TestComputeUSDParityFromBasket(t *testing.T) {
	f := initMineFixture(t)
	parity, err := f.keeper.ComputeUSDParityFromBasketForTest(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "3.649999999999999998", parity.String())
}

// TestBasketMultiplierGrowth verifies the +10% a.a. compound on the multiplier (§13.173).
func TestBasketMultiplierGrowth(t *testing.T) {
	f := initMineFixture(t)

	// §13.170 Grupo D: a âncora do epoch é derivada (bloco 0) — já não há param.
	mult, err := f.keeper.BasketMultiplierAtForTest(f.ctx)
	require.NoError(t, err)
	require.Equal(t, "3.000000000000000000", mult.String())
}

// TestBasketMultiplierFloor verifies the proportional floor 0.75 × multiplier.
func TestBasketMultiplierFloor(t *testing.T) {
	f := initMineFixture(t)

	_, err := f.keeper.UsdParityFromBasketRatesForTest(f.ctx,
		sdkmath.LegacyMustNewDecFromStr("0.01"),
		sdkmath.LegacyMustNewDecFromStr("0.01"),
		sdkmath.LegacyMustNewDecFromStr("0.01"),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "below floor")
}
