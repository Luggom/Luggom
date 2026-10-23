package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// SpendFromReserve gasta a tesouraria (gov) ou a reserva do notariado.
//
// Uma conta de módulo não tem chave e não pode assinar um `MsgSend`: sem esta
// mensagem o saldo das duas reservas acumulava para sempre (§14). A authority é
// o x/gov — é o único caminho de saída.
//
// A tesouraria é discricionária (qualquer destino). A reserva do notariado NÃO:
// o destino tem de constar em `params.notary_payees`. É isso que mantém a
// promessa documentada no whitepaper — a governança muda QUEM é elegível para
// atestar, não PARA ONDE vai o orçamento. Lista vazia ⇒ fail-closed.
func (k msgServer) SpendFromReserve(ctx context.Context, msg *types.MsgSpendFromReserve) (*types.MsgSpendFromReserveResponse, error) {
	if err := k.authorityOk(ctx, msg.Authority); err != nil {
		return nil, err
	}
	if msg.Amount.IsNil() || !msg.Amount.IsPositive() {
		return nil, fmt.Errorf("amount must be positive")
	}
	dest, err := k.addressCodec.StringToBytes(msg.Destination)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}

	var module string
	switch msg.Reserve {
	case types.ReserveGov:
		module = types.GovReserveModuleName

	case types.ReserveNotary:
		module = types.NotaryReserveModuleName
		params, err := k.Params.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read params: %w", err)
		}
		allowed := false
		for _, payee := range params.NotaryPayees {
			if payee == msg.Destination {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf(
				"destination %s is not in notary_payees: the notary budget is not discretionary", msg.Destination)
		}

	default:
		return nil, fmt.Errorf("unknown reserve %q (want %q or %q)", msg.Reserve, types.ReserveGov, types.ReserveNotary)
	}

	coins := sdk.NewCoins(sdk.NewCoin("uluggom", msg.Amount))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, module, dest, coins); err != nil {
		return nil, fmt.Errorf("failed to spend from the %s reserve: %w", msg.Reserve, err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(types.EventTypeReserveSpend,
		sdk.NewAttribute("reserve", msg.Reserve),
		sdk.NewAttribute("reserve_module", module),
		sdk.NewAttribute("destination", msg.Destination),
		sdk.NewAttribute("amount", msg.Amount.String()),
	))

	return &types.MsgSpendFromReserveResponse{}, nil
}
