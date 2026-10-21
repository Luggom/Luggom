package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// FASE 5 — governança de listas operacionais (§13.42).

func TestMsgAddRemoveTaxEntity(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	params := types.DefaultParams()
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	authorityStr, err := f.addressCodec.BytesToString(f.keeper.GetAuthority())
	require.NoError(t, err)

	// Add a new BCA entity
	_, err = ms.AddTaxEntity(f.ctx, &types.MsgAddTaxEntity{Authority: authorityStr, Entity: "br:iptu_nova_cidade"})
	require.NoError(t, err)

	got, err := f.keeper.Params.Get(f.ctx)
	require.NoError(t, err)
	require.True(t, types.IsTaxEntityWhitelisted("br:iptu_nova_cidade", got.TaxEntityWhitelist))

	// Add again (idempotent)
	_, err = ms.AddTaxEntity(f.ctx, &types.MsgAddTaxEntity{Authority: authorityStr, Entity: "br:iptu_nova_cidade"})
	require.NoError(t, err)

	// Remove it
	_, err = ms.RemoveTaxEntity(f.ctx, &types.MsgRemoveTaxEntity{Authority: authorityStr, Entity: "br:iptu_nova_cidade"})
	require.NoError(t, err)
	got, _ = f.keeper.Params.Get(f.ctx)
	require.False(t, types.IsTaxEntityWhitelisted("br:iptu_nova_cidade", got.TaxEntityWhitelist))

	// Wrong authority → rejected
	_, err = ms.AddTaxEntity(f.ctx, &types.MsgAddTaxEntity{Authority: "wrong", Entity: "br:iptu_x"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid authority")

	// Non-BCA slug → rejected (BCA imutável, §13.22)
	_, err = ms.AddTaxEntity(f.ctx, &types.MsgAddTaxEntity{Authority: authorityStr, Entity: "de:finanzamt"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "BCA")
}

func TestMsgAddRemoveBankProvider(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	params := types.DefaultParams()
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	authorityStr, err := f.addressCodec.BytesToString(f.keeper.GetAuthority())
	require.NoError(t, err)

	// Default has the 21 banks (22 - Nubank - C6, app-only sem web via TLSNotary, §13.101/§13.102;
	// +caixa, §13.106)
	got, _ := f.keeper.Params.Get(f.ctx)
	require.Len(t, got.BankProviders, 21)

	// Add a new provider
	_, err = ms.AddBankProvider(f.ctx, &types.MsgAddBankProvider{Authority: authorityStr, ProviderId: "new_bank"})
	require.NoError(t, err)
	got, _ = f.keeper.Params.Get(f.ctx)
	require.Len(t, got.BankProviders, 22)

	// Remove it
	_, err = ms.RemoveBankProvider(f.ctx, &types.MsgRemoveBankProvider{Authority: authorityStr, ProviderId: "new_bank"})
	require.NoError(t, err)
	got, _ = f.keeper.Params.Get(f.ctx)
	require.Len(t, got.BankProviders, 21)

	// Wrong authority
	_, err = ms.AddBankProvider(f.ctx, &types.MsgAddBankProvider{Authority: "wrong", ProviderId: "x"})
	require.Error(t, err)
}

func TestMsgAddRemovePoolAsset(t *testing.T) {
	f := initFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	params := types.DefaultParams()
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	authorityStr, err := f.addressCodec.BytesToString(f.keeper.GetAuthority())
	require.NoError(t, err)

	got, _ := f.keeper.Params.Get(f.ctx)
	require.Equal(t, []string{"btc", "atom", "usdt", "usdc", "xmr"}, got.PoolAssets)

	_, err = ms.AddPoolAsset(f.ctx, &types.MsgAddPoolAsset{Authority: authorityStr, Asset: "dai"})
	require.NoError(t, err)
	got, _ = f.keeper.Params.Get(f.ctx)
	require.Len(t, got.PoolAssets, 6)

	_, err = ms.RemovePoolAsset(f.ctx, &types.MsgRemovePoolAsset{Authority: authorityStr, Asset: "dai"})
	require.NoError(t, err)
	got, _ = f.keeper.Params.Get(f.ctx)
	require.Len(t, got.PoolAssets, 5)

	// Wrong authority
	_, err = ms.RemovePoolAsset(f.ctx, &types.MsgRemovePoolAsset{Authority: "wrong", Asset: "btc"})
	require.Error(t, err)
}
