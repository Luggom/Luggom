package taxnature

import (
	"testing"
)

func mustHash(t *testing.T, f Fragment, err error) string {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected extract error: %v", err)
	}
	return Hash(f)
}

func TestCountryByProvider(t *testing.T) {
	cases := map[string]string{
		"itau": "BR", "bradesco": "BR", "banco_do_brasil": "BR", "santander": "BR",
		"sicredi": "BR", "viacredi": "BR", "sicoob": "BR", "inter": "BR",
		"chase": "US", "wells_fargo": "US", "bank_of_america": "US", "citi": "US", "capital_one": "US", "us_bank": "US", "pnc": "US",
		"rbc": "CA", "td": "CA", "scotiabank": "CA", "bmo": "CA", "cibc": "CA",
		"mock_chase": "US", "mock_itau": "BR", "mock_rbc": "CA",
		// Nubank/C6 removidos (app-only, sem web via TLSNotary — §13.101/§13.102)
		"nubank": "", "c6": "",
		"unknown_bank": "",
	}
	for provider, want := range cases {
		if got := Country(provider); got != want {
			t.Errorf("Country(%q) = %q, want %q", provider, got, want)
		}
	}
}

func TestExtractBR_TaxCollection(t *testing.T) {
	// PIX arrecadação (guia de IPTU) with CNPJ of the city hall.
	frag, err := ExtractBR(Claims{
		ProviderID:   "itau",
		Recipient:    "Prefeitura de São Paulo",
		RecipientName: "PIX Arrecadação BR.GOV.BR/AREC IPTU 2026",
		PayerTaxID:   "12.345.678/0001-90",
		Description:  "Pagamento de IPTU via PIX arrecadação",
	})
	h := mustHash(t, frag, err)
	if len(h) != 64 {
		t.Fatalf("hash len = %d, want 64", len(h))
	}
}

func TestExtractBR_RejectsCommonPix(t *testing.T) {
	// Common PIX to the same CNPJ — must be rejected (no tax nature).
	_, err := ExtractBR(Claims{
		ProviderID:  "itau",
		Recipient:   "Prefeitura de São Paulo",
		PayerTaxID:  "12.345.678/0001-90",
		Description: "PIX Copia e Cola transferência comum",
	})
	if err == nil {
		t.Fatal("expected error for common PIX, got nil")
	}
}

func TestExtractBR_AcceptsPixToWhitelistedTaxEntity(t *testing.T) {
	// Comprovante REAL (Viacredi): PIX para a SECRETARIA MUNICIPAL DA FAZENDA de
	// São Paulo, CNPJ mascarado "**.392.130/0001-**". O rail é PIX, mas o
	// recebedor é ente tributário da whitelist → tributo válido (rail-agnóstico).
	frag, err := ExtractBR(Claims{
		ProviderID:    "viacredi",
		Recipient:     "**.392.130/0001-**",
		RecipientName: "SECRETARIA MUNICIPAL DA FAZENDA",
		Description:   "valorOriginalPix R$ 1.267,82",
	})
	if err != nil {
		t.Fatalf("PIX para ente tributário deveria ser aceito: %v", err)
	}
	if frag.Kind != "br_tax_entity" || frag.Raw != "BR|TAX-ENTITY|3921300001" {
		t.Fatalf("fragmento inesperado: %+v", frag)
	}
}

func TestExtractBR_Barcode(t *testing.T) {
	// FEBRABAN collection barcode (44 digits) + CNPJ.
	frag, err := ExtractBR(Claims{
		ProviderID:   "bradesco",
		Recipient:    "Prefeitura Municipal",
		RecipientName: "Guia DARF",
		PayerTaxID:   "12.345.678/0001-90",
		Description:  "82660000001234567890123456789012345678901234",
	})
	h := mustHash(t, frag, err)
	if len(h) != 64 {
		t.Fatalf("hash len = %d, want 64", len(h))
	}
}

func TestExtractUS_PropertyTax(t *testing.T) {
	frag, err := ExtractUS(Claims{
		ProviderID:   "chase",
		Recipient:    "Yavapai County Treasurer",
		RecipientName: "Property Tax Bill 2026",
		// EIN do PAGADOR: existe no comprovante mas NÃO pode ser a evidência.
		PayerTaxID:  "12-3456789",
		Description: "Payee EIN 86-6000481 — APN 123-45-678 Tax Year 2026",
	})
	h := mustHash(t, frag, err)
	if len(h) != 64 {
		t.Fatalf("hash len = %d, want 64", len(h))
	}
	// O fragmento tem de carregar o EIN do RECEBEDOR, nunca o do pagador.
	if frag.Raw != "US|PROPERTY-TAX|86-6000481" {
		t.Fatalf("unexpected fragment: %+v", frag)
	}
}

