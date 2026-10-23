package keeper_test

import (
	"strings"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

// privacy_test.go — os identificadores do recibo nunca vão em claro para a chain.
//
// `payment_id` (E2E do PIX) e `payer_tax_id` (CNPJ/EIN/BN) são hasheados pelo
// extrator; a chain recusa qualquer valor que não seja um SHA-256 hex e continua
// a aplicar o portão 1206 sobre o hash.

func TestMine_RejectsRawPaymentID(t *testing.T) {
	f := initMineFixture(t)
	miner, _ := makeAddr(t, f.addressCodec, 0x11)

	msg := taxPaymentMsgForTest(t, f, miner, "raw-payment-id", 100)
	msg.PaymentId = "E82639451202607152106LZ1QCIXXYE6" // E2E cru do comprovante
	signAttestationForTest(t, msg)

	_, err := f.keeper.MineTaxPayment(f.ctx, msg)
	require.ErrorContains(t, err, "payment_id must be a SHA-256 hex")
}

func TestMine_RejectsRawPayerTaxID(t *testing.T) {
	f := initMineFixture(t)
	miner, _ := makeAddr(t, f.addressCodec, 0x11)

	msg := taxPaymentMsgForTest(t, f, miner, "raw-payer-id", 100)
	msg.PayerTaxId = "12-3456789"
	signAttestationForTest(t, msg)

	_, err := f.keeper.MineTaxPayment(f.ctx, msg)
	require.ErrorIs(t, err, types.ErrInvalidAttestation)
}

func TestMine_BlocksHashedGovernmentPayer(t *testing.T) {
	const saoPaulo = "46392130000118" // prefeitura de São Paulo (blacklist default)

	t.Run("ID completo", func(t *testing.T) {
		f := initMineFixture(t)
		miner, _ := makeAddr(t, f.addressCodec, 0x11)

		msg := taxPaymentMsgForTest(t, f, miner, "gov-full", 100)
		msg.PayerTaxId = types.HashTaxID(saoPaulo)
		signAttestationForTest(t, msg)

		_, err := f.keeper.MineTaxPayment(f.ctx, msg)
		require.ErrorIs(t, err, types.ErrGovernmentEntityCannotMine)
	})

	t.Run("CNPJ mascarado", func(t *testing.T) {
		f := initMineFixture(t)
		miner, _ := makeAddr(t, f.addressCodec, 0x11)

		msg := taxPaymentMsgForTest(t, f, miner, "gov-masked", 100)
		msg.PayerTaxId = types.HashTaxID("3921300001") // trecho visível de **392.130/0001-**
		signAttestationForTest(t, msg)

		_, err := f.keeper.MineTaxPayment(f.ctx, msg)
		require.ErrorIs(t, err, types.ErrGovernmentEntityCannotMine)
	})

	t.Run("entidade da whitelist fiscal fora da blacklist", func(t *testing.T) {
		f := initMineFixture(t)
		miner, _ := makeAddr(t, f.addressCodec, 0x11)

		// Açailândia/MA está no mapa de entidades fiscais (fallback, camada 2)
		// mas não na blacklist pequena (camada 1).
		msg := taxPaymentMsgForTest(t, f, miner, "gov-whitelist", 100)
		msg.PayerTaxId = types.HashTaxID("07000263000108")
		signAttestationForTest(t, msg)

		_, err := f.keeper.MineTaxPayment(f.ctx, msg)
		require.ErrorIs(t, err, types.ErrGovernmentEntityCannotMine)
	})
}

func TestMine_AllowsHashedPrivatePayer(t *testing.T) {
	f := initMineFixture(t)
	miner, _ := makeAddr(t, f.addressCodec, 0x11)

	msg := taxPaymentMsgForTest(t, f, miner, "private-payer", 100)
	msg.PayerTaxId = types.HashTaxID("12345678000190")
	signAttestationForTest(t, msg)

	_, err := f.keeper.MineTaxPayment(f.ctx, msg)
	require.NoError(t, err)
}

// TestMine_EventPublishesGovernmentIDOnly: o CNPJ/EIN do GOVERNO que recebeu o
// imposto aparece junto ao valor; o do PAGADOR nunca (só o hash).
func TestMine_EventPublishesGovernmentIDOnly(t *testing.T) {
	f := initMineFixture(t)
	miner, _ := makeAddr(t, f.addressCodec, 0x11)

	msg := taxPaymentMsgForTest(t, f, miner, "event-ids", 100) // tax_entity = "irs"
	_, err := f.keeper.MineTaxPayment(f.ctx, msg)
	require.NoError(t, err)

	values := map[string]string{}
	var all strings.Builder
	for _, ev := range sdk.UnwrapSDKContext(f.ctx).EventManager().Events() {
		if ev.Type != types.EventTypeMine {
			continue
		}
		for _, a := range ev.Attributes {
			values[a.Key] = a.Value
			all.WriteString(a.Value)
			all.WriteString("\n")
		}
	}

	require.Equal(t, "530204720", values["tax_entity_id"], "EIN do IRS (governo) tem de aparecer")
	require.Equal(t, msg.PaymentId, values["payment_id"], "payment_id on-chain é o hash")

	// O ID do pagador (123456789) e o E2E cru não podem aparecer em lado nenhum.
	require.NotContains(t, all.String(), "123456789")
	require.NotContains(t, all.String(), "E82639451202607152106LZ1QCIXXYE6")
}
