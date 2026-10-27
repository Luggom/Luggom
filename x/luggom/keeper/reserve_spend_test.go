package keeper_test

import (
	"context"
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

func govAuthority(t *testing.T, f *mineFixture) string {
	t.Helper()
	s, err := f.addressCodec.BytesToString(authtypes.NewModuleAddress(types.GovModuleName))
	require.NoError(t, err)
	return s
}

// Só a authority (x/gov) pode gastar as reservas: uma conta de módulo não tem
// chave, e é esta mensagem que impede o dinheiro de ficar preso para sempre.
func TestSpendFromReserve_AuthorityOnly(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	stranger, _ := makeAddr(t, f.addressCodec, 0x41)
	dest, _ := makeAddr(t, f.addressCodec, 0x42)

	_, err := ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   stranger,
		Reserve:     types.ReserveGov,
		Destination: dest,
		Amount:      sdkmath.NewInt(1_000_000),
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)
}

func TestSpendFromReserve_GovTreasuryIsDiscretionary(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	dest, _ := makeAddr(t, f.addressCodec, 0x43)

	_, err := ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   govAuthority(t, f),
		Reserve:     types.ReserveGov,
		Destination: dest,
		Amount:      sdkmath.NewInt(5_000_000),
	})
	require.NoError(t, err)

	require.Len(t, f.bank.sends, 1)
	require.Equal(t, types.GovReserveModuleName, f.bank.sends[0].module)
	require.Equal(t, dest, f.bank.sends[0].to.String())
}

// A reserva do notariado NÃO é discricionária: o destino tem de estar em
// params.notary_payees. É isto que mantém "a governança muda quem é elegível,
// não para onde vai o orçamento".
func TestSpendFromReserve_NotaryRequiresPayee(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	dest, _ := makeAddr(t, f.addressCodec, 0x44)
	auth := govAuthority(t, f)

	params := devParamsForTest()
	params.NotaryPayees = nil // fail-closed
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	_, err := ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   auth,
		Reserve:     types.ReserveNotary,
		Destination: dest,
		Amount:      sdkmath.NewInt(1_000_000),
	})
	require.Error(t, err, "lista vazia tem de falhar fechado")
	require.Contains(t, err.Error(), "notary_payees")

	// Outro endereço também não passa, mesmo com a lista preenchida.
	other, _ := makeAddr(t, f.addressCodec, 0x45)
	params.NotaryPayees = []string{dest}
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	_, err = ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   auth,
		Reserve:     types.ReserveNotary,
		Destination: other,
		Amount:      sdkmath.NewInt(1_000_000),
	})
	require.Error(t, err)

	// Na lista mas sem tempo de entrada (adicionado agora) → ainda não elegível.
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	_, err = ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   auth,
		Reserve:     types.ReserveNotary,
		Destination: dest,
		Amount:      sdkmath.NewInt(1_000_000),
	})
	require.Error(t, err, "entrar na lista não pode tornar o payee elegível no mesmo bloco")
	require.Contains(t, err.Error(), "entry time")

	// Com 8 dias de entrada e dentro do tecto da janela, passa.
	now := sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix()
	require.NoError(t, f.keeper.NotaryPayeeAddedAt.Set(f.ctx, dest, now-8*24*3600))
	f.bank.spendableByAddr = map[string]sdk.Coins{
		authtypes.NewModuleAddress(types.NotaryReserveModuleName).String(): sdk.NewCoins(sdk.NewCoin("uluggom", sdkmath.NewInt(1_000_000_000))),
	}
	_, err = ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   auth,
		Reserve:     types.ReserveNotary,
		Destination: dest,
		Amount:      sdkmath.NewInt(1_000_000),
	})
	require.NoError(t, err)
	require.Len(t, f.bank.sends, 1)
	require.Equal(t, types.NotaryReserveModuleName, f.bank.sends[0].module)
}

func TestSpendFromReserve_RejectsBadInput(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	auth := govAuthority(t, f)
	dest, _ := makeAddr(t, f.addressCodec, 0x46)

	// Reserva desconhecida.
	_, err := ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   auth,
		Reserve:     "marketing",
		Destination: dest,
		Amount:      sdkmath.NewInt(1),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown reserve")

	// Montante zero.
	_, err = ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   auth,
		Reserve:     types.ReserveGov,
		Destination: dest,
		Amount:      sdkmath.ZeroInt(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "positive")

	// Destino inválido.
	_, err = ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
		Authority:   auth,
		Reserve:     types.ReserveGov,
		Destination: "não-é-um-endereço",
		Amount:      sdkmath.NewInt(1),
	})
	require.Error(t, err)
}

