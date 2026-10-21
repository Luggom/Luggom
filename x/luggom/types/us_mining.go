package types

import (
	"fmt"
	"strings"
	"unicode"
)

// Minimum lengths for MsgMineUSD proof fields (MVP 1.5; tighten for production).
const (
	MinMsgMineUSDDocumentHashLen = 32
	MinMsgMineUSDProofIrsLen     = 8
	MinMsgMineUSDSsnHashLen      = 32
)

// ValidateMsgMineUSDAnchors enforces non-trivial commitment sizes for USD mining.
func ValidateMsgMineUSDAnchors(documentHash, proofIrs, ssnHash string) error {
	if len(strings.TrimSpace(documentHash)) < MinMsgMineUSDDocumentHashLen {
		return fmt.Errorf("document_hash must be at least %d characters", MinMsgMineUSDDocumentHashLen)
	}
	if len(strings.TrimSpace(proofIrs)) < MinMsgMineUSDProofIrsLen {
		return fmt.Errorf("proof_irs must be at least %d characters", MinMsgMineUSDProofIrsLen)
	}
	if len(strings.TrimSpace(ssnHash)) < MinMsgMineUSDSsnHashLen {
		return fmt.Errorf("ssn_hash must be at least %d characters", MinMsgMineUSDSsnHashLen)
	}
	return nil
}

// onlyDigits extracts ASCII digits from s.
func onlyDigits(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CanonicalUSTaxEntityKey builds a stable key for whitelist matching:
//   - 9-digit EIN (with optional formatting) → "ein:#########"
//   - otherwise → "slug:<lower trimmed>"
func CanonicalUSTaxEntityKey(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	d := onlyDigits(v)
	if len(d) == 9 {
		return "ein:" + d
	}
	return "slug:" + strings.ToLower(v)
}

// canonicalWhitelistEntry normalizes a whitelist entry the same way as user input.
func canonicalWhitelistEntry(w string) string {
	return CanonicalUSTaxEntityKey(w)
}
