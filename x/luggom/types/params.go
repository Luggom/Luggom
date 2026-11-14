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
		OtcBtcDenom:               "btc",
		// Tally da governança (por decidir com a comunidade — §13.170 §4).
		GovValidatorMinForWeightShift: 100,
		// Legado: a LBC não tem classe de voto desde 13/11/2026. Mantido para
		// compatibilidade de proto/genesis; o tally ignora-o.
		GovLlcVoteWeightBps: 0,
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
		addr, err := sdk.AccAddressFromBech32(payee)
		if err != nil {
			return fmt.Errorf("notary_payees[%d] is not a valid address: %w", i, err)
		}
		// Forma canónica: sem isto, variantes de maiúsculas passam o teste de
		// duplicados e nunca casam com o destino de um pagamento.
		if addr.String() != payee {
			return fmt.Errorf("notary_payees[%d] is not in canonical form: got %q, want %q", i, payee, addr.String())
		}
		if _, dup := seenPayee[payee]; dup {
			return fmt.Errorf("notary_payees[%d] is duplicated: %s", i, payee)
		}
		seenPayee[payee] = struct{}{}
	}
	if p.OtcBtcDenom == "" {
		return errors.New("otc_btc_denom cannot be empty")
	}
	if err := sdk.ValidateDenom(p.OtcBtcDenom); err != nil {
		return fmt.Errorf("otc_btc_denom is not a valid denom: %w", err)
	}
	if len(p.PoolAssets) == 0 {
		return errors.New("pool_assets cannot be empty")
	}
	if len(p.PoolAssets) > 128 {
		return fmt.Errorf("pool_assets cannot have more than 128 entries, got %d", len(p.PoolAssets))
	}
	seenAsset := make(map[string]struct{}, len(p.PoolAssets))
	for i, denom := range p.PoolAssets {
		if denom == "" {
			return fmt.Errorf("pool_assets[%d] cannot be empty", i)
		}
		if strings.TrimSpace(denom) != denom {
			return fmt.Errorf("pool_assets[%d] has leading or trailing whitespace: %q", i, denom)
		}
		if denom == "uluggom" {
			return errors.New("pool_assets cannot include the base denom uluggom")
		}
		if err := sdk.ValidateDenom(denom); err != nil {
			return fmt.Errorf("pool_assets[%d] is not a valid denom: %w", i, err)
		}
		if strings.HasPrefix(denom, "ibc/") {
			hash := strings.TrimPrefix(denom, "ibc/")
			if len(hash) != 64 {
				return fmt.Errorf("pool_assets[%d] has a malformed IBC voucher (want ibc/<64 hex>): %s", i, denom)
			}
			for _, c := range hash {
				if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F') || (c >= 'a' && c <= 'f')) {
					return fmt.Errorf("pool_assets[%d] has a malformed IBC voucher hash: %s", i, denom)
				}
			}
		}
		if _, dup := seenAsset[denom]; dup {
			return fmt.Errorf("pool_assets[%d] is duplicated: %s", i, denom)
		}
		seenAsset[denom] = struct{}{}
	}

	return nil
}
