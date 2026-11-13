package govtally

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// NewWeightedTallyFn fornece o hook x/gov (CalculateVoteResultsAndVotingPowerFn).
func NewWeightedTallyFn(lk *keeper.Keeper) govkeeper.CalculateVoteResultsAndVotingPowerFn {
	return func(
		ctx context.Context,
		k govkeeper.Keeper,
		proposal v1.Proposal,
		validators map[string]v1.ValidatorGovInfo,
	) (math.LegacyDec, map[v1.VoteOption]math.LegacyDec, error) {
		sdkCtx := sdk.UnwrapSDKContext(ctx)
		p, err := lk.Params.Get(sdkCtx)
		if err != nil {
			return math.LegacyZeroDec(), nil, err
		}
		_, wf, wv := resolveWeights(sdkCtx, *lk, p)
		wfD := math.LegacyNewDecFromInt(math.NewIntFromUint64(uint64(wf))).QuoInt64(10_000)
		wvD := math.LegacyNewDecFromInt(math.NewIntFromUint64(uint64(wv))).QuoInt64(10_000)

		// Fase 2: 1 LUG = 1 voto para todos. A LBC não tem tecto nem regra
		// especial de voto: é um participante como qualquer outro. Na Fase 1
		// só o Fundador tem peso (wf=10000, wv=0).
		return govkeeper.LuggomCalculateVoteResultsAndVotingPower(
			ctx, k, proposal, validators,
			wfD, wvD,
			p.FounderAddress,
		)
	}
}

// A Fase 2 usa 1 LUG = 1 voto (pesos 1.0 no tally). A LBC não tem regra
// especial; só a Fase 1 usa pesos por classe (Fundador 100%).
func resolveWeights(ctx context.Context, k keeper.Keeper, p types.Params) (wl, wf, wv uint32) {
	if k.DAOActive(ctx) {
		return 10_000, 10_000, 10_000
	}
	return p.GovLlcVoteWeightBps, p.GovFounderVoteWeightBps, p.GovValidatorsVoteWeightBps
}

// ResolveWeightsForTest expõe resolveWeights a testes fora do pacote.
func ResolveWeightsForTest(ctx context.Context, k keeper.Keeper, p types.Params) (uint32, uint32, uint32) {
	return resolveWeights(ctx, k, p)
}
