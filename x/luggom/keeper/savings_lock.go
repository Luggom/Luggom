package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// Savings/lock module (Reservoir — §13.64/§13.65).
// Tranches 90/180/365 days; principal + yield return AUTOMATICALLY at lock maturity (§13.38).
// Yield is credited per monthly epoch (drip ~0.83%/month of the Reservoir, §13.65 decision 4),
// distributed among active locks proportionally to amount × tranche_multiplier.
// The ATOM bonus (Atom Reservoir) is paid to qualified holders — SLEEPS until IBC opens (§13.65 decision 2).

// trancheMultiplier is the tranche multiplier (Formula B, §13.43): 90d=1×, 180d=2×, 365d=4×.
func trancheMultiplier(days uint32) sdkmath.LegacyDec {
	switch days {
	case 90:
		return sdkmath.LegacyOneDec()
	case 180:
		return sdkmath.LegacyMustNewDecFromStr("2")
	case 365:
		return sdkmath.LegacyMustNewDecFromStr("4")
	default:
		return sdkmath.LegacyZeroDec()
	}
}

// validTranche reports whether days is a valid tranche (90/180/365).
func validTranche(days uint32) bool {
	return days == 90 || days == 180 || days == 365
}

// LockSavings locks LUG in the Reservoir for a tranche.
func (k Keeper) LockSavings(ctx context.Context, msg *types.MsgLockSavings) (*types.MsgLockSavingsResponse, error) {
	if !validTranche(msg.TrancheDays) {
		return nil, fmt.Errorf("invalid tranche_days %d (must be 90, 180 or 365)", msg.TrancheDays)
	}
	if msg.Amount.IsNil() || !msg.Amount.IsPositive() {
		return nil, fmt.Errorf("amount must be positive")
	}

	depositorAddr, err := sdk.AccAddressFromBech32(msg.Depositor)
	if err != nil {
		return nil, fmt.Errorf("invalid depositor: %w", err)
	}

	// Move LUG from the user to the module account (lock escrow).
	coins := sdk.NewCoins(sdk.NewCoin("uluggom", msg.Amount))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, depositorAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to lock funds: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	id, err := k.SavingsLocksSeq.Next(sdkCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate lock id: %w", err)
	}

	unlockHeight := sdkCtx.BlockHeight() + int64(msg.TrancheDays)*24*60*60/int64(5) // ~5s/bloco
	lock := types.SavingsLock{
		Id:               id,
		Depositor:        msg.Depositor,
		Amount:           msg.Amount,
		TrancheDays:      msg.TrancheDays,
		StartHeight:      sdkCtx.BlockHeight(),
		StartTime:        sdkCtx.BlockTime().Unix(),
		UnlockHeight:     unlockHeight,
		YieldAccumulated: sdkmath.ZeroInt(),
	}
	if err := k.SavingsLocks.Set(sdkCtx, id, lock); err != nil {
		return nil, err
	}
	if err := k.SavingsLocksByDepositor.Set(sdkCtx, collections.Join(msg.Depositor, id)); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_lock_savings",
		sdk.NewAttribute("depositor", msg.Depositor),
		sdk.NewAttribute("lock_id", fmt.Sprintf("%d", id)),
		sdk.NewAttribute("amount", msg.Amount.String()),
		sdk.NewAttribute("tranche_days", fmt.Sprintf("%d", msg.TrancheDays)),
		sdk.NewAttribute("unlock_height", fmt.Sprintf("%d", unlockHeight)),
	))

	return &types.MsgLockSavingsResponse{LockId: id, UnlockHeight: unlockHeight}, nil
}

