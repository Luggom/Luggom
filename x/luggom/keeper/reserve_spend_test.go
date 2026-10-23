package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

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

	// O destinatário elegível passa.
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
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
