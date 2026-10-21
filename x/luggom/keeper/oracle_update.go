package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// UpdateUsdOracleBasket commits a new CHF/USD, EUR/USD, GBP/USD snapshot (Etapa C2).
func (k Keeper) UpdateUsdOracleBasket(ctx context.Context, msg *types.MsgUpdateUsdOracleBasket) (*types.MsgUpdateUsdOracleBasketResponse, error) {
	// §13.170 Grupo B — o oráculo dinâmico é uma constante de código (sempre
	// ligado); deixou de haver o flag que se podia desligar.
	params, err := k.Params.Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get params: %w", err)
	}
	signerAddr, err := sdk.AccAddressFromBech32(msg.Signer)
	if err != nil {
		return nil, fmt.Errorf("invalid signer: %w", err)
	}

	allowed := sdk.AccAddress(k.authority).Equals(signerAddr)
	if params.UsdOracleUpdaterAddress != "" {
		upd, err := sdk.AccAddressFromBech32(params.UsdOracleUpdaterAddress)
		if err != nil {
			return nil, fmt.Errorf("invalid usd_oracle_updater_address in params: %w", err)
		}
		if upd.Equals(signerAddr) {
			allowed = true
		}
	}
	if !allowed {
		return nil, fmt.Errorf("signer is not authorized to update USD oracle")
	}

	if err := validateOracleFxLeg("chf_usd", msg.ChfUsd); err != nil {
		return nil, err
	}
	if err := validateOracleFxLeg("eur_usd", msg.EurUsd); err != nil {
		return nil, err
	}
	if err := validateOracleFxLeg("gbp_usd", msg.GbpUsd); err != nil {
		return nil, err
	}
	if _, err := k.usdParityFromBasketRates(ctx, msg.ChfUsd, msg.EurUsd, msg.GbpUsd); err != nil {
		return nil, fmt.Errorf("basket parity guardrail: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	snap := types.UsdOracleSnapshot{
		ChfUsd:      msg.ChfUsd,
		EurUsd:      msg.EurUsd,
		GbpUsd:      msg.GbpUsd,
		BlockHeight: sdkCtx.BlockHeight(),
		UpdatedAt:   sdkCtx.BlockTime().Unix(),
	}
	if err := k.UsdOracleSnapshot.Set(ctx, snap); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_usd_oracle_update",
		sdk.NewAttribute("chf_usd", msg.ChfUsd.String()),
		sdk.NewAttribute("eur_usd", msg.EurUsd.String()),
		sdk.NewAttribute("gbp_usd", msg.GbpUsd.String()),
		sdk.NewAttribute("block_height", fmt.Sprintf("%d", snap.BlockHeight)),
	))

	return &types.MsgUpdateUsdOracleBasketResponse{}, nil
}
