package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"luggom/api/taxproof"
	"luggom/x/luggom/types"
)

// T-API-2 / T-API-7 (lado API). O `bin` da chain e o notário são FALSOS:
// o primeiro devolve a whitelist on-chain pretendida e o segundo a atestação com
// o proof_commitment calculado do próprio bundle. Assim exercita-se a ordem real
// do handler (prova → whitelist → natureza → notário → gov → pubkey) sem sessão.

const t20BundlePath = "../../fixtures/bank-fixture/bundle-T20.json"
const t20Miner2Path = "../../fixtures/bank-fixture/bundle-T20-miner2.json"

func loadT20Request(t *testing.T) MineTaxPaymentRequest {
	t.Helper()
	raw, err := os.ReadFile(t20BundlePath)
	if err != nil {
		t.Skipf("fixture T20 ausente (%v)", err)
	}
	var req MineTaxPaymentRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		t.Fatalf("bundle T20: %v", err)
	}
	return req
}

func t20Bundle(req MineTaxPaymentRequest) taxproof.Bundle {
	return taxproof.Bundle{
		SessionID:    req.TlsnProof.SessionID,
		ServerName:   req.TlsnProof.ServerName,
		Status:       req.TlsnProof.Status,
		ResponseBody: req.TlsnProof.ResponseBody,
		Transcript: taxproof.Transcript{
			Sent: req.TlsnProof.Transcript.Sent,
			Recv: req.TlsnProof.Transcript.Recv,
		},
	}
}

// fakeLuggomdParams escreve um "binário" que responde a `query luggom params`
// com a whitelist pedida — é assim que se controla a whitelist on-chain.
func fakeLuggomdParams(t *testing.T, bankProviders []string) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "luggomd")
	payload, _ := json.Marshal(map[string]any{
		"params": map[string]any{"bank_providers": bankProviders},
	})
	script := "#!/bin/sh\ncat <<'JSONEOF'\n" + string(payload) + "\nJSONEOF\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

func postMineTaxJSON(t *testing.T, s *Server, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mine/tax", bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	s.handleMineTaxPayment(rec, req)
	return rec
}

// T-API-7 (lado API): provider fora da whitelist ON-CHAIN ⇒ 403 (código 1208),
// ANTES de a API construir a tx. Cenário: a governança removeu o banco (T-GOV-3).
func TestHardTest_ApiProviderNotWhitelisted_403(t *testing.T) {
	req := loadT20Request(t)
	if _, _, err := taxproof.VerifyTlsnProof(req.ProviderId, t20Bundle(req)); err != nil {
		t.Skipf("o bundle T20 já não valida localmente (%v) — coberto por unidade", err)
	}
	bin := fakeLuggomdParams(t, []string{"itau", "caixa", "bradesco"}) // sem viacredi
	s := &Server{cfg: Config{Bin: bin, Home: t.TempDir(), Node: "tcp://127.0.0.1:1"}}

	rec := postMineTaxJSON(t, s, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("esperava 403 (provider fora da whitelist), veio %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "1208") {
		t.Fatalf("esperava o código 1208, veio: %s", rec.Body.String())
	}
}

// T-API-2 (lado API): public_key ≠ dest_address ⇒ 400.
func TestHardTest_ApiPublicKeyMismatch_400(t *testing.T) {
	req := loadT20Request(t)
	_, commit, err := taxproof.VerifyTlsnProof(req.ProviderId, t20Bundle(req))
	if err != nil {
		t.Skipf("o bundle T20 já não valida localmente (%v) — coberto por unidade", err)
	}
	bin := fakeLuggomdParams(t, []string{req.ProviderId})

	fields := types.AttestationFields{
		ProofCommitment: commit,
		PaymentID:       "t20",
		PaymentAmount:        "1",
		TaxEntity:       "irs",
		CurrencyCode:    "USD",
		PaymentDate:     1,
		TaxNatureHash:   strings.Repeat("a", 64),
		PayerTaxID:      "12-3456789",
		ProviderID:      req.ProviderId,
	}
	notary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"fields": fields, "signature": "deadbeef"})
	}))
	defer notary.Close()

	s := &Server{cfg: Config{Bin: bin, Home: t.TempDir(), Node: "tcp://127.0.0.1:1", NotaryURL: notary.URL}}

	req.DestAddress = otherDest(t) // destino trocado: a pubkey do bundle já não bate
	rec := postMineTaxJSON(t, s, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("esperava 400 (pubkey ≠ dest_address), veio %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "public_key does not match") {
		t.Fatalf("esperava a mensagem de pubkey, veio: %s", rec.Body.String())
	}
}

func otherDest(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(t20Miner2Path)
	if err != nil {
		t.Skip("fixture miner2 ausente")
	}
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	d, _ := m["dest_address"].(string)
	if d == "" {
		t.Skip("sem dest_address alternativo")
	}
	return d
}
