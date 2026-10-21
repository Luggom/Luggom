package app

import (
	"context"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"google.golang.org/protobuf/proto"

	luggomtypes "luggom/x/luggom/types"
)

// --- Mocks mínimos ---

type mockTx struct {
	msgs []sdk.Msg
}

func (t mockTx) GetMsgs() []sdk.Msg                  { return t.msgs }
func (t mockTx) GetMsgsV2() ([]proto.Message, error) { return nil, nil }

type recordingBank struct {
	sends []sdk.AccAddress
}

func (b *recordingBank) SendCoins(_ context.Context, from, _ sdk.AccAddress, _ sdk.Coins) error {
	b.sends = append(b.sends, from)
	return nil
}

// mustAddr cria um endereço determinístico a partir de um byte.
func mustAddr(b byte) sdk.AccAddress {
	raw := make([]byte, 20)
	for i := range raw {
		raw[i] = b
	}
	return sdk.AccAddress(raw)
}

func msgSend(t *testing.T, from sdk.AccAddress) sdk.Msg {
	t.Helper()
	return &banktypes.MsgSend{
		FromAddress: from.String(),
		ToAddress:   mustAddr(0xEE).String(),
		Amount:      sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, sdkmath.NewInt(1))),
	}
}

// --- isDustReserveSender ---

func TestIsDustReserveSender(t *testing.T) {
	defer SetDustReserveAddress("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")

	reserve, err := sdk.AccAddressFromBech32("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")
	if err != nil {
		t.Fatalf("endereço da reserva inválido: %v", err)
	}

	if !isDustReserveSender(reserve) {
		t.Fatal("a conta da reserva deve ser reconhecida como isenta")
	}
	if isDustReserveSender(mustAddr(0x01)) {
		t.Fatal("uma conta qualquer NÃO pode ser isenta")
	}
}

func TestIsDustReserveSender_EmptyIsNobody(t *testing.T) {
	defer SetDustReserveAddress("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")

	// Sem reserva definida, ninguém é isento (falha segura).
	SetDustReserveAddress("")
	if isDustReserveSender(mustAddr(0x01)) {
		t.Fatal("com reserva vazia, ninguém pode ser isento")
	}
	// Endereço inválido também não isenta ninguém.
	SetDustReserveAddress("não-é-um-endereço")
	if isDustReserveSender(mustAddr(0x01)) {
		t.Fatal("com endereço inválido, ninguém pode ser isento")
	}
}

// --- wrapAnteHandlerWithLugMovementFee ---

func TestLugMovementFee_ChargeOnNormalSender(t *testing.T) {
	defer SetDustReserveAddress("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")

	bank := &recordingBank{}
	sender := mustAddr(0x01)
	handler := wrapAnteHandlerWithLugMovementFee(bank, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		return ctx, nil
	})

	if _, err := handler(sdk.Context{}, mockTx{msgs: []sdk.Msg{msgSend(t, sender)}}, false); err != nil {
		t.Fatalf("handler devolveu erro: %v", err)
	}

	if len(bank.sends) != 1 {
		t.Fatalf("esperava 1 cobrança, houve %d", len(bank.sends))
	}
	if !bank.sends[0].Equals(sender) {
		t.Fatal("a taxa deve ser cobrada ao sender da tx")
	}
}

func TestLugMovementFee_DustReserveIsExempt(t *testing.T) {
	defer SetDustReserveAddress("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")

	reserve, err := sdk.AccAddressFromBech32("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")
	if err != nil {
		t.Fatalf("endereço da reserva inválido: %v", err)
	}

	bank := &recordingBank{}
	handler := wrapAnteHandlerWithLugMovementFee(bank, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		return ctx, nil
	})

	if _, err := handler(sdk.Context{}, mockTx{msgs: []sdk.Msg{msgSend(t, reserve)}}, false); err != nil {
		t.Fatalf("handler devolveu erro: %v", err)
	}

	// A isenção é o ponto do D5: o envio do pó NÃO paga os 0,005 LUG.
	if len(bank.sends) != 0 {
		t.Fatalf("a reserva devia ser isenta, mas houve %d cobrança(s)", len(bank.sends))
	}
}

func TestLugMovementFee_SimulateSkipsCharge(t *testing.T) {
	defer SetDustReserveAddress("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")

	bank := &recordingBank{}
	handler := wrapAnteHandlerWithLugMovementFee(bank, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		return ctx, nil
	})

	if _, err := handler(sdk.Context{}, mockTx{msgs: []sdk.Msg{msgSend(t, mustAddr(0x01))}}, true); err != nil {
		t.Fatalf("handler devolveu erro: %v", err)
	}
	if len(bank.sends) != 0 {
		t.Fatal("simulate não pode cobrar a taxa")
	}
}

func TestLugMovementFee_NoNativeSenderNoCharge(t *testing.T) {
	defer SetDustReserveAddress("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")

	bank := &recordingBank{}
	handler := wrapAnteHandlerWithLugMovementFee(bank, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		return ctx, nil
	})

	// Tx sem MsgSend (ex.: MsgMineTaxPayment) → não há movimento de LUG, logo
	// não há taxa fixa. É por isto que MINERAR não paga os 0,005.
	if _, err := handler(sdk.Context{}, mockTx{msgs: nil}, false); err != nil {
		t.Fatalf("handler devolveu erro: %v", err)
	}
	if len(bank.sends) != 0 {
		t.Fatalf("tx sem transferência nativa não paga taxa fixa, houve %d", len(bank.sends))
	}
}

// TestLugMovementFee_MiningIsNotCharged prova a economia: a taxa fixa de 0,005 LUG
// (§13.87) só incide sobre MsgSend / MsgMultiSend / MsgSwap. MINERAR
// (MsgMineTaxPayment) NÃO a paga — paga apenas o gas. Os validadores ganham com a
// mineração pela fatia de 3% do mint, não por esta taxa.
func TestLugMovementFee_MiningIsNotCharged(t *testing.T) {
	defer SetDustReserveAddress("luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna")

	miner := mustAddr(0x33)
	msg := &luggomtypes.MsgMineTaxPayment{
		MinerAddress:    miner.String(),
		PaymentAmount:        sdkmath.LegacyMustNewDecFromStr("240.50"),
		TaxEntity:       "irs",
		CurrencyCode:    "USD",
		ProofCommitment: "commitment-0123456789abcdef0123456789abcdef",
		PaymentId:       "payment-1",
		ApiAttestation:  "attestation",
		PaymentDate:     1_700_000_000,
		PayerTaxId:      "12-3456789",
		TaxNatureHash:   "nature-0123456789abcdef0123456789abcdef",
	}

	bank := &recordingBank{}
	handler := wrapAnteHandlerWithLugMovementFee(bank, func(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) {
		return ctx, nil
	})

	if _, err := handler(sdk.Context{}, mockTx{msgs: []sdk.Msg{msg}}, false); err != nil {
		t.Fatalf("handler devolveu erro: %v", err)
	}
	if len(bank.sends) != 0 {
		t.Fatalf("minerar NÃO pode pagar a taxa fixa de 0,005 LUG, mas houve %d cobrança(s)", len(bank.sends))
	}
}