// A lista tem de ser validada nos params: endereços válidos, sem duplicados.
func TestNotaryPayees_Validation(t *testing.T) {
	params := types.DefaultParams()
	good := authtypes.NewModuleAddress("payee").String()

	params.NotaryPayees = []string{"lixo"}
	require.Error(t, params.Validate())

	params.NotaryPayees = []string{good, good}
	require.Error(t, params.Validate(), "duplicados têm de ser recusados")

	params.NotaryPayees = []string{good}
	require.NoError(t, params.Validate())

	params.NotaryPayees = nil
	require.NoError(t, params.Validate(), "lista vazia é válida (fail-closed)")
}

// Tecto por janela: 25% do saldo da reserva em cada 7 dias. Sem isto, uma
// sequência de propostas esvaziaria a reserva em poucos blocos.
func TestSpendFromReserve_NotaryWindowCap(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	dest, _ := makeAddr(t, f.addressCodec, 0x47)
	auth := govAuthority(t, f)

	params := devParamsForTest()
	params.NotaryPayees = []string{dest}
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	now := sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix()
	require.NoError(t, f.keeper.NotaryPayeeAddedAt.Set(f.ctx, dest, now-8*24*3600))

	// 1.000 LUG na reserva → tecto de 250 LUG por janela.
	f.bank.spendableByAddr = map[string]sdk.Coins{
		authtypes.NewModuleAddress(types.NotaryReserveModuleName).String(): sdk.NewCoins(
			sdk.NewCoin("uluggom", sdkmath.NewInt(1_000).MulRaw(1_000_000_000_000_000_000))),
	}
	lug := func(n int64) sdkmath.Int {
		return sdkmath.NewInt(n).MulRaw(1_000_000_000_000_000_000)
	}
	spend := func(amount sdkmath.Int) error {
		_, err := ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
			Authority:   auth,
			Reserve:     types.ReserveNotary,
			Destination: dest,
			Amount:      amount,
		})
		return err
	}

	err := spend(lug(300))
	require.Error(t, err, "300 LUG excede 25%% de 1.000")
	require.Contains(t, err.Error(), "per-window cap")

	require.NoError(t, spend(lug(100)))
	require.NoError(t, spend(lug(100)))

	err = spend(lug(100))
	require.Error(t, err, "300 LUG acumulados na janela excedem o tecto de 250")
	require.Contains(t, err.Error(), "window cap reached")
}

// O buraco que o Carlos apontou: uma proposta podia adicionar o payee e pagá-lo
// na mesma execução. O atraso é imposto no MsgUpdateParams (que marca a entrada),
// por isso este teste tem de passar pelo msg server — não por Params.Set.
func TestUpdateParams_NotaryPayeeNeedsDelay(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	authority := govAuthority(t, f)
	payee, _ := makeAddr(t, f.addressCodec, 0x51)

	params, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	params.NotaryPayees = []string{payee}

	_, err = ms.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: authority, Params: params})
	require.NoError(t, err)

	now := sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix()
	stamp, err := f.keeper.NotaryPayeeAddedAt.Get(f.ctx, payee)
	require.NoError(t, err)
	require.Equal(t, now, stamp, "a entrada tem de ser marcada no update")

	f.bank.spendableByAddr = map[string]sdk.Coins{
		authtypes.NewModuleAddress(types.NotaryReserveModuleName).String(): sdk.NewCoins(
			sdk.NewCoin("uluggom", sdkmath.NewInt(1_000).MulRaw(1_000_000_000_000_000_000))),
	}
	spend := func(ctx context.Context) error {
		_, err := ms.SpendFromReserve(ctx, &types.MsgSpendFromReserve{
			Authority:   authority,
			Reserve:     types.ReserveNotary,
			Destination: payee,
			Amount:      sdkmath.NewInt(10).MulRaw(1_000_000_000_000_000_000),
		})
		return err
	}

	err = spend(f.ctx)
	require.Error(t, err, "adicionar e pagar na mesma execução tem de falhar")
	require.Contains(t, err.Error(), "eligible in")

	// +8 dias: elegível.
	ctx8 := sdk.WrapSDKContext(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(time.Unix(now+8*24*3600, 0)))
	require.NoError(t, spend(ctx8))

	// Remover e voltar a adicionar reinicia o relógio.
	params.NotaryPayees = nil
	_, err = ms.UpdateParams(ctx8, &types.MsgUpdateParams{Authority: authority, Params: params})
	require.NoError(t, err)
	params.NotaryPayees = []string{payee}
	_, err = ms.UpdateParams(ctx8, &types.MsgUpdateParams{Authority: authority, Params: params})
	require.NoError(t, err)
	require.Error(t, spend(ctx8), "re-adicionar tem de reiniciar o atraso")
}

