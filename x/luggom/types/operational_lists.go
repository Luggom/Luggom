package types

import "strings"

// operational_lists.go — Defaults for governance-editable operational lists (FASE 5).
// These are params editable by 67% governance (§13.42) — NEVER tokenomics.

// DefaultBankProviders returns the default TLSNotary bank provider IDs (on-chain
// whitelist). Mirrors api/reclaimverify/providers.go — the supported banks
// (FASE 5.2 decision: keep the current set, Fundador 2026-08-28).
func DefaultBankProviders() []string {
	return []string{
		// EUA (7)
		"chase", "wells_fargo", "bank_of_america", "citi", "capital_one", "us_bank", "pnc",
		// Brasil (9) — Nubank/C6 removidos (app-only, sem web via TLSNotary, §13.101/§13.102);
		// +caixa adicionado (muito usado no Brasil, Fundador 2026-09-02, §13.106).
		"itau", "bradesco", "banco_do_brasil", "santander",
		"sicredi", "viacredi", "sicoob", "inter", "caixa",
		// Canadá (5) — Big Five
		"rbc", "td", "scotiabank", "bmo", "cibc",
	}
}

// DefaultPoolAssets returns the default whitelist of pool assets (denoms).
// Pool final = BTC, ATOM, USDT, USDC, XMR (§13.24/§13.32 — XMR imprescindível).
func DefaultPoolAssets() []string {
	return []string{"btc", "atom", "usdt", "usdc", "xmr"}
}

// IsBankProviderWhitelisted indica se o banco está na whitelist ON-CHAIN
// (governável por 67%, §13.42). Fail-closed: id vazio ou lista sem ele ⇒ false.
//
// Existe porque a verificação tem de ser feita na CHAIN, não só na API: uma API
// comprometida (ou um cliente que fale directamente com a chain) não pode
// escolher o banco — §13.131, achado A4.
func IsBankProviderWhitelisted(providerID string, list []string) bool {
	id := strings.TrimSpace(providerID)
	if id == "" {
		return false
	}
	for _, p := range list {
		if strings.TrimSpace(p) == id {
			return true
		}
	}
	return false
}
