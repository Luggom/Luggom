// Package taxnature implements per-country extractors that prove a TLSNotary
// payment is a real tax collection (anti-cycle, §13.34).
//
// The on-chain keeper only validates the presence of a tax_nature_hash. The
// heavy matching lives here, off-chain: each country extractor inspects the
// verified TLS session fragments and generates a SHA-256 of a non-sensitive
// fragment that proves tax collection. A common PIX/transfer or donation to a
// whitelisted CNPJ/EIN fails the extractor, so no hash is produced and the
// transaction is rejected on-chain with ErrMissingTaxNature.
//
// Fase 1 (trust API): heuristic patterns over proof fields. Fase 2 (ECDSA):
// the same extractor output becomes part of the signed attestation. Fase 3
// (ZK): the fragment is proven in zero-knowledge.
package taxnature

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	govtypes "luggom/x/luggom/types"
)

// Fragment is the non-sensitive proof excerpt that demonstrates tax nature.
type Fragment struct {
	// Raw is the canonical, non-sensitive fragment used for the hash.
	Raw string `json:"raw"`
	// Kind is a human-readable classification of the collection type.
	Kind string `json:"kind"`
}

// Country returns the extractor country code for a provider ID.
// Providers follow the ids in api/reclaimverify/providers.go.
func Country(providerID string) string {
	p := strings.ToLower(strings.TrimSpace(providerID))
	if strings.HasPrefix(p, "ca:") || strings.HasPrefix(p, "mock_ca") {
		return "CA"
	}
	if strings.HasPrefix(p, "br:") || strings.HasPrefix(p, "mock_") && strings.Contains(p, "br") {
		return "BR"
	}
	if strings.HasPrefix(p, "us:") || strings.HasPrefix(p, "mock_us") {
		return "US"
	}
	switch p {
	case "rbc", "td", "scotiabank", "bmo", "cibc", "mock_rbc", "mock_td", "mock_scotiabank", "mock_bmo", "mock_cibc":
		return "CA"
	case "chase", "wells_fargo", "bank_of_america", "citi", "capital_one", "us_bank", "pnc", "mock_chase", "mock_wells_fargo", "mock_boa", "mock_citi", "mock_capital_one", "mock_us_bank", "mock_pnc":
		return "US"
	case "itau", "bradesco", "banco_do_brasil", "santander", "sicredi", "viacredi", "sicoob", "inter", "caixa", "mock_itau", "mock_bradesco", "mock_bb", "mock_sicredi", "mock_viacredi", "mock_sicoob", "mock_inter", "mock_caixa":
		return "BR"
	}
	return ""
}

// Hash returns the hex SHA-256 of the canonical fragment.
func Hash(f Fragment) string {
	sum := sha256.Sum256([]byte(f.Raw))
	return hex.EncodeToString(sum[:])
}

// ---------------------------------------------------------------------------
// Brazil — FEBRABAN barcode / PIX arrecadação / DARF-GPS-IPTU-IPVA text
// ---------------------------------------------------------------------------

// BR patterns: FEBRABAN collection barcode (44 digits), PIX arrecadação QR
// (BR.GOV.BR/AREC), DARF/GPS/IPTU/IPVA receipts.
var (
	brBarcodeRe  = regexp.MustCompile(`\b\d{44}\b`)
	brPixArrecRe  = regexp.MustCompile(`(?i)arrecada[cç][aã]o|BR\.GOV\.BR[^\s]*`)
	brTaxKindRe   = regexp.MustCompile(`(?i)\b(DARF|GPS|IPTU|IPVA|ISS|ICMS|ITR|taxa)\b`)
	// CNPJ: casar tanto o completo ("46.392.130/0001-18") quanto o mascarado
	// ("**392.130/0001-**", comum em comprovantes bancários que ocultam os 2
	// primeiros e os 2 últimos dígitos com `*`). Os extremos aceitam [0-9*].
	// Nota: Go RE2 não interpreta \d dentro de [..] — usar [0-9*].
	brCnpjRe      = regexp.MustCompile(`[0-9*]{2,}(\.[0-9*]{3})?\.?[0-9*]{3}/[0-9*]{4}-[0-9*]{2,}`)
	brCnpjMaskedRe = regexp.MustCompile(`[0-9*]{2,}(\.[0-9*]{3})?\.?[0-9*]{3}/[0-9*]{4}-[0-9*]{2,}`)
	brPixCommonRe = regexp.MustCompile(`(?i)pix|copia e cola|transfer[eê]ncia`)
)

