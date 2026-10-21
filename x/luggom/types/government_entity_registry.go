package types

// government_entity_registry.go — Blacklist of GOVERNMENT PAYERS (who cannot mine LUG).
//
// This is a BLACKLIST of payer tax-IDs (EIN/CNPJ/BN) that belong to government bodies.
// When a TLSNotary proof shows a government body as the PAYER (e.g. a municipality
// paying GST/HST to the Receiver General, or a county paying state tax), mining is
// rejected with ErrGovernmentEntityCannotMine (1206). Only citizens/companies pay
// taxes that mine LUG — never government-to-government transfers (anti-inflation,
// anti-sybil, §13.32/Passo 15).
//
// The list is intentionally SMALL: federal + state/provincial + top ~15 municipalities/
// counties/cities by revenue per country (FASE 4-A..F decision). The fallback in
// IsIDOfWhitelistedEntity (tax_entity_ids.go) covers every government body in the
// tax whitelist — any payer whose ID matches a whitelisted tax entity is also blocked,
// so small municipalities not listed here cannot mine either.
//
// On-chain: the default below is the genesis default; the live list is a gov-67%
// governable param (government_entity_blacklist, §13.42).
//
// US states have NO single EIN — they are blocked by name in the extractor
// (api/taxnature, ExtractUS), not here.

// DefaultGovernmentEntities returns the default blacklist of government payer IDs
// that cannot mine LUG. Format: "br:<CNPJ digits>", "us:<EIN digits>", "ca:<BN digits>".
func DefaultGovernmentEntities() []string {
	return []string{
		// === Brasil — federal ===
		"br:00394460005887", // Receita Federal do Brasil (RFB)
		"br:00394460040950", // Secretaria do Tesouro Nacional (STN)
		"br:29979036000140", // INSS (GPS — previdência social)

		// === Brasil — estados (SEFAZ) — 27 ===
		"br:04032986000134", // SEFAZ AC
		"br:12200259000165", // SEFAZ AL
		"br:23011011000150", // SEFAZ AP
		"br:04312369000144", // SEFAZ AM
		"br:13937073000156", // SEFAZ BA
		"br:07954597000152", // SEFAZ CE
		"br:00394684000153", // SECON DF
		"br:27080571000130", // SEFAZ ES
		"br:01409580000138", // SEFAZ GO
		"br:06354100000139", // SEFAZ MA
		"br:03507415000144", // SEFAZ MT
		"br:15412257000128", // SEFAZ MS
		"br:18715615000160", // SEFAZ MG
		"br:05054857000141", // SEFA PA
		"br:08761341000113", // SEFAZ PB
		"br:76416890000189", // SEFAZ PR
		"br:10572030000120", // SEFAZ PE
		"br:06553481000149", // SEFAZ PI
		"br:42498675000152", // SEFAZ RJ
		"br:08239380000190", // SEFAZ RN
		"br:87958674000181", // SEFAZ RS
		"br:04244200000157", // SEFAZ RO
		"br:02730081000102", // SEFAZ RR
		"br:82951310000114", // SEFAZ SC
		"br:46377222000129", // SEFAZ SP
		"br:13064213000154", // SEFAZ SE
		"br:01760714000140", // SEFAZ TO

		// === Brasil — top municípios IPTU (FASE 4-A: top 15 por arrecadação) ===
		"br:46392130000118", // São Paulo (SP)
		"br:42498733000148", // Rio de Janeiro (RJ)
		"br:18715383000140", // Belo Horizonte (MG)
		"br:00394684000153", // Brasília (DF)
		"br:13922541000187", // Salvador (BA)
		"br:76411534000198", // Curitiba (PR)
		"br:92963560000160", // Porto Alegre (RS)
		"br:46391132000143", // Campinas (SP)
		"br:07954605000165", // Fortaleza (CE)
		"br:10565000000192", // Recife (PE)
		"br:46319000000150", // Guarulhos (SP)
		"br:01612092000123", // Goiânia (GO)
		"br:46523015000135", // Barueri (SP)
		"br:46523239000147", // São Bernardo do Campo (SP)
		"br:04365326000173", // Manaus (AM)

		// === EUA — federal ===
		"us:530204720", // IRS (Internal Revenue Service)

		// === EUA — top counties (property tax, FASE 4-A) ===
		"us:956000927",  // Los Angeles County (CA)
		"us:366004941",  // Cook County (IL — Chicago)
		"us:746001211",  // Harris County (TX)
		"us:956000934",  // Orange County (CA)
		"us:956000930",  // San Diego County (CA)
		"us:756000744",  // Dallas County (TX)
		"us:916001252",  // King County (WA)
		"us:866000302",  // Maricopa County (AZ)
		"us:596000573",  // Miami-Dade County (FL)
		"us:946000531",  // Santa Clara County (CA)
		"us:116002165",  // Nassau County (NY)
		"us:756000787",  // Tarrant County (TX)
		"us:746000450",  // Bexar County (TX)
		"us:591065160",  // Broward County (FL)
		"us:956000933",  // Riverside County (CA)

		// === Canadá — federal ===
		"ca:133691238", // Receiver General for Canada / CRA

		// === Canadá — províncias/territórios (12; Quebec usa NEQ — fora) ===
		"ca:124089201", // Ontario (Ministry of Finance)
		"ca:107864738", // British Columbia
		"ca:106692296", // Alberta
		"ca:107659427", // Manitoba
		"ca:107981508", // Saskatchewan
		"ca:107788739", // Nova Scotia
		"ca:107788572", // New Brunswick
		"ca:107788226", // Newfoundland and Labrador
		"ca:107851925", // Prince Edward Island
		"ca:107788416", // Northwest Territories
		"ca:108223637", // Yukon
		"ca:863897723", // Nunavut

		// === Canadá — top cidades (FASE 4-A) ===
		"ca:122119935", // Toronto (ON)
		"ca:118021160", // Montreal (QC)
		"ca:108083817", // Calgary (AB)
		"ca:131343840", // Ottawa (ON)
		"ca:108083833", // Edmonton (AB)
		"ca:108223801", // Winnipeg (MB)
		"ca:107011280", // Mississauga (ON)
		"ca:130706245", // Vancouver (BC)
		"ca:106808728", // Brampton (ON)
		"ca:107460917", // Hamilton (ON)
		"ca:108048265", // Surrey (BC)
		"ca:118021202", // Quebec City (QC)
		"ca:107788895", // Halifax (NS)
		"ca:118021228", // Laval (QC)
		"ca:108083890", // London (ON)
	}
}

// IsGovernmentEntity checks whether the given tax ID (EIN/CNPJ/BN) is in the
// government blacklist. Both the input and the list entries are normalized
// (uppercase digits/letters, country prefix stripped) before comparison.
func IsGovernmentEntity(payerTaxID string, govList []string) bool {
	normalized := stripCountryPrefix(NormalizeTaxID(payerTaxID))
	if normalized == "" {
		return false
	}
	for _, g := range govList {
		if normalized == stripCountryPrefix(NormalizeTaxID(g)) {
			return true
		}
	}
	return false
}

// stripCountryPrefix removes a leading "BR"/"US"/"CA" country prefix from a
// normalized tax ID (e.g. "BR00394460005887" → "00394460005887").
func stripCountryPrefix(n string) string {
	if len(n) >= 3 {
		prefix := n[:2]
		if prefix == "BR" || prefix == "US" || prefix == "CA" {
			return n[2:]
		}
	}
	return n
}
