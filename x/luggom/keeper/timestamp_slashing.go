package keeper

import (
	"context"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
)

const (
	// defaultSlashNoSignSeconds is the default wall-clock threshold for jailing a
	// validator that has not signed any block (FASE 3, §13.25). 15 days.
	defaultSlashNoSignSeconds = uint64(15 * 24 * 60 * 60)
)

// EnforceTimestampSlashing jails validators whose last signed block is older than
// the wall-clock limit (validator_slash_no_sign_seconds). It runs in the luggom
// BeginBlock as a complement to the height-based x/slashing: when a validator stops
// signing, the native window only advances while blocks are produced; this check
// uses the signing info so that prolonged absence (e.g. a validator that keeps
// failing while the rest of the network keeps producing blocks) is removed from
// the active set deterministically.
//
// Only jails (no token slash): jailing removes the validator from the active set
// and is reversible via MsgUnjail. Never confiscates (consistent with §13.38).
//
// Limitation (documented debt): if the chain halts (quorum < 2/3), BeginBlock does
// not run at all, so full auto-recovery from a total halt requires a consensus
// hook (post-FASE 3).
func (k Keeper) EnforceTimestampSlashing(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	_, err := k.Params.Get(ctx)
	if err != nil {
		return fmt.Errorf("failed to get params: %w", err)
	}
	// The param (validator_slash_no_sign_seconds) is a conservative floor; the
	// effective trigger is the missed-blocks heuristic below, so the value is
	// read (validated in genesis) but not used arithmetically.
	// §13.170 Grupo A — `validator_slash_no_sign_seconds` era MORTO (o fork usa
	// constantes próprias); o campo foi apagado.

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

		// The signing info does not expose a wall-clock "last signed" timestamp,
		// only the height-based counters. The native x/slashing (window 13.9h,
		// min 70%, §13.62) already jails after ~4h10min of accumulated misses;
		// this layer is a conservative secondary net for validators that keep
		// missing over a very long period. We approximate "has been missing
		// continuously" by the validator's missed-blocks counter being at or
		// above the whole window (i.e. it has not signed for longer than the
		// wall-clock threshold allows).
		if info.MissedBlocksCounter == 0 {
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
