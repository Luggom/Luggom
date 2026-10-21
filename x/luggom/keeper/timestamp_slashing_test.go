package keeper_test

import (
	"testing"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	"github.com/stretchr/testify/require"
)

// TestEnforceTimestampSlashing_JailsLongAbsentValidator: a validator with a high
// missed-blocks counter (never signed for the whole window) and not jailed gets
// jailed by the timestamp slashing.
func TestEnforceTimestampSlashing_JailsLongAbsentValidator(t *testing.T) {
	consBytes := []byte{0x11, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	consAddr := sdk.ConsAddress(consBytes)

	sk := &mockStakingKeeper{
		validatorByCons: map[string]stakingtypes.Validator{
			consAddr.String(): {
				OperatorAddress: sdk.ValAddress(consBytes).String(),
				Jailed:          false,
				Status:          stakingtypes.Bonded,
			},
		},
	}
	slk := &mockSlashingKeeper{
		infos: []slashingtypes.ValidatorSigningInfo{
			{
				Address:             consAddr.String(),
				StartHeight:         1,
				IndexOffset:         10000, // full window
				JailedUntil:         time.Time{},
				MissedBlocksCounter: 10000, // never signed
			},
		},
	}
	f := initFixtureWithMocks(t, sk, slk)

	require.NoError(t, f.keeper.EnforceTimestampSlashing(f.ctx))
	require.Len(t, sk.jailedByCons, 1)
	require.Equal(t, consAddr.String(), sk.jailedByCons[0].String())
}

// TestEnforceTimestampSlashing_SkipsSigningValidator: a validator that has been
// signing (MissedBlocksCounter == 0) is NOT jailed.
func TestEnforceTimestampSlashing_SkipsSigningValidator(t *testing.T) {
	consBytes := []byte{0x22, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	consAddr := sdk.ConsAddress(consBytes)

	sk := &mockStakingKeeper{
		validatorByCons: map[string]stakingtypes.Validator{
			consAddr.String(): {
				OperatorAddress: sdk.ValAddress(consBytes).String(),
				Jailed:          false,
				Status:          stakingtypes.Bonded,
			},
		},
	}
	slk := &mockSlashingKeeper{
		infos: []slashingtypes.ValidatorSigningInfo{
			{
				Address:             consAddr.String(),
				StartHeight:         1,
				IndexOffset:         5,
				MissedBlocksCounter: 0, // signing fine
			},
		},
	}
	f := initFixtureWithMocks(t, sk, slk)

	require.NoError(t, f.keeper.EnforceTimestampSlashing(f.ctx))
	require.Len(t, sk.jailedByCons, 0)
}

// TestEnforceTimestampSlashing_SkipsAlreadyJailed: a validator already jailed is skipped.
func TestEnforceTimestampSlashing_SkipsAlreadyJailed(t *testing.T) {
	consBytes := []byte{0x33, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	consAddr := sdk.ConsAddress(consBytes)

	sk := &mockStakingKeeper{
		validatorByCons: map[string]stakingtypes.Validator{
			consAddr.String(): {
				OperatorAddress: sdk.ValAddress(consBytes).String(),
				Jailed:          true,
				Status:          stakingtypes.Bonded,
			},
		},
	}
	slk := &mockSlashingKeeper{
		infos: []slashingtypes.ValidatorSigningInfo{
			{
				Address:             consAddr.String(),
				StartHeight:         1,
				IndexOffset:         10000,
				MissedBlocksCounter: 10000,
			},
		},
	}
	f := initFixtureWithMocks(t, sk, slk)

	require.NoError(t, f.keeper.EnforceTimestampSlashing(f.ctx))
	require.Len(t, sk.jailedByCons, 0)
}
