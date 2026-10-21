package keeper

import (
	"context"
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// GetFounderVotingPower returns the voting power of the founder.
// For MVP, this is calculated based on their stake as a validator.
func (k Keeper) GetFounderVotingPower(ctx context.Context) (sdkmath.Int, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// Get params to find the founder address
	params, err := k.Params.Get(ctx)
	if err != nil {
		return sdkmath.ZeroInt(), fmt.Errorf("failed to get params: %w", err)
	}

	if params.FounderAddress == "" {
		return sdkmath.ZeroInt(), fmt.Errorf("founder_address not configured")
	}

	// Parse the founder address
	founderAddr, err := k.addressCodec.StringToBytes(params.FounderAddress)
	if err != nil {
		return sdkmath.ZeroInt(), fmt.Errorf("invalid founder_address: %w", err)
	}

	// For MVP, voting power is based on validator stake
	// In production, this should integrate with the staking module
	// For now, return a placeholder - will be implemented after proto regeneration
	_ = sdkCtx
	_ = founderAddr

	return sdkmath.ZeroInt(), fmt.Errorf("voting power calculation not yet implemented - requires proto regeneration")
}

// CanVeto checks if the founder can veto a proposal based on their voting power.
func (k Keeper) CanVeto(ctx context.Context) (bool, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get params: %w", err)
	}

	if params.FounderAddress == "" {
		return false, fmt.Errorf("founder_address not configured")
	}

	// Get founder's voting power
	founderPower, err := k.GetFounderVotingPower(ctx)
	if err != nil {
		return false, err
	}

	// Get total voting power (will be implemented after proto regeneration)
	// For MVP, this is a placeholder
	totalPower := sdkmath.ZeroInt()

	// Check if founder has >= veto_threshold
	return types.CheckVetoPower(
		sdk.UnwrapSDKContext(ctx),
		nil, // Will be properly implemented after proto regeneration
		founderPower,
		totalPower,
		types.VetoThresholdFrozen,
	), nil
}
