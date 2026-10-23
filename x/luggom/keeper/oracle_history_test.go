package keeper_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// A tabela de histórico é COMPILADA: o preço de um recibo datado não pode
// depender de nada que um nó leia em runtime. Se este hash mudar sem intenção,
// o preço de todos os recibos da janela muda — é o teste mais importante do
// ficheiro.
const oracleHistorySHA256 = "cfe926208319ff427848a8823987b78fae2e8ecbceff7e250c81f1a0fbfb417c"

func ctxAt(ctx context.Context, at time.Time) context.Context {
	return sdk.WrapSDKContext(sdk.UnwrapSDKContext(ctx).WithBlockTime(at))
}

func TestOracleHistoryIntegrity(t *testing.T) {
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	raw, err := os.ReadFile(filepath.Join(root, "x/luggom/keeper/oracle_history.json"))
	require.NoError(t, err)

	sum := sha256.Sum256(raw)
	require.Equal(t, oracleHistorySHA256, hex.EncodeToString(sum[:]),
		"oracle_history.json mudou: se foi regenerado de propósito, atualiza o hash no teste")
}

func TestOracleHistoryCoversWindow(t *testing.T) {
	start, end, err := keeper.OracleHistoryWindowForTest()
	require.NoError(t, err)

	first, err := time.Parse("2006-01-02", start)
	require.NoError(t, err)
	last, err := time.Parse("2006-01-02", end)
	require.NoError(t, err)

	days, err := keeper.OracleHistoryDaysForTest()
	require.NoError(t, err)

	// A janela de validade é de 365 dias e a fixing usada é a de D-1: é preciso
	// um dia a mais para o recibo mais antigo aceite ainda ter fixing.
	require.GreaterOrEqual(t, int(last.Sub(first).Hours()/24)+1, 366)

	for d := first; !d.After(last); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		snap, ok := days[key]
		require.True(t, ok, "dia em falta na tabela: %s", key)
		for name, v := range map[string]sdkmath.LegacyDec{
			"chf": snap.ChfUsd, "eur": snap.EurUsd, "gbp": snap.GbpUsd,
			"brl": snap.BrlUsd, "cad": snap.CadUsd,
		} {
			require.True(t, v.IsPositive(), "%s inválido em %s: %s", name, key, v)
		}
		// A taxa foi pública até ao fim do seu dia de fixing.
		require.Less(t, snap.UpdatedAt, d.AddDate(0, 0, 1).Unix(),
			"UpdatedAt tem de ser dentro do dia de fixing (%s)", key)
	}
}

// TestMine_UsesPreviousFixing: um recibo datado D preça pela fixing de D-1. A
// fixing do próprio dia nunca conta — só pode ser escrita depois de D começar,
// logo nunca é visível a um recibo datado D.
func TestMine_UsesPreviousFixing(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	minerStr, _ := makeAddr(t, f.addressCodec, 0x21)

	basket := func(chf, eur, gbp string) types.UsdOracleSnapshot {
		return types.UsdOracleSnapshot{
			ChfUsd: sdkmath.LegacyMustNewDecFromStr(chf),
			EurUsd: sdkmath.LegacyMustNewDecFromStr(eur),
			GbpUsd: sdkmath.LegacyMustNewDecFromStr(gbp),
			BrlUsd: sdkmath.LegacyMustNewDecFromStr("5.0"),
			CadUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
		}
	}
	// D-1 barato (paridade 3.0), D caro (paridade 6.0).
	require.NoError(t, f.keeper.UsdOracleDaily.Set(f.ctx, keeper.OracleDayKey(now.AddDate(0, 0, -1).Unix()),
		basket("1.0", "1.0", "1.0")))
	require.NoError(t, f.keeper.UsdOracleDaily.Set(f.ctx, keeper.OracleDayKey(now.Unix()),
		basket("2.0", "2.0", "2.0")))

	msg := taxPaymentMsgForTest(t, f, minerStr, "previous-fixing", 100)
	msg.PaymentDate = now.Unix() // datado de D
	signAttestationForTest(t, msg)

	resp, err := ms.MineTaxPayment(f.ctx, msg)
	require.NoError(t, err)

	// 100 USD a paridade 3.0 ⇒ 33.33 LUG; a 6.0 seriam 16.66.
	require.True(t, resp.AmountMined.ToLegacyDec().GT(sdkmath.LegacyMustNewDecFromStr("30")),
		"devia usar a fixing de D-1 (barata), veio %s", resp.AmountMined)
}

