package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// Helpers locais (o `makeAddr`/`sentToAmount` vivem no mine_test.go).
func preSaleInt(t *testing.T, s string) sdkmath.Int {
	t.Helper()
	v, ok := sdkmath.NewIntFromString(s)
	require.True(t, ok, "bad int %q", s)
	return v
}

// presaleLug returns n whole LUG in uluggom base units.
func presaleLug(n int64) sdkmath.Int { return sdkmath.NewInt(n).MulRaw(1_000_000_000_000_000_000) }

// usdt returns n whole USDT in base units (6 decimals).
func usdt(n int64) sdkmath.Int { return sdkmath.NewInt(n).MulRaw(1_000_000) }

// backing dá à conta do pre-sale o LUG que a venda promete.
func backing(f *mineFixture, amount sdkmath.Int) {
	f.bank.spendableByAddr = map[string]sdk.Coins{
		authtypes.NewModuleAddress(types.PreSaleModuleName).String(): sdk.NewCoins(
			sdk.NewCoin("uluggom", amount)),
	}
}

func TestBuyPreSale_CreatesVestingPositionWithoutMovingLug(t *testing.T) {
	f := initMineFixture(t)
	buyer, buyerAddr := makeAddr(t, f.addressCodec, 0x21)
	backing(f, presaleLug(20_000))

	// 1000 USDT a $0,10 → 10.000 LUG.
	resp, err := f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{
		Buyer: buyer, UsdtAmount: usdt(1_000),
	})
	require.NoError(t, err)
	require.Equal(t, presaleLug(10_000).String(), resp.LugAmount.String())

	// O LUG NÃO sai já: o comprador só recebe à medida que veste.
	require.True(t, sentToAmount(f.bank, buyerAddr).IsZero())

	// O total vendido ficou registado.
	committed, err := f.keeper.PreSaleCommitted.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, presaleLug(10_000).String(), committed.String())

	// E existe uma posição com a janela de vesting certa.
	pos, err := f.keeper.PreSalePositions.Get(f.ctx, 0)
	require.NoError(t, err)
	require.Equal(t, buyer, pos.Buyer)
	require.Equal(t, presaleLug(10_000).String(), pos.TotalLug.String())
	require.Equal(t, types.PreSaleVestingSeconds(), pos.EndTime-pos.StartTime)
}

func TestBuyPreSale_BelowMinimum(t *testing.T) {
	f := initMineFixture(t)
	buyer, _ := makeAddr(t, f.addressCodec, 0x22)
	backing(f, presaleLug(20_000))

	_, err := f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{Buyer: buyer, UsdtAmount: usdt(5)})
	require.ErrorIs(t, err, types.ErrPreSaleBelowMinimum)

	_, err = f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{Buyer: buyer, UsdtAmount: sdkmath.ZeroInt()})
	require.ErrorIs(t, err, types.ErrPreSaleBelowMinimum)
}

func TestBuyPreSale_ClosedAfterTenMillionMined(t *testing.T) {
	f := initMineFixture(t)
	buyer, _ := makeAddr(t, f.addressCodec, 0x23)
	backing(f, presaleLug(20_000))

	// Supply ≥ 10M → fim da Fase 1.
	f.bank.supply = sdk.NewCoin("uluggom", preSaleInt(t, "10000000000000000000000000"))

	_, err := f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{Buyer: buyer, UsdtAmount: usdt(1_000)})
	require.ErrorIs(t, err, types.ErrPreSaleClosed)
}

func TestBuyPreSale_NotBacked(t *testing.T) {
	f := initMineFixture(t)
	buyer, _ := makeAddr(t, f.addressCodec, 0x24)
	// Sem `backing`: a conta do pre-sale está vazia.

	_, err := f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{Buyer: buyer, UsdtAmount: usdt(1_000)})
	require.ErrorIs(t, err, types.ErrPreSaleNotBacked)
}

func TestBuyPreSale_CapIsThreeAndAHalfMillion(t *testing.T) {
	f := initMineFixture(t)
	buyer, _ := makeAddr(t, f.addressCodec, 0x25)
	backing(f, presaleLug(4_000_000))

	// Comprar exactamente o cap passa...
	_, err := f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{Buyer: buyer, UsdtAmount: usdt(350_000)})
	require.NoError(t, err)

	// ...e mais um cêntimo já não.
	_, err = f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{Buyer: buyer, UsdtAmount: usdt(10)})
	require.ErrorIs(t, err, types.ErrPreSaleCapReached)
}

