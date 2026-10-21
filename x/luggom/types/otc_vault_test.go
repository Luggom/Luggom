package types

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

// TestValidateOtcSale valida as regras da venda do cofre: lote mínimo de 10k LUG
// e apenas BTC (§13.117). O PAXG foi descartado (claim regulado com freeze).
func TestValidateOtcSale(t *testing.T) {
	minLot := MinOtcLotLug()

	require.NoError(t, ValidateOtcSale("btc", minLot))
	require.NoError(t, ValidateOtcSale("btc", minLot.MulRaw(2)))

	require.ErrorIs(t, ValidateOtcSale("btc", minLot.SubRaw(1)), ErrOtcLotBelowMinimum)
	require.ErrorIs(t, ValidateOtcSale("btc", sdkmath.ZeroInt()), ErrOtcLotBelowMinimum)
	require.ErrorIs(t, ValidateOtcSale("btc", sdkmath.Int{}), ErrOtcLotBelowMinimum)

	require.ErrorIs(t, ValidateOtcSale("paxg", minLot), ErrOtcAssetNotAllowed)
	require.ErrorIs(t, ValidateOtcSale("eth", minLot), ErrOtcAssetNotAllowed)
	require.ErrorIs(t, ValidateOtcSale("uluggom", minLot), ErrOtcAssetNotAllowed)
	require.ErrorIs(t, ValidateOtcSale("", minLot), ErrOtcAssetNotAllowed)
}

func TestOtcAllowedAssets(t *testing.T) {
	require.True(t, IsOtcAllowedAsset("btc"))
	require.False(t, IsOtcAllowedAsset("paxg"))
	require.False(t, IsOtcAllowedAsset("eth"))

	// A lista devolvida é uma cópia (não expõe o estado interno).
	got := OtcAllowedAssets()
	require.Len(t, got, 1)
	got[0] = "mutated"
	require.Equal(t, "btc", OtcAllowedAssets()[0])
}

func TestMinOtcLotLugIs100k(t *testing.T) {
	want, ok := sdkmath.NewIntFromString("100000000000000000000000") // 100_000 × 10^18
	require.True(t, ok)
	require.Equal(t, want.String(), MinOtcLotLug().String())
}
