package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// FASE 6A — Pool nativa (AMM, §13.76).
// Constant-product AMM (x·y=k) for LUG↔asset pairs (btc/atom/usdt/usdc/xmr — §13.24).
// - Swap fee: 0.3% stays in the pair (grows LP value).
// - Fixed 0.005 LUG fee per swap is charged by the ante-handler (ante_lug_move_fee.go).
// - Yield by imbalance (§13.19): deposits of scarce assets earn more, excess assets earn less.
// - Pool opens at 100M LUG mined (§13.55) — before that, AddLiquidity/Swap are rejected.
// - Treasury (0.5% from genesis — §13.86) is the first LP.

const (
	// poolSwapFeeBps is the 0.3% swap fee (30 bps).
	poolSwapFeeBps = int64(30)
	// poolTargetWeightBps is the target weight of each asset in a pair (50% LUG / 50% asset).
	poolTargetWeightBps = int64(5_000)
	// poolImbalanceYieldBps is the max yield boost for depositing a scarce asset (e.g. 2×).
	poolImbalanceYieldBps = int64(2_000)
)

// poolOpen returns true if the pool is open (mined LUG supply ≥ 50M, §13.81).
// Shared with the tokenomics trigger via Keeper.PoolOpen (single source of truth — §13.84).
func (k Keeper) poolOpen(ctx context.Context) bool {
	return k.PoolOpen(ctx)
}

// poolAssetDenoms returns the on-chain pool asset denoms (fake on testnet).
func poolAssetDenoms() []string {
	return []string{"btc", "atom", "usdt", "usdc", "xmr"}
}

// isPoolAsset reports whether denom is a pool asset.
func isPoolAsset(denom string) bool {
	for _, a := range poolAssetDenoms() {
		if a == denom {
			return true
		}
	}
	return false
}

// AddLiquidity deposits LUG + asset into a pair, minting LP shares.
func (k Keeper) AddLiquidity(ctx context.Context, msg *types.MsgAddLiquidity) (*types.MsgAddLiquidityResponse, error) {
	if !k.poolOpen(ctx) {
		return nil, fmt.Errorf("pool is not open yet (opens at 100M LUG mined)")
	}
	if !isPoolAsset(msg.Asset) {
		return nil, fmt.Errorf("invalid pool asset %q", msg.Asset)
	}
	if msg.AmountLug.IsNil() || !msg.AmountLug.IsPositive() || msg.AmountAsset.IsNil() || !msg.AmountAsset.IsPositive() {
		return nil, fmt.Errorf("amounts must be positive")
	}

	depositorAddr, err := sdk.AccAddressFromBech32(msg.Depositor)
	if err != nil {
		return nil, fmt.Errorf("invalid depositor: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	pair, err := k.PoolPairs.Get(sdkCtx, msg.Asset)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			return nil, err
		}
		pair = types.NewPoolPair(msg.Asset)
	}

	// First deposit sets the ratio; subsequent deposits follow it (constant product).
	lugDeposited := msg.AmountLug
	assetDeposited := msg.AmountAsset
	if pair.LugReserve.IsPositive() && pair.AssetReserve.IsPositive() {
		// Keep the pair ratio: adjust the asset to match the LUG deposited.
		expectedAsset := msg.AmountLug.Mul(pair.AssetReserve).Quo(pair.LugReserve)
		if expectedAsset.LT(msg.AmountAsset) {
			assetDeposited = expectedAsset
		} else {
			lugDeposited = msg.AmountAsset.Mul(pair.LugReserve).Quo(pair.AssetReserve)
		}
	}

	shares := types.LpSharesMinted(pair.LugReserve, pair.AssetReserve, pair.TotalLpShares, lugDeposited, assetDeposited)
	if shares.IsZero() {
		return nil, fmt.Errorf("lp shares would be zero")
	}
	if !msg.MinLpShares.IsNil() && msg.MinLpShares.IsPositive() && shares.LT(msg.MinLpShares) {
		return nil, fmt.Errorf("lp shares %s below minimum %s", shares, msg.MinLpShares)
	}

	// Move funds to the module (pool escrow).
	coinsLug := sdk.NewCoins(sdk.NewCoin("uluggom", lugDeposited))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, depositorAddr, types.ModuleName, coinsLug); err != nil {
		return nil, fmt.Errorf("failed to deposit LUG: %w", err)
	}
	coinsAsset := sdk.NewCoins(sdk.NewCoin(msg.Asset, assetDeposited))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, depositorAddr, types.ModuleName, coinsAsset); err != nil {
		return nil, fmt.Errorf("failed to deposit asset: %w", err)
	}

	pair.LugReserve = pair.LugReserve.Add(lugDeposited)
	pair.AssetReserve = pair.AssetReserve.Add(assetDeposited)
	pair.TotalLpShares = pair.TotalLpShares.Add(shares)
	if err := k.PoolPairs.Set(sdkCtx, msg.Asset, pair); err != nil {
		return nil, err
	}

	// Update or create the LP position.
	posKey := collections.Join(msg.Depositor, msg.Asset)
	pos, err := k.LpPositions.Get(sdkCtx, posKey)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			return nil, err
		}
		pos = types.LpPosition{Depositor: msg.Depositor, Asset: msg.Asset, Shares: sdkmath.ZeroInt()}
	}
	pos.Shares = pos.Shares.Add(shares)
	if err := k.LpPositions.Set(sdkCtx, posKey, pos); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_pool_add_liquidity",
		sdk.NewAttribute("depositor", msg.Depositor),
		sdk.NewAttribute("asset", msg.Asset),
		sdk.NewAttribute("lug_deposited", lugDeposited.String()),
		sdk.NewAttribute("asset_deposited", assetDeposited.String()),
		sdk.NewAttribute("lp_shares", shares.String()),
	))

	return &types.MsgAddLiquidityResponse{
		LpShares:       shares,
		LugDeposited:   lugDeposited,
		AssetDeposited: assetDeposited,
	}, nil
}

