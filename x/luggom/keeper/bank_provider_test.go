package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

// TestMineTaxPayment_BankProviderNotWhitelisted — §13.131, achado A4.
//
// Antes: o portão do BANCO existia **só na API**. Um cliente a falar
// directamente com a chain (ou uma API comprometida) minava com prova de
// QUALQUER banco — mesmo fora da whitelist on-chain `bank_providers`.
//
// Agora: a chain verifica o `provider_id` da msg contra a whitelist
// (governável), e o campo vai no digest assinado pelo notário — logo a API não
// o pode trocar depois de a prova estar feita.
func TestMineTaxPayment_BankProviderNotWhitelisted(t *testing.T) {
	f := initMineFixture(t)
	minerStr, _ := makeAddr(t, f.addressCodec, 0x34)

	params := devParamsForTest()
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	// §13.170 Grupo B: fase 2 — a atestação é ASSINADA; como os subtestes mudam o
	// provider_id (que vai no digest), é preciso assinar de novo em cada um.
	base := taxPaymentMsgForTest(t, f, minerStr, "a4", 66)
	base.PaymentAmount = sdkmath.LegacyMustNewDecFromStr("240.50")
	base.PaymentDate = time.Now().Add(-24 * time.Hour).Unix()
	base.TaxNatureHash = taxNatureHashForTest()

	t.Run("banco fora da whitelist é recusado", func(t *testing.T) {
		msg := *base
		msg.ProviderId = "banco-inventado"
		signAttestationForTest(t, &msg)
		_, err := f.keeper.MineTaxPayment(f.ctx, &msg)
		require.ErrorIs(t, err, types.ErrBankProviderNotWhitelisted)
	})

	t.Run("provider_id vazio é recusado (fail-closed)", func(t *testing.T) {
		msg := *base
		msg.ProviderId = ""
		signAttestationForTest(t, &msg)
		_, err := f.keeper.MineTaxPayment(f.ctx, &msg)
		require.ErrorIs(t, err, types.ErrBankProviderNotWhitelisted)
	})

	t.Run("banco na whitelist passa o portão", func(t *testing.T) {
		msg := *base
		msg.ProviderId = "viacredi"
		signAttestationForTest(t, &msg)
		_, err := f.keeper.MineTaxPayment(f.ctx, &msg)
		require.NoError(t, err)
	})

	t.Run("whitelist ON-CHAIN manda: um banco removido por governança deixa de passar", func(t *testing.T) {
		msg := *base
		msg.ProviderId = "viacredi"

		// A governança (67%) remove o banco da lista…
		p := types.DefaultParams()
		kept := make([]string, 0, len(p.BankProviders))
		for _, id := range p.BankProviders {
			if id != "viacredi" {
				kept = append(kept, id)
			}
		}
		p.BankProviders = kept
		require.NoError(t, f.keeper.Params.Set(f.ctx, p))

		// …e a chain deixa de aceitar a prova desse banco, sem deploy.
		_, err := f.keeper.MineTaxPayment(f.ctx, &msg)
		require.ErrorIs(t, err, types.ErrBankProviderNotWhitelisted)
	})
}