func TestRunPreSaleVesting_LinearRelease(t *testing.T) {
	f := initMineFixture(t)
	buyer, buyerAddr := makeAddr(t, f.addressCodec, 0x26)
	backing(f, presaleLug(20_000))

	_, err := f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{Buyer: buyer, UsdtAmount: usdt(1_000)})
	require.NoError(t, err)

	start := sdk.UnwrapSDKContext(f.ctx).BlockTime()

	// 45 dias de 90 → metade.
	half := start.Add(45 * 24 * time.Hour)
	require.NoError(t, f.keeper.RunPreSaleVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(half)))
	require.Equal(t, presaleLug(5_000).String(), sentToAmount(f.bank, buyerAddr).String())

	// 90 dias → tudo.
	end := start.Add(90 * 24 * time.Hour)
	require.NoError(t, f.keeper.RunPreSaleVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(end)))
	require.Equal(t, presaleLug(10_000).String(), sentToAmount(f.bank, buyerAddr).String())

	// Muito depois do fim → nada mais (nunca acima do total).
	further := start.Add(400 * 24 * time.Hour)
	require.NoError(t, f.keeper.RunPreSaleVesting(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(further)))
	require.Equal(t, presaleLug(10_000).String(), sentToAmount(f.bank, buyerAddr).String())
}

func TestRunPreSaleVesting_IsOncePerDay(t *testing.T) {
	f := initMineFixture(t)
	buyer, buyerAddr := makeAddr(t, f.addressCodec, 0x27)
	backing(f, presaleLug(20_000))

	_, err := f.keeper.BuyPreSale(f.ctx, &types.MsgBuyPreSale{Buyer: buyer, UsdtAmount: usdt(1_000)})
	require.NoError(t, err)

	start := sdk.UnwrapSDKContext(f.ctx).BlockTime()
	at := start.Add(45 * 24 * time.Hour)
	ctx := sdk.UnwrapSDKContext(f.ctx).WithBlockTime(at)

	require.NoError(t, f.keeper.RunPreSaleVesting(ctx))
	first := sentToAmount(f.bank, buyerAddr)

	// Segunda chamada no MESMO dia não liberta outra vez.
	require.NoError(t, f.keeper.RunPreSaleVesting(ctx))
	require.Equal(t, first.String(), sentToAmount(f.bank, buyerAddr).String())
	require.True(t, first.IsPositive())
}

func TestPreSalePositionStateRoundTrip(t *testing.T) {
	f := initMineFixture(t)

	want := types.PreSalePosition{
		Buyer:       "luggom1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq4v8n7",
		TotalLug:    presaleLug(10_000),
		ReleasedLug: presaleLug(2_500),
		StartTime:   1_700_000_000,
		EndTime:     1_700_000_000 + 7_776_000,
	}
	require.NoError(t, f.keeper.PreSalePositions.Set(f.ctx, 7, want))

	got, err := f.keeper.PreSalePositions.Get(f.ctx, 7)
	require.NoError(t, err)
	require.Equal(t, want.Buyer, got.Buyer)
	require.Equal(t, want.TotalLug.String(), got.TotalLug.String())
	require.Equal(t, want.ReleasedLug.String(), got.ReleasedLug.String())
	require.Equal(t, want.StartTime, got.StartTime)
	require.Equal(t, want.EndTime, got.EndTime)
}

func TestDistributePreSaleProceeds_AuthorityOnly(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	authority, err := f.addressCodec.BytesToString(authtypes.NewModuleAddress(types.GovModuleName))
	require.NoError(t, err)
	notAuthority, _ := makeAddr(t, f.addressCodec, 0x31)
	dest, _ := makeAddr(t, f.addressCodec, 0x32)

	// Conta errada → recusado.
	_, err = ms.DistributePreSaleProceeds(f.ctx, &types.MsgDistributePreSaleProceeds{
		Authority: notAuthority, Destination: dest, Amount: usdt(100),
	})
	require.ErrorIs(t, err, types.ErrInvalidSigner)

	// Authority correcta → passa.
	_, err = ms.DistributePreSaleProceeds(f.ctx, &types.MsgDistributePreSaleProceeds{
		Authority: authority, Destination: dest, Amount: usdt(100),
	})
	require.NoError(t, err)
}
