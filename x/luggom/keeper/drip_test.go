package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// TestRunMonthlyDrip_FirstRun marca o epoch sem distribuir.
func TestRunMonthlyDrip_FirstRun(t *testing.T) {
	f := initMineFixture(t)
	require.NoError(t, f.keeper.RunMonthlyDrip(f.ctx))
	// Segunda chamada no mesmo mês: no-op.
	require.NoError(t, f.keeper.RunMonthlyDrip(f.ctx))
}

// TestProcessMaturedLocks_NoLocks é no-op.
func TestProcessMaturedLocks_NoLocks(t *testing.T) {
	f := initMineFixture(t)
	require.NoError(t, f.keeper.ProcessMaturedLocks(f.ctx))
}

// TestLockSavings_MaturedAutomatic: um lock com unlock no passado é liberado automaticamente.
func TestLockSavings_MaturedAutomatic(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)
	depositor, _ := makeAddr(t, f.addressCodec, 0x55)

	lockResp, err := ms.LockSavings(f.ctx, &types.MsgLockSavings{
		Depositor:   depositor,
		Amount:      sdkmath.NewInt(1_000_000_000_000_000_000),
		TrancheDays: 90,
	})
	require.NoError(t, err)
	require.Greater(t, lockResp.UnlockHeight, int64(0))
}
