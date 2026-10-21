package keeper

import (
	"bytes"
	"context"

	errorsmod "cosmossdk.io/errors"

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

	return &types.MsgUpdateParamsResponse{}, nil
}
