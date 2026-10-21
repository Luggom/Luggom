package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"
)

// TestMiningCapVigente_Year0 verifies the annual cap: 7B LUG at year 0 (§13.116).
func TestMiningCapVigente_Year0(t *testing.T) {
	f := initMineFixture(t)
	cap, err := f.keeper.MiningCapVigenteForTest(sdk.UnwrapSDKContext(f.ctx))
	require.NoError(t, err)
	// 7B LUG in uluggom = 7 × 10^9 × 10^18 = 7e27
	expected := sdkmath.NewInt(7).Mul(sdkmath.NewIntWithDecimal(1, 27))
	require.Equal(t, expected.String(), cap.String())
}

// TestRecordMinedAmount_CapRejectsOverflow: a mine above the cap is rejected.
func TestRecordMinedAmount_CapRejectsOverflow(t *testing.T) {
	f := initMineFixture(t)
	cap, err := f.keeper.MiningCapVigenteForTest(sdk.UnwrapSDKContext(f.ctx))
	require.NoError(t, err)
	err = f.keeper.RecordMinedAmountForTest(f.ctx, cap.MulRaw(2))
	require.Error(t, err)
	require.Contains(t, err.Error(), "mining cap exceeded")
}

// TestRecordMinedAmount_NormalMine: a normal mine is recorded.
func TestRecordMinedAmount_NormalMine(t *testing.T) {
	f := initMineFixture(t)
	err := f.keeper.RecordMinedAmountForTest(f.ctx, sdkmath.NewInt(1000))
	require.NoError(t, err)
	// A second normal mine also passes.
	err = f.keeper.RecordMinedAmountForTest(f.ctx, sdkmath.NewInt(1000))
	require.NoError(t, err)
}
