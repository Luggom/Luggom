package types

import (
	"errors"
	"fmt"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// NewParams cria os params por omissão, JÁ na classificação §13.170 A–E.
//
// Só ficam aqui: os endereços (C), as listas operacionais governáveis (E) e os
// pesos do tally da governança. Tudo o resto foi APAGADO (A), CONGELADO em
// constantes de código (B — ver frozen.go) ou DERIVADO (D).
func NewParams() Params {
	return Params{
		// C — endereços. founder_address é preenchido no genesis e fica TRANCADO.
		FounderAddress:          "",
		ApiVerifierAddress:      "",
		UsdOracleUpdaterAddress: "",
		// E — listas operacionais (67% de governança).
		PoolActive:                false,
		TaxEntityWhitelist:        DefaultGlobalTaxEntities(),
		GovernmentEntityBlacklist: DefaultGovernmentEntities(),
		BankProviders:             DefaultBankProviders(),
		PoolAssets:                DefaultPoolAssets(),
		// Tally da governança (por decidir com a comunidade — §13.170 §4).
		GovValidatorMinForWeightShift: 100,
		GovLlcVoteWeightBps:           0,
		// Pré-pool o Fundador decide sozinho (decisão do Fundador, 2026-09-18).
		GovFounderVoteWeightBps:    10_000,
		GovValidatorsVoteWeightBps: 0,
	}
}

// DefaultParams returns a default set of parameters.
func DefaultParams() Params {
	return NewParams()
}

// Validate validates the set of params (só os campos que restam — §13.170).
func (p Params) Validate() error {
	// C — endereços, quando preenchidos.
	if p.FounderAddress != "" {
		if _, err := sdk.AccAddressFromBech32(p.FounderAddress); err != nil {
			return fmt.Errorf("invalid founder_address: %w", err)
		}
	}
	if p.ApiVerifierAddress != "" {
		if _, err := sdk.AccAddressFromBech32(p.ApiVerifierAddress); err != nil {
			return fmt.Errorf("invalid api_verifier_address: %w", err)
		}
	}
	if p.UsdOracleUpdaterAddress != "" {
		if _, err := sdk.AccAddressFromBech32(p.UsdOracleUpdaterAddress); err != nil {
			return fmt.Errorf("invalid usd_oracle_updater_address: %w", err)
		}
	}

	// Tally da governança.
	sum := uint64(p.GovLlcVoteWeightBps) + uint64(p.GovFounderVoteWeightBps) + uint64(p.GovValidatorsVoteWeightBps)
	if sum != 10_000 {
		return fmt.Errorf("gov vote weights bps must sum to 10000, got %d", sum)
	}
	if p.GovLlcVoteWeightBps > 10_000 || p.GovFounderVoteWeightBps > 10_000 || p.GovValidatorsVoteWeightBps > 10_000 {
		return errors.New("each gov_*_vote_weight_bps must be <= 10000")
	}
	if p.GovValidatorMinForWeightShift > 1_000_000 {
		return errors.New("gov_validator_min_for_weight_shift too large")
	}

	// E — listas operacionais.
	if len(p.TaxEntityWhitelist) == 0 {
		return errors.New("tax_entity_whitelist cannot be empty")
	}
	for i, id := range p.GovernmentEntityBlacklist {
		raw := strings.TrimSpace(id)
		if len(raw) < 5 {
			return fmt.Errorf("government_entity_blacklist[%d] too short: %s", i, id)
		}
		prefix := strings.ToLower(raw[:3])
		if prefix != "br:" && prefix != "us:" && prefix != "ca:" {
			return fmt.Errorf("government_entity_blacklist[%d] must start with br:/us:/ca: (got %q)", i, id)
		}
		digits := len(NormalizeTaxID(raw[3:]))
		if digits != 14 && digits != 9 {
			return fmt.Errorf("government_entity_blacklist[%d] must be 9 (EIN/BN) or 14 (CNPJ) digits, got %d: %s", i, digits, id)
		}
	}
	if len(p.BankProviders) == 0 {
		return errors.New("bank_providers cannot be empty")
	}
	// §14 — destinos permitidos para a reserva do notariado. Vazia é válido
	// (fail-closed: a reserva não se gasta), mas cada entrada tem de ser um
	// endereço válido e não pode haver duplicados.
	if len(p.NotaryPayees) > 64 {
		return fmt.Errorf("notary_payees cannot have more than 64 entries, got %d", len(p.NotaryPayees))
	}
	seenPayee := make(map[string]struct{}, len(p.NotaryPayees))
	for i, payee := range p.NotaryPayees {
		if _, err := sdk.AccAddressFromBech32(payee); err != nil {
			return fmt.Errorf("notary_payees[%d] is not a valid address: %w", i, err)
		}
		if _, dup := seenPayee[payee]; dup {
			return fmt.Errorf("notary_payees[%d] is duplicated: %s", i, payee)
		}
		seenPayee[payee] = struct{}{}
	}
	if len(p.PoolAssets) == 0 {
		return errors.New("pool_assets cannot be empty")
	}

	return nil
}
