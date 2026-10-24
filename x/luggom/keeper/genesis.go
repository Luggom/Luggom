package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// InitGenesis initializes the module's state from a provided genesis state.
func (k Keeper) InitGenesis(ctx context.Context, genState types.GenesisState) error {
	// §13.146 (review multi-agente): validar os params ANTES de os gravar. Sem
	// isto, um genesis com a fase TLSNotary a 0 (o default do protobuf quando o
	// campo é omitido) ou outro param inválido entrava em produção em silêncio —
	// e os params só eram validados num `genesis validate` explícito. Fail-closed.
	if err := genState.Params.Validate(); err != nil {
		return fmt.Errorf("invalid %s genesis params: %w", types.ModuleName, err)
	}
	if err := k.Params.Set(ctx, genState.Params); err != nil {
		return err
	}

	// §16 — um payee do genesis é configuração de confiança, não uma proposta:
	// entra já elegível (stamp recuado pelo atraso). Sem isto, um genesis com
	// notary_payees deixava a reserva do notariado impossível de gastar.
	if len(genState.Params.NotaryPayees) > 0 {
		stamp := sdk.UnwrapSDKContext(ctx).BlockTime().Unix() - notaryPayeeDelaySeconds
		for _, payee := range genState.Params.NotaryPayees {
			if err := k.NotaryPayeeAddedAt.Set(ctx, payee, stamp); err != nil {
				return err
			}
		}
	}

	// §13.170 Grupo D — ancora do epoch anual (bloco 0). So se fixa uma vez.
	if _, err := k.ChainStartTime.Get(ctx); errors.Is(err, collections.ErrNotFound) {
		if err := k.ChainStartTime.Set(ctx, uint64(sdk.UnwrapSDKContext(ctx).BlockTime().Unix())); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// §13.171 — o relogio do cliff do Fundador comeca no bloco 0 (genesis). So se
	// fixa uma vez; um genesis exportado/importado mantem o valor original.
	if _, err := k.FounderVestingStart.Get(ctx); errors.Is(err, collections.ErrNotFound) {
		start := uint64(sdk.UnwrapSDKContext(ctx).BlockTime().Unix())
		if err := k.FounderVestingStart.Set(ctx, start); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return nil
}

// ExportGenesis returns the module's exported genesis.
func (k Keeper) ExportGenesis(ctx context.Context) (*types.GenesisState, error) {
	var err error

	genesis := types.DefaultGenesis()
	genesis.Params, err = k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	return genesis, nil
}
