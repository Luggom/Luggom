package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

// taxNatureHashForTest returns a valid-length tax nature hash (64 hex chars).
func taxNatureHashForTest() string {
	return "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
}

func TestMineTaxPayment_TaxNatureHashRequired(t *testing.T) {
	f := initMineFixture(t)
	minerStr, _ := makeAddr(t, f.addressCodec, 0x33)

	params := devParamsForTest()
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	base := taxPaymentMsgForTest(t, f, minerStr, "tax-nature", 66)
	base.PaymentAmount = sdkmath.LegacyMustNewDecFromStr("240.50")
	base.PaymentDate = time.Now().Add(-24 * time.Hour).Unix()

	t.Run("missing tax_nature_hash is rejected", func(t *testing.T) {
		msg := *base
		msg.TaxNatureHash = ""
		signAttestationForTest(t, &msg)
		_, err := f.keeper.MineTaxPayment(f.ctx, &msg)
		require.Error(t, err)
		require.ErrorIs(t, err, types.ErrMissingTaxNature)
	})

	t.Run("short tax_nature_hash is rejected", func(t *testing.T) {
		msg := *base
		msg.TaxNatureHash = "short"
		signAttestationForTest(t, &msg)
		_, err := f.keeper.MineTaxPayment(f.ctx, &msg)
		require.Error(t, err)
		require.ErrorIs(t, err, types.ErrMissingTaxNature)
	})

	t.Run("valid tax_nature_hash mines successfully", func(t *testing.T) {
		msg := *base
		msg.TaxNatureHash = taxNatureHashForTest()
		signAttestationForTest(t, &msg)
		resp, err := f.keeper.MineTaxPayment(f.ctx, &msg)
		require.NoError(t, err)
		require.True(t, resp.AmountMined.IsPositive())
	})

	t.Run("same tax_nature_hash with different proof/payment mines ok (hash is NOT anti-replay, §13.94)", func(t *testing.T) {
		// §13.94 (BUG 3): o tax_nature_hash representa a NATUREZA FISCAL (igual para
		// muitos comprovantes do mesmo tipo), NÃO é anti-replay. O anti-replay real
		// é proof_commitment + payment_id. Reutilizar o hash com proof/payment
		// diferentes DEVE minerar com sucesso (não é falso posit).
		msg := *base
		msg.TaxNatureHash = taxNatureHashForTest()
		msg.ProofCommitment = "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
		msg.PaymentId = types.HashPaymentID("pay-67890")
		signAttestationForTest(t, &msg)
		resp, err := f.keeper.MineTaxPayment(f.ctx, &msg)
		require.NoError(t, err)
		require.True(t, resp.AmountMined.IsPositive())
	})
}
