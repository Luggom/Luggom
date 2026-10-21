// Package govmeta agrega metadados de governança para a Etapa H1 (resumo on-chain via CLI).
package govmeta

import "encoding/json"

// PolicyMVP15 reflecte a governança MVP 2.0 (§13.57/§13.84): fundador autônomo
// até 50M LUG (IBC), depois validadores em igualdade. LLC EXTINTA (2026-06-25).
const PolicyMVP15 = "docs/GOVERNANCE_MVP15.md"

// ValidatorMinForDilutionGate validadores mínimos antes de aplicar peso <100% LLC.
const ValidatorMinForDilutionGate = 100

// VoteWeightsTier0to10M percentuais inteiros (soma 100) para [0, 10M).
// LLC extinta: fundador 100% até 50M (autonomia), validadores 0%.
var VoteWeightsTier0to10M = struct {
	LLC, Founder, Validators int
}{0, 100, 0}

// LuggomGovSummary campos expostos ao JSON da API (H1).
type LuggomGovSummary struct {
	FounderAddress string `json:"fundador_address,omitempty"`
	PoolActive     bool   `json:"pool_active"`
}

// Summary resposta de GET /api/v1/gov/luggom (H1).
type Summary struct {
	Reference              string                 `json:"reference"`
	Luggom                 LuggomGovSummary       `json:"luggom"`
	LuggomParamsQueryError string                 `json:"luggom_params_query_error,omitempty"`
	CosmosGov              map[string]interface{} `json:"cosmos_gov,omitempty"`
	CosmosGovQueryError    string                 `json:"cosmos_gov_query_error,omitempty"`
	Policy                 struct {
		ValidatorMinForDilution int `json:"validator_min_for_dilution"`
		VoteWeights0to10M       struct {
			LlcPercent        int `json:"llc_percent"` // deprecated: LLC extinta — sempre 0
			FounderPercent    int `json:"founder_percent"`
			ValidatorsPercent int `json:"validators_percent"`
		} `json:"vote_weights_0_to_10m_lug_mined"`
	} `json:"policy_mvp15"`
	Notes []string `json:"notes"`
}

// BuildSummary monta o resumo a partir do JSON de `query luggom params` (mapa genérico).
func BuildSummary(luggomParams map[string]interface{}, cosmosGov map[string]interface{}, cosmosErr string) Summary {
	var s Summary
	s.Reference = PolicyMVP15
	s.Policy.ValidatorMinForDilution = ValidatorMinForDilutionGate
	s.Policy.VoteWeights0to10M.LlcPercent = VoteWeightsTier0to10M.LLC
	s.Policy.VoteWeights0to10M.FounderPercent = VoteWeightsTier0to10M.Founder
	s.Policy.VoteWeights0to10M.ValidatorsPercent = VoteWeightsTier0to10M.Validators
	s.Notes = []string{
		"LLC DAO extinta (2026-06-25): sem peso LLC.",
		"Fundador autônomo até 50M LUG (IBC abre); depois governança igual entre validadores (§13.57/§13.84).",
		"Governança on-chain 67% decide listas operacionais (entidades fiscais, bancos, ativos da pool); tokenomics imutável.",
	}
	if p, ok := luggomParams["params"].(map[string]interface{}); ok {
		// §13.170 Grupo B: governance_quorum/veto_threshold deixaram de ser params
		// do módulo (constantes de código) — já não vêm no JSON da chain.
		if v, ok := p["founder_address"].(string); ok {
			s.Luggom.FounderAddress = v
		} else if v, ok := p["fundador_address"].(string); ok { // compat genesis antigo
			s.Luggom.FounderAddress = v
		}
		if v, ok := p["pool_active"].(bool); ok {
			s.Luggom.PoolActive = v
		}
		if g, ok := uintFromJSON(p["gov_validator_min_for_weight_shift"]); ok && g > 0 {
			s.Policy.ValidatorMinForDilution = int(g)
		}
		if llc, ok := uintFromJSON(p["gov_llc_vote_weight_bps"]); ok {
			s.Policy.VoteWeights0to10M.LlcPercent = bpsToPercent(int(llc))
		}
		if f, ok := uintFromJSON(p["gov_founder_vote_weight_bps"]); ok {
			s.Policy.VoteWeights0to10M.FounderPercent = bpsToPercent(int(f))
		}
		if v, ok := uintFromJSON(p["gov_validators_vote_weight_bps"]); ok {
			s.Policy.VoteWeights0to10M.ValidatorsPercent = bpsToPercent(int(v))
		}
	}
	if cosmosGov != nil {
		s.CosmosGov = cosmosGov
	}
	s.CosmosGovQueryError = cosmosErr
	return s
}

// WithLuggomQueryError anexa erro da query `luggom params` (uso no handler HTTP).
func (s Summary) WithLuggomQueryError(msg string) Summary {
	s.LuggomParamsQueryError = msg
	return s
}

func uintFromJSON(v interface{}) (uint64, bool) {
	switch t := v.(type) {
	case float64:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case int64:
		if t < 0 {
			return 0, false
		}
		return uint64(t), true
	case json.Number:
		n, err := t.Int64()
		if err != nil || n < 0 {
			return 0, false
		}
		return uint64(n), true
	default:
		return 0, false
	}
}

func bpsToPercent(bps int) int {
	return bps / 100
}
