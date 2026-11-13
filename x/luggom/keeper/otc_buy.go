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

// O IBC voucher do BTC usa satoshis (8 casas). O denom real vem de
// params.otc_btc_denom (ex.: ibc/<HASH>).
const otcBtcBaseUnits = int64(100_000_000)

// BuyOtc vende LUG do cofre OTC ao preço da cesta do dia. O comprador paga o
// equivalente em BTC (IBC) e recebe o LUG; o BTC fica no cofre como lastro
// permanente. Falha fechada se faltar o preço da cesta, o BTC/USD ou o LUG.
func (k Keeper) BuyOtc(ctx context.Context, msg *types.MsgBuyOtc) (*types.MsgBuyOtcResponse, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get params: %w", err)
	}
	if params.OtcBtcDenom == "" {
		return nil, types.ErrOtcPriceUnavailable.Wrap("otc_btc_denom is not configured")
	}
	if msg.BtcDenom != params.OtcBtcDenom {
		return nil, types.ErrOtcAssetNotAllowed.Wrapf("%q (expected %q)", msg.BtcDenom, params.OtcBtcDenom)
	}
	if err := types.ValidateOtcLot(msg.LugAmount); err != nil {
		return nil, err
	}

	vault, err := k.OtcVault.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, types.ErrOtcVaultInsufficientLug.Wrap("vault is empty")
		}
		return nil, err
	}
	if vault.AccumulatedLug.IsNil() || vault.AccumulatedLug.LT(msg.LugAmount) {
		return nil, types.ErrOtcVaultInsufficientLug.Wrapf("vault has %s, need %s", vault.AccumulatedLug, msg.LugAmount)
	}

	snap, err := k.UsdOracleSnapshot.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return nil, types.ErrOtcPriceUnavailable.Wrap("no oracle snapshot")
		}
		return nil, err
	}
	if snap.BtcUsd.IsNil() || !snap.BtcUsd.IsPositive() {
		return nil, types.ErrOtcPriceUnavailable.Wrap("btc_usd is missing or not positive")
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	usdPerLug, err := k.usdParityFromBasketRatesAt(ctx, sdkCtx.BlockTime(), snap.ChfUsd, snap.EurUsd, snap.GbpUsd)
	if err != nil {
		return nil, types.ErrOtcPriceUnavailable.Wrap(err.Error())
	}

	// LUG (18 casas) → USD → BTC → satoshis (8 casas).
	lugDec := sdkmath.LegacyNewDecFromInt(msg.LugAmount).QuoInt64(1_000_000_000_000_000_000)
	btcAmount := lugDec.Mul(usdPerLug).Quo(snap.BtcUsd).MulInt64(otcBtcBaseUnits).TruncateInt()
	if !btcAmount.IsPositive() {
		return nil, types.ErrOtcBtcAmountZero.Wrapf("LUG %s at basket %s / btc_usd %s",
			msg.LugAmount, usdPerLug, snap.BtcUsd)
	}

	buyerAddr, err := k.addressCodec.StringToBytes(msg.Buyer)
	if err != nil {
		return nil, fmt.Errorf("invalid buyer: %w", err)
	}

	// O BTC entra no cofre; o LUG sai do cofre para o comprador. As duas
	// transferências partilham o cache do SDK: se a segunda falhar, a primeira
	// reverte com a tx.
	btcCoins := sdk.NewCoins(sdk.NewCoin(msg.BtcDenom, btcAmount))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(ctx, buyerAddr, types.OtcVaultModuleName, btcCoins); err != nil {
		return nil, fmt.Errorf("failed to send BTC to the OTC vault: %w", err)
	}
	lugCoins := sdk.NewCoins(sdk.NewCoin("uluggom", msg.LugAmount))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.OtcVaultModuleName, buyerAddr, lugCoins); err != nil {
		return nil, fmt.Errorf("failed to send LUG to the buyer: %w", err)
	}

	vault.AccumulatedLug = vault.AccumulatedLug.Sub(msg.LugAmount)
	retained := false
	for i := range vault.RetainedAssets {
		if vault.RetainedAssets[i].Denom == msg.BtcDenom {
			if vault.RetainedAssets[i].Amount.IsNil() {
				vault.RetainedAssets[i].Amount = sdkmath.ZeroInt()
			}
			vault.RetainedAssets[i].Amount = vault.RetainedAssets[i].Amount.Add(btcAmount)
			retained = true
			break
		}
	}
	if !retained {
		vault.RetainedAssets = append(vault.RetainedAssets, types.OtcRetainedAsset{
			Denom:  msg.BtcDenom,
			Amount: btcAmount,
		})
	}
	if err := k.OtcVault.Set(ctx, vault); err != nil {
		return nil, fmt.Errorf("failed to update OTC vault: %w", err)
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent("otc_buy",
		sdk.NewAttribute("buyer", msg.Buyer),
		sdk.NewAttribute("lug_amount", msg.LugAmount.String()),
		sdk.NewAttribute("btc_denom", msg.BtcDenom),
		sdk.NewAttribute("btc_amount", btcAmount.String()),
	))

	return &types.MsgBuyOtcResponse{
		BtcAmount: btcAmount,
		LugAmount: msg.LugAmount,
	}, nil
}
