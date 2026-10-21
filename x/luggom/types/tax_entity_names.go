package types

import (
	"regexp"
	"strings"
)

// tax_entity_names.go — resolve um ente tributário da whitelist a partir do NOME
// do recebedor.
//
// Por quê: no Brasil o PIX mostra o CNPJ do recebedor (a chave PIX é vinculada ao
// documento), então basta olhar o ID. Nos EUA e no Canadá o comprovante bancário
// costuma NÃO trazer o EIN/BN do governo — só o NOME do fisco (ex.: "Yavapai
// County Treasurer", "City of Vancouver"), porque o "account number" é o do
// próprio contribuinte (parcel/roll/folio).
//
// Regra: resolver SÓ quando for inequívoco. Se o nome casar com mais de um ente
// da whitelist (ex.: "Orange County" → CA/FL/NY), NÃO resolve.

// accentFold mapeia acentos comuns para a letra base (usado na normalização de
// nomes canadenses: "Montréal" → "montreal").
var accentFold = map[rune]rune{
	'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a',
	'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e',
	'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i',
	'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o',
	'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u',
	'ç': 'c', 'ñ': 'n', 'ý': 'y',
}

// caProvinceCodes mapeia nome da província → código do slug (ca:<code>).
var caProvinceCodes = map[string]string{
	"british columbia": "bc", "alberta": "ab", "saskatchewan": "sk",
	"manitoba": "mb", "ontario": "on", "quebec": "qc", "new brunswick": "nb",
	"nova scotia": "ns", "prince edward island": "pe",
	"newfoundland and labrador": "nl", "yukon": "yt",
	"northwest territories": "nt", "nunavut": "nu",
}

// normalizeEntityName lowercases, remove acentos/pontuação e colapsa espaços.
func normalizeEntityName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'à' && r <= 'ÿ':
			if f, ok := accentFold[r]; ok {
				b.WriteRune(f)
			} else {
				b.WriteRune(' ')
			}
		default:
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func hasSlug(wl []string, slug string) bool {
	for _, e := range wl {
		if e == slug {
			return true
		}
	}
	return false
}

// resolveUnique junta as palavras (do começo ou do fim) com "_" e devolve o slug
// SOMENTE se houver exatamente um candidato na whitelist para `prefix+joined`
// (com sufixo `_` opcional, para condados do tipo us:county:<nome>_<uf>).
func resolveUnique(wl []string, prefix string, words []string, fromEnd bool) (string, bool) {
	maxN := len(words)
	if maxN > 4 {
		maxN = 4
	}
	for n := maxN; n >= 1; n-- {
		seg := words[:n]
		if fromEnd {
			seg = words[len(words)-n:]
		}
		joined := strings.ReplaceAll(strings.Join(seg, " "), " ", "_")
		var cands []string
		for _, e := range wl {
			if e == prefix+joined || strings.HasPrefix(e, prefix+joined+"_") {
				cands = append(cands, e)
			}
		}
		if len(cands) == 1 {
			return cands[0], true
		}
	}
	return "", false
}

// nonTaxServiceWords: pagamentos de concessionária/serviço (água, esgoto, luz,
// telecom) NÃO são imposto — mesmo que o nome contenha "county"/"city". Serve de
// trava para não aceitar, por exemplo, "Yavapai County Water District".
var nonTaxServiceWords = []string{
	"water district", "water works", "waterworks", "water authority",
	"sewer", "wastewater", "sanitation", "utility", "utilities",
	"electric", "power company", "energy", "gas company", "internet",
	"telecom", "cable",
}

func looksLikeNonTaxService(norm string) bool {
	for _, w := range nonTaxServiceWords {
		if strings.Contains(norm, w) {
			return true
		}
	}
	return false
}

// usCountyStateCodeRe casa o código de duas letras do estado IMEDIATAMENTE
// depois de "County" (ex.: "Orange County, CA"). Exige MAIÚSCULAS no texto
// original: em minúsculas, "in"/"or"/"me"/"ok"/"de" casariam palavras comuns
// do inglês e o estado sairia errado.
var usCountyStateCodeRe = regexp.MustCompile(`(?i:county)\s*,?\s*\b([A-Z]{2})\b`)

