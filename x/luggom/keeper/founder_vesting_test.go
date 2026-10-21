package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

// §13.171 — cliff PURO de 24 meses do Fundador.

func founderVestingParams(t *testing.T, f *mineFixture, founderStr string) {
	t.Helper()
	p := devParamsForTest()
	p.FounderAddress = founderStr
	p.PoolActive = false
	require.NoError(t, f.keeper.Params.Set(f.ctx, p))
}

func TestFounderVesting_RetainsBeforeCliff(t *testing.T) {
	f := initMineFixture(t)
	minerStr, _ := makeAddr(t, f.addressCodec, 0x41)
	founderStr, founderAddr := makeAddr(t, f.addressCodec, 0x42)
	founderVestingParams(t, f, founderStr)

	start := sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix()
	require.NoError(t, f.keeper.FounderVestingStart.Set(f.ctx, uint64(start)))

	_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "fv-before-cliff", 100))
	require.NoError(t, err)

	vestingAddr := authtypes.NewModuleAddress(types.FounderVestingModuleName)
	require.True(t, sentToAmount(f.bank, vestingAddr).IsPositive(),
		"antes do cliff a fatia do Fundador fica retida no modulo")
	require.True(t, sentToAmount(f.bank, founderAddr).IsZero(),
		"nada sai para o founder_address antes do cliff")
}

func TestFounderVesting_ReleasesAllAtCliff(t *testing.T) {
	f := initMineFixture(t)
	minerStr, _ := makeAddr(t, f.addressCodec, 0x43)
	founderStr, founderAddr := makeAddr(t, f.addressCodec, 0x44)
	founderVestingParams(t, f, founderStr)

	start := sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix()
	require.NoError(t, f.keeper.FounderVestingStart.Set(f.ctx, uint64(start)))

	_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "fv-at-cliff", 100))
	require.NoError(t, err)

	vestingAddr := authtypes.NewModuleAddress(types.FounderVestingModuleName)
	retained := sentToAmount(f.bank, vestingAddr)
	require.True(t, retained.IsPositive())

	// Um segundo antes do cliff: continua tudo retido.
	before := time.Unix(start, 0).UTC().
		Add(time.Duration(types.FounderVestingCliffSeconds())*time.Second - time.Second)
	require.NoError(t, f.keeper.RunFounderVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(before)))
	require.True(t, sentToAmount(f.bank, founderAddr).IsZero())

	// No cliff: liberta TUDO o que estava retido.
	at := time.Unix(start, 0).UTC().Add(time.Duration(types.FounderVestingCliffSeconds()) * time.Second)
	require.NoError(t, f.keeper.RunFounderVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(at)))
	require.Equal(t, retained.String(), sentToAmount(f.bank, founderAddr).String())

	// Segunda chamada: ja nao ha nada retido (no-op).
	require.NoError(t, f.keeper.RunFounderVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(at)))
	require.Equal(t, retained.String(), sentToAmount(f.bank, founderAddr).String())
}

func TestFounderVesting_DirectAfterCliff(t *testing.T) {
	f := initMineFixture(t)
	minerStr, _ := makeAddr(t, f.addressCodec, 0x45)
	founderStr, founderAddr := makeAddr(t, f.addressCodec, 0x46)
	founderVestingParams(t, f, founderStr)

	// Inicio no passado → o cliff ja passou.
	require.NoError(t, f.keeper.FounderVestingStart.Set(f.ctx, uint64(1)))

	_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "fv-after-cliff", 100))
	require.NoError(t, err)

	require.True(t, sentToAmount(f.bank, founderAddr).IsPositive(),
		"depois do cliff a fatia vai directa para o multisig")
	vestingAddr := authtypes.NewModuleAddress(types.FounderVestingModuleName)
	require.True(t, sentToAmount(f.bank, vestingAddr).IsZero())
}

func TestFounderVesting_NoOpWhenNothingRetained(t *testing.T) {
	f := initMineFixture(t)
	founderStr, founderAddr := makeAddr(t, f.addressCodec, 0x47)
	founderVestingParams(t, f, founderStr)
	require.NoError(t, f.keeper.FounderVestingStart.Set(f.ctx, uint64(1)))

	require.NoError(t, f.keeper.RunFounderVesting(f.ctx))
	require.True(t, sentToAmount(f.bank, founderAddr).IsZero())
}

func TestFounderVesting_CliffIs24Months(t *testing.T) {
	require.Equal(t, int64(24*30*24*60*60), types.FounderVestingCliffSeconds())
	require.Equal(t, sdkmath.NewInt(0).String(), sdkmath.ZeroInt().String())
}
