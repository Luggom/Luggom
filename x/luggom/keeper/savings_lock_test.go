package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// TestLockSavings_Basic validates creating and querying a lock.
func TestLockSavings_Basic(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	depositor, _ := makeAddr(t, f.addressCodec, 0x33)

	resp, err := ms.LockSavings(f.ctx, &types.MsgLockSavings{
		Depositor:   depositor,
		Amount:      sdkmath.NewInt(1_000_000_000_000_000_000), // 1 LUG
		TrancheDays: 90,
	})
	require.NoError(t, err)
	require.Greater(t, resp.UnlockHeight, int64(0))

	lock, err := f.keeper.GetLock(sdk.UnwrapSDKContext(f.ctx), resp.LockId)
	require.NoError(t, err)
	require.Equal(t, depositor, lock.Depositor)
	require.Equal(t, uint32(90), lock.TrancheDays)
	require.Equal(t, sdkmath.NewInt(1_000_000_000_000_000_000).String(), lock.Amount.String())
}

// TestLockSavings_InvalidTranche rejeita tranches inválidas.
func TestLockSavings_InvalidTranche(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	depositor, _ := makeAddr(t, f.addressCodec, 0x44)
	_, err := ms.LockSavings(f.ctx, &types.MsgLockSavings{
		Depositor:   depositor,
		Amount:      sdkmath.NewInt(1_000_000_000_000_000_000),
		TrancheDays: 45,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid tranche_days")
}

// TestLockSavings_ZeroAmount rejeita valor zero.
func TestLockSavings_ZeroAmount(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	depositor, _ := makeAddr(t, f.addressCodec, 0x45)
	_, err := ms.LockSavings(f.ctx, &types.MsgLockSavings{
		Depositor:   depositor,
		Amount:      sdkmath.ZeroInt(),
		TrancheDays: 90,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "amount must be positive")
}

// TestUnlockSavings_EarlyExit devolve o principal (o mock não rastreia, só valida a lógica).
func TestUnlockSavings_EarlyExit(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	depositor, _ := makeAddr(t, f.addressCodec, 0x46)
	lockResp, err := ms.LockSavings(f.ctx, &types.MsgLockSavings{
		Depositor:   depositor,
		Amount:      sdkmath.NewInt(1_000_000_000_000_000_000),
		TrancheDays: 365,
	})
	require.NoError(t, err)
	_ = lockResp

	// Early exit devolve o principal.
	unlockResp, err := ms.UnlockSavings(f.ctx, &types.MsgUnlockSavings{
		Depositor: depositor,
		LockId:    lockResp.LockId,
	})
	require.NoError(t, err)
	require.Equal(t, sdkmath.NewInt(1_000_000_000_000_000_000).String(), unlockResp.Released.String())

	// Lock não existe mais.
	_, err = f.keeper.GetLock(sdk.UnwrapSDKContext(f.ctx), lockResp.LockId)
	require.Error(t, err)
}

// TestUnlockSavings_WrongOwner rejeita unlock de outro endereço.
func TestUnlockSavings_WrongOwner(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	depositor, _ := makeAddr(t, f.addressCodec, 0x47)
	stranger, _ := makeAddr(t, f.addressCodec, 0x48)
	lockResp, err := ms.LockSavings(f.ctx, &types.MsgLockSavings{
		Depositor:   depositor,
		Amount:      sdkmath.NewInt(1_000_000_000_000_000_000),
		TrancheDays: 90,
	})
	require.NoError(t, err)

	_, err = ms.UnlockSavings(f.ctx, &types.MsgUnlockSavings{
		Depositor: stranger,
		LockId:    lockResp.LockId,
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "does not belong")
}
