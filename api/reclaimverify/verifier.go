package reclaimverify

import (
	"fmt"
	"time"
)

// VerifiedClaims holds the structured data extracted from a TLSNotary proof.
type VerifiedClaims struct {
	PaymentAmount    string `json:"payment_amount"`
	PaymentCurrency  string `json:"payment_currency"`
	RecipientAccount string `json:"recipient_account"`
	RecipientName    string `json:"recipient_name"`
	BankIdentity     string `json:"bank_identity"`
	TransactionID    string `json:"transaction_id"`
	PaymentDate      int64  `json:"payment_date"`
	TaxEntityMatch   string `json:"tax_entity_match"`
	PayerAccount     string `json:"payer_account"` // account number/name of the payer (from bank TLS session)
	PayerTaxID       string `json:"payer_tax_id"`  // EIN (EUA) or CNPJ (Brasil) of the payer
	Description      string `json:"description"`   // tax-nature fragment (barcode/PIX arrecadação/property tax bill/CRA BN+RT)
}

// Verify foi REMOVIDO (2026-09-10): parseava JSON sem qualquer verificação
// criptográfica, portanto qualquer pessoa podia forjar um comprovante. Como a
// API é o único guardião (assina a attestation que a chain confia), esse caminho
// invalidava o portão da mineração e o "1 prova = 1 conta" da ativação.
//
// A única forma aceite é o bundle TLSNotary real: `taxproof.VerifyTlsnProof`.
// ValidateClaims (abaixo) continua a ser usado pelo taxproof para validar as
// claims extraídas do corpo atestado.

// ValidateClaims performs sanity checks on extracted claims.
func ValidateClaims(claims *VerifiedClaims) error {
	if claims == nil {
		return fmt.Errorf("claims is nil")
	}
	if claims.PaymentAmount == "" {
		return fmt.Errorf("payment_amount is empty")
	}
	cc := claims.PaymentCurrency
	if cc != "USD" && cc != "BRL" && cc != "EUR" && cc != "CAD" {
		return fmt.Errorf("unsupported currency: %s", cc)
	}
	if claims.TransactionID == "" {
		return fmt.Errorf("transaction_id is empty")
	}
	if claims.PaymentDate > time.Now().Unix()+86400 {
		return fmt.Errorf("payment_date is in the future")
	}
	if claims.TaxEntityMatch == "" {
		return fmt.Errorf("tax_entity_match is empty")
	}
	return nil
}
