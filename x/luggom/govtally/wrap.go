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
		wl, wf, wv := resolveWeights(sdkCtx, *lk, p)
		wlD := math.LegacyNewDecFromInt(math.NewIntFromUint64(uint64(wl))).QuoInt64(10_000)
		wfD := math.LegacyNewDecFromInt(math.NewIntFromUint64(uint64(wf))).QuoInt64(10_000)
		wvD := math.LegacyNewDecFromInt(math.NewIntFromUint64(uint64(wv))).QuoInt64(10_000)

		// Fase 2: a LBC entra no tally pelo endereço compilado, com o tecto de
		// 20% aplicado dentro do tally. Na Fase 1 fica vazia (só o Fundador vota).
		llcAddr := ""
		if lk.DAOActive(ctx) {
			llcAddr = types.BusinessCoreAddress
		}

		return govkeeper.LuggomCalculateVoteResultsAndVotingPower(
			ctx, k, proposal, validators,
			wlD, wfD, wvD,
			p.FounderAddress,
			llcAddr,
		)
	}
}

// A Fase 2 usa 1 LUG = 1 voto (pesos 1.0 no tally). O tecto de 20% da LBC é
// aplicado dentro do tally do x/gov, não por pesos de classe.
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
