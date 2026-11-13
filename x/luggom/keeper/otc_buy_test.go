package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

func mustInt(t *testing.T, s string) sdkmath.Int {
	t.Helper()
	v, ok := sdkmath.NewIntFromString(s)
	require.True(t, ok)
	return v
}

func otcBuyFixture(t *testing.T) (*mineFixture, sdkmath.Int, string) {
	t.Helper()
	f := initMineFixture(t)

	params := devParamsForTest()
	params.OtcBtcDenom = "ibc/BTC"
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	snap, err := f.keeper.UsdOracleSnapshot.Get(f.ctx)
	require.NoError(t, err)
	snap.BtcUsd = sdkmath.LegacyNewDec(100_000)
	require.NoError(t, f.keeper.UsdOracleSnapshot.Set(f.ctx, snap))

	lug := mustInt(t, "200000000000000000000000") // 200k LUG
	require.NoError(t, f.keeper.OtcVault.Set(f.ctx, types.OtcVault{AccumulatedLug: lug}))

	buyer, _ := makeAddr(t, f.addressCodec, 0x51)
	return f, lug, buyer
}

func TestBuyOtc_PaysBasketInBtcAndMovesLug(t *testing.T) {
	f, vaultLug, buyer := otcBuyFixture(t)
	buyLug := mustInt(t, "100000000000000000000000") // 100k LUG

	snap, err := f.keeper.UsdOracleSnapshot.Get(f.ctx)
	require.NoError(t, err)
	parity, err := f.keeper.UsdParityFromBasketRatesForTest(f.ctx, snap.ChfUsd, snap.EurUsd, snap.GbpUsd)
	require.NoError(t, err)
	lugDec := sdkmath.LegacyNewDecFromInt(buyLug).QuoInt64(1_000_000_000_000_000_000)
	wantBtc := lugDec.Mul(parity).Quo(snap.BtcUsd).MulInt64(100_000_000).TruncateInt()
	require.True(t, wantBtc.IsPositive())

	resp, err := f.keeper.BuyOtc(f.ctx, &types.MsgBuyOtc{
		Buyer:     buyer,
		LugAmount: buyLug,
		BtcDenom:  "ibc/BTC",
	})
	require.NoError(t, err)
	require.Equal(t, wantBtc.String(), resp.BtcAmount.String())
	require.Equal(t, buyLug.String(), resp.LugAmount.String())

	vault, err := f.keeper.OtcVault.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, vaultLug.Sub(buyLug).String(), vault.AccumulatedLug.String())
	require.Len(t, vault.RetainedAssets, 1)
	require.Equal(t, "ibc/BTC", vault.RetainedAssets[0].Denom)
	require.Equal(t, wantBtc.String(), vault.RetainedAssets[0].Amount.String())

	// O LUG sai do cofre para o comprador.
	buyerAddr, err := f.addressCodec.StringToBytes(buyer)
	require.NoError(t, err)
	require.Equal(t, buyLug.String(), sentToAmount(f.bank, buyerAddr).String())
}

func TestBuyOtc_RejectsBelowLot(t *testing.T) {
	f, _, buyer := otcBuyFixture(t)
	below := types.MinOtcLotLug().SubRaw(1)

	_, err := f.keeper.BuyOtc(f.ctx, &types.MsgBuyOtc{
		Buyer:     buyer,
		LugAmount: below,
		BtcDenom:  "ibc/BTC",
	})
	require.ErrorIs(t, err, types.ErrOtcLotBelowMinimum)
}

func TestBuyOtc_RejectsWrongDenom(t *testing.T) {
	f, _, buyer := otcBuyFixture(t)
	lug := mustInt(t, "100000000000000000000000")

	_, err := f.keeper.BuyOtc(f.ctx, &types.MsgBuyOtc{
		Buyer:     buyer,
		LugAmount: lug,
		BtcDenom:  "ibc/OTHER",
	})
	require.ErrorIs(t, err, types.ErrOtcAssetNotAllowed)
}

func TestBuyOtc_RejectsInsufficientVaultLug(t *testing.T) {
	f, _, buyer := otcBuyFixture(t)
	lug := mustInt(t, "300000000000000000000000") // 300k > 200k

	_, err := f.keeper.BuyOtc(f.ctx, &types.MsgBuyOtc{
		Buyer:     buyer,
		LugAmount: lug,
		BtcDenom:  "ibc/BTC",
	})
	require.ErrorIs(t, err, types.ErrOtcVaultInsufficientLug)
}

func TestBuyOtc_RejectsMissingBtcUsd(t *testing.T) {
	f, _, buyer := otcBuyFixture(t)
	snap, err := f.keeper.UsdOracleSnapshot.Get(f.ctx)
	require.NoError(t, err)
	snap.BtcUsd = sdkmath.LegacyZeroDec()
	require.NoError(t, f.keeper.UsdOracleSnapshot.Set(f.ctx, snap))

	lug := mustInt(t, "100000000000000000000000")
	_, err = f.keeper.BuyOtc(f.ctx, &types.MsgBuyOtc{
		Buyer:     buyer,
		LugAmount: lug,
		BtcDenom:  "ibc/BTC",
	})
	require.ErrorIs(t, err, types.ErrOtcPriceUnavailable)
}
