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

// CLIFF DA LUGGOM BUSINESS CORE — 24 meses contados do PRIMEIRO mint da Fase 2.
//
// Na Fase 1 os 5% de cada cunhagem ficam retidos no module account
// `luggom_business_core_vesting` com o relogio CONGELADO. O relogio arranca no
// primeiro mint da Fase 2 e, ate ao fim do cliff, os 5% continuam a ser
// retidos. Ao fim do cliff, TODO o acumulado (Fase 1 + Fase 2) e libertado
// para BusinessCoreAddress e, dai em diante, o credito passa a ir directo para
// a carteira. Este metodo corre no BeginBlock.

// businessCoreVestingStart fixa o inicio do relogio no primeiro mint da Fase 2.
// Se ja existir, devolve o valor guardado. Deterministico (BlockTime).
func (k Keeper) businessCoreVestingStart(sdkCtx sdk.Context) (int64, error) {
	start, err := k.BusinessCoreVestingStart.Get(sdkCtx)
	if err == nil && start != 0 {
		return int64(start), nil
	}
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return 0, err
	}
	now := sdkCtx.BlockTime().Unix()
	if err := k.BusinessCoreVestingStart.Set(sdkCtx, uint64(now)); err != nil {
		return 0, err
	}
	return now, nil
}

// businessCoreCliffPassed diz se o cliff de 24 meses ja passou. Start a 0
// (Fase 2 ainda nao comecou) => false, fail-closed.
func (k Keeper) businessCoreCliffPassed(sdkCtx sdk.Context) bool {
	start, err := k.BusinessCoreVestingStart.Get(sdkCtx)
	if err != nil || start == 0 {
		return false
	}
	cliffEnd := time.Unix(int64(start), 0).UTC().Add(time.Duration(types.FounderVestingCliffSeconds()) * time.Second)
	return !sdkCtx.BlockTime().Before(cliffEnd)
}

// businessCoreVestingHeld devolve o total retido por libertar.
func (k Keeper) businessCoreVestingHeld(sdkCtx sdk.Context) (sdkmath.Int, error) {
	v, err := k.BusinessCoreVestingHeld.Get(sdkCtx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return sdkmath.ZeroInt(), nil
		}
		return sdkmath.ZeroInt(), err
	}
	return v, nil
}

// addBusinessCoreVestingHeld soma `amount` ao total retido.
func (k Keeper) addBusinessCoreVestingHeld(sdkCtx sdk.Context, amount sdkmath.Int) error {
	cur, err := k.businessCoreVestingHeld(sdkCtx)
	if err != nil {
		return err
	}
	return k.BusinessCoreVestingHeld.Set(sdkCtx, cur.Add(amount))
}

// RunBusinessCoreVesting liberta o LUG retido da Business Core quando o cliff
// ja passou (BeginBlock). Cliff PURO: liberta TUDO de uma vez e passa a 0.
func (k Keeper) RunBusinessCoreVesting(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if !k.businessCoreCliffPassed(sdkCtx) {
		return nil
	}

	held, err := k.businessCoreVestingHeld(sdkCtx)
	if err != nil {
		return err
	}
	if !held.IsPositive() {
		return nil
	}

	if types.BusinessCoreAddress == "" {
		return fmt.Errorf("business core address is not configured: refusing to release the business core vesting")
	}
	addr, err := k.addressCodec.StringToBytes(types.BusinessCoreAddress)
	if err != nil {
		return fmt.Errorf("business core address invalid: %w", err)
	}

	coins := sdk.NewCoins(sdk.NewCoin("uluggom", held))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(sdkCtx, types.BusinessCoreVestingModuleName, addr, coins); err != nil {
		return fmt.Errorf("business core vesting: release failed: %w", err)
	}
	if err := k.BusinessCoreVestingHeld.Set(sdkCtx, sdkmath.ZeroInt()); err != nil {
		return err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent("luggom_business_core_vesting",
		sdk.NewAttribute("action", "cliff_release"),
		sdk.NewAttribute("to", types.BusinessCoreAddress),
		sdk.NewAttribute("lug_amount", held.String()),
	))
	return nil
}
