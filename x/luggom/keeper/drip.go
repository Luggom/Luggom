package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"luggom/x/luggom/types"
)

// Drip mensal dos reservatórios de poupança (§13.65 decisão 4 + §13.119).
//
//	O Staking Reservoir e o Atom Reservoir seguem a MESMA regra: acumulam a fatia
//	da mineração e liberam 1/12 de 10% do saldo por mês (~0,83%/mês), rateado entre
//	os locks ATIVOS proporcionalmente a `amount × multiplicador_da_tranche`
//	(90/180/365d = ×1/×2/×4).
//
//	Diferença: a camada ATOM só vai a locks cujo dono tenha ATOM travado ≥ mínimo
//	(§13.119) e só quando o IBC estiver aberto (`params.AtomBonusActive`). Antes
//	disso o Atom Reservoir apenas acumula.
//
//	Cap 25% a.a. COMBINADO (§13.119): base + ATOM ≤ 25% do principal por lock.
//	O excedente NÃO é pago e permanece no reservatório (o cofre auto-protege-se,
//	não é drenado) — §13.64.
const (
	// dripsPerYear is 12 (monthly).
	dripsPerYear = 12
	// dripAnnualFraction is 10% of the balance per year.
	dripAnnualFraction = "0.10"
	// yieldCapAnnual is the 25% a.a. combined cap (§13.43/§13.119).
	yieldCapAnnual = "0.25"
	// dripEpochDays is the monthly epoch duration (30 days).
	dripEpochDays = 30
)

// reservoirMonthlyDrip devolve 1/12 do drip anual (10%) do saldo do reservatório.
func (k Keeper) reservoirMonthlyDrip(ctx context.Context, reservoirModule string) sdkmath.Int {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	reservoirAddr := authtypes.NewModuleAddress(reservoirModule)
	balance := k.bankKeeper.SpendableCoins(sdkCtx, reservoirAddr).AmountOf("uluggom")
	if !balance.IsPositive() {
		return sdkmath.ZeroInt()
	}
	annual := sdkmath.LegacyMustNewDecFromStr(dripAnnualFraction).MulInt(balance)
	return annual.QuoInt64(dripsPerYear).TruncateInt()
}

