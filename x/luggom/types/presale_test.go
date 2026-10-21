package types

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

func TestPreSaleLugForUSDT(t *testing.T) {
	// $0,10/LUG: 1000 USDT (10^9 base) → 10.000 LUG.
	n1000 := sdkmath.NewInt(1_000).MulRaw(1_000_000)
	require.Equal(t, sdkmath.NewInt(10_000).MulRaw(1_000_000_000_000_000_000).String(),
		PreSaleLugForUSDT(n1000).String())

	// 350.000 USDT → o cap inteiro (3,5M LUG).
	n350k := sdkmath.NewInt(350_000).MulRaw(1_000_000)
	require.Equal(t, PreSaleCapLug().String(), PreSaleLugForUSDT(n350k).String())

	// Truncagem para baixo: 1 base de USDT (10^-6 USD) → 10^13 base de LUG.
	require.Equal(t, "10000000000000", PreSaleLugForUSDT(sdkmath.NewInt(1)).String())

	// Não positivo → zero (nunca dá LUG de graça).
	require.True(t, PreSaleLugForUSDT(sdkmath.ZeroInt()).IsZero())
	require.True(t, PreSaleLugForUSDT(sdkmath.NewInt(-5)).IsZero())
}

func TestPreSalePositionVestedAtIsLinear(t *testing.T) {
	p := PreSalePosition{TotalLug: sdkmath.NewInt(1_000), StartTime: 100, EndTime: 200}

	require.True(t, p.VestedAt(50).IsZero(), "antes do início")
	require.True(t, p.VestedAt(100).IsZero(), "no início")
	require.Equal(t, "500", p.VestedAt(150).String(), "a meio")
	require.Equal(t, "1000", p.VestedAt(200).String(), "no fim")
	require.Equal(t, "1000", p.VestedAt(99_999).String(), "depois do fim (nunca acima do total)")
}

func TestPreSaleConstants(t *testing.T) {
	// Constantes compiladas (§13.134 §3): fora do alcance da governança.
	require.True(t, PreSalePriceUSD().Equal(sdkmath.LegacyMustNewDecFromStr("0.10")))
	require.Equal(t, sdkmath.NewInt(3_500_000).MulRaw(1_000_000_000_000_000_000).String(),
		PreSaleCapLug().String())
	require.Equal(t, int64(90*24*60*60), PreSaleVestingSeconds())
	require.Equal(t, "usdt", PreSaleUsdtDenom)
}
