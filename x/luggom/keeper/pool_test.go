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
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/keeper"
	module "luggom/x/luggom/module"
	"luggom/x/luggom/types"
)

// memBankKeeper is an in-memory bank keeper with real balances, for pool tests.
type memBankKeeper struct {
	balances map[string]sdk.Coins // key: bech32 address
	supply   map[string]sdkmath.Int
	module   string
}

func newMemBankKeeper() *memBankKeeper {
	return &memBankKeeper{
		balances: map[string]sdk.Coins{},
		supply:   map[string]sdkmath.Int{},
	}
}

func (m *memBankKeeper) fund(addr sdk.AccAddress, coins sdk.Coins) {
	key := addr.String()
	existing, ok := m.balances[key]
	if !ok {
		existing = sdk.NewCoins()
	}
	m.balances[key] = existing.Add(coins...)
	for _, c := range coins {
		if _, ok := m.supply[c.Denom]; !ok {
			m.supply[c.Denom] = sdkmath.ZeroInt()
		}
		m.supply[c.Denom] = m.supply[c.Denom].Add(c.Amount)
	}
}

func (m *memBankKeeper) SpendableCoins(_ context.Context, addr sdk.AccAddress) sdk.Coins {
	return m.balances[addr.String()]
}

func (m *memBankKeeper) MintCoins(_ context.Context, _ string, amt sdk.Coins) error {
	return nil
}

func (m *memBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, moduleName string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	modAddr := authtypes.NewModuleAddress(moduleName)
	from := m.balances[modAddr.String()]
	if !from.IsAllGTE(amt) {
		return sdkerrorsInsufficient()
	}
	m.balances[modAddr.String()] = from.Sub(amt...)
	to := m.balances[recipientAddr.String()]
	m.balances[recipientAddr.String()] = to.Add(amt...)
	return nil
}

func (m *memBankKeeper) SendCoinsFromAccountToModule(_ context.Context, senderAddr sdk.AccAddress, moduleName string, amt sdk.Coins) error {
	from := m.balances[senderAddr.String()]
	if !from.IsAllGTE(amt) {
		return sdkerrorsInsufficient()
	}
	m.balances[senderAddr.String()] = from.Sub(amt...)
	modAddr := authtypes.NewModuleAddress(moduleName)
	to := m.balances[modAddr.String()]
	m.balances[modAddr.String()] = to.Add(amt...)
	return nil
}

func (m *memBankKeeper) SendCoinsFromModuleToModule(_ context.Context, fromModule, toModule string, amt sdk.Coins) error {
	fromAddr := authtypes.NewModuleAddress(fromModule)
	toAddr := authtypes.NewModuleAddress(toModule)
	from := m.balances[fromAddr.String()]
	if !from.IsAllGTE(amt) {
		return sdkerrorsInsufficient()
	}
	m.balances[fromAddr.String()] = from.Sub(amt...)
	m.balances[toAddr.String()] = m.balances[toAddr.String()].Add(amt...)
	return nil
}

func (m *memBankKeeper) GetSupply(_ context.Context, denom string) sdk.Coin {
	amt, ok := m.supply[denom]
	if !ok {
		amt = sdkmath.ZeroInt()
	}
	return sdk.NewCoin(denom, amt)
}

func sdkerrorsInsufficient() error {
	return sdkerrors.ErrInsufficientFunds
}

type poolFixture struct {
	ctx          context.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	bank         *memBankKeeper
}

func initPoolFixture(t *testing.T) *poolFixture {
	t.Helper()

	encCfg := moduletestutil.MakeTestEncodingConfig(module.AppModule{})
	addressCodec := addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix())
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	storeService := runtime.NewKVStoreService(storeKey)
	ctx := testutil.DefaultContextWithDB(t, storeKey, storetypes.NewTransientStoreKey("transient_test")).Ctx

	authority := authtypes.NewModuleAddress(types.GovModuleName)
	bank := newMemBankKeeper()

	k := keeper.NewKeeper(
		storeService,
		encCfg.Codec,
		addressCodec,
		authority,
		bank,
		&mockStakingKeeper{},
		&mockSlashingKeeper{},
	)

	require.NoError(t, k.Params.Set(ctx, types.DefaultParams()))

	return &poolFixture{
		ctx:          ctx,
		keeper:       k,
		addressCodec: addressCodec,
		bank:         bank,
	}
}

// openPool opens the pool via the governable params.PoolActive flag (§13.114).
// The former 50M supply gate is extinct — the pool opens after the pre-sale.
func (f *poolFixture) openPool(t *testing.T) {
	t.Helper()
	params := types.DefaultParams()
	params.PoolActive = true
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
}

func (f *poolFixture) addr(t *testing.T, b byte) (string, sdk.AccAddress) {
	t.Helper()
	return makeAddr(t, f.addressCodec, b)
}

// lug parses a LUG amount in base units (uluggom) from a decimal string.
func lug(t *testing.T, s string) sdkmath.Int {
	t.Helper()
	v, ok := sdkmath.NewIntFromString(s)
	require.True(t, ok, "bad int %q", s)
	return v
}

