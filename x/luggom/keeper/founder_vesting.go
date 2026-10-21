package keeper

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// VESTING DO FUNDADOR (§13.171) — cliff PURO de 24 meses.
//
// Antes do cliff, a fatia de 5% de cada mineracao fica retida no module account
// `luggom_founder_vesting` (o credito trata disso em otc_vault.go). Aos 24 meses
// liberta-se TODO o acumulado para o founder_address e, dai em diante, o credito
// passa a ir directo para o multisig. Este metodo corre no BeginBlock.

// founderVestingStart devolve o inicio do relogio do cliff. Se estiver a 0 (nunca
// inicializado), fixa-o ao bloco actual — deterministico (BlockTime, nao relogio
// de parede) — e grava.
func (k Keeper) founderVestingStart(ctx sdk.Context) (int64, error) {
	start, err := k.FounderVestingStart.Get(ctx)
	if err == nil && start != 0 {
		return int64(start), nil
	}
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return 0, err
	}
	now := ctx.BlockTime().Unix()
	if err := k.FounderVestingStart.Set(ctx, uint64(now)); err != nil {
		return 0, err
	}
	return now, nil
}

// founderCliffPassed diz se o cliff de 24 meses ja passou.
func (k Keeper) founderCliffPassed(ctx sdk.Context) bool {
	start, err := k.founderVestingStart(ctx)
	if err != nil {
		return false // fail-closed: na duvida, retem (nao liberta cedo)
	}
	cliffEnd := time.Unix(start, 0).UTC().Add(time.Duration(types.FounderVestingCliffSeconds()) * time.Second)
	return !ctx.BlockTime().Before(cliffEnd)
}

// founderVestingHeld devolve o total retido por libertar.
func (k Keeper) founderVestingHeld(ctx sdk.Context) (sdkmath.Int, error) {
	v, err := k.FounderVestingHeld.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return sdkmath.ZeroInt(), nil
		}
		return sdkmath.ZeroInt(), err
	}
	return v, nil
}

// addFounderVestingHeld soma `amount` ao total retido.
func (k Keeper) addFounderVestingHeld(ctx sdk.Context, amount sdkmath.Int) error {
	cur, err := k.founderVestingHeld(ctx)
	if err != nil {
		return err
	}
	return k.FounderVestingHeld.Set(ctx, cur.Add(amount))
}

// RunFounderVesting liberta o LUG do Fundador retido, quando o cliff ja passou
// (BeginBlock). Cliff PURO: liberta TUDO de uma vez e passa a 0.
func (k Keeper) RunFounderVesting(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if !k.founderCliffPassed(sdkCtx) {
		return nil
	}

	held, err := k.founderVestingHeld(sdkCtx)
	if err != nil {
		return err
	}
	if !held.IsPositive() {
		return nil
	}

	params, err := k.Params.Get(sdkCtx)
	if err != nil {
		return err
	}
	if params.FounderAddress == "" {
		return fmt.Errorf("founder_address is not configured: refusing to release the founder vesting")
	}
	founderAddr, err := k.addressCodec.StringToBytes(params.FounderAddress)
	if err != nil {
		return fmt.Errorf("founder_address invalido: %w", err)
	}

	coins := sdk.NewCoins(sdk.NewCoin("uluggom", held))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(sdkCtx, types.FounderVestingModuleName, founderAddr, coins); err != nil {
		return fmt.Errorf("founder vesting: release failed: %w", err)
	}
	if err := k.FounderVestingHeld.Set(sdkCtx, sdkmath.ZeroInt()); err != nil {
		return err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(types.EventTypeFounderVesting,
		sdk.NewAttribute("action", "cliff_release"),
		sdk.NewAttribute("to", params.FounderAddress),
		sdk.NewAttribute("lug_amount", held.String()),
	))
	return nil
}
