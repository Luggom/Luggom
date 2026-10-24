package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

func (k msgServer) UpdateParams(ctx context.Context, req *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	authority, err := k.addressCodec.StringToBytes(req.Authority)
	if err != nil {
		return nil, errorsmod.Wrap(err, "invalid authority address")
	}

	if !bytes.Equal(k.GetAuthority(), authority) {
		expectedAuthorityStr, _ := k.addressCodec.BytesToString(k.GetAuthority())
		return nil, errorsmod.Wrapf(types.ErrInvalidSigner, "invalid authority; expected %s, got %s", expectedAuthorityStr, req.Authority)
	}

	if err := req.Params.Validate(); err != nil {
		return nil, err
	}

	// §13.170 §4 / T-GOV-1: o filtro estrito do ante é contornável (o x/gov
	// executa msgs internas sem ante). O guard no msg_server compara com os
	// params ATUAIS e é o que torna a promessa verdadeira por qualquer caminho.
	current, err := k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateUpdateParamsProtected(current, req.Params); err != nil {
		return nil, err
	}

	if err := k.Params.Set(ctx, req.Params); err != nil {
		return nil, err
	}

	// §16 — marca a entrada de cada payee novo (e limpa os removidos). Sem isto,
	// o mesmo bloco que adiciona um endereço podia pagá-lo.
	if err := k.syncNotaryPayees(ctx, current.NotaryPayees, req.Params.NotaryPayees); err != nil {
		return nil, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}

// syncNotaryPayees mantém o registo "entrou em" alinhado com a lista. Remover e
// voltar a adicionar reinicia o relógio do atraso, como deve ser.
func (k Keeper) syncNotaryPayees(ctx context.Context, before, after []string) error {
	now := sdk.UnwrapSDKContext(ctx).BlockTime().Unix()

	prev := make(map[string]struct{}, len(before))
	for _, a := range before {
		prev[a] = struct{}{}
	}
	next := make(map[string]struct{}, len(after))
	for _, a := range after {
		next[a] = struct{}{}
	}

	for _, a := range after {
		if _, ok := prev[a]; ok {
			continue
		}
		if err := k.NotaryPayeeAddedAt.Set(ctx, a, now); err != nil {
			return err
		}
	}
	for _, a := range before {
		if _, ok := next[a]; ok {
			continue
		}
		if err := k.NotaryPayeeAddedAt.Remove(ctx, a); err != nil {
			return err
		}
	}
	return nil
}
