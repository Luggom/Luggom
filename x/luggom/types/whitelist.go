package types

// DefaultWhitelistCNPJs returns the default whitelist of authorized CNPJs for proof-of-sacrifice.
// These are the government entities that can receive tax payments for mining LUG$ tokens.
func DefaultWhitelistCNPJs() []string {
	return []string{
		"02.952.035/0001-56", // STN - Secretaria do Tesouro Nacional (Principal)
		"00.394.460/0001-41", // Receita Federal do Brasil
		"00.394.460/0051-00", // Fundo de Garantia do Tempo de Serviço (FGTS)
		"46.377.222/0001-29", // SEFAZ - São Paulo
		"42.498.675/0001-52", // SEFAZ - Rio de Janeiro
		"18.715.532/0001-31", // SEFAZ - Minas Gerais
		"82.951.310/0001-49", // SEFAZ - Santa Catarina
		"76.416.965/0001-21", // SEFAZ - Paraná
		"87.958.674/0001-81", // SEFAZ - Rio Grande do Sul
		"13.937.073/0001-56", // SEFAZ - Bahia
		"10.571.305/0001-08", // SEFAZ - Pernambuco
		"01.409.645/0001-30", // SEFAZ - Goiás
		"03.507.415/0001-44", // SEFAZ - Mato Grosso
		"07.954.597/0001-52", // SEFAZ - Ceará
		"00.394.460/0001-41", // SEFAZ - Distrito Federal
	}
}

// IsCNPJWhitelisted checks if a CNPJ is in the whitelist.
func IsCNPJWhitelisted(cnpj string, whitelist []string) bool {
	// Normalize CNPJ (remove dots, slashes, dashes)
	normalizedCNPJ := NormalizeCNPJ(cnpj)
	
	for _, wlCNPJ := range whitelist {
		if NormalizeCNPJ(wlCNPJ) == normalizedCNPJ {
			return true
		}
	}
	return false
}

// NormalizeCNPJ removes formatting characters from CNPJ for comparison.
func NormalizeCNPJ(cnpj string) string {
	normalized := ""
	for _, char := range cnpj {
		if char >= '0' && char <= '9' {
			normalized += string(char)
		}
	}
	return normalized
}
