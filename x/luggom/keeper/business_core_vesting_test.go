package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

// Cliff da Luggom Business Core: 5% em cada fase; o relogio fica CONGELADO na
// Fase 1, arranca no primeiro mint da Fase 2 e, 24 meses depois, liberta todo o
// acumulado (Fase 1 + Fase 2).

func TestBusinessCoreVesting_Phase1RetainsWithFrozenClock(t *testing.T) {
	f := initMineFixture(t)
	minerStr, _ := makeAddr(t, f.addressCodec, 0x61)

	params := devParamsForTest()
	params.PoolActive = false
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	f.bank.supply = sdk.NewCoin("uluggom", sdkmath.ZeroInt())

	_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "bc-frozen-clock", 100))
	require.NoError(t, err)

	vestingAddr := authtypes.NewModuleAddress(types.BusinessCoreVestingModuleName)
	require.True(t, sentToAmount(f.bank, vestingAddr).IsPositive(),
		"na Fase 1 os 5%% ficam retidos no module account de vesting")

	businessCoreAddr, err := sdk.AccAddressFromBech32(types.BusinessCoreAddress)
	require.NoError(t, err)
	require.True(t, sentToAmount(f.bank, businessCoreAddr).IsZero(),
		"com o relogio congelado nada sai para a LBC")

	_, err = f.keeper.BusinessCoreVestingStart.Get(f.ctx)
	require.ErrorIs(t, err, collections.ErrNotFound,
		"o cliff nao pode arrancar na Fase 1")
}

func TestBusinessCoreVesting_ClockStartsInPhase2AndReleasesAll(t *testing.T) {
	f := initMineFixture(t)
	minerStr, _ := makeAddr(t, f.addressCodec, 0x62)

	params := devParamsForTest()
	params.PoolActive = false
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	vestingAddr := authtypes.NewModuleAddress(types.BusinessCoreVestingModuleName)
	businessCoreAddr, err := sdk.AccAddressFromBech32(types.BusinessCoreAddress)
	require.NoError(t, err)

	// Fase 1: acumula com o relogio parado.
	f.bank.supply = sdk.NewCoin("uluggom", sdkmath.ZeroInt())
	_, err = f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "bc-phase1", 100))
	require.NoError(t, err)
	phase1Held := sentToAmount(f.bank, vestingAddr)
	require.True(t, phase1Held.IsPositive())

	// Fase 2: o primeiro mint arranca o relogio.
	thr, ok := sdkmath.NewIntFromString("10000000000000000000000000") // 10M LUG
	require.True(t, ok)
	f.bank.supply = sdk.NewCoin("uluggom", thr)
	_, err = f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "bc-phase2", 100))
	require.NoError(t, err)

	start, err := f.keeper.BusinessCoreVestingStart.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix()), start)

	held := sentToAmount(f.bank, vestingAddr)
	require.True(t, held.GT(phase1Held), "a Fase 2 soma ao retido da Fase 1")

	// Um segundo antes do cliff: continua tudo retido.
	before := time.Unix(int64(start), 0).UTC().
		Add(time.Duration(types.FounderVestingCliffSeconds())*time.Second - time.Second)
	require.NoError(t, f.keeper.RunBusinessCoreVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(before)))
	require.True(t, sentToAmount(f.bank, businessCoreAddr).IsZero())

	// No cliff: liberta TUDO o acumulado (Fase 1 + Fase 2) de uma vez.
	at := time.Unix(int64(start), 0).UTC().Add(time.Duration(types.FounderVestingCliffSeconds()) * time.Second)
	require.NoError(t, f.keeper.RunBusinessCoreVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(at)))
	require.Equal(t, held.String(), sentToAmount(f.bank, businessCoreAddr).String())

	// Segunda chamada: ja nao ha nada retido (no-op).
	require.NoError(t, f.keeper.RunBusinessCoreVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(at)))
	require.Equal(t, held.String(), sentToAmount(f.bank, businessCoreAddr).String())
}