// RunMonthlyDrip processa o drip mensal dos DOIS reservatórios se a época virou:
// Staking (sempre) + ATOM (só qualificados, só com o IBC aberto).
// Chamado no BeginBlock de cada bloco.
func (k Keeper) RunMonthlyDrip(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().UTC()
	currentEpoch := now.Format("200601")

	lastEpoch, err := k.LastDripEpoch.Get(sdkCtx)
	if err != nil {
		if !errors.Is(err, collections.ErrNotFound) {
			return err
		}
		// Primeira passagem: marca a época sem distribuir.
		return k.LastDripEpoch.Set(sdkCtx, currentEpoch)
	}
	if lastEpoch == currentEpoch {
		return nil // já processado este mês
	}

	stakingMonthly := k.reservoirMonthlyDrip(sdkCtx, types.StakingReservoirModuleName)

	// A camada ATOM dorme até o IBC abrir (§13.65 decisão 2).
	// §13.170 Grupo D — em vez do antigo flag `atom_bonus_active`, deriva-se do
	// MARCO: a camada ATOM só acorda quando a pool/IBC abre (como as fases).
	atomMonthly := sdkmath.ZeroInt()
	if k.PoolOpen(sdkCtx) {
		atomMonthly = k.reservoirMonthlyDrip(sdkCtx, types.AtomReservoirModuleName)
	}

	// Colhe os locks ativos, o peso de cada um e quem qualifica para a camada ATOM.
	type lockEntry struct {
		lock      types.SavingsLock
		weight    sdkmath.Int
		qualified bool
	}
	var entries []lockEntry
	totalWeight := sdkmath.ZeroInt()
	atomWeight := sdkmath.ZeroInt()
	err = k.SavingsLocks.Walk(sdkCtx, nil, func(_ uint64, lock types.SavingsLock) (bool, error) {
		if sdkCtx.BlockHeight() >= lock.UnlockHeight {
			return false, nil // vencido — tratado em ProcessMaturedLocks
		}
		mult := trancheMultiplier(lock.TrancheDays)
		weight := mult.MulInt(lock.Amount).TruncateInt()
		qualified := false
		if atomMonthly.IsPositive() {
			q, err := k.IsAtomQualified(sdkCtx, lock.Depositor)
			if err != nil {
				return true, err
			}
			qualified = q
		}
		entries = append(entries, lockEntry{lock: lock, weight: weight, qualified: qualified})
		totalWeight = totalWeight.Add(weight)
		if qualified {
			atomWeight = atomWeight.Add(weight)
		}
		return false, nil
	})
	if err != nil {
		return err
	}

	stakingPaid := sdkmath.ZeroInt()
	atomPaid := sdkmath.ZeroInt()
	capFraction := sdkmath.LegacyMustNewDecFromStr(yieldCapAnnual)

	if totalWeight.IsPositive() {
		for _, e := range entries {
			stakingShare := sdkmath.ZeroInt()
			if stakingMonthly.IsPositive() {
				stakingShare = stakingMonthly.Mul(e.weight).Quo(totalWeight)
			}
			atomShare := sdkmath.ZeroInt()
			if atomMonthly.IsPositive() && atomWeight.IsPositive() && e.qualified {
				atomShare = atomMonthly.Mul(e.weight).Quo(atomWeight)
			}

			// Cap 25% a.a. COMBINADO (§13.119): a base tem prioridade; o ATOM recebe
			// o que sobrar até ao teto. Sem cap individual por cofre.
			maxMonthly := capFraction.MulInt(e.lock.Amount).QuoInt64(dripsPerYear).TruncateInt()
			if stakingShare.GT(maxMonthly) {
				stakingShare = maxMonthly
			}
			remaining := maxMonthly.Sub(stakingShare)
			if atomShare.GT(remaining) {
				atomShare = remaining
			}

			lock := e.lock
			yield := lock.YieldAccumulated
			if yield.IsNil() {
				yield = sdkmath.ZeroInt()
			}
			lock.YieldAccumulated = yield.Add(stakingShare).Add(atomShare)
			// Guarda a origem do yield: um unlock antecipado devolve-o ao
			// reservatório de onde veio (nunca fica órfão no módulo — §13.119).
			stakingPrev := lock.StakingYieldAccumulated
			if stakingPrev.IsNil() {
				stakingPrev = sdkmath.ZeroInt()
			}
			lock.StakingYieldAccumulated = stakingPrev.Add(stakingShare)
			atomPrev := lock.AtomYieldAccumulated
			if atomPrev.IsNil() {
				atomPrev = sdkmath.ZeroInt()
			}
			lock.AtomYieldAccumulated = atomPrev.Add(atomShare)
			if err := k.SavingsLocks.Set(sdkCtx, lock.Id, lock); err != nil {
				return err
			}
			stakingPaid = stakingPaid.Add(stakingShare)
			atomPaid = atomPaid.Add(atomShare)
		}
	}

	// Move apenas o que foi efetivamente creditado (o excedente fica no
	// reservatório, que assim nunca é drenado — §13.64).
	if err := k.moveDripToEscrow(sdkCtx, types.StakingReservoirModuleName, stakingPaid); err != nil {
		return err
	}
	if err := k.moveDripToEscrow(sdkCtx, types.AtomReservoirModuleName, atomPaid); err != nil {
		return err
	}

	if err := k.LastDripEpoch.Set(sdkCtx, currentEpoch); err != nil {
		return err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"luggom_monthly_drip",
		sdk.NewAttribute("epoch", currentEpoch),
		sdk.NewAttribute("staking_released", stakingPaid.String()),
		sdk.NewAttribute("atom_released", atomPaid.String()),
		sdk.NewAttribute("active_locks", fmt.Sprintf("%d", len(entries))),
	))
	return nil
}

// moveDripToEscrow move o yield creditado do reservatório para o escrow do
// módulo (que o retém até ao vencimento do lock).
func (k Keeper) moveDripToEscrow(ctx context.Context, reservoirModule string, amount sdkmath.Int) error {
	if amount.IsNil() || !amount.IsPositive() {
		return nil
	}
	coins := sdk.NewCoins(sdk.NewCoin("uluggom", amount))
	if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, reservoirModule, types.ModuleName, coins); err != nil {
		return fmt.Errorf("failed to move drip (%s) to escrow: %w", reservoirModule, err)
	}
	return nil
}

// returnYieldToReservoir devolve ao reservatório de origem o yield creditado a um
// lock que saiu antecipadamente — sem isto o LUG ficaria órfão no escrow (§13.119).
func (k Keeper) returnYieldToReservoir(ctx context.Context, reservoirModule string, amount sdkmath.Int) error {
	if amount.IsNil() || !amount.IsPositive() {
		return nil
	}
	coins := sdk.NewCoins(sdk.NewCoin("uluggom", amount))
	if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, reservoirModule, coins); err != nil {
		return fmt.Errorf("failed to return yield (%s) to reservoir: %w", reservoirModule, err)
	}
	return nil
}