func TestExtractUS_PayerEINIsNotEvidence(t *testing.T) {
	// O EIN do PAGADOR não serve de evidência: se servisse, o atacante forneceria
	// a própria prova. Sem EIN do lado do recebedor e sem nome resolúvel → nada.
	_, err := ExtractUS(Claims{
		ProviderID:    "chase",
		Recipient:     "Some Office",
		RecipientName: "Property Tax Bill 2026",
		PayerTaxID:    "12-3456789",
		Description:   "APN 123-45-678 Tax Year 2026",
	})
	if err == nil {
		t.Fatal("esperado erro: o EIN do pagador não pode produzir o fragmento")
	}
}

func TestExtractUS_RejectsNonTax(t *testing.T) {
	_, err := ExtractUS(Claims{
		ProviderID:  "chase",
		Recipient:   "Some Corp",
		PayerTaxID:  "12-3456789",
		Description: "Invoice payment",
	})
	if err == nil {
		t.Fatal("expected error for non-tax US payment, got nil")
	}
}

func TestExtractCA_BNRT(t *testing.T) {
	frag, err := ExtractCA(Claims{
		ProviderID:   "rbc",
		Recipient:    "CRA",
		RecipientName: "Payment to CRA",
		PayerTaxID:   "123456789",
		Description:  "BN 123456789 RT0001 account payment",
	})
	h := mustHash(t, frag, err)
	if len(h) != 64 {
		t.Fatalf("hash len = %d, want 64", len(h))
	}
}

func TestExtractCA_RejectsCommonPayment(t *testing.T) {
	_, err := ExtractCA(Claims{
		ProviderID:  "td",
		Recipient:   "Utility Company",
		Description: "Monthly bill",
	})
	if err == nil {
		t.Fatal("expected error for non-tax CA payment, got nil")
	}
}

func TestExtractUS_StateByRecipientName(t *testing.T) {
	// California FTB has no EIN — recognized by recipient name.
	frag, err := ExtractUS(Claims{
		ProviderID:   "chase",
		Recipient:    "Franchise Tax Board",
		RecipientName: "California State Income Tax",
		PayerTaxID:   "12-3456789",
		Description:  "State income tax payment",
	})
	h := mustHash(t, frag, err)
	if len(h) != 64 {
		t.Fatalf("hash len = %d, want 64", len(h))
	}
	if frag.Kind != "us_state" || frag.Raw != "US|STATE|ca_ftb" {
		t.Fatalf("unexpected fragment: %+v", frag)
	}
}

