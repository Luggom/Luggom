package keeper

import (
	"context"
	"fmt"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

const (
	// luggomMinBondedForStakeLimit is the number of bonded validators that
	// activates the 1% per-validator stake limit (bootstrap clause §13.26
	// point 7: the limit does not apply while there are fewer than 50
	// validators, so the founder can bootstrap the network).
	luggomMinBondedForStakeLimit = 50

	// luggomStakeLimitBps is the maximum stake (self + delegated) per validator,
	// in basis points of total bonded stake: 100 bps = 1% (§13.24 point 3).
	luggomStakeLimitBps = int64(100)
)

// LuggomEnforceStakeLimit rejects a delegation (or redelegation) that would push
// a validator's total stake (self + delegated) above 1% of the total bonded
// stake, once there are ≥50 bonded validators. Below that (bootstrap) the limit
// does not apply. It must be called BEFORE the validator's tokens are updated.
func (k Keeper) LuggomEnforceStakeLimit(
	ctx context.Context,
	valAddr sdk.ValAddress,
	additional math.Int,
) error {
	// Bootstrap clause: with fewer than 50 bonded validators the limit is off.
	bondedCount := int64(0)
	err := k.IterateValidators(ctx, func(_ int64, v stakingtypes.ValidatorI) (stop bool) {
		if v.IsBonded() && !v.IsJailed() {
			bondedCount++
		}
		return false
	})
	if err != nil {
		return err
	}
	if bondedCount < luggomMinBondedForStakeLimit {
		return nil
	}

	totalBonded, err := k.TotalBondedTokens(ctx)
	if err != nil {
		return err
	}
	if !totalBonded.IsPositive() {
		return nil
	}

	validator, err := k.GetValidator(ctx, valAddr)
	if err != nil {
		return err
	}

	// Projected stake after this delegation/redelegation.
	projected := validator.GetBondedTokens().Add(additional)
	limit := totalBonded.MulRaw(luggomStakeLimitBps).QuoRaw(10_000)

	if projected.GT(limit) {
		return fmt.Errorf(
			"%w: validator %s would have %s (limit %s = 1%% of %s)",
			stakingtypes.ErrDelegationExceedsStakeLimit,
			valAddr.String(), projected.String(), limit.String(), totalBonded.String(),
		)
	}
	return nil
}
