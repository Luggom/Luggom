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

// TestMiningSplitInvariantSumsTo100Percent verifica que os 3 regimes (§13.117)
// alocam exactamente o total minerado: nenhuma fatia se perde nem se cria.
func TestMiningSplitInvariantSumsTo100Percent(t *testing.T) {
	total, ok := sdkmath.NewIntFromString("100000000000000000000") // 100 LUG
	require.True(t, ok)

	cases := []struct {
		phase types.OtcPhase
	}{
		{types.OtcPhase1},
		{types.OtcPhaseTransition},
		{types.OtcPhase2},
	}

	for _, tc := range cases {
		t.Run(tc.phase.String(), func(t *testing.T) {
			s := keeper.ComputeMiningSplitForTest(total, tc.phase)
			require.Equal(t, tc.phase, s.Phase)
			require.Equal(t, total.String(), s.Total().String())
			// Founder/validators são constantes em todas as fases. O Gov Reserve
			// e a POOL são POR FASE (§13.132): a POOL só na Fase 1, e o Gov passa a
			// ter 0,5% de notariado na Fase 2 (§13.154) — ver abaixo.
			require.Equal(t, "5000000000000000000", s.Founder.String())   // 5%
			require.Equal(t, "3000000000000000000", s.Validator.String()) // 3%
		})
	}

	// Fase 1 — pré-pool com pre-sale activa. §13.132: 30/35/7,5/7,5/5/3/2/10.
	p1 := keeper.ComputeMiningSplitForTest(total, types.OtcPhase1)
	require.Equal(t, "30000000000000000000", p1.Miner.String())   // 30%
	require.Equal(t, "35000000000000000000", p1.PreSale.String()) // 35%
	require.Equal(t, "0", p1.OtcVault.String())
	require.Equal(t, "7500000000000000000", p1.Staking.String())    // 7,5%
	require.Equal(t, "7500000000000000000", p1.Atom.String())       // 7,5%
	require.Equal(t, "2000000000000000000", p1.GovReserve.String()) // 2%
	require.Equal(t, "10000000000000000000", p1.Pool.String())      // 10% (dotação única)

	// Transição — pre-sale e POOL encerradas; as fatias vão aos reservatórios (30 cada).
	pt := keeper.ComputeMiningSplitForTest(total, types.OtcPhaseTransition)
	require.Equal(t, "30000000000000000000", pt.Miner.String()) // 30%
	require.Equal(t, "0", pt.PreSale.String())
	require.Equal(t, "0", pt.OtcVault.String())
	require.Equal(t, "30000000000000000000", pt.Staking.String())   // 30%
	require.Equal(t, "30000000000000000000", pt.Atom.String())      // 30%
	require.Equal(t, "2000000000000000000", pt.GovReserve.String()) // 2%
	require.Equal(t, "0", pt.Pool.String())                         // §13.132: POOL encerra

	// Fase 2 — definitiva (pool/IBC abertos). §13.154: miner 30 / OTC 40 /
	// tesouraria 1,5 / notariado 0,5 (o §13.132 tinha OTC 42 e Gov 0).
	p2 := keeper.ComputeMiningSplitForTest(total, types.OtcPhase2)
	require.Equal(t, "30000000000000000000", p2.Miner.String()) // 30%
	require.Equal(t, "0", p2.PreSale.String())
	require.Equal(t, "40000000000000000000", p2.OtcVault.String())    // 40% (era 42)
	require.Equal(t, "10000000000000000000", p2.Staking.String())     // 10%
	require.Equal(t, "10000000000000000000", p2.Atom.String())        // 10%
	require.Equal(t, "1500000000000000000", p2.GovReserve.String())   // 1,5% tesouraria
	require.Equal(t, "500000000000000000", p2.NotaryReserve.String()) // 0,5% notariado
	require.Equal(t, "0", p2.Pool.String())                           // §13.132: POOL só na Fase 1
}

