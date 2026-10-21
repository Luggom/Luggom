package app

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	luggomtypes "luggom/x/luggom/types"
)

// ante_strict_filter.go — defesa em profundidade (§13.42 / §13.170).
//
// Depois da classificação A–E do §13.170, a maior parte dos "params imutáveis"
// DEIXOU de existir (apagados ou congelados em constantes de código). O que resta
// de protegido é: `founder_address` (trancado no genesis) e os pesos do tally da
// governança. As listas operacionais (Grupo E) e as chaves rotáveis (Grupo C)
// são governáveis de propósito.
//
// ⚠️ Este filtro só vê `MsgUpdateParams` no TOPO da tx. O freio que vale por
// QUALQUER caminho (incl. uma proposta de gov com a msg embutida — T-GOV-1) é o
// guard do msg_server: `x/luggom/keeper/params_guard.go`.

// protectedParamsFields: params que a governança não pode alterar.
var protectedParamsFields = map[string]bool{
	"founder_address":                    true,
	"gov_validator_min_for_weight_shift": true,
	"gov_llc_vote_weight_bps":            true,
	"gov_founder_vote_weight_bps":        true,
	"gov_validators_vote_weight_bps":     true,
}

// allowedOperationalFields: params que PODEM mudar por MsgUpdateParams (67%).
var allowedOperationalFields = map[string]bool{
	"tax_entity_whitelist":        true,
	"government_entity_blacklist": true,
	"bank_providers":              true,
	"pool_assets":                 true,
	"pool_active":                 true,
	// Grupo C — chaves que rodam de propósito.
	"api_verifier_address":       true,
	"usd_oracle_updater_address": true,
}

// strictFilterAnteHandler rejeita txs cujo MsgUpdateParams toque num campo protegido.
func strictFilterAnteHandler(inner sdk.AnteHandler) sdk.AnteHandler {
	if inner == nil {
		return nil
	}
	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		for _, msg := range tx.GetMsgs() {
			up, ok := msg.(*luggomtypes.MsgUpdateParams)
			if !ok {
				continue
			}
			if err := validateUpdateParamsAllowed(up); err != nil {
				return ctx, err
			}
		}
		return inner(ctx, tx, simulate)
	}
}

func validateUpdateParamsAllowed(up *luggomtypes.MsgUpdateParams) error {
	for field := range protectedParamsFields {
		if allowedOperationalFields[field] {
			continue
		}
		if paramsFieldChanged(up, field) {
			return fmt.Errorf("strict filter: %s is protected — only operational lists and rotatable keys are editable via governance", field)
		}
	}
	return nil
}

func paramsFieldChanged(up *luggomtypes.MsgUpdateParams, field string) bool {
	p := up.Params
	switch field {
	case "founder_address":
		return p.FounderAddress != ""
	case "gov_validator_min_for_weight_shift":
		return p.GovValidatorMinForWeightShift != 0
	case "gov_llc_vote_weight_bps":
		return p.GovLlcVoteWeightBps != 0
	case "gov_founder_vote_weight_bps":
		return p.GovFounderVoteWeightBps != 0
	case "gov_validators_vote_weight_bps":
		return p.GovValidatorsVoteWeightBps != 0
	}
	return false
}
