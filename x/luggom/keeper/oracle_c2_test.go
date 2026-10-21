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

	_, err := ms.UpdateUsdOracleBasket(ctx10, &types.MsgUpdateUsdOracleBasket{
		Signer: strangerStr,
		ChfUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
		EurUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
		GbpUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "not authorized")

	_, err = ms.UpdateUsdOracleBasket(ctx10, &types.MsgUpdateUsdOracleBasket{
		Signer: oracleStr,
		ChfUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
		EurUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
		GbpUsd: sdkmath.LegacyMustNewDecFromStr("1.2"),
	})
	require.NoError(t, err)

	snap, err := f.keeper.UsdOracleSnapshot.Get(ctx10)
	require.NoError(t, err)
	require.Equal(t, int64(10), snap.BlockHeight)

	minerStr, _ := makeAddr(t, f.addressCodec, 0xaa)
	_, err = ms.MineTaxPayment(ctx10, taxPaymentMsgForTest(t, f, minerStr, "oracle-c2", 100))
	require.NoError(t, err)
	_ = oracleAddr
}

// ⚠️ MUDANÇA DE ESPECIFICAÇÃO (§13.28/§6b/§13.56, 2026-09-18, decidida pelo
// Fundador): um snapshot STALE deixa de cair na cesta fixa C1 e passa a PAUSAR a
// mineração (fail-closed). Este teste chamava-se `..._StaleFallsBackToC1` e
// afirmava o contrário; foi reescrito para afirmar a decisão — não é um teste
// ajustado para passar.
func TestMineTaxPayment_DynamicOracle_StalePausesMining(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	// §13.170: o oráculo é sempre dinâmico (constante); a staleness por bloco usa
	// o default de código (60) — o snapshot fica a 10 e mina-se no bloco 100.
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	ctx10 := wrapCtxWithHeight(f.ctx, 10)
	// Sem UpdatedAt ⇒ cai no caminho por BLOCOS (snapshots legados).
	require.NoError(t, f.keeper.UsdOracleSnapshot.Set(ctx10, types.UsdOracleSnapshot{
		ChfUsd:      sdkmath.LegacyMustNewDecFromStr("2"),
		EurUsd:      sdkmath.LegacyMustNewDecFromStr("2"),
		GbpUsd:      sdkmath.LegacyMustNewDecFromStr("2"),
		BlockHeight: 10,
	}))

	ctx100 := wrapCtxWithHeight(f.ctx, 100)
	minerStr, _ := makeAddr(t, f.addressCodec, 0xcc)
	_, err := ms.MineTaxPayment(ctx100, taxPaymentMsgForTest(t, f, minerStr, "stale-pauses", 100))
	require.Error(t, err, "snapshot stale TEM de pausar a mineração (fail-closed)")
	require.Contains(t, err.Error(), "PAUSED")
}

// Sem snapshot nenhum, a mineração também pára — não cai no C1. E com o oráculo
// dinâmico DESLIGADO (dev/teste) o C1 continua a servir: é o único caminho em que
// a cesta estática é legítima.
func TestMineTaxPayment_DynamicOracle_NoSnapshotPausesMining(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))
	require.NoError(t, f.keeper.UsdOracleSnapshot.Remove(f.ctx))

	minerStr, _ := makeAddr(t, f.addressCodec, 0xdd)
	_, err := ms.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "no-snapshot", 100))
	require.Error(t, err, "sem snapshot a mineração TEM de parar")
	require.Contains(t, err.Error(), "no snapshot")
	// §13.170 Grupo B: deixou de haver o flag que "desligava" o oráculo — o C1
	// estático deixou de ser um caminho de produção.
}
