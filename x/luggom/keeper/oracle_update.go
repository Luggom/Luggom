package keeper

import (
	"context"
	"fmt"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

// UpdateUsdOracleBasket commits the FX fixing of ONE day.
//
// The message carries the fixing date (`rate_date`), not the moment the keeper
// happened to write: a rate published for day D is stored under D, and D is
// written at most once. That is what makes the price of a receipt a function of
// its payment date alone — a late or backfilled write can never change what a
// receipt already resolved to.
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

	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// 1. A fixing date tem de ser explícita e só pode ser a de hoje (UTC).
	//    Nunca se escreve o passado (seria backdating) nem o futuro. O feeder
	//    corre depois das publicações do dia (PTAX ao fim da tarde, ECB às
	//    16:00 CET), por isso a fixing de hoje está disponível.
	day := msg.RateDate
	if _, err := time.Parse(oracleDayLayout, day); err != nil {
		return nil, fmt.Errorf("rate_date must be YYYY-MM-DD, got %q: %w", day, err)
	}
	if today := sdkCtx.BlockTime().UTC().Format(oracleDayLayout); day != today {
		return nil, fmt.Errorf("rate_date %s is not today's fixing (%s): only the day being published can be written", day, today)
	}

	// 2. Imutável: um dia escrito nunca é reescrito. Dias cobertos pela tabela
	//    embutida são imutáveis por construção — aceitar a escrita deixaria no
	//    store um valor que nunca preça nada (a tabela ganha na leitura) e faria
	//    o registo on-chain discordar do preço usado.
	has, err := k.UsdOracleDaily.Has(ctx, day)
	if err != nil {
		return nil, err
	}
	if has {
		return nil, types.ErrOracleDayAlreadyWritten.Wrapf("rate_date %s", day)
	}
	if _, covered, err := oracleHistoryForDay(day); err != nil {
		return nil, err
	} else if covered {
		return nil, types.ErrOracleDayAlreadyWritten.Wrapf(
			"rate_date %s is compiled into oracle_history.json", day)
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
	// BRL/CAD are quoted the other way round (units per 1 USD) and do not enter
	// the basket parity — they only convert a payment amount into USD. Os limites
	// são próprios: com a banda [0.05, 50] das pernas da cesta, um BRL invertido
	// (0.198) ou no fundo da banda (0.05) passava e sobre-cunhava até 100×.
	if err := validateOraclePerUsdLeg("brl_usd", msg.BrlUsd); err != nil {
		return nil, err
	}
	if err := validateOraclePerUsdLeg("cad_usd", msg.CadUsd); err != nil {
		return nil, err
	}

	if _, err := k.usdParityFromBasketRates(ctx, msg.ChfUsd, msg.EurUsd, msg.GbpUsd); err != nil {
		return nil, fmt.Errorf("basket parity guardrail: %w", err)
	}

	snap := types.UsdOracleSnapshot{
		ChfUsd:      msg.ChfUsd,
		EurUsd:      msg.EurUsd,
		GbpUsd:      msg.GbpUsd,
		BrlUsd:      msg.BrlUsd,
		CadUsd:      msg.CadUsd,
		BlockHeight: sdkCtx.BlockHeight(),
		UpdatedAt:   sdkCtx.BlockTime().Unix(),
	}
	// Último snapshot (leitura rápida/API). Deixou de ser lido pela mineração.
	if err := k.UsdOracleSnapshot.Set(ctx, snap); err != nil {
		return nil, err
	}
	// Histórico diário, chaveado pela FIXING DATE do msg.
	if err := k.UsdOracleDaily.Set(ctx, day, snap); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_usd_oracle_update",
		sdk.NewAttribute("rate_date", day),
		sdk.NewAttribute("chf_usd", msg.ChfUsd.String()),
		sdk.NewAttribute("eur_usd", msg.EurUsd.String()),
		sdk.NewAttribute("gbp_usd", msg.GbpUsd.String()),
		sdk.NewAttribute("brl_usd", msg.BrlUsd.String()),
		sdk.NewAttribute("cad_usd", msg.CadUsd.String()),
		sdk.NewAttribute("block_height", fmt.Sprintf("%d", snap.BlockHeight)),
	))

	return &types.MsgUpdateUsdOracleBasketResponse{}, nil
}
