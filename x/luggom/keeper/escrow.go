package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// FASE 6B — Escrow P2P on-chain (§13.76/§13.24, §13.80).
// Atomic two-sided escrow: the seller deposits LUG; the buyer deposits the
// requested asset; when BOTH are deposited, the swap executes automatically —
// the asset goes to the seller and the LUG to the buyer. No trust, no manual
// confirmation step. Timeout (default 60 min) → both sides reversed
// (never confiscates).
// Scope (§13.80): ONLY Cosmos-native denoms (atom/usdt/usdc — enter via IBC,
// 100% code). BTC and XMR are pool-only (external on-ramp via multisig 2-de-3).

// isEscrowAsset reports whether a denom is accepted in the P2P escrow
// (§13.80: only Cosmos-native denoms — atom/usdt/usdc).
func isEscrowAsset(denom string) bool {
	switch denom {
	case "atom", "usdt", "usdc":
		return true
	}
	return false
}

// CreateEscrow creates a P2P offer: the seller locks LUG in the escrow.
func (k Keeper) CreateEscrow(ctx context.Context, msg *types.MsgCreateEscrow) (*types.MsgCreateEscrowResponse, error) {
	if msg.AmountLug.IsNil() || !msg.AmountLug.IsPositive() {
		return nil, fmt.Errorf("amount_lug must be positive")
	}
	if msg.AmountAsset.IsNil() || !msg.AmountAsset.IsPositive() {
		return nil, fmt.Errorf("amount_asset must be positive")
	}
	if !isEscrowAsset(msg.Asset) {
		return nil, fmt.Errorf("invalid asset %q (escrow accepts atom/usdt/usdc only — BTC/XMR are pool-only, §13.80)", msg.Asset)
	}

	sellerAddr, err := sdk.AccAddressFromBech32(msg.Seller)
	if err != nil {
		return nil, fmt.Errorf("invalid seller: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	id, err := k.EscrowSeq.Next(sdkCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to allocate escrow id: %w", err)
	}

	timeout := msg.TimeoutMinutes
	if timeout == 0 {
		timeout = types.DefaultEscrowTimeoutMinutes
	}

	now := sdkCtx.BlockTime().Unix()
	trade := types.EscrowTrade{
		Id:          id,
		Seller:      msg.Seller,
		AmountLug:   msg.AmountLug,
		Asset:       msg.Asset,
		AmountAsset: msg.AmountAsset,
		Status:      types.EscrowStatus_ESCROW_STATUS_OPEN,
		CreatedAt:   now,
		ExpiresAt:   now + int64(timeout)*60,
	}

	// Lock the LUG from the seller into the module escrow.
	coins := sdk.NewCoins(sdk.NewCoin("uluggom", msg.AmountLug))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, sellerAddr, types.ModuleName, coins); err != nil {
		return nil, fmt.Errorf("failed to lock LUG: %w", err)
	}

	if err := k.EscrowTrades.Set(sdkCtx, id, trade); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_escrow_create",
		sdk.NewAttribute("escrow_id", fmt.Sprintf("%d", id)),
		sdk.NewAttribute("seller", msg.Seller),
		sdk.NewAttribute("amount_lug", msg.AmountLug.String()),
		sdk.NewAttribute("asset", msg.Asset),
		sdk.NewAttribute("amount_asset", msg.AmountAsset.String()),
		sdk.NewAttribute("expires_at", fmt.Sprintf("%d", trade.ExpiresAt)),
	))

	return &types.MsgCreateEscrowResponse{EscrowId: id}, nil
}

// AcceptEscrow accepts an offer and DEPOSITS the requested asset.
// When the deposit lands, the swap executes ATOMICALLY:
//   - the asset goes to the seller;
//   - the LUG goes to the buyer;
//   - the trade is COMPLETED.
// No manual confirmation — neither party can back out after both deposited.
func (k Keeper) AcceptEscrow(ctx context.Context, msg *types.MsgAcceptEscrow) (*types.MsgAcceptEscrowResponse, error) {
	buyerAddr, err := sdk.AccAddressFromBech32(msg.Buyer)
	if err != nil {
		return nil, fmt.Errorf("invalid buyer: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	trade, err := k.EscrowTrades.Get(sdkCtx, msg.EscrowId)
	if err != nil {
		return nil, fmt.Errorf("escrow not found: %w", err)
	}
	if trade.Status != types.EscrowStatus_ESCROW_STATUS_OPEN {
		return nil, fmt.Errorf("escrow %d is not open (status %s)", msg.EscrowId, trade.Status)
	}
	if trade.Seller == msg.Buyer {
		return nil, fmt.Errorf("seller cannot accept their own escrow")
	}
	if sdkCtx.BlockTime().Unix() > trade.ExpiresAt {
		return nil, fmt.Errorf("escrow %d expired — cancel and re-create", msg.EscrowId)
	}
	// The buyer must deposit exactly the agreed amount.
	if msg.AmountAsset.IsNil() || !msg.AmountAsset.Equal(trade.AmountAsset) {
		return nil, fmt.Errorf("amount_asset must equal the offer (%s)", trade.AmountAsset)
	}

	// Deposit the asset from the buyer into the module escrow.
	coinsAsset := sdk.NewCoins(sdk.NewCoin(trade.Asset, msg.AmountAsset))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, buyerAddr, types.ModuleName, coinsAsset); err != nil {
		return nil, fmt.Errorf("failed to deposit asset: %w", err)
	}

	// Atomic swap: asset → seller, LUG → buyer.
	sellerAddr, err := sdk.AccAddressFromBech32(trade.Seller)
	if err != nil {
		return nil, fmt.Errorf("invalid seller address: %w", err)
	}
	// Send the asset to the seller.
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sellerAddr, coinsAsset); err != nil {
		return nil, fmt.Errorf("failed to send asset to seller: %w", err)
	}
	// Send the LUG to the buyer.
	coinsLug := sdk.NewCoins(sdk.NewCoin("uluggom", trade.AmountLug))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, buyerAddr, coinsLug); err != nil {
		return nil, fmt.Errorf("failed to send LUG to buyer: %w", err)
	}

	trade.Buyer = msg.Buyer
	trade.Status = types.EscrowStatus_ESCROW_STATUS_COMPLETED
	if err := k.EscrowTrades.Set(sdkCtx, msg.EscrowId, trade); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_escrow_complete",
		sdk.NewAttribute("escrow_id", fmt.Sprintf("%d", msg.EscrowId)),
		sdk.NewAttribute("seller", trade.Seller),
		sdk.NewAttribute("buyer", msg.Buyer),
		sdk.NewAttribute("amount_lug", trade.AmountLug.String()),
		sdk.NewAttribute("asset", trade.Asset),
		sdk.NewAttribute("amount_asset", msg.AmountAsset.String()),
	))

	return &types.MsgAcceptEscrowResponse{}, nil
}

