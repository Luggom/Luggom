package keeper

import (
	"bytes"
	"context"
	"fmt"

	errorsmod "cosmossdk.io/errors"

	"luggom/x/luggom/types"
)

// msg_operational_lists.go — FASE 5 (governança/imutabilidade, §13.42).
// Handlers para editar LISTAS OPERACIONAIS (entidades fiscais, bancos TLSNotary,
// ativos da pool) via governança on-chain 67% (authority = x/gov).
// NUNCA tokenomics — o filtro estrito (ante-handler) rejeita mudanças de splits/cesta/teto.

func (k msgServer) authorityOk(ctx context.Context, authority string) error {
	addr, err := k.addressCodec.StringToBytes(authority)
	if err != nil {
		return errorsmod.Wrap(err, "invalid authority address")
	}
	if !bytes.Equal(k.GetAuthority(), addr) {
		expected, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expected, authority)
	}
	return nil
}

func (k msgServer) addToList(ctx context.Context, list []string, item string) ([]string, bool) {
	for _, v := range list {
		if v == item {
			return list, false // already present
		}
	}
	return append(list, item), true
}

func (k msgServer) removeFromList(ctx context.Context, list []string, item string) ([]string, bool) {
	out := list[:0]
	found := false
	for _, v := range list {
		if v == item {
			found = true
			continue
		}
		out = append(out, v)
	}
	return out, found
}

// --- Tax entities (whitelist fiscal) ---

func (k msgServer) AddTaxEntity(ctx context.Context, msg *types.MsgAddTaxEntity) (*types.MsgAddTaxEntityResponse, error) {
	if err := k.authorityOk(ctx, msg.Authority); err != nil {
		return nil, err
	}
	if msg.Entity == "" {
		return nil, fmt.Errorf("entity cannot be empty")
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	// BCA imutável (§13.22): só entidades DENTRO dos 3 países (validação de slug).
	if !types.IsBCASlug(msg.Entity) {
		return nil, fmt.Errorf("entity %q is not a valid BCA slug (br:/us:/ca:)", msg.Entity)
	}
	list, added := k.addToList(ctx, params.TaxEntityWhitelist, msg.Entity)
	if !added {
		return &types.MsgAddTaxEntityResponse{}, nil
	}
	params.TaxEntityWhitelist = list
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if err := k.Params.Set(ctx, params); err != nil {
		return nil, err
	}
	return &types.MsgAddTaxEntityResponse{}, nil
}

func (k msgServer) RemoveTaxEntity(ctx context.Context, msg *types.MsgRemoveTaxEntity) (*types.MsgRemoveTaxEntityResponse, error) {
	if err := k.authorityOk(ctx, msg.Authority); err != nil {
		return nil, err
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	list, removed := k.removeFromList(ctx, params.TaxEntityWhitelist, msg.Entity)
	if !removed {
		return &types.MsgRemoveTaxEntityResponse{}, nil
	}
	params.TaxEntityWhitelist = list
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if err := k.Params.Set(ctx, params); err != nil {
		return nil, err
	}
	return &types.MsgRemoveTaxEntityResponse{}, nil
}

// --- Bank providers (TLSNotary) ---

func (k msgServer) AddBankProvider(ctx context.Context, msg *types.MsgAddBankProvider) (*types.MsgAddBankProviderResponse, error) {
	if err := k.authorityOk(ctx, msg.Authority); err != nil {
		return nil, err
	}
	if msg.ProviderId == "" {
		return nil, fmt.Errorf("provider_id cannot be empty")
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	list, added := k.addToList(ctx, params.BankProviders, msg.ProviderId)
	if !added {
		return &types.MsgAddBankProviderResponse{}, nil
	}
	params.BankProviders = list
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if err := k.Params.Set(ctx, params); err != nil {
		return nil, err
	}
	return &types.MsgAddBankProviderResponse{}, nil
}

func (k msgServer) RemoveBankProvider(ctx context.Context, msg *types.MsgRemoveBankProvider) (*types.MsgRemoveBankProviderResponse, error) {
	if err := k.authorityOk(ctx, msg.Authority); err != nil {
		return nil, err
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	list, removed := k.removeFromList(ctx, params.BankProviders, msg.ProviderId)
	if !removed {
		return &types.MsgRemoveBankProviderResponse{}, nil
	}
	params.BankProviders = list
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if err := k.Params.Set(ctx, params); err != nil {
		return nil, err
	}
	return &types.MsgRemoveBankProviderResponse{}, nil
}

// --- Pool assets ---

func (k msgServer) AddPoolAsset(ctx context.Context, msg *types.MsgAddPoolAsset) (*types.MsgAddPoolAssetResponse, error) {
	if err := k.authorityOk(ctx, msg.Authority); err != nil {
		return nil, err
	}
	if msg.Asset == "" {
		return nil, fmt.Errorf("asset cannot be empty")
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	list, added := k.addToList(ctx, params.PoolAssets, msg.Asset)
	if !added {
		return &types.MsgAddPoolAssetResponse{}, nil
	}
	params.PoolAssets = list
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if err := k.Params.Set(ctx, params); err != nil {
		return nil, err
	}
	return &types.MsgAddPoolAssetResponse{}, nil
}

func (k msgServer) RemovePoolAsset(ctx context.Context, msg *types.MsgRemovePoolAsset) (*types.MsgRemovePoolAssetResponse, error) {
	if err := k.authorityOk(ctx, msg.Authority); err != nil {
		return nil, err
	}
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	list, removed := k.removeFromList(ctx, params.PoolAssets, msg.Asset)
	if !removed {
		return &types.MsgRemovePoolAssetResponse{}, nil
	}
	params.PoolAssets = list
	if err := params.Validate(); err != nil {
		return nil, err
	}
	if err := k.Params.Set(ctx, params); err != nil {
		return nil, err
	}
	return &types.MsgRemovePoolAssetResponse{}, nil
}
