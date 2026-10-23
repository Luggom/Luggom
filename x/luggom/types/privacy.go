package types

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// privacy.go — pseudonimização dos identificadores do recibo antes de irem para a chain.
//
// `payment_id` (E2E do PIX / id do comprovante) e `payer_tax_id` (CNPJ/EIN/BN do
// pagador) são os únicos campos do recibo que identificam diretamente o pagamento
// ou o contribuinte. Ambos vão no payload assinado e na mensagem, por isso são
// hasheados ANTES de a tx ser construída: a chain guarda um pseudónimo, não o E2E
// nem o CNPJ.
//
// O keeper recusa qualquer valor que não seja um SHA-256 hex (IsHash64), pelo que
// um cliente modificado não consegue voltar a pôr o valor cru on-chain.
//
// O match por subconjunto (recibos que mascaram parte do CNPJ) continua a
// funcionar: em vez de comparar strings, comparam-se hashes dos substrings com
// ≥6 caracteres — a mesma semântica de `IsIDOfWhitelistedEntity`.

const identifierHashLen = 64 // sha256 em hex

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HashTaxID normaliza um EIN/CNPJ/BN e devolve o SHA-256 em hex. Vazio → vazio.
// A normalização é a mesma usada pela blacklist e pelo mapa de entidades fiscais,
// para os hashes serem comparáveis.
func HashTaxID(raw string) string {
	n := stripCountryPrefix(NormalizeTaxID(raw))
	if n == "" {
		return ""
	}
	return hashString(n)
}

// HashPaymentID devolve o SHA-256 em hex de um identificador de pagamento
// (E2E do PIX / id do comprovante). Vazio → vazio.
func HashPaymentID(raw string) string {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if s == "" {
		return ""
	}
	return hashString(s)
}

// IsHash64 reporta se s é exatamente um SHA-256 em hex (64 caracteres).
func IsHash64(s string) bool {
	if len(s) != identifierHashLen {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}

// taxIDHashes devolve os hashes de um ID e de todos os seus substrings com pelo
// menos minSubstringLen caracteres. É o equivalente hasheado do match por
// subconjunto: um recibo que mascara o CNPJ deixa visível um trecho, e o hash
// desse trecho tem de continuar a bater com o ID completo.
func taxIDHashes(normalizedID string, minSubstringLen int) []string {
	if normalizedID == "" {
		return nil
	}
	out := make([]string, 0, 48)
	seen := make(map[string]struct{}, 48)
	add := func(s string) {
		if len(s) < minSubstringLen {
			return
		}
		h := hashString(s)
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		out = append(out, h)
	}
	add(normalizedID)
	for i := 0; i < len(normalizedID); i++ {
		for j := i + minSubstringLen; j <= len(normalizedID); j++ {
			add(normalizedID[i:j])
		}
	}
	return out
}

// IsGovernmentEntityHash reporta se o hash corresponde ao ID de uma entidade
// governamental da lista (ou a um trecho dele, ≥6 caracteres).
func IsGovernmentEntityHash(hash string, govList []string) bool {
	h := strings.ToLower(strings.TrimSpace(hash))
	if !IsHash64(h) {
		return false
	}
	for _, g := range govList {
		norm := stripCountryPrefix(NormalizeTaxID(g))
		for _, candidate := range taxIDHashes(norm, 6) {
			if candidate == h {
				return true
			}
		}
	}
	return false
}

// taxEntityIDHashes é o conjunto pré-computado dos hashes dos IDs da whitelist
// fiscal (e dos seus substrings ≥6). Calculado uma vez no arranque.
var taxEntityIDHashes = func() map[string]struct{} {
	set := make(map[string]struct{}, 32768)
	for _, ids := range taxEntityIDs {
		for _, full := range ids {
			norm := stripCountryPrefix(NormalizeTaxID(full))
			for _, h := range taxIDHashes(norm, 6) {
				set[h] = struct{}{}
			}
		}
	}
	return set
}()

// IsHashOfWhitelistedEntity reporta se o hash corresponde ao ID de uma entidade
// da whitelist fiscal (ou a um trecho dele, ≥6 caracteres) — o fallback que
// bloqueia qualquer governo que receba impostos, mesmo os que não estão na
// blacklist pequena.
func IsHashOfWhitelistedEntity(hash string) bool {
	h := strings.ToLower(strings.TrimSpace(hash))
	if !IsHash64(h) {
		return false
	}
	_, ok := taxEntityIDHashes[h]
	return ok
}
