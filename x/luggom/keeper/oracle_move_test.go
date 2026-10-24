package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// A fixing é imutável e os recibos seguintes resolvem para ela durante 365 dias.
// A mediana das fontes protege de UMA fonte errada; não protege de uma escrita
// errada (chave do updater comprometida, bug do feeder). A guarda de movimento é
// o que impede que uma escrita de 0.50 BRL/USD (≈10×) envenene a janela toda.
func TestUpdateUsdOracleBasket_RejectsWildMove(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	oracleStr, _ := makeAddr(t, f.addressCodec, 0x99)
	params := devParamsForTest()
	params.UsdOracleUpdaterAddress = oracleStr
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	day := sdk.UnwrapSDKContext(f.ctx).BlockTime().UTC().Format("2006-01-02")

	// BRL a 0.50 contra 5.0 da fixing anterior: dentro da banda absoluta
	// [0.5, 20] mas 90% abaixo da anterior (converteria o recibo a ~10×).
	_, err := ms.UpdateUsdOracleBasket(f.ctx, &types.MsgUpdateUsdOracleBasket{
		Signer:   oracleStr,
		ChfUsd:   sdkmath.LegacyMustNewDecFromStr("1.10"),
		EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.18"),
		GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
		BrlUsd:   sdkmath.LegacyMustNewDecFromStr("0.50"),
		CadUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
		RateDate: day,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "would move")
	require.Contains(t, err.Error(), "brl_usd")

	// Nada foi escrito.
	_, err = f.keeper.UsdOracleDaily.Get(f.ctx, day)
	require.Error(t, err)

	// E uma perna da cesta também não salta 10×.
	_, err = ms.UpdateUsdOracleBasket(f.ctx, &types.MsgUpdateUsdOracleBasket{
		Signer:   oracleStr,
		ChfUsd:   sdkmath.LegacyMustNewDecFromStr("10.0"),
		EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.18"),
		GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
		BrlUsd:   sdkmath.LegacyMustNewDecFromStr("5.0"),
		CadUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
		RateDate: day,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "chf_usd")
}

func TestUpdateUsdOracleBasket_AcceptsNormalMove(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	oracleStr, _ := makeAddr(t, f.addressCodec, 0x99)
	params := devParamsForTest()
	params.UsdOracleUpdaterAddress = oracleStr
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	day := sdk.UnwrapSDKContext(f.ctx).BlockTime().UTC().Format("2006-01-02")

	// Movimentos de ~4% num dia: normais em FX, têm de passar.
	_, err := ms.UpdateUsdOracleBasket(f.ctx, &types.MsgUpdateUsdOracleBasket{
		Signer:   oracleStr,
		ChfUsd:   sdkmath.LegacyMustNewDecFromStr("1.15"),
		EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.23"),
		GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.31"),
		BrlUsd:   sdkmath.LegacyMustNewDecFromStr("5.20"),
		CadUsd:   sdkmath.LegacyMustNewDecFromStr("1.32"),
		RateDate: day,
	})
	require.NoError(t, err)

	_, err = f.keeper.UsdOracleDaily.Get(f.ctx, day)
	require.NoError(t, err, "a fixing do dia tem de ficar escrita")
}

// O limite é por dia decorrido desde a fixing anterior: um feeder parado três
// dias pode recuperar 30% de uma vez, mas não 40%.
func TestUpdateUsdOracleBasket_MoveLimitScalesWithGap(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	oracleStr, _ := makeAddr(t, f.addressCodec, 0x99)
	params := devParamsForTest()
	params.UsdOracleUpdaterAddress = oracleStr
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	day := now.UTC().Format("2006-01-02")

	// Deixa só a fixing de D-3 (a fixture semeia D-1, D-2, D-3).
	for i := 1; i <= 2; i++ {
		key := keeper.OracleDayKey(now.AddDate(0, 0, -i).Unix())
		require.NoError(t, f.keeper.UsdOracleDaily.Remove(f.ctx, key))
	}

	mk := func(brl string) *types.MsgUpdateUsdOracleBasket {
		return &types.MsgUpdateUsdOracleBasket{
			Signer:   oracleStr,
			ChfUsd:   sdkmath.LegacyMustNewDecFromStr("1.10"),
			EurUsd:   sdkmath.LegacyMustNewDecFromStr("1.18"),
			GbpUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
			BrlUsd:   sdkmath.LegacyMustNewDecFromStr(brl),
			CadUsd:   sdkmath.LegacyMustNewDecFromStr("1.37"),
			RateDate: day,
		}
	}

	// +40% em 3 dias (limite 30%) → recusado.
	_, err := ms.UpdateUsdOracleBasket(f.ctx, mk("7.00"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "30%")

	// +26% em 3 dias → aceite.
	_, err = ms.UpdateUsdOracleBasket(f.ctx, mk("6.30"))
	require.NoError(t, err)
}
