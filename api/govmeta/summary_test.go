package govmeta

import (
	"encoding/json"
	"testing"
)

func TestBuildSummary_extractsLuggomFields(t *testing.T) {
	raw := `{"params":{"founder_address":"luggom1abc","pool_active":true}}`
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		t.Fatal(err)
	}
	s := BuildSummary(root, map[string]interface{}{"min_deposit": []interface{}{}}, "")
	if s.Luggom.FounderAddress != "luggom1abc" || !s.Luggom.PoolActive {
		t.Fatalf("fundador/pool: %+v", s.Luggom)
	}
	if s.Policy.ValidatorMinForDilution != ValidatorMinForDilutionGate {
		t.Fatal(s.Policy.ValidatorMinForDilution)
	}
	if s.Policy.VoteWeights0to10M.LlcPercent != 0 || s.Policy.VoteWeights0to10M.FounderPercent != 100 || s.Policy.VoteWeights0to10M.ValidatorsPercent != 0 {
		t.Fatalf("weights: %+v", s.Policy.VoteWeights0to10M)
	}
	if s.CosmosGov == nil {
		t.Fatal("expected cosmos gov")
	}
}

func TestBuildSummary_nilInputs(t *testing.T) {
	s := BuildSummary(nil, nil, "no node")
	if s.Reference != PolicyMVP15 {
		t.Fatal(s.Reference)
	}
	if s.CosmosGovQueryError != "no node" {
		t.Fatal(s.CosmosGovQueryError)
	}
}

func TestWithLuggomQueryError(t *testing.T) {
	s := BuildSummary(nil, nil, "").WithLuggomQueryError("rpc down")
	if s.LuggomParamsQueryError != "rpc down" {
		t.Fatal(s.LuggomParamsQueryError)
	}
}