// CancelEscrow cancels a trade before completion:
// - OPEN (no buyer): the seller gets the LUG back immediately.
// - after timeout, either party cancels → both sides reversed
//   (LUG to the seller, asset back to the buyer).
// Note: a FUNDED (both deposited) trade is COMPLETED instantly — there is no
// "funded but pending" state to cancel; the timeout only applies to OPEN offers.
func (k Keeper) CancelEscrow(ctx context.Context, msg *types.MsgCancelEscrow) (*types.MsgCancelEscrowResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	trade, err := k.EscrowTrades.Get(sdkCtx, msg.EscrowId)
	if err != nil {
		return nil, fmt.Errorf("escrow not found: %w", err)
	}
	if trade.Status == types.EscrowStatus_ESCROW_STATUS_COMPLETED || trade.Status == types.EscrowStatus_ESCROW_STATUS_CANCELLED {
		return nil, fmt.Errorf("escrow %d already closed", msg.EscrowId)
	}

	// OPEN: only the seller can cancel.
	if trade.Status == types.EscrowStatus_ESCROW_STATUS_OPEN {
		if msg.Canceller != trade.Seller {
			return nil, fmt.Errorf("only the seller can cancel an open escrow")
		}
		return k.reverseEscrow(ctx, msg.EscrowId, trade, msg.Canceller)
	}

	return nil, fmt.Errorf("escrow %d is not cancellable", msg.EscrowId)
}

// reverseEscrow returns the LUG to the seller and marks the trade cancelled.
func (k Keeper) reverseEscrow(ctx context.Context, id uint64, trade types.EscrowTrade, canceller string) (*types.MsgCancelEscrowResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sellerAddr, err := sdk.AccAddressFromBech32(trade.Seller)
	if err != nil {
		return nil, fmt.Errorf("invalid seller address: %w", err)
	}

	coins := sdk.NewCoins(sdk.NewCoin("uluggom", trade.AmountLug))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, sellerAddr, coins); err != nil {
		return nil, fmt.Errorf("failed to return LUG: %w", err)
	}

	trade.Status = types.EscrowStatus_ESCROW_STATUS_CANCELLED
	if err := k.EscrowTrades.Set(ctx, id, trade); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_escrow_cancel",
		sdk.NewAttribute("escrow_id", fmt.Sprintf("%d", id)),
		sdk.NewAttribute("canceller", canceller),
		sdk.NewAttribute("amount_lug_returned", trade.AmountLug.String()),
	))

	return &types.MsgCancelEscrowResponse{}, nil
}

// GetEscrow returns a trade by id (queries/CLI).
func (k Keeper) GetEscrow(ctx context.Context, id uint64) (types.EscrowTrade, error) {
	return k.EscrowTrades.Get(ctx, id)
}

// GetEscrowsBySeller lists the trades of a seller.
func (k Keeper) GetEscrowsBySeller(ctx context.Context, seller string) ([]types.EscrowTrade, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	var trades []types.EscrowTrade
	err := k.EscrowTrades.Walk(sdkCtx, nil, func(key uint64, trade types.EscrowTrade) (bool, error) {
		if trade.Seller == seller {
			trades = append(trades, trade)
		}
		return false, nil
	})
	return trades, err
}

// ExpireStaleEscrows reverses expired OPEN offers that were never accepted.
// Called at the beginning of each block (BeginBlock) — automatic timeout protection.
func (k Keeper) ExpireStaleEscrows(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()

	var expired []types.EscrowTrade
	err := k.EscrowTrades.Walk(sdkCtx, nil, func(key uint64, trade types.EscrowTrade) (bool, error) {
		if trade.Status == types.EscrowStatus_ESCROW_STATUS_OPEN && now > trade.ExpiresAt {
			expired = append(expired, trade)
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	for _, trade := range expired {
		if _, err := k.reverseEscrow(ctx, trade.Id, trade, "timeout"); err != nil {
			return err
		}
	}
	return nil
}