// RemoveLiquidity burns LP shares and returns the proportional share of the reserves.
func (k Keeper) RemoveLiquidity(ctx context.Context, msg *types.MsgRemoveLiquidity) (*types.MsgRemoveLiquidityResponse, error) {
	if !k.poolOpen(ctx) {
		return nil, fmt.Errorf("pool is not open")
	}
	if !isPoolAsset(msg.Asset) {
		return nil, fmt.Errorf("invalid pool asset %q", msg.Asset)
	}
	if msg.LpShares.IsNil() || !msg.LpShares.IsPositive() {
		return nil, fmt.Errorf("lp_shares must be positive")
	}

	depositorAddr, err := sdk.AccAddressFromBech32(msg.Depositor)
	if err != nil {
		return nil, fmt.Errorf("invalid depositor: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	posKey := collections.Join(msg.Depositor, msg.Asset)
	pos, err := k.LpPositions.Get(sdkCtx, posKey)
	if err != nil {
		return nil, fmt.Errorf("no LP position for %s/%s", msg.Depositor, msg.Asset)
	}
	if pos.Shares.LT(msg.LpShares) {
		return nil, fmt.Errorf("insufficient LP shares (have %s, want %s)", pos.Shares, msg.LpShares)
	}

	pair, err := k.PoolPairs.Get(sdkCtx, msg.Asset)
	if err != nil {
		return nil, fmt.Errorf("pair not found: %w", err)
	}
	if pair.TotalLpShares.IsZero() {
		return nil, fmt.Errorf("pair has no shares")
	}

	lugOut := pair.LugReserve.Mul(msg.LpShares).Quo(pair.TotalLpShares)
	assetOut := pair.AssetReserve.Mul(msg.LpShares).Quo(pair.TotalLpShares)
	if !msg.MinLugOut.IsNil() && msg.MinLugOut.IsPositive() && lugOut.LT(msg.MinLugOut) {
		return nil, fmt.Errorf("lug out %s below minimum %s", lugOut, msg.MinLugOut)
	}
	if !msg.MinAssetOut.IsNil() && msg.MinAssetOut.IsPositive() && assetOut.LT(msg.MinAssetOut) {
		return nil, fmt.Errorf("asset out %s below minimum %s", assetOut, msg.MinAssetOut)
	}

	// Return funds from the module to the LP.
	coinsLug := sdk.NewCoins(sdk.NewCoin("uluggom", lugOut))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, depositorAddr, coinsLug); err != nil {
		return nil, fmt.Errorf("failed to return LUG: %w", err)
	}
	coinsAsset := sdk.NewCoins(sdk.NewCoin(msg.Asset, assetOut))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, depositorAddr, coinsAsset); err != nil {
		return nil, fmt.Errorf("failed to return asset: %w", err)
	}

	pair.LugReserve = pair.LugReserve.Sub(lugOut)
	pair.AssetReserve = pair.AssetReserve.Sub(assetOut)
	pair.TotalLpShares = pair.TotalLpShares.Sub(msg.LpShares)
	if err := k.PoolPairs.Set(sdkCtx, msg.Asset, pair); err != nil {
		return nil, err
	}

	pos.Shares = pos.Shares.Sub(msg.LpShares)
	if pos.Shares.IsZero() {
		if err := k.LpPositions.Remove(sdkCtx, posKey); err != nil {
			return nil, err
		}
	} else {
		if err := k.LpPositions.Set(sdkCtx, posKey, pos); err != nil {
			return nil, err
		}
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_pool_remove_liquidity",
		sdk.NewAttribute("depositor", msg.Depositor),
		sdk.NewAttribute("asset", msg.Asset),
		sdk.NewAttribute("lp_shares", msg.LpShares.String()),
		sdk.NewAttribute("lug_out", lugOut.String()),
		sdk.NewAttribute("asset_out", assetOut.String()),
	))

	return &types.MsgRemoveLiquidityResponse{LugOut: lugOut, AssetOut: assetOut}, nil
}

