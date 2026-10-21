package types

import "testing"

// §13.170 A–E: os params de comissão/staleness/flags foram apagados ou
// congelados em código (frozen.go). Este teste cobre o que RESTA.

func TestDefaultParams_GovWeights(t *testing.T) {
	params := DefaultParams()

	if params.GovValidatorMinForWeightShift != 100 {
		t.Fatalf("expected default gov_validator_min_for_weight_shift=100, got %d", params.GovValidatorMinForWeightShift)
	}
	// DECISÃO DO FUNDADOR (2026-09-18): pré-pool o Fundador decide sozinho.
	if params.GovLlcVoteWeightBps != 0 || params.GovFounderVoteWeightBps != 10000 || params.GovValidatorsVoteWeightBps != 0 {
		t.Fatalf("unexpected default gov BPS: %d/%d/%d", params.GovLlcVoteWeightBps, params.GovFounderVoteWeightBps, params.GovValidatorsVoteWeightBps)
	}
	if params.UsdOracleUpdaterAddress != "" {
		t.Fatalf("expected default usd_oracle_updater_address empty")
	}
}

func TestParamsValidate_GovBpsSum(t *testing.T) {
	params := DefaultParams()
	params.GovValidatorsVoteWeightBps = 1
	if err := params.Validate(); err == nil {
		t.Fatalf("expected validation error when gov BPS sum != 10000")
	}
}

func TestFrozenConstants(t *testing.T) {
	if TlsnotaryPhaseFrozen != 2 {
		t.Fatalf("tlsnotary_phase deve estar congelado em 2 (não há fase 1)")
	}
	if !DynamicUsdOracleEnabledFrozen {
		t.Fatalf("o oráculo dinâmico está congelado ligado")
	}
	if UsdOracleMaxStaleSecondsFrozen != 129_600 {
		t.Fatalf("staleness congelada em 36 h")
	}
	if TaxPaymentMaxAgeDaysFrozen != 365 {
		t.Fatalf("janela do comprovante congelada em 365 dias")
	}
}