// ExtractBR returns a tax-nature fragment from a Brazilian proof, or an error
// when the payment is not a tax collection (common PIX, donation, transfer).
//
// O rail de pagamento é irrelevante: o que identifica o tributo é o dinheiro ter
// ido para um ENTE TRIBUTÁRIO (CNPJ na whitelist). Um PIX para a prefeitura paga
// IPTU do mesmo jeito que um boleto/DARF; um PIX para pessoa/empresa comum cai
// fora sozinho, porque o destinatário NÃO está na whitelist.
func ExtractBR(claims Claims) (Fragment, error) {
	all := joinAll(claims)
	full := strings.ToLower(all)
	hasArrec := brPixArrecRe.MatchString(full)
	hasBarcode := brBarcodeRe.MatchString(all)
	hasKind := brTaxKindRe.MatchString(full)
	recipientID := cnpj(all)
	hasCnpj := recipientID != ""
	commonPix := brPixCommonRe.MatchString(full) && !hasArrec
	// CNPJ do recebedor é de um ente tributário da whitelist (completo ou
	// mascarado — match por miolo de dígitos). Rail-agnóstico.
	_, whitelistedTaxEntity := govtypes.WhitelistedEntityForID(recipientID)

	switch {
	case whitelistedTaxEntity:
		return Fragment{Kind: "br_tax_entity", Raw: "BR|TAX-ENTITY|" + recipientID}, nil
	case commonPix && !hasBarcode && !hasArrec:
		return Fragment{}, fmt.Errorf("BR: common PIX/transfer — not a tax collection")
	case hasBarcode && hasCnpj:
		return Fragment{Kind: "br_barcode_cnpj", Raw: "BR|BARCODE|" + barcode(all) + "|" + recipientID}, nil
	case hasArrec && hasCnpj:
		return Fragment{Kind: "br_pix_arrec", Raw: "BR|PIX-AREC|" + recipientID}, nil
	case hasKind && hasCnpj:
		return Fragment{Kind: "br_kind_cnpj", Raw: "BR|KIND|" + taxKind(all) + "|" + recipientID}, nil
	default:
		return Fragment{}, fmt.Errorf("BR: no tax-collection evidence (barcode/PIX arrecadação/DARF-GPS-IPTU-IPVA + CNPJ)")
	}
}

func barcode(s string) string {
	if m := brBarcodeRe.FindString(s); m != "" {
		return m
	}
	return ""
}

