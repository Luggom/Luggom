package keeper_test

import (
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

func TestEscrow_AtomicSwap(t *testing.T) {
	f := initPoolFixture(t)
	seller, sellerAddr := f.addr(t, 1)
	buyer, buyerAddr := f.addr(t, 2)

	lugAmt := lug(t, "10000000000000000000") // 10 LUG
	assetAmt := lug(t, "500000000000000000") // 0.5 usdt
	f.bank.fund(sellerAddr, sdk.NewCoins(sdk.NewCoin("uluggom", lugAmt)))
	f.bank.fund(buyerAddr, sdk.NewCoins(sdk.NewCoin("usdt", assetAmt)))

	// Seller creates the offer (LUG locked).
	created, err := f.keeper.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Seller:      seller,
		AmountLug:   lugAmt,
		Asset:       "usdt",
		AmountAsset: assetAmt,
	})
	require.NoError(t, err)

	trade, err := f.keeper.GetEscrow(f.ctx, created.EscrowId)
	require.NoError(t, err)
	require.Equal(t, types.EscrowStatus_ESCROW_STATUS_OPEN, trade.Status)

	// Buyer accepts and deposits the asset → atomic swap executes immediately.
	_, err = f.keeper.AcceptEscrow(f.ctx, &types.MsgAcceptEscrow{
		Buyer:       buyer,
		EscrowId:    created.EscrowId,
		AmountAsset: assetAmt,
	})
	require.NoError(t, err)

	trade, _ = f.keeper.GetEscrow(f.ctx, created.EscrowId)
	require.Equal(t, types.EscrowStatus_ESCROW_STATUS_COMPLETED, trade.Status)
	require.Equal(t, buyer, trade.Buyer)

	// Asset went to the seller, LUG went to the buyer.
	require.Equal(t, assetAmt, f.bank.SpendableCoins(f.ctx, sellerAddr).AmountOf("usdt"))
	require.Equal(t, lugAmt, f.bank.SpendableCoins(f.ctx, buyerAddr).AmountOf("uluggom"))
	// No manual confirmation needed — both sides settled.
	require.True(t, f.bank.SpendableCoins(f.ctx, sellerAddr).AmountOf("uluggom").IsZero())
	require.True(t, f.bank.SpendableCoins(f.ctx, buyerAddr).AmountOf("usdt").IsZero())
}

func TestEscrow_AcceptWrongAmountRejected(t *testing.T) {
	f := initPoolFixture(t)
	seller, sellerAddr := f.addr(t, 1)
	buyer, buyerAddr := f.addr(t, 2)

	lugAmt := lug(t, "10000000000000000000")
	assetAmt := lug(t, "500000000000000000")
	f.bank.fund(sellerAddr, sdk.NewCoins(sdk.NewCoin("uluggom", lugAmt)))
	f.bank.fund(buyerAddr, sdk.NewCoins(sdk.NewCoin("usdt", lug(t, "1000000000000000000"))))

	created, err := f.keeper.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Seller: seller, AmountLug: lugAmt, Asset: "usdt", AmountAsset: assetAmt,
	})
	require.NoError(t, err)

	// Deposit less than agreed → rejected, trade stays OPEN.
	_, err = f.keeper.AcceptEscrow(f.ctx, &types.MsgAcceptEscrow{
		Buyer: buyer, EscrowId: created.EscrowId, AmountAsset: lug(t, "100000000000000000"),
	})
	require.Error(t, err)

	trade, _ := f.keeper.GetEscrow(f.ctx, created.EscrowId)
	require.Equal(t, types.EscrowStatus_ESCROW_STATUS_OPEN, trade.Status)
}

func TestEscrow_SellerCancelsOpen(t *testing.T) {
	f := initPoolFixture(t)
	seller, sellerAddr := f.addr(t, 1)

	lugAmt := lug(t, "10000000000000000000")
	f.bank.fund(sellerAddr, sdk.NewCoins(sdk.NewCoin("uluggom", lugAmt)))

	created, err := f.keeper.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Seller: seller, AmountLug: lugAmt, Asset: "usdt", AmountAsset: lug(t, "100000000000000000"),
	})
	require.NoError(t, err)

	// Another party cannot cancel an open escrow.
	_, err = f.keeper.CancelEscrow(f.ctx, &types.MsgCancelEscrow{Canceller: "luggom1other", EscrowId: created.EscrowId})
	require.Error(t, err)

	// Seller cancels → LUG returned.
	_, err = f.keeper.CancelEscrow(f.ctx, &types.MsgCancelEscrow{Canceller: seller, EscrowId: created.EscrowId})
	require.NoError(t, err)
	trade, _ := f.keeper.GetEscrow(f.ctx, created.EscrowId)
	require.Equal(t, types.EscrowStatus_ESCROW_STATUS_CANCELLED, trade.Status)
	require.Equal(t, lugAmt, f.bank.SpendableCoins(f.ctx, sellerAddr).AmountOf("uluggom"))
}

func TestEscrow_TimeoutReversesOpen(t *testing.T) {
	f := initPoolFixture(t)
	seller, sellerAddr := f.addr(t, 1)

	lugAmt := lug(t, "10000000000000000000")
	f.bank.fund(sellerAddr, sdk.NewCoins(sdk.NewCoin("uluggom", lugAmt)))

	created, err := f.keeper.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Seller: seller, AmountLug: lugAmt, Asset: "usdt", AmountAsset: lug(t, "100000000000000000"),
		TimeoutMinutes: 1,
	})
	require.NoError(t, err)

	// Advance the block time past the 1-minute expiry.
	sdkCtx := sdk.UnwrapSDKContext(f.ctx)
	sdkCtx = sdkCtx.WithBlockTime(sdkCtx.BlockTime().Add(2 * 60 * 1e9)) // +2 min (nanoseconds)
	f.ctx = sdkCtx

	require.NoError(t, f.keeper.ExpireStaleEscrows(f.ctx))

	trade, _ := f.keeper.GetEscrow(f.ctx, created.EscrowId)
	require.Equal(t, types.EscrowStatus_ESCROW_STATUS_CANCELLED, trade.Status)
	require.Equal(t, lugAmt, f.bank.SpendableCoins(f.ctx, sellerAddr).AmountOf("uluggom"))
}

func TestEscrow_Validations(t *testing.T) {
	f := initPoolFixture(t)
	seller, sellerAddr := f.addr(t, 1)

	lugAmt := lug(t, "10000000000000000000")
	f.bank.fund(sellerAddr, sdk.NewCoins(sdk.NewCoin("uluggom", lugAmt)))

	// Non-escrow assets rejected (BTC/XMR are pool-only, §13.80; eth not supported).
	for _, asset := range []string{"btc", "xmr", "eth"} {
		_, err := f.keeper.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
			Seller: seller, AmountLug: lugAmt, Asset: asset, AmountAsset: lug(t, "100000000000000000"),
		})
		require.Error(t, err, "asset %q should be rejected", asset)
	}

	// Self-accept → rejected.
	created, err := f.keeper.CreateEscrow(f.ctx, &types.MsgCreateEscrow{
		Seller: seller, AmountLug: lugAmt, Asset: "usdt", AmountAsset: lug(t, "100000000000000000"),
	})
	require.NoError(t, err)
	_, err = f.keeper.AcceptEscrow(f.ctx, &types.MsgAcceptEscrow{
		Buyer: seller, EscrowId: created.EscrowId, AmountAsset: lug(t, "100000000000000000"),
	})
	require.Error(t, err)
}
