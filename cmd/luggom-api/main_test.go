package main

import (
	"testing"
)

func TestParsePoolActiveFromQueryParamsJSON(t *testing.T) {
	raw := []byte(`{"params":{"pool_active":false,"whitelist_cnpjs":[]}}`)
	v, atom, err := parsePoolActiveFromQueryParamsJSON(raw)
	if err != nil || v || atom {
		t.Fatalf("got %v atom %v err %v", v, atom, err)
	}
	raw2 := []byte(`{"params":{"poolActive":true}}`)
	v2, _, err := parsePoolActiveFromQueryParamsJSON(raw2)
	if err != nil || !v2 {
		t.Fatalf("got %v err %v", v2, err)
	}
}

func TestRpcHTTPEndpoint(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"tcp://127.0.0.1:26657", "http://127.0.0.1:26657"},
		{"http://10.0.2.2:26657", "http://10.0.2.2:26657"},
		{"", ""},
	}
	for _, tc := range tests {
		if got := rpcHTTPEndpoint(tc.in); got != tc.want {
			t.Fatalf("rpcHTTPEndpoint(%q) = %q want %q", tc.in, got, tc.want)
		}
	}
}

// §13.145 (A5/A10): sem notário, a mineração é RECUSADA — fail-closed.
//
// Antes, sem `LUGGOM_NOTARY_URL` a API assinava sozinha o que ela própria tinha
// extraído de um transcript que o CLIENTE enviou (bytes que o cliente controla):
// a mineração era forjável e bastava uma mainnet mal configurada.
func TestNotaryRequiredError_FailClosed(t *testing.T) {
	if err := notaryRequiredError(""); err == nil {
		t.Error("sem NOTARY_URL devia falhar (fail-closed)")
	}
	if err := notaryRequiredError("   "); err == nil {
		t.Error("NOTARY_URL em branco devia falhar")
	}
	if err := notaryRequiredError("http://127.0.0.1:7047"); err != nil {
		t.Errorf("com notário configurado não devia falhar: %v", err)
	}
}
