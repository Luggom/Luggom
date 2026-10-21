package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

func TestMsgUpdateParams(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	params := types.DefaultParams()
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	authorityStr, err := f.addressCodec.BytesToString(f.keeper.GetAuthority())
	require.NoError(t, err)

	// default params
	testCases := []struct {
		name      string
		input     *types.MsgUpdateParams
		expErr    bool
		expErrMsg string
	}{
		{
			name: "invalid authority",
			input: &types.MsgUpdateParams{
				Authority: "invalid",
				Params:    params,
			},
			expErr:    true,
			expErrMsg: "invalid authority",
		},
		{
			name: "invalid params empty bank list",
			input: func() *types.MsgUpdateParams {
				p := types.DefaultParams()
				p.BankProviders = nil
				return &types.MsgUpdateParams{Authority: authorityStr, Params: p}
			}(),
			expErr:    true,
			expErrMsg: "bank_providers",
		},
		{
			name: "all good",
			input: &types.MsgUpdateParams{
				Authority: authorityStr,
				Params:    params,
			},
			expErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ms.UpdateParams(f.ctx, tc.input)

			if tc.expErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// T-GOV-1 (§13.170 §4) — o freio que vale por QUALQUER caminho.
//
// O filtro estrito do ante só inspecciona `MsgUpdateParams` no TOPO da tx; o
// x/gov executa as msgs internas de uma proposta SEM ante (provado ao vivo no
// §13.169). Por isso o guard vive no msg_server, comparando com os params ATUAIS.
// ---------------------------------------------------------------------------

func TestUpdateParams_RejectsFounderAddressChange(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	authorityStr, err := f.addressCodec.BytesToString(f.keeper.GetAuthority())
	require.NoError(t, err)
	require.NoError(t, f.keeper.Params.Set(f.ctx, types.DefaultParams()))

	other, _ := makeAddr(t, f.addressCodec, 0x10)

	next := types.DefaultParams()
	next.FounderAddress = other // tenta fixar/trocar o endereço do Fundador
	_, err = ms.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: authorityStr, Params: next})
	require.Error(t, err, "mudar o founder_address TEM de ser recusado (Grupo C: trancado no genesis)")
	require.Contains(t, err.Error(), "founder_address is locked")

	got, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	require.Empty(t, got.FounderAddress, "os params não podem ter mudado")
}

func TestUpdateParams_AllowsOperationalListsAndKeyRotation(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	authorityStr, err := f.addressCodec.BytesToString(f.keeper.GetAuthority())
	require.NoError(t, err)
	require.NoError(t, f.keeper.Params.Set(f.ctx, types.DefaultParams()))

	apiAddr, _ := makeAddr(t, f.addressCodec, 0x11)

	next := types.DefaultParams()
	next.ApiVerifierAddress = apiAddr                             // Grupo C: chave ROTÁVEL
	next.BankProviders = append(next.BankProviders, "banco-novo") // Grupo E: lista operacional
	_, err = ms.UpdateParams(f.ctx, &types.MsgUpdateParams{Authority: authorityStr, Params: next})
	require.NoError(t, err, "listas operacionais e rotação de chaves TÊM de ser permitidas")

	got, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, apiAddr, got.ApiVerifierAddress)
	require.Contains(t, got.BankProviders, "banco-novo")
}