// UnlockSavings cancels a lock BEFORE maturity (early exit).
// Returns the principal; the yield CREDITED so far is returned to the reservoir it
// came from (never confiscated, and never left orphaned in the module escrow) —
// leaving early only forfeits the incomplete period's yield (§13.38/§13.119).
func (k Keeper) UnlockSavings(ctx context.Context, msg *types.MsgUnlockSavings) (*types.MsgUnlockSavingsResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	lock, err := k.SavingsLocks.Get(sdkCtx, msg.LockId)
	if err != nil {
		return nil, fmt.Errorf("lock not found: %w", err)
	}
	if lock.Depositor != msg.Depositor {
		return nil, fmt.Errorf("lock %d does not belong to %s", msg.LockId, msg.Depositor)
	}
	if sdkCtx.BlockHeight() >= lock.UnlockHeight {
		return nil, fmt.Errorf("lock %d already matured — use the automatic release", msg.LockId)
	}

	// Devolve o principal.
	depositorAddr, err := sdk.AccAddressFromBech32(lock.Depositor)
	if err != nil {
		return nil, fmt.Errorf("invalid depositor address: %w", err)
	}
	coins := sdk.NewCoins(sdk.NewCoin("uluggom", lock.Amount))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, depositorAddr, coins); err != nil {
		return nil, fmt.Errorf("failed to release principal: %w", err)
	}

	// O yield já creditado volta ao reservatório de origem — o lock inexiste,
	// portanto o LUG tem de sair do escrow do módulo (senão ficaria sem dono).
	if err := k.returnYieldToReservoir(sdkCtx, types.StakingReservoirModuleName, lock.StakingYieldAccumulated); err != nil {
		return nil, err
	}
	if err := k.returnYieldToReservoir(sdkCtx, types.AtomReservoirModuleName, lock.AtomYieldAccumulated); err != nil {
		return nil, err
	}

	if err := k.deleteLock(ctx, lock); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_unlock_savings",
		sdk.NewAttribute("depositor", msg.Depositor),
		sdk.NewAttribute("lock_id", fmt.Sprintf("%d", msg.LockId)),
		sdk.NewAttribute("released", lock.Amount.String()),
	))

	return &types.MsgUnlockSavingsResponse{Released: lock.Amount}, nil
}

// ProcessMaturedLocks libera automaticamente principal + yield dos locks vencidos (§13.38).
// Chamado no BeginBlock de cada bloco.
func (k Keeper) ProcessMaturedLocks(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	height := sdkCtx.BlockHeight()

	var matured []types.SavingsLock
	err := k.SavingsLocks.Walk(sdkCtx, nil, func(key uint64, lock types.SavingsLock) (bool, error) {
		if height >= lock.UnlockHeight {
			matured = append(matured, lock)
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	for _, lock := range matured {
		total := lock.Amount.Add(lock.YieldAccumulated)
		coins := sdk.NewCoins(sdk.NewCoin("uluggom", total))
		depositorAddr, err := sdk.AccAddressFromBech32(lock.Depositor)
		if err != nil {
			continue
		}
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, depositorAddr, coins); err != nil {
			// Never confiscates: if the transfer fails, keep the lock (retried on the next block).
			continue
		}
		if err := k.deleteLock(ctx, lock); err != nil {
			return err
		}
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"luggom_lock_matured",
			sdk.NewAttribute("depositor", lock.Depositor),
			sdk.NewAttribute("lock_id", fmt.Sprintf("%d", lock.Id)),
			sdk.NewAttribute("principal", lock.Amount.String()),
			sdk.NewAttribute("yield", lock.YieldAccumulated.String()),
		))
	}
	return nil
}

func (k Keeper) deleteLock(ctx context.Context, lock types.SavingsLock) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	if err := k.SavingsLocks.Remove(sdkCtx, lock.Id); err != nil {
		return err
	}
	return k.SavingsLocksByDepositor.Remove(sdkCtx, collections.Join(lock.Depositor, lock.Id))
}

// GetLock retorna um lock pelo ID (queries/CLI).
func (k Keeper) GetLock(ctx context.Context, id uint64) (types.SavingsLock, error) {
	return k.SavingsLocks.Get(ctx, id)
}

// GetLocksByDepositor lists the locks of an address.
func (k Keeper) GetLocksByDepositor(ctx context.Context, depositor string) ([]types.SavingsLock, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	var locks []types.SavingsLock
	err := k.SavingsLocksByDepositor.Walk(sdkCtx, collections.NewPrefixedPairRange[string, uint64](depositor), func(key collections.Pair[string, uint64]) (bool, error) {
		lock, err := k.SavingsLocks.Get(sdkCtx, key.K2())
		if err != nil {
			return false, err
		}
		locks = append(locks, lock)
		return false, nil
	})
	return locks, err
}
