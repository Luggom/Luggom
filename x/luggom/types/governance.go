package types

import (
	"fmt"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// GovernanceKeeper defines the expected interface for governance operations.
// This allows the luggom module to interact with governance without direct dependency.
type GovernanceKeeper interface {
	// GetVotingPower returns the voting power of an address (as validator)
	GetVotingPower(ctx sdk.Context, addr sdk.AccAddress) sdkmath.Int
	
	// IsValidator checks if an address is a validator
	IsValidator(ctx sdk.Context, addr sdk.AccAddress) bool
	
	// GetTotalVotingPower returns the total voting power of all validators
	GetTotalVotingPower(ctx sdk.Context) sdkmath.Int
}

// StakingKeeperLegacy defines the expected interface for staking operations
// (legacy — kept for governance helpers; the keeper uses types.StakingKeeper from
// expected_keepers.go, FASE 3).
type StakingKeeperLegacy interface {
	// GetValidator returns a validator by address
	GetValidator(ctx sdk.Context, addr sdk.ValAddress) (stakingtypes.ValidatorI, error)
	
	// IterateValidators iterates over all validators
	IterateValidators(ctx sdk.Context, fn func(int64, stakingtypes.ValidatorI) bool)
}

// CheckVetoPower checks if the founder has enough voting power to veto a proposal.
// Returns true if the founder can veto (has >= veto_threshold of total voting power).
func CheckVetoPower(
	ctx sdk.Context,
	founderAddr sdk.AccAddress,
	fundadorVotingPower sdkmath.Int,
	totalVotingPower sdkmath.Int,
	vetoThreshold sdkmath.LegacyDec,
) bool {
	if totalVotingPower.IsZero() {
		return false
	}
	
	// Calculate founder's percentage of total voting power
	founderPowerDec := sdkmath.LegacyNewDecFromInt(fundadorVotingPower)
	totalPowerDec := sdkmath.LegacyNewDecFromInt(totalVotingPower)
	founderPercentage := founderPowerDec.Quo(totalPowerDec)
	
	// Check if founder has >= veto_threshold
	return founderPercentage.GTE(vetoThreshold)
}

// ValidateProposalVoting validates that only validators can vote on proposals.
func ValidateProposalVoting(
	ctx sdk.Context,
	voterAddr sdk.AccAddress,
	stakingKeeper StakingKeeperLegacy,
) error {
	// Convert account address to validator address
	valAddr := sdk.ValAddress(voterAddr)
	
	// Check if the address is a validator
	_, err := stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return fmt.Errorf("address %s is not a validator: only validators can vote", voterAddr.String())
	}
	
	return nil
}

// CalculateQuorum calculates if a proposal has reached the required quorum.
// quorum is the minimum percentage of voting power that must vote.
func CalculateQuorum(
	votesPower sdkmath.Int,
	totalVotingPower sdkmath.Int,
	quorumThreshold sdkmath.LegacyDec,
) bool {
	if totalVotingPower.IsZero() {
		return false
	}
	
	votesPowerDec := sdkmath.LegacyNewDecFromInt(votesPower)
	totalPowerDec := sdkmath.LegacyNewDecFromInt(totalVotingPower)
	votesPercentage := votesPowerDec.Quo(totalPowerDec)
	
	return votesPercentage.GTE(quorumThreshold)
}