// TestMiningPhaseResolution resolve o regime a partir do supply (pre-sale) e do
// flag da pool (§13.117).
func TestMiningPhaseResolution(t *testing.T) {
	thr10M, ok := sdkmath.NewIntFromString("10000000000000000000000000") // 10M LUG
	require.True(t, ok)

	setPoolActive := func(f *mineFixture, active bool) {
		t.Helper()
		params := devParamsForTest()
		params.PoolActive = active
		require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	}

	t.Run("supply <10M → Fase 1", func(t *testing.T) {
		f := initMineFixture(t)
		f.bank.supply = sdk.NewCoin("uluggom", sdkmath.ZeroInt())
		phase, err := f.keeper.MiningPhaseForTest(f.ctx)
		require.NoError(t, err)
		require.Equal(t, types.OtcPhase1, phase)
	})

	t.Run("supply ≥10M e pool fechada → Transição", func(t *testing.T) {
		f := initMineFixture(t)
		f.bank.supply = sdk.NewCoin("uluggom", thr10M)
		setPoolActive(f, false)
		phase, err := f.keeper.MiningPhaseForTest(f.ctx)
		require.NoError(t, err)
		require.Equal(t, types.OtcPhaseTransition, phase)
	})

	t.Run("supply ≥10M e pool aberta → Fase 2", func(t *testing.T) {
		f := initMineFixture(t)
		f.bank.supply = sdk.NewCoin("uluggom", thr10M)
		setPoolActive(f, true)
		phase, err := f.keeper.MiningPhaseForTest(f.ctx)
		require.NoError(t, err)
		require.Equal(t, types.OtcPhase2, phase)
	})
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
// (§13.117): lote mínimo 10k LUG e só BTC. A liquidação fica desligada até o IBC
// abrir.
func TestSellOtcVaultBatchFailClosesUntilIbc(t *testing.T) {
	f := initMineFixture(t)

	minLot := types.MinOtcLotLug()
	belowMin := minLot.SubRaw(1)

	require.ErrorIs(t, f.keeper.SellOtcVaultBatch(f.ctx, "btc", belowMin), types.ErrOtcLotBelowMinimum)
	require.ErrorIs(t, f.keeper.SellOtcVaultBatch(f.ctx, "paxg", minLot), types.ErrOtcAssetNotAllowed)
	require.ErrorIs(t, f.keeper.SellOtcVaultBatch(f.ctx, "btc", minLot), types.ErrOtcIbcNotOpen)
}

// TestMineContinuesFeedingOtcVaultWithoutCap verifica que, na Fase 2, cada
// mineração alimenta o cofre com 30,5% (§13.128) e não há marco que pare a acumulação.
func TestMineContinuesFeedingOtcVaultWithoutCap(t *testing.T) {
	f := initMineFixture(t)

	thr10M, ok := sdkmath.NewIntFromString("10000000000000000000000000") // 10M LUG
	require.True(t, ok)
	f.bank.supply = sdk.NewCoin("uluggom", thr10M)

	params := devParamsForTest()
	// Fase 2: pool/IBC abertos (§13.170: o antigo atom_bonus_active era um
	// no-op neste teste — a fase OTC sai de supply+pool).
	params.PoolActive = true
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	minerStr, _ := makeAddr(t, f.addressCodec, 0x13)
	// §13.146 A: mina-se pelo caminho COM prova (MsgMineTaxPayment).
	for i := 0; i < 2; i++ {
		_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, fmt.Sprintf("otc-%d", i), 100))
		require.NoError(t, err, "mining %d", i)
	}

	// Cada mineração alimenta o cofre com 42% (§13.132); o esperado deriva do
	// total efectivamente cunhado (a paridade LegacyDec deixa os últimos wei
	// oscilar), somando a fatia de CADA mineração (é assim que o código a calcula).
	want := sdkmath.ZeroInt()
	for _, m := range f.bank.mints {
		want = want.Add(m.coins.AmountOf("uluggom").MulRaw(4000).QuoRaw(10_000))
	}
	got, err := f.keeper.OtcVaultAccumulated(f.ctx)
	require.NoError(t, err)
	require.Equal(t, want.String(), got.String())
}
