package keeper

import (
	"context"
	"strings"

	"luggom/x/luggom/types"
)

// VerifyApiAttestation valida a atestação da proof para MsgMineTaxPayment.
//
//	Fase 1 (MVP local): confia na API — só valida o comprimento mínimo.
//	Fase 2 (P0, §13.97): verifica uma assinatura ECDSA secp256k1 cujo assinante
//	  tem de ser `params.api_verifier_address` (o notário/attestor on-chain).
//	  A chave pública é RECUPERADA da própria assinatura (formato compacto
//	  65 bytes), pelo que não é preciso guardá-la em params.
//	Fase 3: verificação ZK on-chain (futuro).
func (k Keeper) VerifyApiAttestation(ctx context.Context, msg *types.MsgMineTaxPayment) error {
	phase := k.GetTlsnotaryPhase(ctx)

	switch phase {
	case 1:
		// Fase 1: confia na API, valida apenas que a atestação não está vazia.
		if len(strings.TrimSpace(msg.ApiAttestation)) < 64 {
			return types.ErrInvalidAttestation.Wrap("attestation too short (min 64 chars)")
		}
		return nil
	case 2:
		params, err := k.Params.Get(ctx)
		if err != nil {
			return types.ErrInvalidAttestation.Wrapf("cannot read params: %v", err)
		}
		// Fail-closed: sem endereço do attestor configurado não há como verificar
		// nada — rejeita em vez de aceitar (era exatamente o furo do P0).
		if strings.TrimSpace(params.ApiVerifierAddress) == "" {
			return types.ErrInvalidAttestation.Wrap("phase 2: api_verifier_address is not configured")
		}

		// A1.4: o digest cobre TODOS os campos que afetam o resultado — não só
		// os 5 originais. Sem isto, uma API comprometida poderia manipular
		// `payment_date` (janela de 365d, 1205) e `tax_nature_hash` (1207)
		// sem quebrar a assinatura. E o `provider_id` (A4, §13.131): sem ele no
		// digest, a API trocava o banco e a chain não dava por isso.
		digest := types.BuildAttestationDigest(types.AttestationFields{
			ProofCommitment: msg.ProofCommitment,
			PaymentID:       msg.PaymentId,
			PaymentAmount:        msg.PaymentAmount.String(),
			TaxEntity:       msg.TaxEntity,
			CurrencyCode:    msg.CurrencyCode,
			PaymentDate:     msg.PaymentDate,
			TaxNatureHash:   msg.TaxNatureHash,
			PayerTaxID:      msg.PayerTaxId,
			ProviderID:      msg.ProviderId,
		})
		if err := types.VerifyAttestationSignature(msg.ApiAttestation, digest, params.ApiVerifierAddress); err != nil {
			return types.ErrInvalidAttestation.Wrap(err.Error())
		}
		return nil
	case 3:
		// Fase 3 (future): ZK proof verification on-chain
		return types.ErrInvalidAttestation.Wrap("phase 3 ZK verification not yet implemented")
	default:
		return types.ErrInvalidAttestation.Wrapf("unsupported tlsnotary phase: %d", phase)
	}
}
