package keeper_test

import (
	"go.uber.org/mock/gomock"

	"cosmossdk.io/math"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/ed25519"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// TestLuggomEnforceStakeLimit_BootstrapIsExempt: with fewer than 50 bonded
// validators the 1% limit does not apply (bootstrap clause §13.26).
func (s *KeeperTestSuite) TestLuggomEnforceStakeLimit_BootstrapIsExempt() {
	ctx, keeper := s.ctx, s.stakingKeeper
	require := s.Require()

	// One validator only → bootstrap.
	valAddr := sdk.ValAddress(sdk.AccAddress([]byte("bootstrapvalidator")))
	require.NoError(keeper.SetValidator(ctx, stakingtypes.Validator{
		OperatorAddress: valAddr.String(),
		Status:          stakingtypes.Bonded,
		Tokens:          math.NewInt(10_000),
		DelegatorShares: math.LegacyNewDec(10_000),
	}))

	// A huge delegation would exceed 1% but there are <50 validators → allowed.
	err := keeper.LuggomEnforceStakeLimit(ctx, valAddr, math.NewInt(1_000_000_000))
	require.NoError(err)
}

// TestLuggomEnforceStakeLimit_RejectsOverLimit: with ≥50 bonded validators, a
// delegation that pushes a validator above 1% of total bonded is rejected.
func (s *KeeperTestSuite) TestLuggomEnforceStakeLimit_RejectsOverLimit() {
	ctx, keeper := s.ctx, s.stakingKeeper
	require := s.Require()

	// 50 validators with equal stake of 1000 each → total bonded 50_000; 1% = 500.
	for i := 0; i < 50; i++ {
		pk := ed25519.GenPrivKey().PubKey()
		pkAny, err := codectypes.NewAnyWithValue(pk)
		require.NoError(err)
		valAddr := sdk.ValAddress(sdk.AccAddress([]byte{byte(i + 1), 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
		require.NoError(keeper.SetValidator(ctx, stakingtypes.Validator{
			OperatorAddress: valAddr.String(),
			ConsensusPubkey: pkAny,
			Status:          stakingtypes.Bonded,
			Tokens:          math.NewInt(1000),
			DelegatorShares: math.LegacyNewDec(1000),
		}))
	}
	// A small validator (100 tokens) for the "passes" case: 100 + 100 = 200 ≤ 500.
	smallPk := ed25519.GenPrivKey().PubKey()
	smallPkAny, err := codectypes.NewAnyWithValue(smallPk)
	require.NoError(err)
	smallTarget := sdk.ValAddress(sdk.AccAddress([]byte{0xBB, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))
	require.NoError(keeper.SetValidator(ctx, stakingtypes.Validator{
		OperatorAddress: smallTarget.String(),
		ConsensusPubkey: smallPkAny,
		Status:          stakingtypes.Bonded,
		Tokens:          math.NewInt(100),
		DelegatorShares: math.LegacyNewDec(100),
	}))

	// Mock the bonded pool balance so TotalBondedTokens = 50_000.
	s.accountKeeper.EXPECT().GetModuleAccount(gomock.Any(), stakingtypes.BondedPoolName).Return(bondedAcc).AnyTimes()
	s.bankKeeper.EXPECT().
		GetBalance(gomock.Any(), bondedAcc.GetAddress(), "stake").
		Return(sdk.NewInt64Coin("stake", 50_000)).
		AnyTimes()

	target := sdk.ValAddress(sdk.AccAddress([]byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}))

	// Any additional delegation of 501+ to one validator exceeds 1% of 50_000 (500).
	err = keeper.LuggomEnforceStakeLimit(ctx, target, math.NewInt(501))
	require.ErrorIs(err, stakingtypes.ErrDelegationExceedsStakeLimit, "unexpected err: %v", err)

	// A small delegation to a small validator is fine (100 + 100 = 200 ≤ 500).
	err = keeper.LuggomEnforceStakeLimit(ctx, smallTarget, math.NewInt(100))
	require.NoError(err)
}