// TestMine_RejectsFuturePaymentDate: com o lookback a andar para trás, uma data
// futura resolvia para uma fixing passada em vez de falhar fechado — o prover
// escolhia o preço.
func TestMine_RejectsFuturePaymentDate(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	minerStr, _ := makeAddr(t, f.addressCodec, 0x22)

	for _, delta := range []time.Duration{time.Hour, 24 * time.Hour, 30 * 24 * time.Hour} {
		msg := taxPaymentMsgForTest(t, f, minerStr, "future-"+delta.String(), 100)
		msg.PaymentDate = now.Add(delta).Unix()
		signAttestationForTest(t, msg)

		_, err := ms.MineTaxPayment(f.ctx, msg)
		require.Error(t, err, "data futura (%s) tem de ser recusada", delta)
		require.ErrorIs(t, err, types.ErrFuturePayment)
	}
}

// TestMine_BrlNoFallbackWhenDayLacksRate: as constantes 5,0/1,37 foram
// removidas. Um dia sem BRL/USD no snapshot pára a mineração em vez de cunhar
// com uma taxa inventada.
func TestMine_BrlNoFallbackWhenDayLacksRate(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	minerStr, _ := makeAddr(t, f.addressCodec, 0x23)

	// A fixing que serve um recibo datado de hoje é a de ontem.
	require.NoError(t, f.keeper.UsdOracleDaily.Set(f.ctx, keeper.OracleDayKey(now.AddDate(0, 0, -1).Unix()),
		types.UsdOracleSnapshot{
			ChfUsd: sdkmath.LegacyMustNewDecFromStr("1.10"),
			EurUsd: sdkmath.LegacyMustNewDecFromStr("1.18"),
			GbpUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
			// BrlUsd/CadUsd ausentes (snapshot antigo, antes do feed).
			CadUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
		}))

	msg := taxPaymentMsgForTest(t, f, minerStr, "no-brl", 100)
	msg.CurrencyCode = "BRL"
	msg.PaymentAmount = sdkmath.LegacyMustNewDecFromStr("1000")
	signAttestationForTest(t, msg)

	_, err := ms.MineTaxPayment(f.ctx, msg)
	require.Error(t, err, "sem BRL/USD não se cunha (a constante foi removida)")
	require.Contains(t, err.Error(), "PAUSED")
}

// TestMine_UsesEmbeddedHistoryInsideWindow: um recibo datado DENTRO da janela da
// tabela preça pela tabela (dados compilados) e o valor usado fica registado
// on-chain na primeira utilização.
func TestMine_UsesEmbeddedHistoryInsideWindow(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	_, end, err := keeper.OracleHistoryWindowForTest()
	require.NoError(t, err)
	tableEnd, err := time.Parse("2006-01-02", end)
	require.NoError(t, err)
	table, err := keeper.OracleHistoryDaysForTest()
	require.NoError(t, err)

	// Bloco dentro da janela; recibo de 10 dias antes.
	ctx := ctxAt(f.ctx, tableEnd.AddDate(0, 0, -1).Add(12*time.Hour))
	pay := tableEnd.AddDate(0, 0, -10)

	minerStr, _ := makeAddr(t, f.addressCodec, 0x31)
	msg := taxPaymentMsgForTest(t, f, minerStr, "inside-table", 100)
	msg.PaymentDate = pay.Unix()
	signAttestationForTest(t, msg)

	resp, err := ms.MineTaxPayment(ctx, msg)
	require.NoError(t, err, "um recibo dentro da tabela tem de minerar")
	require.True(t, resp.AmountMined.IsPositive())

	// A fixing em vigor é a de D-1, e ficou registada on-chain.
	usedDay := keeper.OracleDayKey(pay.AddDate(0, 0, -1).Unix())
	want, ok := table[usedDay]
	require.True(t, ok, "o dia usado (%s) tem de estar na tabela", usedDay)
	pinned, err := f.keeper.UsdOracleDaily.Get(ctx, usedDay)
	require.NoError(t, err, "a fixing usada tem de ficar registada on-chain")
	require.True(t, pinned.ChfUsd.Equal(want.ChfUsd), "pin %s != tabela %s", pinned.ChfUsd, want.ChfUsd)
}

// TestMine_NeverUsesThePaymentDaysOwnFixing: se só existir a fixing do próprio
// dia do pagamento, a mineração PÁRA — é isto que impede o preço escolhido.
func TestMine_NeverUsesThePaymentDaysOwnFixing(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	minerStr, _ := makeAddr(t, f.addressCodec, 0x32)

	// Dia do pagamento com fixing; os 7 dias anteriores vazios (a fixture só
	// semeia agora e os 3 dias anteriores, por isso usa-se um dia afastado).
	pay := now.AddDate(0, 0, -20)
	require.NoError(t, f.keeper.UsdOracleDaily.Set(f.ctx, keeper.OracleDayKey(pay.Unix()),
		types.UsdOracleSnapshot{
			ChfUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			EurUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			GbpUsd: sdkmath.LegacyMustNewDecFromStr("1.0"),
			BrlUsd: sdkmath.LegacyMustNewDecFromStr("5.0"),
			CadUsd: sdkmath.LegacyMustNewDecFromStr("1.37"),
		}))

	msg := taxPaymentMsgForTest(t, f, minerStr, "own-fixing", 100)
	msg.PaymentDate = pay.Unix()
	signAttestationForTest(t, msg)

	_, err := ms.MineTaxPayment(f.ctx, msg)
	require.Error(t, err, "a fixing do próprio dia do pagamento não pode ser usada")
	require.Contains(t, err.Error(), "PAUSED")
}

