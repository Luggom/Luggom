package keeper

import (
	"fmt"

	"luggom/x/luggom/types"
)

// §13.170 §4 — a promessa "imutável" tem de ser verdadeira POR QUALQUER CAMINHO.
//
// O filtro estrito do ante só inspecciona `MsgUpdateParams` no TOPO da tx; o
// x/gov executa as msgs internas de uma proposta **sem ante handler** — o T-GOV-1
// provou-o ao vivo (§13.169). Este check vive no **msg_server**, por onde a msg
// passa SEMPRE, e compara com os params ATUAIS (não com os defaults).
//
// Protegidos após o genesis:
//   - `founder_address` — Grupo C: trancado no genesis.
//   - pesos do tally da governança (`gov_*_vote_weight_bps`,
//     `gov_validator_min_for_weight_shift`).
//
// Deliberadamente NÃO bloqueados aqui:
//   - `api_verifier_address`, `usd_oracle_updater_address` — Grupo C: chaves
//     que TÊM de rodar (§13.121; o notário evolui para t-de-n);
//   - `tax_entity_whitelist`, `government_entity_blacklist`, `bank_providers`,
//     `pool_assets`, `pool_active` — Grupo E: listas operacionais a 67%.
func validateUpdateParamsProtected(current, next types.Params) error {
	if next.FounderAddress != current.FounderAddress {
		return fmt.Errorf("strict filter: founder_address is locked at genesis (§13.170 Grupo C) — refusing to change it")
	}
	if next.GovLlcVoteWeightBps != current.GovLlcVoteWeightBps ||
		next.GovFounderVoteWeightBps != current.GovFounderVoteWeightBps ||
		next.GovValidatorsVoteWeightBps != current.GovValidatorsVoteWeightBps ||
		next.GovValidatorMinForWeightShift != current.GovValidatorMinForWeightShift {
		return fmt.Errorf("strict filter: governance tally weights are protected — refusing to change them via UpdateParams")
	}
	return nil
}