func TestPool_ClosedBefore100M(t *testing.T) {
	f := initPoolFixture(t)
	dep, depAddr := f.addr(t, 1)
	f.bank.fund(depAddr, sdk.NewCoins(sdk.NewCoin("uluggom", sdkmath.NewInt(1_000_000_000_000_000_000))))

	// Supply is 0 → pool closed.
	_, err := f.keeper.AddLiquidity(f.ctx, &types.MsgAddLiquidity{
		Depositor:   dep,
		Asset:       "btc",
		AmountLug:   sdkmath.NewInt(1_000_000_000_000_000_000),
		AmountAsset: sdkmath.NewInt(1_000_000_000_000_000_000),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pool is not open")

	_, err = f.keeper.Swap(f.ctx, &types.MsgSwap{
		Trader:       dep,
		Asset:        "btc",
		BuyLug:       false,
		AmountIn:     sdkmath.NewInt(1_000_000_000_000_000_000),
		MinAmountOut: sdkmath.ZeroInt(),
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pool is not open")
}

func TestPool_AddRemoveLiquidity(t *testing.T) {
	f := initPoolFixture(t)
	f.openPool(t)
	dep, depAddr := f.addr(t, 1)

	lugAmt := lug(t, "10000000000000000000")  // 10 LUG
	assetAmt := lug(t, "5000000000000000000") // 5 btc (fake)
	f.bank.fund(depAddr, sdk.NewCoins(
		sdk.NewCoin("uluggom", lugAmt),
		sdk.NewCoin("btc", assetAmt),
	))

	// Add liquidity: first deposit → shares = sqrt(lug × asset).
	resp, err := f.keeper.AddLiquidity(f.ctx, &types.MsgAddLiquidity{
		Depositor:   dep,
		Asset:       "btc",
		AmountLug:   lugAmt,
		AmountAsset: assetAmt,
	})
	require.NoError(t, err)
	require.True(t, resp.LpShares.IsPositive())
	require.Equal(t, lugAmt, resp.LugDeposited)
	require.Equal(t, assetAmt, resp.AssetDeposited)

	// Remove it all back.
	out, err := f.keeper.RemoveLiquidity(f.ctx, &types.MsgRemoveLiquidity{
		Depositor: dep,
		Asset:     "btc",
		LpShares:  resp.LpShares,
	})
	require.NoError(t, err)
	require.Equal(t, lugAmt, out.LugOut)
	require.Equal(t, assetAmt, out.AssetOut)
}

func TestPool_SwapBothDirections(t *testing.T) {
	f := initPoolFixture(t)
	f.openPool(t)
	lp, lpAddr := f.addr(t, 1)
	trader, traderAddr := f.addr(t, 2)

	lugAmt := lug(t, "10000000000000000000")
	assetAmt := lug(t, "5000000000000000000")
	f.bank.fund(lpAddr, sdk.NewCoins(sdk.NewCoin("uluggom", lugAmt), sdk.NewCoin("btc", assetAmt)))
	f.bank.fund(traderAddr, sdk.NewCoins(sdk.NewCoin("uluggom", sdkmath.NewInt(1_000_000_000_000_000_000))))

	_, err := f.keeper.AddLiquidity(f.ctx, &types.MsgAddLiquidity{
		Depositor: lp, Asset: "btc", AmountLug: lugAmt, AmountAsset: assetAmt,
	})
	require.NoError(t, err)

	// Trader sells 1 LUG → gets btc.
	sell := sdkmath.NewInt(1_000_000_000_000_000_000) // 1 LUG
	swapOut, err := f.keeper.Swap(f.ctx, &types.MsgSwap{
		Trader: trader, Asset: "btc", BuyLug: false, AmountIn: sell,
	})
	require.NoError(t, err)
	require.True(t, swapOut.AmountOut.IsPositive())

	// Trader sells the btc back → gets LUG (less fee).
	back, err := f.keeper.Swap(f.ctx, &types.MsgSwap{
		Trader: trader, Asset: "btc", BuyLug: true, AmountIn: swapOut.AmountOut,
	})
	require.NoError(t, err)
	require.True(t, back.AmountOut.IsPositive())
	require.True(t, back.AmountOut.LT(sell), "round-trip must lose the 0.3%% fee")
}

func TestPool_ImbalanceYield(t *testing.T) {
	f := initPoolFixture(t)
	f.openPool(t)
	lp, lpAddr := f.addr(t, 1)

	lugAmt := lug(t, "10000000000000000000")
	assetAmt := lug(t, "10000000000000000000") // 10 btc → 50/50 with 10 LUG
	f.bank.fund(lpAddr, sdk.NewCoins(sdk.NewCoin("uluggom", lugAmt), sdk.NewCoin("btc", assetAmt)))

	_, err := f.keeper.AddLiquidity(f.ctx, &types.MsgAddLiquidity{
		Depositor: lp, Asset: "btc", AmountLug: lugAmt, AmountAsset: assetAmt,
	})
	require.NoError(t, err)

	// Balanced (50/50) → multiplier = 1.
	mult, err := f.keeper.PoolImbalanceYield(f.ctx, "btc")
	require.NoError(t, err)
	require.Equal(t, sdkmath.LegacyOneDec(), mult)
}
