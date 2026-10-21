package keeper_test

import (
	"context"
	"testing"

	"cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"luggom/x/luggom/keeper"
	module "luggom/x/luggom/module"
	"luggom/x/luggom/types"
)

type mockBankKeeper struct {
	supply sdk.Coin // configurable total uluggom supply (PoolOpen trigger, §13.84)
}

func (mockBankKeeper) SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins { return nil }
func (mockBankKeeper) MintCoins(context.Context, string, sdk.Coins) error       { return nil }
func (mockBankKeeper) SendCoinsFromModuleToAccount(context.Context, string, sdk.AccAddress, sdk.Coins) error {
	return nil
}
func (mockBankKeeper) SendCoinsFromModuleToModule(context.Context, string, string, sdk.Coins) error {
	return nil
}
func (mockBankKeeper) SendCoinsFromAccountToModule(context.Context, sdk.AccAddress, string, sdk.Coins) error {
	return nil
}
func (m mockBankKeeper) GetSupply(_ context.Context, _ string) sdk.Coin {
	if m.supply.IsNil() || m.supply.IsZero() {
		return sdk.NewCoin("uluggom", sdkmath.ZeroInt())
	}
	return m.supply
}

// mockStakingKeeper implements types.StakingKeeper for tests (FASE 3).
// Default: no bonded tokens → the validator cap falls back to no cap.
type mockStakingKeeper struct {
	totalBonded     sdkmath.Int
	bonded          []stakingtypes.Validator
	jailedByCons    []sdk.ConsAddress
	validatorByCons map[string]stakingtypes.Validator
}

func (m *mockStakingKeeper) TotalBondedTokens(context.Context) (sdkmath.Int, error) {
	if m.totalBonded.IsNil() {
		return sdkmath.ZeroInt(), nil
	}
	return m.totalBonded, nil
}

func (m *mockStakingKeeper) GetBondedValidatorsByPower(context.Context) ([]stakingtypes.Validator, error) {
	return m.bonded, nil
}

func (m *mockStakingKeeper) GetValidator(context.Context, sdk.ValAddress) (stakingtypes.Validator, error) {
	return stakingtypes.Validator{}, nil
}

func (m *mockStakingKeeper) GetValidatorByConsAddr(_ context.Context, consAddr sdk.ConsAddress) (stakingtypes.Validator, error) {
	if m.validatorByCons != nil {
		if v, ok := m.validatorByCons[consAddr.String()]; ok {
			return v, nil
		}
	}
	return stakingtypes.Validator{}, stakingtypes.ErrNoValidatorFound
}

func (m *mockStakingKeeper) GetLastValidatorPower(context.Context, sdk.ValAddress) (int64, error) {
	return 0, nil
}

func (m *mockStakingKeeper) Jail(_ context.Context, consAddr sdk.ConsAddress) error {
	m.jailedByCons = append(m.jailedByCons, consAddr)
	return nil
}

// mockSlashingKeeper implements types.SlashingKeeper for tests (FASE 3).
type mockSlashingKeeper struct {
	infos []slashingtypes.ValidatorSigningInfo
}

func (m *mockSlashingKeeper) GetValidatorSigningInfo(context.Context, sdk.ConsAddress) (slashingtypes.ValidatorSigningInfo, error) {
	return slashingtypes.ValidatorSigningInfo{}, slashingtypes.ErrNoSigningInfoFound
}

func (m *mockSlashingKeeper) IterateValidatorSigningInfos(_ context.Context, fn func(sdk.ConsAddress, slashingtypes.ValidatorSigningInfo) (stop bool)) error {
	for _, info := range m.infos {
		consAddr, err := sdk.ConsAddressFromBech32(info.Address)
		if err != nil {
			continue
		}
		if fn(consAddr, info) {
			break
		}
	}
	return nil
}

type fixture struct {
	ctx          context.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	staking      *mockStakingKeeper
	slashing     *mockSlashingKeeper
	bank         *mockBankKeeper
}

func initFixture(t *testing.T) *fixture {
	t.Helper()
	return initFixtureWithMocks(t, &mockStakingKeeper{}, &mockSlashingKeeper{})
}

// initFixtureWithMocks builds a fixture with custom staking/slashing mocks.
func initFixtureWithMocks(t *testing.T, sk *mockStakingKeeper, slk *mockSlashingKeeper) *fixture {
	t.Helper()

	encCfg := moduletestutil.MakeTestEncodingConfig(module.AppModule{})
	addressCodec := addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix())
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)

	storeService := runtime.NewKVStoreService(storeKey)
	ctx := testutil.DefaultContextWithDB(t, storeKey, storetypes.NewTransientStoreKey("transient_test")).Ctx

	authority := authtypes.NewModuleAddress(types.GovModuleName)

	bank := &mockBankKeeper{}
	k := keeper.NewKeeper(
		storeService,
		encCfg.Codec,
		addressCodec,
		authority,
		bank,
		sk,
		slk,
	)

	// Initialize params
	if err := k.Params.Set(ctx, types.DefaultParams()); err != nil {
		t.Fatalf("failed to set params: %v", err)
	}

	return &fixture{
		ctx:          ctx,
		keeper:       k,
		addressCodec: addressCodec,
		staking:      sk,
		slashing:     slk,
		bank:         bank,
	}
}