func cnpj(s string) string {
	// Extrai apenas os DÍGITOS do CNPJ (completo ou mascarado). Devolver o miolo
	// de dígitos torna o hash estável e igual entre banco e portal do governo
	// (ex: "**392.130/0001-**" e "46.392.130/0001-18" → "46392130000118").
	m := brCnpjRe.FindString(s)
	if m == "" {
		m = brCnpjMaskedRe.FindString(s)
	}
	if m == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range m {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func taxKind(s string) string {
	if m := brTaxKindRe.FindString(s); m != "" {
		return m
	}
	return ""
}

// ---------------------------------------------------------------------------
// USA — property tax bill (county portal) / IRS payment reference / state revenue dept
// ---------------------------------------------------------------------------

// US patterns: property tax bills carry APN/parcel + tax year + "property
// tax"; IRS payments carry "federal tax" / "1040" / "estimated tax";
// state revenue departments are recognized BY NAME (states have no single EIN).
var (
	usPropertyTaxRe = regexp.MustCompile(`(?i)property tax|parcel|apn\b|tax year`)
	usIrsRe         = regexp.MustCompile(`(?i)\b(irs|federal tax|1040|estimated tax|tax payment)\b`)
	usEinRe         = regexp.MustCompile(`\b\d{2}-\d{7}\b`)
	// State revenue departments by name/domain (FASE 4-D). No EIN for states —
	// the recipient name identifies the state tax authority.
	usStateRe = regexp.MustCompile(`(?i)(franchise tax board|cdtfa|department of (taxation|revenue)|dept of (taxation|revenue)|comptroller|department of treasury|department of finance|tax commission|revenue service|board of equalization|dorr|dor\b|mytax|revenue department)`)
	usStateSlugRe = regexp.MustCompile(`(?i)(california|calif|ca\b|new york|ny\b|texas|tx\b|florida|fl\b|illinois|il\b|pennsylvania|pa\b|ohio|oh\b|georgia|ga\b|north carolina|nc\b|michigan|mi\b|virginia|va\b|washington|wa\b|new jersey|nj\b|massachusetts|ma\b|arizona|az\b|colorado|co\b|maryland|md\b|minnesota|mn\b|wisconsin|wi\b|tennessee|tn\b|missouri|mo\b|indiana|in\b|south carolina|sc\b|alabama|al\b|kentucky|ky\b|louisiana|la\b|oklahoma|ok\b|oregon|or\b|connecticut|ct\b|utah|ut\b|nevada|nv\b|new mexico|nm\b|arkansas|ar\b|mississippi|ms\b|kansas|ks\b|iowa|ia\b|nebraska|ne\b|idaho|id\b|new hampshire|nh\b|maine|me\b|rhode island|ri\b|montana|mt\b|delaware|de\b|south dakota|sd\b|north dakota|nd\b|alaska|ak\b|hawaii|hi\b|vermont|vt\b|west virginia|wv\b|wyoming|wy\b)`)
	// Evidência fiscal mínima (opção b): exige-se, além do EIN na whitelist, um
	// indício de que o pagamento é fiscal (evita aceitar taxa de água/licença de
	// condado como imposto). Counties/tesourarias entram por "county"/"treasur".
	usFiscalHintRe = regexp.MustCompile(`(?i)\b(tax|taxes|taxation|taxpayer|treasur\w*|county|assessment|assessor|revenue)\b`)
)

// usStateSlug maps a state name in the recipient to its whitelist slug.
// Uses word-boundary regexes so abbreviations ("CA", "TX") don't match inside
// words ("comptroller" contains "co").
func usStateSlug(low string) string {
	// Full names first (avoid "new york" matching "york" etc. via boundary).
	fullNames := []struct {
		re   *regexp.Regexp
		slug string
	}{
		{regexp.MustCompile(`\bnew york\b`), "ny_dtf"},
		{regexp.MustCompile(`\bnorth carolina\b`), "us:nc"},
		{regexp.MustCompile(`\bsouth carolina\b`), "us:sc"},
		{regexp.MustCompile(`\bnorth dakota\b`), "us:nd"},
		{regexp.MustCompile(`\bsouth dakota\b`), "us:sd"},
		{regexp.MustCompile(`\bnew jersey\b`), "us:nj"},
		{regexp.MustCompile(`\bnew mexico\b`), "us:nm"},
		{regexp.MustCompile(`\bnew hampshire\b`), "us:nh"},
		{regexp.MustCompile(`\bwest virginia\b`), "us:wv"},
		{regexp.MustCompile(`\brhode island\b`), "us:ri"},
		{regexp.MustCompile(`\bcalifornia\b`), "ca_ftb"},
		{regexp.MustCompile(`\btexas\b`), "us:tx"},
		{regexp.MustCompile(`\bflorida\b`), "us:fl"},
		{regexp.MustCompile(`\billinois\b`), "us:il"},
		{regexp.MustCompile(`\bpennsylvania\b`), "us:pa"},
		{regexp.MustCompile(`\bohio\b`), "us:oh"},
		{regexp.MustCompile(`\bgeorgia\b`), "us:ga"},
		{regexp.MustCompile(`\bmichigan\b`), "us:mi"},
		{regexp.MustCompile(`\bvirginia\b`), "us:va"},
		{regexp.MustCompile(`\bwashington\b`), "us:wa"},
		{regexp.MustCompile(`\bmassachusetts\b`), "us:ma"},
		{regexp.MustCompile(`\barizona\b`), "us:az"},
		{regexp.MustCompile(`\bcolorado\b`), "us:co"},
		{regexp.MustCompile(`\bmaryland\b`), "us:md"},
		{regexp.MustCompile(`\bminnesota\b`), "us:mn"},
		{regexp.MustCompile(`\bwisconsin\b`), "us:wi"},
		{regexp.MustCompile(`\btennessee\b`), "us:tn"},
		{regexp.MustCompile(`\bmissouri\b`), "us:mo"},
		{regexp.MustCompile(`\bindiana\b`), "us:in"},
		{regexp.MustCompile(`\balabama\b`), "us:al"},
		{regexp.MustCompile(`\bkentucky\b`), "us:ky"},
		{regexp.MustCompile(`\blouisiana\b`), "us:la"},
		{regexp.MustCompile(`\boklahoma\b`), "us:ok"},
		{regexp.MustCompile(`\boregon\b`), "us:or"},
		{regexp.MustCompile(`\bconnecticut\b`), "us:ct"},
		{regexp.MustCompile(`\butah\b`), "us:ut"},
		{regexp.MustCompile(`\bnevada\b`), "us:nv"},
		{regexp.MustCompile(`\barkansas\b`), "us:ar"},
		{regexp.MustCompile(`\bmississippi\b`), "us:ms"},
		{regexp.MustCompile(`\bkansas\b`), "us:ks"},
		{regexp.MustCompile(`\biowa\b`), "us:ia"},
		{regexp.MustCompile(`\bnebraska\b`), "us:ne"},
		{regexp.MustCompile(`\bidaho\b`), "us:id"},
		{regexp.MustCompile(`\bmaine\b`), "us:me"},
		{regexp.MustCompile(`\bmontana\b`), "us:mt"},
		{regexp.MustCompile(`\bdelaware\b`), "us:de"},
		{regexp.MustCompile(`\balaska\b`), "us:ak"},
		{regexp.MustCompile(`\bhawaii\b`), "us:hi"},
		{regexp.MustCompile(`\bvermont\b`), "us:vt"},
		{regexp.MustCompile(`\bwyoming\b`), "us:wy"},
	}
	for _, f := range fullNames {
		if f.re.MatchString(low) {
			return f.slug
		}
	}
	// Abbreviations with word boundaries (fallback).
	abbr := map[string]string{
		"ca": "ca_ftb", "ny": "ny_dtf", "tx": "us:tx", "fl": "us:fl", "il": "us:il",
		"pa": "us:pa", "oh": "us:oh", "ga": "us:ga", "nc": "us:nc", "mi": "us:mi",
		"va": "us:va", "wa": "us:wa", "nj": "us:nj", "ma": "us:ma", "az": "us:az",
		"co": "us:co", "md": "us:md", "mn": "us:mn", "wi": "us:wi", "tn": "us:tn",
		"mo": "us:mo", "in": "us:in", "sc": "us:sc", "al": "us:al", "ky": "us:ky",
		"la": "us:la", "ok": "us:ok", "or": "us:or", "ct": "us:ct", "ut": "us:ut",
		"nv": "us:nv", "nm": "us:nm", "ar": "us:ar", "ms": "us:ms", "ks": "us:ks",
		"ia": "us:ia", "ne": "us:ne", "id": "us:id", "nh": "us:nh", "me": "us:me",
		"ri": "us:ri", "mt": "us:mt", "de": "us:de", "sd": "us:sd", "nd": "us:nd",
		"ak": "us:ak", "hi": "us:hi", "vt": "us:vt", "wv": "us:wv", "wy": "us:wy",
	}
	for ab, slug := range abbr {
		if regexp.MustCompile(`\b` + ab + `\b`).MatchString(low) {
			return slug
		}
	}
	return ""
}

// IsUSStateRevenueDept reports whether a text names a US state revenue
// department. Um fisco ESTADUAL não tem EIN único — identifica-se por nome.
// Deve ser aplicado ao texto do BENEFICIÁRIO, não ao corpo inteiro.
func IsUSStateRevenueDept(text string) bool {
	return usStateRe.MatchString(strings.ToLower(text))
}

// USStateSlug resolve o slug do fisco ESTADUAL a partir de um texto curto
// (tipicamente o nome do recebedor). Devolve "" quando o estado não é
// inequívoco.
//
// NÃO usar sobre o corpo inteiro do comprovante: as abreviaturas de dois
// caracteres ("in", "or", "me", "ok", "de") casariam palavras comuns do inglês
// e o estado seria escolhido ao acaso.
func USStateSlug(text string) string {
	return usStateSlug(strings.ToLower(text))
}

// recipientBene devolve só os campos do BENEFICIÁRIO (nome do recebedor). É
// deles — e nunca da prosa do corpo — que sai o ente tributário: uma página
// qualquer do banco pode CITAR um condado da whitelist sem que exista pagamento.
func recipientBene(c Claims) string {
	return strings.Join([]string{c.Recipient, c.RecipientName}, " ")
}

// recipientEINText devolve os campos do LADO DO RECEBEDOR onde o EIN do fisco
// pode aparecer (inclui o detalhe do comprovante). Exclui Payer/PayerTaxID: um
// comprovante de imposto de renda traz o EIN do PRÓPRIO contribuinte, e aceitá-lo
// como se fosse o do fisco deixaria o atacante fornecer a sua própria evidência.
func recipientEINText(c Claims) string {
	return strings.Join([]string{c.Recipient, c.RecipientName, c.Description}, " ")
}

// ExtractUS returns a tax-nature fragment from a US proof, or an error.
func ExtractUS(claims Claims) (Fragment, error) {
	full := joinAll(claims)
	low := strings.ToLower(full)
	bene := recipientBene(claims)
	blow := strings.ToLower(bene)
	// O EIN tem de vir do lado do RECEBEDOR — nunca pode ser o do pagador.
	rein := recipientEINText(claims)
	hasEin := usEinRe.MatchString(rein)
	switch {
	// State revenue department BY NAME (checked before IRS — "tax payment" also
	// matches IRS patterns). States have no single EIN.
	case usStateRe.MatchString(blow):
		slug := usStateSlug(blow)
		if slug == "" {
			return Fragment{}, fmt.Errorf("US: state revenue dept recognized but state not mapped to slug")
		}
		return Fragment{Kind: "us_state", Raw: "US|STATE|" + slug}, nil
	case usPropertyTaxRe.MatchString(low) && hasEin:
		return Fragment{Kind: "us_property_tax", Raw: "US|PROPERTY-TAX|" + ein(rein)}, nil
	case usIrsRe.MatchString(low) && hasEin:
		return Fragment{Kind: "us_irs", Raw: "US|IRS|" + ein(rein)}, nil
	// Opção (b): destinatário é ENTE TRIBUTÁRIO da whitelist (EIN) + evidência
	// fiscal mínima. Rail-agnóstico, mas com trava: só o EIN não basta (evita
	// aceitar taxa de condado/licença).
	case hasEin && usFiscalHintRe.MatchString(low) && isWhitelistedTaxID(ein(rein)):
		return Fragment{Kind: "us_tax_entity", Raw: "US|TAX-ENTITY|" + NormalizeTaxID(ein(rein))}, nil
	}
	// Fallback: comprovantes de property tax dos EUA normalmente NÃO trazem o EIN
	// do fisco — só o NOME ("Yavapai County Treasurer"). (d) Exige evidência
	// ESPECÍFICA (property tax/parcel/APN/tax year, ou IRS): "county"/"tax"
	// genérico não basta, senão conta de água de condado viraria imposto. O nome
	// é procurado no BENEFICIÁRIO, não na prosa do corpo.
	if usPropertyTaxRe.MatchString(low) || usIrsRe.MatchString(low) {
		if slug, ok := govtypes.ResolveTaxEntityByName(bene); ok {
			return Fragment{Kind: "us_tax_entity", Raw: "US|TAX-ENTITY|" + slug}, nil
		}
	}
	return Fragment{}, fmt.Errorf("US: no tax evidence (property tax bill/IRS reference/state revenue + EIN)")
}

func ein(s string) string {
	if m := usEinRe.FindString(s); m != "" {
		return m
	}
	return ""
}

// ---------------------------------------------------------------------------
// Canada — CRA BN + RT account / municipal property tax
// ---------------------------------------------------------------------------

// CA patterns: CRA business number (9 digits + RT0001 account) or municipal
// property tax references. NEQ (Québec, 11 digits) is NOT accepted (FASE 4-D).
var (
	caBnRtRe    = regexp.MustCompile(`(?i)\b\d{9}\s*RT\d{4}\b|\bRT\d{4}\b`)
	caPropTaxRe = regexp.MustCompile(`(?i)property tax|municipal tax|tax bill`)
	caBnRe      = regexp.MustCompile(`\b\d{9}\b`)
	caNeqRe     = regexp.MustCompile(`(?i)\b\d{11}\b|NEQ`)
)

// ExtractCA returns a tax-nature fragment from a Canadian proof, or an error.
// The fragment includes the PAYER BN (9 digits) so the on-chain government
// blacklist can block municipalities/provinces paying federal GST/HST.
func ExtractCA(claims Claims) (Fragment, error) {
	full := joinAll(claims)
	low := strings.ToLower(full)
	// NEQ (Québec) is not a CRA BN — reject (no BN+RT semantics).
	if caNeqRe.MatchString(full) {
		return Fragment{}, fmt.Errorf("CA: NEQ (Québec) not accepted — BN+RT required")
	}
	payerBN := bn(claims.PayerTaxID)
	recipientBN := bn(claims.Recipient)
	switch {
	case caBnRtRe.MatchString(full):
		// include the payer BN when present, else the recipient BN
		b := payerBN
		if b == "" {
			b = bn(full)
		}
		return Fragment{Kind: "ca_cra_bn_rt", Raw: "CA|CRA|" + b}, nil
	case caPropTaxRe.MatchString(low) && caBnRe.MatchString(full):
		return Fragment{Kind: "ca_property_tax", Raw: "CA|PROPERTY-TAX|" + bn(full)}, nil
	// Opção (b) Canadá: recebedor é ente tributário da whitelist (BN). O Canadá
	// é mais seguro que os EUA (a whitelist é CRA/agências municipais), então a
	// própria whitelist basta — sem exigir palavra-chave.
	case recipientBN != "" && isWhitelistedTaxID(recipientBN):
		return Fragment{Kind: "ca_tax_entity", Raw: "CA|TAX-ENTITY|" + NormalizeTaxID(recipientBN)}, nil
	}
	// Fallback: o comprovante canadense de property tax costuma trazer só o NOME
	// do município (o "account number" é o roll/folio do próprio pagador). (d)
	// Exige evidência de property tax OU roll/folio antes de aceitar por nome. O
	// nome é procurado no BENEFICIÁRIO, não na prosa do corpo.
	if caPropTaxRe.MatchString(low) || strings.Contains(low, "roll number") || strings.Contains(low, "folio") {
		if slug, ok := govtypes.ResolveTaxEntityByName(recipientBene(claims)); ok {
			return Fragment{Kind: "ca_tax_entity", Raw: "CA|TAX-ENTITY|" + slug}, nil
		}
	}
	return Fragment{}, fmt.Errorf("CA: no tax evidence (CRA BN+RT or municipal property tax)")
}

func bn(s string) string {
	if m := caBnRe.FindString(s); m != "" {
		return m
	}
	return ""
}

// ---------------------------------------------------------------------------
// Generic dispatch
// ---------------------------------------------------------------------------

// Claims is the minimal set of proof fields the extractors read.
type Claims struct {
	// ProviderID is the bank/provider slug (chase, itau, rbc, ...).
	ProviderID string
	// Recipient is the recipient account/EIN/CNPJ from the proof.
	Recipient string
	// RecipientName is the recipient name line from the proof.
	RecipientName string
	// Payer is the payer account/name.
	Payer string
	// PayerTaxID is the payer EIN/CNPJ.
	PayerTaxID string
	// Description is any extra receipt/bill description text.
	Description string
}

// joinAll concatenates the claims into a single searchable string.
func joinAll(c Claims) string {
	return strings.Join([]string{c.ProviderID, c.Recipient, c.RecipientName, c.Payer, c.PayerTaxID, c.Description}, " ")
}

// Extract dispatches to the country extractor based on the provider ID.
func Extract(providerID string, claims Claims) (Fragment, error) {
	switch Country(providerID) {
	case "BR":
		return ExtractBR(claims)
	case "US":
		return ExtractUS(claims)
	case "CA":
		return ExtractCA(claims)
	default:
		return Fragment{}, fmt.Errorf("unknown provider country: %s", providerID)
	}
}

// NormalizeTaxID strips formatting (dots, slashes, dashes) from a tax ID so it
// can be matched against the whitelist's digit-only entries.
func NormalizeTaxID(raw string) string { return govtypes.NormalizeTaxID(raw) }

// isWhitelistedTaxID reports whether a raw tax ID (CNPJ/EIN/BN, complete or
// masked) belongs to a tax entity in the whitelist.
func isWhitelistedTaxID(raw string) bool {
	_, ok := govtypes.WhitelistedEntityForID(NormalizeTaxID(raw))
	return ok
}
