package keeper

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// MineTaxPayment processes a TLSNotary tax-payment mining request (Fase B).
// Unified global flow: EUA + Brasil via Reclaim Protocol proof.
func (k Keeper) MineTaxPayment(ctx context.Context, msg *types.MsgMineTaxPayment) (*types.MsgMineTaxPaymentResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get params: %w", err)
	}

	// 1. Validate tax_entity against global whitelist
	taxEntity := strings.TrimSpace(msg.TaxEntity)
	if taxEntity == "" {
		return nil, types.ErrInvalidTaxEntity.Wrap("tax_entity is empty")
	}
	whitelisted, err := k.IsTaxEntityWhitelisted(ctx, taxEntity)
	if err != nil {
		return nil, fmt.Errorf("whitelist check: %w", err)
	}
	if !whitelisted {
		return nil, types.ErrInvalidTaxEntity.Wrap(taxEntity)
	}

	// 1c. O BANCO tem de estar na whitelist ON-CHAIN (§13.131, achado A4).
	// Antes, só a API o verificava — e a API pode ser contornada (um cliente a
	// falar directamente com a chain, ou uma API comprometida). O provider_id vai
	// no digest assinado pelo notário, logo não pode ser trocado depois.
	// Fail-closed: id vazio ou fora da lista ⇒ rejeita.
	providerID := strings.TrimSpace(msg.ProviderId)
	if !types.IsBankProviderWhitelisted(providerID, params.BankProviders) {
		return nil, types.ErrBankProviderNotWhitelisted.Wrapf("%q", providerID)
	}

	// 1b. Block government-to-government mining (anti-inflation / anti-sybil, FASE 4-A..F)
	// Two layers:
	//   (1) payer_tax_id matches the on-chain government blacklist (params, gov-67% editable)
	//   (2) fallback: payer_tax_id matches ANY tax entity in the whitelist (slug→ID map) —
	//       covers every municipality/county/province in the whitelist without listing them all.
	payerTaxID := strings.TrimSpace(msg.PayerTaxId)
	if payerTaxID != "" {
		norm := types.NormalizeTaxID(payerTaxID)
		if norm != "" {
			// Layer 1: blacklist params (governable list)
			if types.IsGovernmentEntity(norm, params.GovernmentEntityBlacklist) {
				return nil, types.ErrGovernmentEntityCannotMine.Wrapf(
					"payer %s is a government entity — government-to-government transfers cannot mine LUG",
					payerTaxID,
				)
			}
			// Layer 2: fallback — payer ID belongs to a whitelisted tax entity (government)
			if types.IsIDOfWhitelistedEntity(norm) {
				return nil, types.ErrGovernmentEntityCannotMine.Wrapf(
					"payer %s is a government entity (tax whitelist) — government-to-government transfers cannot mine LUG",
					payerTaxID,
				)
			}
		}
	}

	// 2. Validate currency_code (BCA: USD, BRL, CAD)
	cc := strings.ToUpper(strings.TrimSpace(msg.CurrencyCode))
	if cc == "" {
		cc = "USD"
	}
	if cc != "USD" && cc != "BRL" && cc != "CAD" {
		return nil, types.ErrInvalidCurrencyCode.Wrap(cc)
	}

	// 3. Validate proof_commitment (anti-double-spend)
	pc := strings.TrimSpace(msg.ProofCommitment)
	if len(pc) < 32 {
		return nil, fmt.Errorf("proof_commitment too short (min 32 chars)")
	}
	alreadyUsed, err := k.UsedDocumentHashes.Has(ctx, pc)
	if err != nil {
		return nil, fmt.Errorf("failed to check proof_commitment: %w", err)
	}
	if alreadyUsed {
		return nil, types.ErrProofAlreadyUsed
	}

	// 4. Validate payment_id (anti-replay)
	pid := strings.TrimSpace(msg.PaymentId)
	if len(pid) < 4 {
		return nil, fmt.Errorf("payment_id too short (min 4 chars)")
	}
	pidUsed, err := k.UsedPaymentIDs.Has(ctx, pid)
	if err != nil {
		return nil, fmt.Errorf("failed to check payment_id: %w", err)
	}
	if pidUsed {
		return nil, types.ErrPaymentIDAlreadyUsed
	}

	// 5. Check staleness
	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime()
	maxAge := types.TaxPaymentMaxAgeDaysFrozen
	if maxAge == 0 {
		maxAge = 365
	}
	paymentTime := time.Unix(msg.PaymentDate, 0)
	if blockTime.Sub(paymentTime) > time.Duration(maxAge)*24*time.Hour {
		return nil, types.ErrStalePayment.Wrapf("payment date %s exceeds max age %d days",
			paymentTime.Format("2006-01-02"), maxAge)
	}

	// 5b. Validate tax_nature_hash (anti-cycle, §13.34): the API extractor must
	// have proven the payment is a real tax collection (DARF/IPTU/IPVA, property
	// tax, CRA BN+RT). A common PIX/transfer to a whitelisted CNPJ/EIN is rejected.
	//
	// O tax_nature_hash NÃO é anti-replay: ele representa a NATUREZA FISCAL do
	// comprovante (igual para muitos comprovantes do mesmo tipo, ex: todo PIX
	// IPTU tem o mesmo fragmento). O anti-replay real é proof_commitment +
	// payment_id (passos 3 e 4 acima). Registar o tax_nature_hash em
	// UsedDocumentHashes faria a 2ª mineração do MESMO tipo de imposto ser
	// rejeitada como double-spend — falso positivo.
	taxNatureHash := strings.TrimSpace(msg.TaxNatureHash)
	if len(taxNatureHash) < 32 {
		return nil, types.ErrMissingTaxNature.Wrap("tax_nature_hash too short (min 32 chars)")
	}

	// 6. Verify API attestation (Fase 1: trust API, validate minimum length)
	if err := k.VerifyApiAttestation(ctx, msg); err != nil {
		return nil, err
	}

	// 7. Validate payment_amount
	if msg.PaymentAmount.LTE(sdkmath.LegacyZeroDec()) {
		return nil, fmt.Errorf("payment_amount must be positive")
	}

	// 8. Convert BRL/CAD → USD if needed
	amountUSD := msg.PaymentAmount
	if cc == "BRL" {
		brlPerUSD, err := k.brlUsdRate(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve BRL/USD rate: %w", err)
		}
		amountUSD = msg.PaymentAmount.Quo(brlPerUSD)
	} else if cc == "CAD" {
		cadPerUSD, err := k.cadUsdRate(ctx)
		if err != nil {
			return nil, fmt.Errorf("cannot resolve CAD/USD rate: %w", err)
		}
		amountUSD = msg.PaymentAmount.Quo(cadPerUSD)
	}

	// 9. Resolve USD/LUG parity via existing oracle
	usdParity, err := k.usdParityForMineUSD(ctx)
	if err != nil {
		return nil, err
	}

	// 10. Calculate LUG amount
	amountLUG := amountUSD.Quo(usdParity)
	amountLUGInt := amountLUG.MulInt64(1_000_000_000_000_000_000).TruncateInt()
	if amountLUGInt.IsZero() {
		return nil, fmt.Errorf("calculated LUG amount is zero (payment_amount too small)")
	}

	// 10b. Annual mining cap (MVP 2.0 §13.49/§13.52): 365d sliding window, 15B −5% a.a.
	if err := k.recordMinedAmount(ctx, amountLUGInt); err != nil {
		return nil, err
	}

	// §13.117: fase vigente (Fase 1 / Transição / Fase 2) e split da mineração.
	phase, err := k.miningPhase(ctx)
	if err != nil {
		return nil, err
	}
	split := computeMiningSplit(amountLUGInt, phase)

	// 12. Resolve miner address
	minerAddr, err := k.addressCodec.StringToBytes(msg.MinerAddress)
	if err != nil {
		return nil, fmt.Errorf("invalid miner_address: %w", err)
	}

	// 13. Mint + distribuir o split (o envio ao cofre OTC é fail-closed).
	if err := k.distributeMiningSplit(ctx, params, split, minerAddr); err != nil {
		return nil, err
	}

	// 14. Record anti-replay hashes (proof_commitment + payment_id).
	// O tax_nature_hash NÃO é registado: é natureza fiscal (igual por tipo de
	// imposto), não identifica uma transação única.
	if err := k.UsedDocumentHashes.Set(ctx, pc); err != nil {
		return nil, fmt.Errorf("failed to record proof_commitment: %w", err)
	}
	if err := k.UsedPaymentIDs.Set(ctx, pid); err != nil {
		return nil, fmt.Errorf("failed to record payment_id: %w", err)
	}

	// 15. Emit event
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	attrs := []sdk.Attribute{
		sdk.NewAttribute("miner", msg.MinerAddress),
		sdk.NewAttribute("payment_amount", msg.PaymentAmount.String()),
		sdk.NewAttribute("tax_entity", msg.TaxEntity),
		sdk.NewAttribute("currency_code", cc),
		sdk.NewAttribute("proof_commitment", pc),
		sdk.NewAttribute("tax_nature_hash", taxNatureHash),
		sdk.NewAttribute("payment_id", pid),
	}
	attrs = append(attrs, miningSplitAttributes(split)...)
	attrs = append(attrs,
		sdk.NewAttribute("pool_active", fmt.Sprintf("%t", k.PoolOpen(ctx))),
		sdk.NewAttribute("presale_active", fmt.Sprintf("%t", phase == types.OtcPhase1)),
	)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(types.EventTypeMine, attrs...))

	return &types.MsgMineTaxPaymentResponse{
		AmountMined:  split.Miner,
		TaxRoot:      split.Founder,
		TaxStaking:   split.Staking,
		TaxValidator: split.Pool,
	}, nil
}

// brlUsdRate returns a fixed BRL/USD rate (MVP ~5.0 BRL per 1 USD).
// In production this should be replaced by an oracle.
func (k Keeper) brlUsdRate(ctx context.Context) (sdkmath.LegacyDec, error) {
	return sdkmath.LegacyNewDecWithPrec(500000, 5), nil // 5.0
}

// cadUsdRate returns a fixed CAD/USD rate (MVP ~1.37 CAD per 1 USD).
// In production this should be replaced by an oracle.
func (k Keeper) cadUsdRate(ctx context.Context) (sdkmath.LegacyDec, error) {
	return sdkmath.LegacyNewDecWithPrec(137000, 5), nil // 1.37
}
