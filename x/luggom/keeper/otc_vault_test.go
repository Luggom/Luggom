package keeper_test

import (
	"fmt"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// TestMiningSplitInvariantSumsTo100Percent verifica o split único e permanente:
// nenhuma fatia se perde nem se cria.
func TestMiningSplitInvariantSumsTo100Percent(t *testing.T) {
	total, ok := sdkmath.NewIntFromString("100000000000000000000") // 100 LUG
	require.True(t, ok)

	s := keeper.ComputeMiningSplitForTest(total)
	require.Equal(t, total.String(), s.Total().String())
	require.Equal(t, "30000000000000000000", s.Miner.String())       // 30%
	require.Equal(t, "5000000000000000000", s.BusinessCore.String()) // 5%
	require.Equal(t, "40000000000000000000", s.OtcVault.String())    // 40%
	require.Equal(t, "7500000000000000000", s.Staking.String())      // 7,5%
	require.Equal(t, "7500000000000000000", s.Atom.String())         // 7,5%
	require.Equal(t, "5000000000000000000", s.Founder.String())      // 5%
	require.Equal(t, "3000000000000000000", s.Validator.String())    // 3%
	require.Equal(t, "1500000000000000000", s.GovReserve.String())   // 1,5%
	require.Equal(t, "500000000000000000", s.NotaryReserve.String()) // 0,5%
	require.Equal(t, "0", s.Pool.String())                           // sem dotação
}

// TestDAOActiveThreshold garante que a governança só abre em ≥10M, sem mexer no split.
func TestDAOActiveThreshold(t *testing.T) {
	thr10M, ok := sdkmath.NewIntFromString("10000000000000000000000000") // 10M LUG
	require.True(t, ok)

	f := initMineFixture(t)
	f.bank.supply = sdk.NewCoin("uluggom", sdkmath.ZeroInt())
	require.False(t, f.keeper.DAOActive(f.ctx))

	f.bank.supply = sdk.NewCoin("uluggom", thr10M)
	require.True(t, f.keeper.DAOActive(f.ctx))
}

// TestOtcVaultStateRoundTrip garante que o estado do cofre (LUG acumulado +
// lastro BTC) sobrevive ao codec da store.
func TestOtcVaultStateRoundTrip(t *testing.T) {
	f := initMineFixture(t)

	want := types.OtcVault{
		AccumulatedLug: sdkmath.NewInt(123_456),
		RetainedAssets: []types.OtcRetainedAsset{
			{Denom: "btc", Amount: sdkmath.NewInt(789)},
		},
	}
	require.NoError(t, f.keeper.OtcVault.Set(f.ctx, want))

	got, err := f.keeper.OtcVault.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, want.AccumulatedLug.String(), got.AccumulatedLug.String())
	require.Len(t, got.RetainedAssets, 1)
	require.Equal(t, "btc", got.RetainedAssets[0].Denom)
	require.Equal(t, "789", got.RetainedAssets[0].Amount.String())
}

// TestSellOtcVaultBatchFailClosesUntilIbc valida as regras da venda do cofre
// (§13.117): lote mínimo 100k LUG e só BTC. A liquidação fica desligada até o IBC
// abrir.
func TestSellOtcVaultBatchFailClosesUntilIbc(t *testing.T) {
	f := initMineFixture(t)

	minLot := types.MinOtcLotLug()
	belowMin := minLot.SubRaw(1)

	require.ErrorIs(t, f.keeper.SellOtcVaultBatch(f.ctx, "btc", belowMin), types.ErrOtcLotBelowMinimum)
	require.ErrorIs(t, f.keeper.SellOtcVaultBatch(f.ctx, "paxg", minLot), types.ErrOtcAssetNotAllowed)
	require.ErrorIs(t, f.keeper.SellOtcVaultBatch(f.ctx, "btc", minLot), types.ErrOtcIbcNotOpen)
}

// TestMineContinuesFeedingOtcVaultWithoutCap verifica que cada mineração
// alimenta o cofre com 40% e não há marco que pare a acumulação.
func TestMineContinuesFeedingOtcVaultWithoutCap(t *testing.T) {
	f := initMineFixture(t)

	params := devParamsForTest()
	params.PoolActive = false
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	minerStr, _ := makeAddr(t, f.addressCodec, 0x13)
	for i := 0; i < 2; i++ {
		_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, fmt.Sprintf("otc-%d", i), 100))
		require.NoError(t, err, "mining %d", i)
	}

	want := sdkmath.ZeroInt()
	for _, m := range f.bank.mints {
		want = want.Add(m.coins.AmountOf("uluggom").MulRaw(4000).QuoRaw(10_000))
	}
	got, err := f.keeper.OtcVaultAccumulated(f.ctx)
	require.NoError(t, err)
	require.Equal(t, want.String(), got.String())
}