// Swap swaps LUG ↔ asset using the constant-product curve with a 0.3% fee.
func (k Keeper) Swap(ctx context.Context, msg *types.MsgSwap) (*types.MsgSwapResponse, error) {
	if !k.poolOpen(ctx) {
		return nil, fmt.Errorf("pool is not open yet (opens at 100M LUG mined)")
	}
	if !isPoolAsset(msg.Asset) {
		return nil, fmt.Errorf("invalid pool asset %q", msg.Asset)
	}
	if msg.AmountIn.IsNil() || !msg.AmountIn.IsPositive() {
		return nil, fmt.Errorf("amount_in must be positive")
	}

	traderAddr, err := sdk.AccAddressFromBech32(msg.Trader)
	if err != nil {
		return nil, fmt.Errorf("invalid trader: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	pair, err := k.PoolPairs.Get(sdkCtx, msg.Asset)
	if err != nil {
		return nil, fmt.Errorf("pair not found (no liquidity yet): %w", err)
	}
	if pair.LugReserve.IsZero() || pair.AssetReserve.IsZero() {
		return nil, fmt.Errorf("pair has no liquidity")
	}

	// Apply the 0.3% fee: 0.3% of the input stays in the pair (grows LP value).
	fee := msg.AmountIn.MulRaw(poolSwapFeeBps).QuoRaw(10_000)
	amountInAfterFee := msg.AmountIn.Sub(fee)

	var amountOut sdkmath.Int
	if msg.BuyLug {
		// Sell asset → buy LUG: asset in, LUG out.
		amountOut = pair.LugReserve.Mul(amountInAfterFee).Quo(pair.AssetReserve.Add(amountInAfterFee))
		if amountOut.GT(pair.LugReserve) {
			return nil, fmt.Errorf("swap exceeds LUG reserve")
		}
	} else {
		// Sell LUG → buy asset: LUG in, asset out.
		amountOut = pair.AssetReserve.Mul(amountInAfterFee).Quo(pair.LugReserve.Add(amountInAfterFee))
		if amountOut.GT(pair.AssetReserve) {
			return nil, fmt.Errorf("swap exceeds asset reserve")
		}
	}
	if !msg.MinAmountOut.IsNil() && msg.MinAmountOut.IsPositive() && amountOut.LT(msg.MinAmountOut) {
		return nil, fmt.Errorf("amount out %s below minimum %s", amountOut, msg.MinAmountOut)
	}

	// Move the input to the module and the output to the trader.
	if msg.BuyLug {
		coinsIn := sdk.NewCoins(sdk.NewCoin(msg.Asset, msg.AmountIn))
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, traderAddr, types.ModuleName, coinsIn); err != nil {
			return nil, fmt.Errorf("failed to send asset: %w", err)
		}
		coinsOut := sdk.NewCoins(sdk.NewCoin("uluggom", amountOut))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, traderAddr, coinsOut); err != nil {
			return nil, fmt.Errorf("failed to send LUG: %w", err)
		}
		// Fee stays in the pair: the full input is added to the asset reserve.
		pair.AssetReserve = pair.AssetReserve.Add(msg.AmountIn)
		pair.LugReserve = pair.LugReserve.Sub(amountOut)
	} else {
		coinsIn := sdk.NewCoins(sdk.NewCoin("uluggom", msg.AmountIn))
		if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, traderAddr, types.ModuleName, coinsIn); err != nil {
			return nil, fmt.Errorf("failed to send LUG: %w", err)
		}
		coinsOut := sdk.NewCoins(sdk.NewCoin(msg.Asset, amountOut))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, traderAddr, coinsOut); err != nil {
			return nil, fmt.Errorf("failed to send asset: %w", err)
		}
		pair.LugReserve = pair.LugReserve.Add(msg.AmountIn)
		pair.AssetReserve = pair.AssetReserve.Sub(amountOut)
	}
	if err := k.PoolPairs.Set(sdkCtx, msg.Asset, pair); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_pool_swap",
		sdk.NewAttribute("trader", msg.Trader),
		sdk.NewAttribute("asset", msg.Asset),
		sdk.NewAttribute("buy_lug", fmt.Sprintf("%t", msg.BuyLug)),
		sdk.NewAttribute("amount_in", msg.AmountIn.String()),
		sdk.NewAttribute("amount_out", amountOut.String()),
		sdk.NewAttribute("fee", fee.String()),
	))

	return &types.MsgSwapResponse{AmountOut: amountOut}, nil
}

