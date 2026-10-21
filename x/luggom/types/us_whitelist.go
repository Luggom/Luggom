package types

import "strings"

type USTaxEntityKind string

const (
	USTaxEntityKindTax USTaxEntityKind = "tax"
)

// DefaultUSTaxEntities defines the initial MVP 1.5 whitelist for US tax entities.
// These are canonical identifiers used by MsgMineUSD.tax_entity.
func DefaultUSTaxEntities() []string {
	return []string{
		"irs",
		"ca_ftb", // California Franchise Tax Board (state pilot)
		"ny_dtf", // New York Department of Taxation and Finance (state pilot)
	}
}

// NormalizeUSTaxEntity normalizes identifiers for comparison (legacy slug-only).
// Prefer CanonicalUSTaxEntityKey for new code.
func NormalizeUSTaxEntity(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

// IsUSTaxEntityWhitelisted returns true when tax entity is allowed for mining.
func IsUSTaxEntityWhitelisted(taxEntity string, whitelist []string) bool {
	key := CanonicalUSTaxEntityKey(taxEntity)
	if key == "" {
		return false
	}
	for _, w := range whitelist {
		if canonicalWhitelistEntry(w) == key {
			return true
		}
	}
	return false
}

// ResolveUSTaxEntityKind resolves whether an entity belongs to the tax flow (whitelisted).
func ResolveUSTaxEntityKind(taxEntity string) (USTaxEntityKind, bool) {
	key := CanonicalUSTaxEntityKey(taxEntity)
	if key == "" {
		return "", false
	}
	for _, w := range DefaultUSTaxEntities() {
		if canonicalWhitelistEntry(w) == key {
			return USTaxEntityKindTax, true
		}
	}
	return "", false
}
