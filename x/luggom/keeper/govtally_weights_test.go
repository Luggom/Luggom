package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/govtally"
	"luggom/x/luggom/types"
)

// A governança vira por SUPPLY, não pela pool: Fase 1 (<10M) é do Fundador;
// Fase 2 (≥10M) é 1 LUG = 1 voto (pesos 1.0 em todas as classes). O tecto de
// 20% da LBC é aplicado dentro do tally, não por pesos de classe.
func TestGovWeights_PhaseBySupply(t *testing.T) {
	f := initMineFixture(t)

	p := types.DefaultParams()
	p.PoolActive = false
	p.GovLlcVoteWeightBps = 0
	p.GovFounderVoteWeightBps = 10_000
	p.GovValidatorsVoteWeightBps = 0
	require.NoError(t, f.keeper.Params.Set(f.ctx, p))

	// Fase 1: supply < 10M → params (só o Fundador).
	f.bank.supply = sdk.NewCoin("uluggom", sdkmath.ZeroInt())
	wl, wf, wv := govtally.ResolveWeightsForTest(f.ctx, f.keeper, p)
	require.Equal(t, uint32(0), wl, "Fase 1: LBC sem peso")
	require.Equal(t, uint32(10_000), wf, "Fase 1: Fundador decide sozinho")
	require.Equal(t, uint32(0), wv, "Fase 1: validadores a 0")

	// Fase 2: supply ≥ 10M → 1 LUG = 1 voto.
	thr10M, ok := sdkmath.NewIntFromString("10000000000000000000000000") // 10M LUG
	require.True(t, ok)
	f.bank.supply = sdk.NewCoin("uluggom", thr10M)

	// A pool NÃO muda a fase: fechada ou aberta, os pesos são 1.0.
	for _, poolActive := range []bool{false, true} {
		p.PoolActive = poolActive
		require.NoError(t, f.keeper.Params.Set(f.ctx, p))

		wl, wf, wv = govtally.ResolveWeightsForTest(f.ctx, f.keeper, p)
		require.Equal(t, uint32(10_000), wl)
		require.Equal(t, uint32(10_000), wf)
		require.Equal(t, uint32(10_000), wv)
	}
}

// O default dos params TEM de ser o estado de Fase 1 (0/10000/0) — senão uma
// chain inicializada sem o script do genesis arrancava com validadores a 10%.
func TestGovWeights_DefaultsSaoPrePool(t *testing.T) {
	p := types.DefaultParams()
	require.Equal(t, uint32(0), p.GovLlcVoteWeightBps)
	require.Equal(t, uint32(10_000), p.GovFounderVoteWeightBps)
	require.Equal(t, uint32(0), p.GovValidatorsVoteWeightBps)
}
