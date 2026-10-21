package reclaimverify

import (
	"fmt"
	"strings"
)

// ProviderInfo describes a supported bank/provider for TLSNotary proofs.
type ProviderInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Country string `json:"country"`
	// Domain é o domínio raiz VÁLIDO do banco. O `server_name` da prova TEM de ser
	// esse domínio ou um subdomínio dele — senão um atacante poderia provar contra
	// um servidor próprio (com certificado válido dele) e declarar o banco.
	// Vazio = provider ainda não habilitado para produção (fail-closed).
	Domain string `json:"domain"`
}

// KnownProviders returns the list of known banking providers for TLSNotary proof generation.
// Fase 1: fixed list. Future: dynamic registry via Reclaim Protocol / on-chain governance
// (§13.23: whitelist de bancos on-chain governável 67% — MsgAddBankProvider/MsgRemoveBankProvider).
func KnownProviders() []ProviderInfo {
	return []ProviderInfo{
		// EUA (§13.31 — centrar nos maiores bancos)
		{ID: "chase", Name: "Chase", Country: "US", Domain: "chase.com"},
		{ID: "wells_fargo", Name: "Wells Fargo", Country: "US", Domain: "wellsfargo.com"},
		{ID: "bank_of_america", Name: "Bank of America", Country: "US", Domain: "bankofamerica.com"},
		{ID: "citi", Name: "Citi", Country: "US", Domain: "citi.com"},
		{ID: "capital_one", Name: "Capital One", Country: "US", Domain: "capitalone.com"},
		{ID: "us_bank", Name: "US Bank", Country: "US", Domain: "usbank.com"},
		{ID: "pnc", Name: "PNC", Country: "US", Domain: "pnc.com"},

		// Brasil (§13.31 — grandes + cooperativas do interior)
		{ID: "itau", Name: "Itaú", Country: "BR", Domain: "itau.com.br"},
		{ID: "bradesco", Name: "Bradesco", Country: "BR", Domain: "bradesco.com.br"},
		{ID: "banco_do_brasil", Name: "Banco do Brasil", Country: "BR", Domain: "bb.com.br"},
		{ID: "santander", Name: "Santander", Country: "BR", Domain: "santander.com.br"},
		{ID: "sicredi", Name: "Sicredi", Country: "BR", Domain: "sicredi.com.br"},
		{ID: "viacredi", Name: "Viacredi", Country: "BR", Domain: "viacredi.coop.br"},
		{ID: "sicoob", Name: "Sicoob", Country: "BR", Domain: "sicoob.com.br"},
		{ID: "inter", Name: "Inter", Country: "BR", Domain: "bancointer.com.br"},
		{ID: "caixa", Name: "Caixa Econômica Federal", Country: "BR", Domain: "caixa.gov.br"},

		// Canadá (MVP 2.0 BCA — Big Five; §13.31)
		{ID: "rbc", Name: "RBC", Country: "CA", Domain: "rbcroyalbank.com"},
		{ID: "td", Name: "TD Bank", Country: "CA", Domain: "td.com"},
		{ID: "scotiabank", Name: "Scotiabank", Country: "CA", Domain: "scotiabank.com"},
		{ID: "bmo", Name: "BMO", Country: "CA", Domain: "bmo.com"},
		{ID: "cibc", Name: "CIBC", Country: "CA", Domain: "cibc.com"},
	}
}

// ProviderHostAllowed reports whether `serverName` (o server_name atestado na
// prova TLSNotary) pertence ao banco do provider: precisa ser o domínio raiz do
// banco OU um subdomínio dele.
//
// Fail-closed: provider desconhecido, sem domínio configurado, ou server_name
// vazio => NÃO permite. Já **NÃO há escape por variável de ambiente** (§13.145,
// A10): uma mainnet mal configurada não pode ser convencida a aceitar a máquina
// de outra pessoa.
func ProviderHostAllowed(providerID, serverName string) error {
	host := strings.ToLower(strings.TrimSpace(serverName))
	host = strings.TrimSuffix(host, ".")
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		return fmt.Errorf("proof has no server_name")
	}
	pid := strings.ToLower(strings.TrimSpace(providerID))
	for _, p := range KnownProviders() {
		if p.ID != pid {
			continue
		}
		if p.Domain == "" {
			return fmt.Errorf("provider %q has no domain configured (fail-closed); refusing", providerID)
		}
		d := strings.ToLower(p.Domain)
		if host == d || strings.HasSuffix(host, "."+d) {
			return nil
		}
		return fmt.Errorf("proof server_name %q does not belong to provider %q (expected %s)", serverName, providerID, p.Domain)
	}
	return fmt.Errorf("unknown provider %q", providerID)
}
