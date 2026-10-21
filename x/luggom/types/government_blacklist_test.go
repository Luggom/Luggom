package types

import "testing"

// FASE 4-A..F — government-payer blacklist + whitelist fallback tests.

func TestNormalizeTaxID(t *testing.T) {
	cases := map[string]string{
		"46.392.130/0001-18":     "46392130000118",
		"95-6000927":             "956000927",
		"122119935":              "122119935",
		"br:00.394.460/0058-87":  "BR00394460005887",
		" 12-3456789 ":           "123456789",
		"":                       "",
	}
	for in, want := range cases {
		if got := NormalizeTaxID(in); got != want {
			t.Errorf("NormalizeTaxID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsGovernmentEntity_Blacklist(t *testing.T) {
	list := DefaultGovernmentEntities()
	// Toronto BN (top city) is in the blacklist.
	if !IsGovernmentEntity(NormalizeTaxID("122119935"), list) {
		t.Fatal("Toronto BN should be in the government blacklist")
	}
	// SEFAZ-SP CNPJ is in the blacklist.
	if !IsGovernmentEntity(NormalizeTaxID("46.377.222/0001-29"), list) {
		t.Fatal("SEFAZ-SP CNPJ should be in the government blacklist")
	}
	// IRS EIN is in the blacklist.
	if !IsGovernmentEntity(NormalizeTaxID("53-0204720"), list) {
		t.Fatal("IRS EIN should be in the government blacklist")
	}
	// INSS CNPJ is in the blacklist.
	if !IsGovernmentEntity(NormalizeTaxID("29.979.036/0001-40"), list) {
		t.Fatal("INSS CNPJ should be in the government blacklist")
	}
	// A citizen EIN is NOT in the blacklist.
	if IsGovernmentEntity(NormalizeTaxID("12-3456789"), list) {
		t.Fatal("citizen EIN should NOT be in the government blacklist")
	}
}

func TestIsIDOfWhitelistedEntity_Fallback(t *testing.T) {
	// Americana (SP) is NOT in the top-15 blacklist, but IS a whitelisted tax
	// entity (br:iptu_americana) → the fallback blocks it as a government payer.
	if !IsIDOfWhitelistedEntity(NormalizeTaxID("46.126.249/0001-01")) {
		t.Fatal("Americana CNPJ should match a whitelisted tax entity (fallback)")
	}
	// Los Angeles County EIN matches the whitelist (us:county:los_angeles_ca).
	if !IsIDOfWhitelistedEntity(NormalizeTaxID("95-6000927")) {
		t.Fatal("LA County EIN should match a whitelisted tax entity (fallback)")
	}
	// A citizen EIN does not match any whitelisted entity.
	if IsIDOfWhitelistedEntity(NormalizeTaxID("12-3456789")) {
		t.Fatal("citizen EIN should NOT match any whitelisted entity")
	}
	// CNPJ mascarado (ex: comprovante Viacredi "**392.130/0001-**" → miolo
	// "3921300001") deve bater por subconjunto com br:iptu_sao_paulo (46392130000118).
	if !IsIDOfWhitelistedEntity(NormalizeTaxID("**392.130/0001-**")) {
		t.Fatal("masked São Paulo CNPJ should match br:iptu_sao_paulo via subset match")
	}
	// Miolo curto demais não deve gerar falso positivo (mínimo 6 dígitos).
	if IsIDOfWhitelistedEntity(NormalizeTaxID("**392.***")) {
		t.Fatal("short masked CNPJ should NOT match (min length guard)")
	}
}

func TestGovernmentBlacklist_Validates(t *testing.T) {
	p := NewParams()
	if err := p.Validate(); err != nil {
		t.Fatalf("default params with government_entity_blacklist must validate: %v", err)
	}
	if len(p.GovernmentEntityBlacklist) == 0 {
		t.Fatal("default government_entity_blacklist should not be empty")
	}
}

func TestWhitelist_HasNYCAndCapitals(t *testing.T) {
	list := DefaultGlobalTaxEntities()
	// NYC boroughs
	for _, slug := range []string{
		"us:county:new_york_ny", "us:county:kings_ny", "us:county:bronx_ny",
		"us:county:queens_ny", "us:county:richmond_ny",
	} {
		if !IsTaxEntityWhitelisted(slug, list) {
			t.Fatalf("NYC borough %s should be whitelisted (FASE 4-B)", slug)
		}
	}
	// 50 states present
	for _, st := range []string{"us:al", "us:ca", "us:ny", "us:tx", "us:wy"} {
		if !IsTaxEntityWhitelisted(st, list) {
			t.Fatalf("state %s should be whitelisted", st)
		}
	}
	// Removed entries
	if IsTaxEntityWhitelisted("br:iptu_cuiaba", list) {
		t.Fatal("br:iptu_cuiaba should have been removed (FASE 4-B)")
	}
	if IsTaxEntityWhitelisted("ca:qc", list) {
		t.Fatal("ca:qc should have been removed (FASE 4-B)")
	}
}