// O atraso deriva do x/gov: max_deposit_period + voting_period + 1 dia, com piso
// de 7 dias. Na testnet (2d+2d+1d=5d) manda o piso; com períodos maiores o
// atraso acompanha — é isso que fecha o buraco de "cronometrar a proposta para
// terminar quando o payee fica elegível".
func TestSpendFromReserve_DelayFollowsGovPeriods(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	dest, _ := makeAddr(t, f.addressCodec, 0x52)
	auth := govAuthority(t, f)

	params := devParamsForTest()
	params.NotaryPayees = []string{dest}
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	// 7 dias de depósito + 10 de votação + 1 ⇒ 18 dias.
	f.gov.deposit = 7 * 24 * time.Hour
	f.gov.voting = 10 * 24 * time.Hour

	f.bank.spendableByAddr = map[string]sdk.Coins{
		authtypes.NewModuleAddress(types.NotaryReserveModuleName).String(): sdk.NewCoins(
			sdk.NewCoin("uluggom", sdkmath.NewInt(1_000).MulRaw(1_000_000_000_000_000_000))),
	}
	now := sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix()
	spend := func() error {
		_, err := ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
			Authority:   auth,
			Reserve:     types.ReserveNotary,
			Destination: dest,
			Amount:      sdkmath.NewInt(10).MulRaw(1_000_000_000_000_000_000),
		})
		return err
	}

	// 12 dias: passou o piso de 7 mas não os 18 ⇒ recusado.
	require.NoError(t, f.keeper.NotaryPayeeAddedAt.Set(f.ctx, dest, now-12*24*3600))
	err := spend()
	require.Error(t, err, "12 dias não chegam para 7+10+1")
	require.Contains(t, err.Error(), "eligible in")

	// 19 dias: elegível.
	require.NoError(t, f.keeper.NotaryPayeeAddedAt.Set(f.ctx, dest, now-19*24*3600))
	require.NoError(t, spend())
}

// Piso de 7 dias: com períodos do gov a zero (ou nil), o atraso não desaparece.
func TestSpendFromReserve_DelayFloor(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	dest, _ := makeAddr(t, f.addressCodec, 0x53)
	auth := govAuthority(t, f)

	params := devParamsForTest()
	params.NotaryPayees = []string{dest}
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	f.gov.deposit, f.gov.voting = 0, 0
	f.bank.spendableByAddr = map[string]sdk.Coins{
		authtypes.NewModuleAddress(types.NotaryReserveModuleName).String(): sdk.NewCoins(
			sdk.NewCoin("uluggom", sdkmath.NewInt(1_000).MulRaw(1_000_000_000_000_000_000))),
	}
	now := sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix()
	spend := func() error {
		_, err := ms.SpendFromReserve(f.ctx, &types.MsgSpendFromReserve{
			Authority:   auth,
			Reserve:     types.ReserveNotary,
			Destination: dest,
			Amount:      sdkmath.NewInt(10).MulRaw(1_000_000_000_000_000_000),
		})
		return err
	}

	require.NoError(t, f.keeper.NotaryPayeeAddedAt.Set(f.ctx, dest, now-6*24*3600))
	require.Error(t, spend(), "6 dias não chegam ao piso de 7")

	require.NoError(t, f.keeper.NotaryPayeeAddedAt.Set(f.ctx, dest, now-8*24*3600))
	require.NoError(t, spend())
}