// PoolImbalanceYield computes the yield multiplier for depositing an asset into a pair
// based on its imbalance vs the 50/50 target (§13.19).
// Returns a multiplier: scarce asset → higher yield (up to 2×), excess → lower (down to 0.5×).
func (k Keeper) PoolImbalanceYield(ctx context.Context, asset string) (sdkmath.LegacyDec, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	pair, err := k.PoolPairs.Get(sdkCtx, asset)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return sdkmath.LegacyOneDec(), nil
		}
		return sdkmath.LegacyDec{}, err
	}
	if pair.LugReserve.IsZero() || pair.AssetReserve.IsZero() {
		return sdkmath.LegacyOneDec(), nil
	}

	total := pair.LugReserve.Add(pair.AssetReserve)
	assetWeight := pair.AssetReserve.MulRaw(10_000).Quo(total) // bps
	target := sdkmath.NewInt(poolTargetWeightBps)

	// Deviation from target (0 = balanced, + = excess, − = scarce).
	dev := assetWeight.Sub(target) // in bps
	// Yield multiplier: 1.0 at target; up to 2× when scarce, down to 0.5× when excess.
	// multiplier = 1 − dev/10000 × (maxBoost − 1) ... simplified linear:
	// scarce (dev<0): 1 + (−dev/5000) × 1.0  → up to 2×
	// excess (dev>0): 1 − (dev/5000) × 0.5   → down to 0.5×
	var mult sdkmath.LegacyDec
	if dev.IsNegative() {
		boost := dev.Neg().MulRaw(1_000).QuoRaw(5_000) // up to 1000 bps = 1.0
		mult = sdkmath.LegacyOneDec().Add(sdkmath.LegacyNewDecFromInt(boost).QuoInt64(10_000))
	} else {
		reduce := dev.MulRaw(500).QuoRaw(5_000) // up to 500 bps = 0.5
		mult = sdkmath.LegacyOneDec().Sub(sdkmath.LegacyNewDecFromInt(reduce).QuoInt64(10_000))
	}
	if mult.LT(sdkmath.LegacyMustNewDecFromStr("0.5")) {
		mult = sdkmath.LegacyMustNewDecFromStr("0.5")
	}
	if mult.GT(sdkmath.LegacyMustNewDecFromStr("2")) {
		mult = sdkmath.LegacyMustNewDecFromStr("2")
	}
	return mult, nil
}