// usCountyStateCode devolve o código do estado (em minúsculas) que segue
// "County" no texto original, ou "" quando não há.
func usCountyStateCode(text string) string {
	if m := usCountyStateCodeRe.FindStringSubmatch(text); len(m) > 1 {
		return strings.ToLower(m[1])
	}
	return ""
}

// ResolveTaxEntityByName tenta resolver o slug do ente tributário pelo NOME do
// recebedor. Retorna ok=false quando o texto não é reconhecido OU quando há
// ambiguidade (mais de um ente possível).
func ResolveTaxEntityByName(text string) (string, bool) {
	norm := normalizeEntityName(text)
	if norm == "" {
		return "", false
	}
	padded := " " + norm + " "
	wl := DefaultGlobalTaxEntities()

	// (d) Trava: concessionárias/serviços não são entes tributários.
	if looksLikeNonTaxService(padded) {
		return "", false
	}

	// --- Especiais (federal / agências conhecidas) ---
	if strings.Contains(padded, " internal revenue ") || strings.Contains(padded, " irs ") {
		if hasSlug(wl, "irs") {
			return "irs", true
		}
	}
	if strings.Contains(padded, " franchise tax board ") && hasSlug(wl, "ca_ftb") {
		return "ca_ftb", true
	}
	if strings.Contains(padded, " canada revenue agency ") || strings.Contains(padded, " cra ") {
		if hasSlug(wl, "ca:cra") {
			return "ca:cra", true
		}
	}

	// --- Canadá: cidade ("City of X" / "Ville de X") ---
	for _, marker := range []string{"city of ", "ville de ", "city "} {
		if idx := strings.Index(norm, marker); idx >= 0 {
			words := strings.Fields(norm[idx+len(marker):])
			if slug, ok := resolveUnique(wl, "ca:city:", words, false); ok {
				return slug, true
			}
		}
	}

	// --- Canadá: província ("Province of X" / "X province") ---
	for name, code := range caProvinceCodes {
		if strings.Contains(padded, " "+name+" ") {
			slug := "ca:" + code
			if hasSlug(wl, slug) {
				return slug, true
			}
		}
	}

	// --- EUA: condado ("X County" / "County of X") ---
	if idx := strings.Index(norm, " county"); idx > 0 {
		words := strings.Fields(norm[:idx])
		code := usCountyStateCode(text)
		if slug, ok := resolveUnique(wl, "us:county:", words, true); ok {
			// Ser o único com esse nome na whitelist não garante que é ESTE: se
			// o documento declara outro estado, não aceitar ("Suffolk County NY"
			// não pode ser creditado a suffolk_ma, o único no whitelist).
			if code == "" || strings.HasSuffix(slug, "_"+code) {
				return slug, true
			}
		}
		// "X County, ST": o estado vem DEPOIS do nome. Sem isto, "Orange
		// County, CA" caía na ambiguidade, porque só se lêem as palavras antes
		// de "county" (e "orange" existe em CA, FL e NY). Aceita só o slug
		// EXATO us:county:<nome>_<uf> — a whitelist decide, sem adivinhar.
		if code != "" {
			maxN := len(words)
			if maxN > 4 {
				maxN = 4
			}
			for n := maxN; n >= 1; n-- {
				joined := strings.ReplaceAll(strings.Join(words[len(words)-n:], " "), " ", "_")
				cand := "us:county:" + joined + "_" + code
				if hasSlug(wl, cand) {
					return cand, true
				}
			}
		}
	}
	if idx := strings.Index(norm, "county of "); idx >= 0 {
		words := strings.Fields(norm[idx+len("county of "):])
		if slug, ok := resolveUnique(wl, "us:county:", words, false); ok {
			return slug, true
		}
	}

	return "", false
}
