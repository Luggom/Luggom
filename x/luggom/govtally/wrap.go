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

		return govkeeper.LuggomCalculateVoteResultsAndVotingPower(
			ctx, k, proposal, validators,
			wlD, wfD, wvD,
			p.FounderAddress,
			"", // §13.170 Grupo A: gov_llc_governance_validator_address FOI APAGADO (morto)
		)
	}
}

// Pesos PÓS-POOL, em basis points (decisão do Fundador, 2026-09-18, sobre o
// §13.84 "validadores em pé de igualdade"):
//
//	Fundador 33% · Validadores 67% · LLC 0% (extinta).
//
// São CONSTANTES COMPILADAS — como a cesta ×3 — para ficarem fora
// do alcance da governança. (Os params homónimos já estão travados pelo filtro
// estrito; aqui a garantia é o próprio código.)
const (
	postPoolLlcBps       = 0
	postPoolFounderBps   = 3_300
	postPoolValidatorBps = 6_700
)

// resolveWeights devolve (LLC, Fundador, Validadores) em basis points.
//
// GATILHO: a **abertura da pool** (`params.pool_active`) — não um marco de
// supply. O §13.114 tornou a pool uma DECISÃO do Fundador e extinguiu o portão
// quantitativo dos 50M do §13.84/§13.81.
//
//   - pool FECHADA → pesos dos **params** (genesis: 0 / 10000 / 0): o Fundador
//     decide sozinho, que é o desenho até ao "dia da virada";
//   - pool ABERTA  → **0 / 3300 / 6700**.
//
// ⚠️ Os pesos são um **MULTIPLICADOR por voto**, não um bolo repartido: um voto
// que não seja do Fundador (nem da LLC) vale o seu LUG em stake × 0,67; o do
// Fundador vale × 0,33. Logo, se só o Fundador votar, ele fica com 100% do
// apurado — quem trava é o turnout + o quorum do x/gov.
//
// ⚠️ BUG CORRIGIDO em 2026-09-18: antes, esta função **ignorava os params** e
// devolvia uma tabela fixa do MVP 1.5 — 90% Fundador / 10% validadores **desde o
// bloco 1**, com escalões que davam peso à **LLC extinta** (15/25/33/12%) e a
// mudança por thresholds de supply (10M/100M/500M/1B). Ou seja: a decisão
// "fundador autónomo até à pool" não estava implementada, e o filtro estrito
// protegia params que ninguém lia.
func resolveWeights(ctx context.Context, k keeper.Keeper, p types.Params) (wl, wf, wv uint32) {
	if k.PoolOpen(ctx) {
		return postPoolLlcBps, postPoolFounderBps, postPoolValidatorBps
	}
	return p.GovLlcVoteWeightBps, p.GovFounderVoteWeightBps, p.GovValidatorsVoteWeightBps
}

// ResolveWeightsForTest expõe resolveWeights a testes fora do pacote.
func ResolveWeightsForTest(ctx context.Context, k keeper.Keeper, p types.Params) (uint32, uint32, uint32) {
	return resolveWeights(ctx, k, p)
}
