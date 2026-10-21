package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Testes de regressão do hard test local (Bloco 1 — API).
// Regra do §13.73: cada achado vira caso de teste ANTES de avançar.

// A-1 — `/api/v1/send_mnemonic` aceitava uma seed BIP39 sem autenticação.
// Foi REMOVIDO: a rota tem de responder 404 (rota inexistente).
func TestHardTest_SendMnemonicRouteRemoved(t *testing.T) {
	mux := http.NewServeMux()
	registerRoutes(mux, &Server{})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/send_mnemonic", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("/api/v1/send_mnemonic devia ser 404 (removida), veio %d", rec.Code)
	}

	// Controlo: uma rota viva continua a responder (o mux está funcional).
	recOK := httptest.NewRecorder()
	mux.ServeHTTP(recOK, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recOK.Code != http.StatusOK {
		t.Fatalf("/health devia ser 200, veio %d", recOK.Code)
	}
}

// A-3 — com a lab do escrow desligada, a resposta tem de ser 404 (rota
// inexistente), não 403 (que confirmava a existência da superfície).
func TestHardTest_EscrowLabDisabledReturns404(t *testing.T) {
	srv := &Server{} // cfg zero ⇒ EscrowLab=false

	for _, path := range []string{
		"/api/v1/escrow/lab/status",
		"/api/v1/escrow/lab/offers",
	} {
		rec := httptest.NewRecorder()
		if path == "/api/v1/escrow/lab/status" {
			srv.handleEscrowLabStatus(rec, httptest.NewRequest(http.MethodGet, path, nil))
		} else {
			srv.handleEscrowLabOffersList(rec, httptest.NewRequest(http.MethodGet, path, nil))
		}
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s devia ser 404 (lab desligada), veio %d", path, rec.Code)
		}
	}
}

// A-4 — fail-closed: se a query on-chain da whitelist falhar, a API RECUSA.
// (a) `onChainBankProviders` devolve erro em vez de uma lista por omissão;
// (b) `/reclaim/providers` responde 503 em vez da lista completa hardcoded.
func TestHardTest_OnChainBankProviders_NoFallback(t *testing.T) {
	srv := &Server{cfg: Config{Bin: "/nonexistent-luggomd", Node: "tcp://127.0.0.1:1"}}

	if _, err := srv.onChainBankProviders(); err == nil {
		t.Fatal("onChainBankProviders devia devolver erro (sem fallback)")
	}

	rec := httptest.NewRecorder()
	srv.handleReclaimProviders(rec, httptest.NewRequest(http.MethodGet, "/api/v1/reclaim/providers", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/reclaim/providers com a chain inacessível devia ser 503 (fail-closed), veio %d", rec.Code)
	}
}

// T-API-6 — rate-limit de ativações: 5 por hora por IP (anti-spam de dust).
// A ativação só corre DEPOIS de a prova TLSNotary validar (main.go:1286), logo o
// abuso do dust está preso a uma prova real; aqui fecha-se o LIMITE em si.
func TestHardTest_ActivationRateLimit_PerIP(t *testing.T) {
	const ip = "203.0.113.77" // TEST-NET-3, único para este teste
	for i := 1; i <= activationMaxPerIPPerHour; i++ {
		if !activationRateAllowed(ip) {
			t.Fatalf("ativação %d devia ser permitida (limite=%d)", i, activationMaxPerIPPerHour)
		}
	}
	if activationRateAllowed(ip) {
		t.Fatalf("a ativação %d devia ser RECUSADA (limite %d/h por IP)",
			activationMaxPerIPPerHour+1, activationMaxPerIPPerHour)
	}
	if !activationRateAllowed("203.0.113.78") {
		t.Fatalf("outro IP tem de ter o seu próprio balde")
	}
}

// T-API-6 (observação de higiene) — `clientIPFromRequest` confia em
// `X-Forwarded-For` sem lista de proxies de confiança: quem fala DIRECTAMENTE com
// a API pode falsificar o IP e contornar o limite por IP. Impacto limitado (cada
// ativação exige uma prova real e é 1× por endereço), mas convém fixar o XFF
// quando a API ficar atrás de um proxy na testnet.
func TestHardTest_ClientIP_TrustsXForwardedFor(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mine/tax", nil)
	req.RemoteAddr = "198.51.100.9:1234"
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 10.0.0.1")
	if got := clientIPFromRequest(req); got != "1.2.3.4" {
		t.Fatalf("esperava o IP do XFF (1.2.3.4), veio %q", got)
	}
}
