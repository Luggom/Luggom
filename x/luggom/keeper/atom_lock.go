package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// ATOM provisioning — qualificador da camada ATOM do savings (§13.119).
//
// O ATOM travado não rende por si: habilita a camada ATOM sobre o LUG em staking
// do mesmo dono. O ATOM fica em escrow no módulo e é devolvido no unlock —
// NUNCA confiscado (§13.38).
//
// Fica INOPERÁVEL até o IBC abrir: antes disso não existe denom ATOM on-chain.
// A qualificação é avaliada por época (mensal) no drip: um lock de LUG antigo
// cujo dono passe a ter ATOM ≥ mínimo começa a receber a camada ATOM no epoch
// seguinte.

// LockAtom deposita ATOM como qualificador. Um lock por depositor (substitui o
// anterior, somando). O ATOM tem de satisfazer o mínimo vigente (100 fixo).
func (k Keeper) LockAtom(ctx context.Context, depositor, denom string, amount sdkmath.Int) (types.AtomLock, error) {
	if denom == "" {
		return types.AtomLock{}, fmt.Errorf("atom denom is empty")
	}
	if amount.IsNil() || !amount.IsPositive() {
		return types.AtomLock{}, fmt.Errorf("amount must be positive")
	}

	depositorAddr, err := sdk.AccAddressFromBech32(depositor)
	if err != nil {
		return types.AtomLock{}, fmt.Errorf("invalid depositor: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	lock, err := k.AtomLocks.Get(sdkCtx, depositor)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			return types.AtomLock{}, err
		}
		lock = types.AtomLock{
			Depositor:   depositor,
			Denom:       denom,
			StartHeight: sdkCtx.BlockHeight(),
			MinAtLock:   types.MinAtomLock(),
		}
	}
	if lock.Denom != "" && lock.Denom != denom {
		return types.AtomLock{}, fmt.Errorf("depositor already has ATOM locked in %q (cannot mix denoms)", lock.Denom)
	}

	// Move o ATOM do utilizador para o escrow do módulo (cofre do qualificador).
	coin := sdk.NewCoins(sdk.NewCoin(denom, amount))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(sdkCtx, depositorAddr, types.ModuleName, coin); err != nil {
		return types.AtomLock{}, fmt.Errorf("failed to lock ATOM: %w", err)
	}

	before := lock.Amount
	if before.IsNil() {
		before = sdkmath.ZeroInt()
	}
	lock.Amount = before.Add(amount)
	lock.Denom = denom
	if err := k.AtomLocks.Set(sdkCtx, depositor, lock); err != nil {
		return types.AtomLock{}, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_lock_atom",
		sdk.NewAttribute("depositor", depositor),
		sdk.NewAttribute("denom", denom),
		sdk.NewAttribute("amount", amount.String()),
		sdk.NewAttribute("total_locked", lock.Amount.String()),
	))
	return lock, nil
}

// UnlockAtom devolve o ATOM travado ao depositor (nunca confisca) e remove o
// qualificador. Falha se o depositor não tiver ATOM travado.
func (k Keeper) UnlockAtom(ctx context.Context, depositor string) (sdkmath.Int, error) {
	depositorAddr, err := sdk.AccAddressFromBech32(depositor)
	if err != nil {
		return sdkmath.Int{}, fmt.Errorf("invalid depositor: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	lock, err := k.AtomLocks.Get(sdkCtx, depositor)
	if err != nil {
		return sdkmath.Int{}, fmt.Errorf("no ATOM locked for %s", depositor)
	}

	coin := sdk.NewCoins(sdk.NewCoin(lock.Denom, lock.Amount))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(sdkCtx, types.ModuleName, depositorAddr, coin); err != nil {
		return sdkmath.Int{}, fmt.Errorf("failed to release ATOM: %w", err)
	}
	if err := k.AtomLocks.Remove(sdkCtx, depositor); err != nil {
		return sdkmath.Int{}, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_unlock_atom",
		sdk.NewAttribute("depositor", depositor),
		sdk.NewAttribute("denom", lock.Denom),
		sdk.NewAttribute("released", lock.Amount.String()),
	))
	return lock.Amount, nil
}

// GetAtomLock devolve o lock de ATOM de um depositor (ok=false se não existe).
func (k Keeper) GetAtomLock(ctx context.Context, depositor string) (types.AtomLock, bool, error) {
	lock, err := k.AtomLocks.Get(ctx, depositor)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.AtomLock{}, false, nil
		}
		return types.AtomLock{}, false, err
	}
	return lock, true, nil
}

// IsAtomQualified indica se o depositor tem ATOM travado ≥ mínimo vigente —
// condição para receber a camada ATOM no drip mensal (§13.119).
func (k Keeper) IsAtomQualified(ctx context.Context, depositor string) (bool, error) {
	lock, ok, err := k.GetAtomLock(ctx, depositor)
	if err != nil || !ok {
		return false, err
	}
	return types.AtomLockQualifies(lock.Amount), nil
}