// TestMine_MaxAgeIsCalendarDays: a janela de 365 dias é medida em dias UTC, não
// em instantes. Um recibo de exactamente 365 dias atrás ainda mina; 366 não.
func TestMine_MaxAgeIsCalendarDays(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	blockDay := now.UTC().Truncate(24 * time.Hour)
	minerStr, _ := makeAddr(t, f.addressCodec, 0x33)

	// 365 dias: dentro da janela (a fixing de D-1 cai na tabela).
	okMsg := taxPaymentMsgForTest(t, f, minerStr, "age-365", 100)
	okMsg.PaymentDate = blockDay.AddDate(0, 0, -365).Unix()
	signAttestationForTest(t, okMsg)
	_, err := ms.MineTaxPayment(f.ctx, okMsg)
	require.NoError(t, err, "365 dias ainda tem de minerar")

	// 366 dias: fora da janela.
	oldMsg := taxPaymentMsgForTest(t, f, minerStr, "age-366", 100)
	oldMsg.PaymentDate = blockDay.AddDate(0, 0, -366).Unix()
	signAttestationForTest(t, oldMsg)
	_, err = ms.MineTaxPayment(f.ctx, oldMsg)
	require.ErrorIs(t, err, types.ErrStalePayment)
}

// TestUpdateUsdOracleBasket_RejectsCoveredDay: um dia compilado na tabela não se
// escreve — deixar passar criava no store um valor que nunca preça nada e um
// registo on-chain diferente do preço usado.
func TestUpdateUsdOracleBasket_RejectsCoveredDay(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	oracleStr, _ := makeAddr(t, f.addressCodec, 0x99)
	params := devParamsForTest()
	params.UsdOracleUpdaterAddress = oracleStr
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	_, end, err := keeper.OracleHistoryWindowForTest()
	require.NoError(t, err)
	tableEnd, err := time.Parse("2006-01-02", end)
	require.NoError(t, err)

	ctx := ctxAt(f.ctx, tableEnd.Add(22*time.Hour)) // dia coberto, "hoje"
	day := sdk.UnwrapSDKContext(ctx).BlockTime().UTC().Format("2006-01-02")

	_, err = ms.UpdateUsdOracleBasket(ctx, &types.MsgUpdateUsdOracleBasket{
		Signer:   oracleStr,
		ChfUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		BrlUsd:   sdkmath.LegacyMustNewDecFromStr("5.0"),
		CadUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
		RateDate: day,
	})
	require.Error(t, err, "um dia da tabela não pode ser escrito (%s)", day)
	require.ErrorIs(t, err, types.ErrOracleDayAlreadyWritten)
	require.Contains(t, err.Error(), "oracle_history.json")
}

// TestOracleRateDateMustBeToday: só se escreve a fixing do próprio dia. Sem isto
// um backfill mudava o que um recibo já resolveu.
func TestOracleRateDateMustBeToday(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	oracleStr, _ := makeAddr(t, f.addressCodec, 0x99)
	params := devParamsForTest()
	params.UsdOracleUpdaterAddress = oracleStr
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime().UTC()
	days := map[string]string{
		"ontem":  now.AddDate(0, 0, -1).Format("2006-01-02"),
		"amanhã": now.AddDate(0, 0, 1).Format("2006-01-02"),
		"lixo":   "23/10/2026",
	}
	for name, day := range days {
		_, err := ms.UpdateUsdOracleBasket(f.ctx, &types.MsgUpdateUsdOracleBasket{
			Signer:   oracleStr,
			ChfUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
			EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
			GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
			BrlUsd:   sdkmath.LegacyMustNewDecFromStr("5.0"),
			CadUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
			RateDate: day,
		})
		require.Error(t, err, "rate_date %q (%s) devia ser recusada", day, name)
	}

	// BRL/CAD são obrigatórios: sem eles não se escreve a fixing do dia.
	_, err := ms.UpdateUsdOracleBasket(f.ctx, &types.MsgUpdateUsdOracleBasket{
		Signer:   oracleStr,
		ChfUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.2"),
		RateDate: now.Format("2006-01-02"),
	})
	require.Error(t, err)
}
