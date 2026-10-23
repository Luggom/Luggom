package keeper_test

import (
	"context"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

func wrapCtxWithHeight(ctx context.Context, h int64) context.Context {
	return sdk.WrapSDKContext(sdk.UnwrapSDKContext(ctx).WithBlockHeight(h))
}

// §13.146 A: as msgs legadas MsgMine/MsgMineUSD foram desactivadas — o caminho
// de mineração destes testes é o MsgMineTaxPayment (com prova).
func TestUpdateUsdOracleBasket_AuthAndMine(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	oracleStr, oracleAddr := makeAddr(t, f.addressCodec, 0x99)
	strangerStr, _ := makeAddr(t, f.addressCodec, 0x77)

	params := devParamsForTest()
	params.UsdOracleUpdaterAddress = oracleStr
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	ctx10 := wrapCtxWithHeight(f.ctx, 10)
	fixingDay := sdk.UnwrapSDKContext(ctx10).BlockTime().UTC().Format("2006-01-02")

	_, err := ms.UpdateUsdOracleBasket(ctx10, &types.MsgUpdateUsdOracleBasket{
		Signer: strangerStr,
		ChfUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
		EurUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
		GbpUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
		BrlUsd: sdkmath.LegacyMustNewDecFromStr("5.0"),
		CadUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
		RateDate: fixingDay,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not authorized")

	_, err = ms.UpdateUsdOracleBasket(ctx10, &types.MsgUpdateUsdOracleBasket{
		Signer:   oracleStr,
		ChfUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		BrlUsd:   sdkmath.LegacyMustNewDecFromStr("5.0"),
		CadUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
		RateDate: fixingDay,
	})
	require.NoError(t, err)

	// A fixing é imutável: a segunda escrita do mesmo dia é recusada.
	_, err = ms.UpdateUsdOracleBasket(ctx10, &types.MsgUpdateUsdOracleBasket{
		Signer:   oracleStr,
		ChfUsd:   sdkmath.LegacyMustNewDecFromStr("1.5"),
		EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.5"),
		GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.5"),
		BrlUsd:   sdkmath.LegacyMustNewDecFromStr("5.0"),
		CadUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
		RateDate: fixingDay,
	})
	require.Error(t, err)
	require.ErrorIs(t, err, types.ErrOracleDayAlreadyWritten)

	snap, err := f.keeper.UsdOracleSnapshot.Get(ctx10)
	require.NoError(t, err)
	require.Equal(t, int64(10), snap.BlockHeight)

	// O histórico diário tem de ser escrito no mesmo update (é o que a cunhagem
	// consulta na data do pagamento), já com BRL/CAD preenchidos.
	dayKey := keeper.OracleDayKey(sdk.UnwrapSDKContext(ctx10).BlockTime().Unix())
	daily, err := f.keeper.UsdOracleDaily.Get(ctx10, dayKey)
	require.NoError(t, err)
	require.True(t, daily.ChfUsd.Equal(sdkmath.LegacyMustNewDecFromStr("1.2")))
	require.True(t, daily.BrlUsd.IsPositive(), "BRL/USD tem de ir no snapshot diário")
	require.True(t, daily.CadUsd.IsPositive(), "CAD/USD tem de ir no snapshot diário")

	minerStr, _ := makeAddr(t, f.addressCodec, 0xaa)
	_, err = ms.MineTaxPayment(ctx10, taxPaymentMsgForTest(t, f, minerStr, "oracle-c2", 100))
	require.NoError(t, err)
	_ = oracleAddr
}

// ⚠️ MUDANÇA DE ESPECIFICAÇÃO (revisão do Zealynx, 2026-10-22): a cunhagem passou
// a ser preçada na DATA DO PAGAMENTO. O que bloqueia já não é a frescura do
// último snapshot, mas a EXISTÊNCIA do snapshot do dia do pagamento. Sem isto, um
// recibo era uma opção gratuita durante 365 dias (esperar pela cesta mais baixa,
// e pelo dia antes do aniversário, quando o multiplicador salta +10%).
func TestMineTaxPayment_NoSnapshotForPaymentDayPausesMining(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	minerStr, _ := makeAddr(t, f.addressCodec, 0xcc)

	// Pagamento de há 30 dias: dentro da janela de 365 dias, mas sem snapshot
	// diário (a fixture só semeia hoje e os dois dias anteriores).
	msg := taxPaymentMsgForTest(t, f, minerStr, "no-day-snapshot", 100)
	msg.PaymentDate = now.AddDate(0, 0, -30).Unix()
	signAttestationForTest(t, msg)

	_, err := ms.MineTaxPayment(f.ctx, msg)
	require.Error(t, err, "sem snapshot do dia do pagamento a mineração TEM de parar")
	require.Contains(t, err.Error(), "no usd oracle snapshot")
	require.Contains(t, err.Error(), "PAUSED")
}

// TestMineTaxPayment_PricesAtPaymentDate: o mesmo recibo, submetido noutro dia,
// dá o MESMO LUG — a paridade é a do dia do pagamento, não a da submissão.
func TestMineTaxPayment_PricesAtPaymentDate(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	minerStr, _ := makeAddr(t, f.addressCodec, 0x11)

	// Dia A: cesta 1.0/1.0/1.0 → paridade 3.0 (mais LUG por USD).
	dayA := now.AddDate(0, 0, -10)
	require.NoError(t, f.keeper.UsdOracleDaily.Set(f.ctx, keeper.OracleDayKey(dayA.Unix()),
		types.UsdOracleSnapshot{
			ChfUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			EurUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			GbpUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			BrlUsd: sdkmath.LegacyMustNewDecFromStr("5.0"),
			CadUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
		}))

	// Dia B (hoje, da fixture): cesta 1.10/1.18/1.37 → paridade 3.65.
	// Um recibo datado D usa a fixing de D-1: o do "dia A" é datado D+1.
	msgA := taxPaymentMsgForTest(t, f, minerStr, "price-day-a", 100)
	msgA.PaymentDate = dayA.AddDate(0, 0, 1).Unix()
	signAttestationForTest(t, msgA)

	msgB := taxPaymentMsgForTest(t, f, minerStr, "price-day-b", 100)
	signAttestationForTest(t, msgB)

	respA, err := ms.MineTaxPayment(f.ctx, msgA)
	require.NoError(t, err)
	respB, err := ms.MineTaxPayment(f.ctx, msgB)
	require.NoError(t, err)

	// O recibo do dia A (paridade 3.0) cunha 3.65/3.0 = 1.2167× mais LUG.
	ratio := respA.AmountMined.ToLegacyDec().Quo(respB.AmountMined.ToLegacyDec())
	expected := sdkmath.LegacyMustNewDecFromStr("3.65").Quo(sdkmath.LegacyMustNewDecFromStr("3.0"))
	diff, err := ratio.Sub(expected).Abs().Float64()
	require.NoError(t, err)
	require.Less(t, diff, 0.01, "a paridade tem de ser a do dia do pagamento (ratio=%s)", ratio)
}

// TestMineTaxPayment_AnniversaryStepUsesPaymentDate: o salto de +10% no
// aniversário da chain é aplicado na DATA DO PAGAMENTO. Um recibo do dia 364 não
// pode ser minerado no dia 365 para "escolher" o multiplicador.
func TestMineTaxPayment_AnniversaryStepUsesPaymentDate(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	minerStr, _ := makeAddr(t, f.addressCodec, 0x11)

	// Chain start há 400 dias: o dia 364 cai a -36 d e o dia 365 a -35 d (ambos
	// dentro da janela de 365 dias).
	start := now.AddDate(0, 0, -400)
	require.NoError(t, f.keeper.ChainStartTime.Set(f.ctx, uint64(start.Unix())))

	legs := func() types.UsdOracleSnapshot {
		return types.UsdOracleSnapshot{
			ChfUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			EurUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			GbpUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			BrlUsd: sdkmath.LegacyMustNewDecFromStr("5.0"),
			CadUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
		}
	}
	// Um recibo datado D usa a fixing de D-1, por isso o snapshot que serve o
	// pagamento do dia 364 é o do dia 363 (e o do 365 é o do 364).
	day363 := start.AddDate(0, 0, 363)
	day364 := start.AddDate(0, 0, 364)
	require.NoError(t, f.keeper.UsdOracleDaily.Set(f.ctx, keeper.OracleDayKey(day363.Unix()), legs()))
	require.NoError(t, f.keeper.UsdOracleDaily.Set(f.ctx, keeper.OracleDayKey(day364.Unix()), legs()))

	day365 := start.AddDate(0, 0, 365)

	msg364 := taxPaymentMsgForTest(t, f, minerStr, "anniv-364", 100)
	msg364.PaymentDate = day364.Unix()
	signAttestationForTest(t, msg364)

	msg365 := taxPaymentMsgForTest(t, f, minerStr, "anniv-365", 100)
	msg365.PaymentDate = day365.Unix()
	signAttestationForTest(t, msg365)

	resp364, err := ms.MineTaxPayment(f.ctx, msg364)
	require.NoError(t, err)
	resp365, err := ms.MineTaxPayment(f.ctx, msg365)
	require.NoError(t, err)

	// Multiplicador 3 no dia 364 e 3.3 no dia 365 ⇒ ~10% menos LUG no dia 365.
	ratio := resp364.AmountMined.ToLegacyDec().Quo(resp365.AmountMined.ToLegacyDec())
	diff, err := ratio.Sub(sdkmath.LegacyMustNewDecFromStr("1.1")).Abs().Float64()
	require.NoError(t, err)
	require.Less(t, diff, 0.01, "o multiplicador tem de ser o do dia do pagamento (ratio=%s)", ratio)
}

// TestMineTaxPayment_BrlUsesPaymentDayRate: a taxa BRL/USD vem do snapshot do
// dia do pagamento, não da constante do dia da mineração.
func TestMineTaxPayment_BrlUsesPaymentDayRate(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	minerStr, _ := makeAddr(t, f.addressCodec, 0x11)

	// Dia com BRL mais forte: 4.0 BRL por USD ⇒ mais USD pelo mesmo BRL.
	// O recibo é datado de dayB e resolve pela fixing de dayB-1.
	dayB := now.AddDate(0, 0, -5)
	require.NoError(t, f.keeper.UsdOracleDaily.Set(f.ctx, keeper.OracleDayKey(dayB.AddDate(0, 0, -1).Unix()),
		types.UsdOracleSnapshot{
			ChfUsd: sdkmath.LegacyMustNewDecFromStr("1.10"),
			EurUsd: sdkmath.LegacyMustNewDecFromStr("1.18"),
			GbpUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
			BrlUsd: sdkmath.LegacyMustNewDecFromStr("4.0"),
			CadUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
		}))

	msgA := taxPaymentMsgForTest(t, f, minerStr, "brl-5", 100)
	msgA.CurrencyCode = "BRL"
	msgA.PaymentAmount = sdkmath.LegacyMustNewDecFromStr("1000")
	signAttestationForTest(t, msgA)

	msgB := taxPaymentMsgForTest(t, f, minerStr, "brl-4", 100)
	msgB.CurrencyCode = "BRL"
	msgB.PaymentAmount = sdkmath.LegacyMustNewDecFromStr("1000")
	msgB.PaymentDate = dayB.Unix()
	signAttestationForTest(t, msgB)

	respA, err := ms.MineTaxPayment(f.ctx, msgA)
	require.NoError(t, err)
	respB, err := ms.MineTaxPayment(f.ctx, msgB)
	require.NoError(t, err)

	// 1000 BRL a 5.0 = 200 USD; a 4.0 = 250 USD ⇒ 25% mais LUG.
	ratio := respB.AmountMined.ToLegacyDec().Quo(respA.AmountMined.ToLegacyDec())
	diff, err := ratio.Sub(sdkmath.LegacyMustNewDecFromStr("1.25")).Abs().Float64()
	require.NoError(t, err)
	require.Less(t, diff, 0.01, "a taxa BRL/USD tem de ser a do dia do pagamento (ratio=%s)", ratio)
}
