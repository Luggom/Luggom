package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"luggom/x/luggom/govtally"
	"luggom/x/luggom/types"
)

// A governança "acorda" quando a POOL abre (§13.84 + decisão do Fundador,
// 2026-09-18): antes disso o Fundador decide sozinho; depois, Fundador 33% e
// validadores 67%.
//
// Este teste tranca o BUG corrigido: antes, `resolveWeights` **ignorava os
// params** e devolvia uma tabela fixa do MVP 1.5 — 90% Fundador / 10%
// validadores **desde o bloco 1**, com escalões que davam peso à LLC extinta e
// mudança por thresholds de supply.
//
// Semântica (importante): os pesos são um **multiplicador por voto**, não um
// bolo repartido — quem não é o Fundador vale o seu stake × 0,67; o Fundador
// × 0,33. Um teste unitário não prova o apuramento (isso é o Bloco 4 da FASE 8).
func TestGovWeights_AcordamComAPool(t *testing.T) {
	f := initMineFixture(t)

	// Como o genesis: Fundador autónomo antes da pool.
	p := types.DefaultParams()
	p.PoolActive = false
	p.GovLlcVoteWeightBps = 0
	p.GovFounderVoteWeightBps = 10_000
	p.GovValidatorsVoteWeightBps = 0
	require.NoError(t, f.keeper.Params.Set(f.ctx, p))

	wl, wf, wv := govtally.ResolveWeightsForTest(f.ctx, f.keeper, p)
	require.Equal(t, uint32(0), wl, "LLC extinta — nunca tem peso")
	require.Equal(t, uint32(10_000), wf, "pré-pool: Fundador decide sozinho")
	require.Equal(t, uint32(0), wv, "pré-pool: validadores a 0")

	// Pool aberta ⇒ "dia da virada" (§13.84).
	p.PoolActive = true
	require.NoError(t, f.keeper.Params.Set(f.ctx, p))

	wl, wf, wv = govtally.ResolveWeightsForTest(f.ctx, f.keeper, p)
	require.Equal(t, uint32(0), wl)
	require.Equal(t, uint32(3_300), wf, "pós-pool: Fundador 33%")
	require.Equal(t, uint32(6_700), wv, "pós-pool: validadores 67%")
	require.Equal(t, uint32(10_000), wl+wf+wv, "os pesos somam sempre 10000")
}

// O default dos params TEM de ser o estado pré-pool (0/10000/0) — senão uma
// chain inicializada sem o script do genesis arrancava com validadores a 10%.
func TestGovWeights_DefaultsSaoPrePool(t *testing.T) {
	p := types.DefaultParams()
	require.Equal(t, uint32(0), p.GovLlcVoteWeightBps)
	require.Equal(t, uint32(10_000), p.GovFounderVoteWeightBps)
	require.Equal(t, uint32(0), p.GovValidatorsVoteWeightBps)
}
