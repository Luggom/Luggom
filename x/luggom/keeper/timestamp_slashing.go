package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
)

// EnforceTimestampSlashing é uma rede secundária conservadora por cima do
// x/slashing nativo: jaila o validador que falhou a janela INTEIRA
// (`signed_blocks_window`), ou seja que não assinou nada durante a janela.
//
// Só julga validadores **BONDED**. Um validador unbonding/unbonded não faz parte
// do conjunto activo e por isso nunca assina: julgar pelo contador nesse estado é
// um ciclo infinito, porque o `MsgUnjail` do SDK não repõe o contador e o bloco
// seguinte volta a julgar. Foi o que aconteceu a 2026-10-28 (§27): o primeiro
// bloco falhado prendia o validador para sempre.
//
// Only jails (no token slash): jailing removes the validator from the active set
// and is reversible via MsgUnjail. Never confiscates (consistent with §13.38).
//
// Limitation (documented debt): if the chain halts (quorum < 2/3), BeginBlock does
// not run at all, so full auto-recovery from a total halt requires a consensus
// hook (post-FASE 3).
func (k Keeper) EnforceTimestampSlashing(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	if _, err := k.Params.Get(ctx); err != nil {
		return fmt.Errorf("failed to get params: %w", err)
	}

	// O limiar é a janela do x/slashing: só se julga quem falhou a janela
	// inteira (nunca assinou), não quem falhou um bloco.
	window, err := k.slashingKeeper.SignedBlocksWindow(ctx)
	if err != nil {
		return fmt.Errorf("failed to get signed_blocks_window: %w", err)
	}
	if window <= 0 {
		return nil
	}

	now := sdkCtx.BlockTime()

	var jailed int
	err = k.slashingKeeper.IterateValidatorSigningInfos(ctx, func(consAddr sdk.ConsAddress, info slashingtypes.ValidatorSigningInfo) bool {
		// Already jailed (or currently serving a jail sentence): skip.
		if info.JailedUntil.After(now) {
			return false
		}

		validator, err := k.stakingKeeper.GetValidatorByConsAddr(ctx, consAddr)
		if err != nil {
			return false
		}
		if validator.IsJailed() {
			return false
		}
		// Só quem está no conjunto activo pode assinar; julgar os restantes é o
		// ciclo infinito descrito acima.
		if !validator.IsBonded() {
			return false
		}
		// Falhou a janela inteira? Um bloco falhado não é ausência.
		if info.MissedBlocksCounter < window {
			return false
		}

		if err := k.stakingKeeper.Jail(ctx, consAddr); err != nil {
			sdkCtx.Logger().Error("timestamp slashing failed to jail validator", "consensus_address", consAddr.String(), "err", err)
			return false
		}
		jailed++

		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			"luggom_timestamp_slash",
			sdk.NewAttribute("consensus_address", consAddr.String()),
			sdk.NewAttribute("missed_blocks", fmt.Sprintf("%d", info.MissedBlocksCounter)),
		))
		return false
	})
	if err != nil {
		return err
	}

	if jailed > 0 {
		sdkCtx.Logger().Info("timestamp slashing jailed validators", "count", jailed)
	}
	return nil
}
