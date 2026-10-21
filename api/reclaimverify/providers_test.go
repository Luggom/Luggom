package reclaimverify

import "testing"

// T-APP-2 — o `server_name` atestado tem de pertencer ao domínio do banco.
// É esta a defesa de identidade do servidor no desktop (não há pinning no
// cliente): o notário valida a cadeia de certificados (webpki/Mozilla) e a API
// exige que o host atestado caia no domínio do provider. Um servidor próprio do
// atacante declara o banco mas é recusado aqui.
func TestProviderHostAllowed(t *testing.T) {
	allowed := []struct{ provider, host string }{
		{"viacredi", "contaonline.viacredi.coop.br"},
		{"viacredi", "viacredi.coop.br"},
		{"viacredi", "VIACREDI.COOP.BR"},              // case-insensitive
		{"viacredi", "contaonline.viacredi.coop.br."}, // ponto final
		{"viacredi", "viacredi.coop.br:443"},          // porta
		{"caixa", "internetbanking.caixa.gov.br"},
	}
	for _, c := range allowed {
		if err := ProviderHostAllowed(c.provider, c.host); err != nil {
			t.Errorf("%s + %q devia ser PERMITIDO, veio: %v", c.provider, c.host, err)
		}
	}

	rejected := []struct{ provider, host, why string }{
		{"viacredi", "", "host vazio"},
		{"viacredi", "evil.com", "outro dominio"},
		{"viacredi", "viacredi.coop.br.evil.com", "sufixo forjado"},
		{"viacredi", "evilviacredi.coop.br", "prefixo colado ao dominio"},
		{"viacredi", "viacredi.coop.br.attacker.net", "dominio do atacante"},
		{"viacredi", "contaonline.viacredi.coop.br.evil.io", "subdominio falso"},
		{"nubank", "nubank.com.br", "provider desconhecido (fail-closed)"},
	}
	for _, c := range rejected {
		if err := ProviderHostAllowed(c.provider, c.host); err == nil {
			t.Errorf("%s + %q devia ser RECUSADO (%s)", c.provider, c.host, c.why)
		}
	}
}