func TestExtractUS_StateTexasComptroller(t *testing.T) {
	frag, err := ExtractUS(Claims{
		ProviderID:   "wells_fargo",
		Recipient:    "Texas Comptroller of Public Accounts",
		RecipientName: "Sales Tax",
		PayerTaxID:   "12-3456789",
		Description:  "Sales and Use Tax return",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frag.Kind != "us_state" || frag.Raw != "US|STATE|us:tx" {
		t.Fatalf("unexpected fragment: %+v", frag)
	}
}

func TestUSStateSlug_ShortTextOnly(t *testing.T) {
	// Deve ser usado só em texto CURTO (nome do recebedor): num corpo inteiro as
	// abreviaturas de dois caracteres ("in", "or", "me") casariam palavras
	// comuns do inglês e o estado sairia ao acaso.
	cases := map[string]string{
		"Texas Comptroller of Public Accounts": "us:tx",
		"California State Income Tax":          "ca_ftb",
		"New York Department of Taxation":      "ny_dtf",
		"Sales Tax":                            "",
		"":                                     "",
	}
	for in, want := range cases {
		if got := USStateSlug(in); got != want {
			t.Errorf("USStateSlug(%q) = %q; quer %q", in, got, want)
		}
	}
}

func TestExtractCA_PayerBN(t *testing.T) {
	// A municipality (BN 122119935 = Toronto) paying GST federal must have its
	// payer BN in the fragment, so the government blacklist can block it.
	frag, err := ExtractCA(Claims{
		ProviderID:   "rbc",
		Recipient:    "Receiver General for Canada",
		RecipientName: "CRA GST/HST",
		PayerTaxID:   "122119935",
		Description:  "BN 122119935 RT0001",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frag.Raw != "CA|CRA|122119935" {
		t.Fatalf("expected payer BN in fragment, got %q", frag.Raw)
	}
}

func TestExtractCA_RejectsNEQ(t *testing.T) {
	// Québec NEQ (11 digits) is not accepted.
	_, err := ExtractCA(Claims{
		ProviderID:   "rbc",
		Recipient:    "Revenu Québec",
		RecipientName: "Québec provincial tax",
		PayerTaxID:   "1143491417",
		Description:  "NEQ 1143491417",
	})
	if err == nil {
		t.Fatal("expected error for NEQ (Québec), got nil")
	}
}

func TestExtractUS_WhitelistedCountyWithMinimalHint(t *testing.T) {
	// Opção (b): EIN de um county da whitelist + evidência mínima ("county/
	// treasurer"), sem as palavras-chave específicas de property tax/IRS.
	frag, err := ExtractUS(Claims{
		ProviderID:    "chase",
		Recipient:     "Yavapai County Treasurer",
		RecipientName: "Yavapai County",
		Description:   "Payer EIN 86-6000481",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frag.Kind != "us_tax_entity" || frag.Raw != "US|TAX-ENTITY|866000481" {
		t.Fatalf("unexpected fragment: %+v", frag)
	}
}

func TestExtractUS_WhitelistedEINWithoutHintRejected(t *testing.T) {
	// Trava da opção (b): só o EIN na whitelist NÃO basta (evita taxa de
	// condado/licença ser tratada como imposto).
	_, err := ExtractUS(Claims{
		ProviderID:  "chase",
		Recipient:   "Some Office",
		Description: "Payer EIN 86-6000481",
	})
	if err == nil {
		t.Fatal("esperado erro: EIN na whitelist sem evidência fiscal")
	}
}

func TestExtractCA_WhitelistedMunicipality(t *testing.T) {
	// Opção (b) Canadá: recebedor com BN na whitelist basta (ca:city:abbotsford).
	frag, err := ExtractCA(Claims{
		ProviderID:    "rbc",
		Recipient:     "108084096",
		RecipientName: "City of Abbotsford",
		Description:   "Municipal payment",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frag.Kind != "ca_tax_entity" || frag.Raw != "CA|TAX-ENTITY|108084096" {
		t.Fatalf("unexpected fragment: %+v", frag)
	}
}

func TestExtractCA_NonWhitelistedBNRejected(t *testing.T) {
	_, err := ExtractCA(Claims{
		ProviderID:  "td",
		Recipient:   "999888777",
		Description: "Payment",
	})
	if err == nil {
		t.Fatal("esperado erro: BN fora da whitelist")
	}
}

func TestExtractUS_NameResolvedWithoutEIN(t *testing.T) {
	// Comprovante real dos EUA: property tax de condado, SEM EIN (o recibo traz
	// só o nome do fisco + parcel). Resolve pelo nome → us:county:yavapai_az.
	frag, err := ExtractUS(Claims{
		ProviderID:    "chase",
		Recipient:     "Yavapai County Treasurer",
		RecipientName: "Yavapai County Treasurer",
		Description:   "Property tax 2026 parcel 123-45-678",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frag.Kind != "us_tax_entity" || frag.Raw != "US|TAX-ENTITY|us:county:yavapai_az" {
		t.Fatalf("unexpected fragment: %+v", frag)
	}
}

func TestExtractCA_NameResolved(t *testing.T) {
	// Comprovante canadense: o "account number" é o folio do pagador, não o BN do
	// município — resolve pelo nome.
	frag, err := ExtractCA(Claims{
		ProviderID:    "rbc",
		Recipient:     "City of Vancouver",
		RecipientName: "City of Vancouver - Property Tax",
		Description:   "Folio 1234-5678-9012",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if frag.Kind != "ca_tax_entity" || frag.Raw != "CA|TAX-ENTITY|ca:city:vancouver" {
		t.Fatalf("unexpected fragment: %+v", frag)
	}
}

func TestExtractCA_UnknownNameRejected(t *testing.T) {
	_, err := ExtractCA(Claims{
		ProviderID:  "td",
		Recipient:   "Acme Comercio Ltda",
		Description: "Payment",
	})
	if err == nil {
		t.Fatal("esperado erro para recebedor não-fiscal")
	}
}

func TestExtractUS_RejectsCountyUtilityBill(t *testing.T) {
	// (d) Conta de ÁGUA de um distrito do condado NÃO é imposto: nome
	// "quase-fiscal" sem evidência específica de property tax → rejeita.
	_, err := ExtractUS(Claims{
		ProviderID:    "chase",
		Recipient:     "Yavapai County Water District",
		RecipientName: "Yavapai County Water District",
		Description:   "Water service bill 2026",
	})
	if err == nil {
		t.Fatal("esperado erro: conta de água não é imposto")
	}
}

func TestExtractCA_RejectsUtilityBill(t *testing.T) {
	_, err := ExtractCA(Claims{
		ProviderID:  "rbc",
		Recipient:   "City of Vancouver Utilities",
		Description: "Utility bill",
	})
	if err == nil {
		t.Fatal("esperado erro: conta de concessionária não é imposto")
	}
}

func TestExtract_UnknownProvider(t *testing.T) {
	_, err := Extract("unknown_bank", Claims{})
	if err == nil {
		t.Fatal("expected error for unknown provider, got nil")
	}
}

func TestHash_Deterministic(t *testing.T) {
	f := Fragment{Kind: "us_property_tax", Raw: "US|PROPERTY-TAX|12-3456789"}
	if Hash(f) != Hash(f) {
		t.Fatal("hash must be deterministic")
	}
}
