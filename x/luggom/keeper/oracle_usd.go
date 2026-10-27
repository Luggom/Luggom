package keeper

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

var (
	// MVP 2.0 basket (§13.132, 2026-09-17; ritmo revisto no §13.173):
	// multiplicador 3 desde o ano 0, +10% a.a. composto, sem tecto. Substitui o 6
	// do §13.116 (que por sua vez substituiu o 7 do §13.52). HISTÓRICO:
	// 2 → 7 → 6 → 3 — a decisão do §13.130 (×5) nunca chegou ao código, por isso
	// este salto é 6 → 3; o ritmo de valorização subiu de +5% (§13.132) a +10%
	// (§13.173).
	// LUG = (EUR/USD + GBP/USD + CHF/USD) / 3 × multiplier.
	// É CONSTANTE COMPILADA (não é param) ⇒ a governança não a pode tocar; é
	// isto que torna a regra de venda do cofre (preço = cesta) imutável de facto.
	defaultCHFUSD = sdkmath.LegacyMustNewDecFromStr("1.10")
	defaultEURUSD = sdkmath.LegacyMustNewDecFromStr("1.18")
	defaultGBPUSD = sdkmath.LegacyMustNewDecFromStr("1.37")
	basketMult    = sdkmath.LegacyMustNewDecFromStr("3")

	// Guardrails (§13.48/§13.52): proportional floor 0.75 × multiplier (anti-crash sanity check).
	// maxUSDPerLUG removed (no cap — §13.11/§13.48/§13.51).

	// Per-leg FX sanity (quote currency USD per 1 unit CHF/EUR/GBP).
	minFxLegUSD = sdkmath.LegacyMustNewDecFromStr("0.05")
	maxFxLegUSD = sdkmath.LegacyMustNewDecFromStr("50.00")

	// Pernas cotadas em UNIDADES POR 1 USD (BRL, CAD): banda estreita. Inverter
	// uma delas (USD por BRL ≈ 0.20) tem de ser rejeitado, não aceite.
	minPerUsdLeg = sdkmath.LegacyMustNewDecFromStr("0.50")
	maxPerUsdLeg = sdkmath.LegacyMustNewDecFromStr("20.00")

	// Annual basket epoch: +10% compound every 365 days (§13.173).
	basketAnnualGrowth = sdkmath.LegacyMustNewDecFromStr("1.10")
	basketYearDuration = 365 * 24 * time.Hour
)

// O preço é resolvido por DIA (ver usdParityForMineUSDAt); a frescura do último
// snapshot deixou de ser o que decide a mineração — o que decide é existir o
// snapshot do dia do PAGAMENTO.

// basketMultiplierAt returns the current basket multiplier applying the +10% a.a. compound
// since the chain start (annual epoch, deterministic by timestamp).
// Immutable tokenomics (§13.173): starts at 3, grows 10% compound per year, no cap.
func (k Keeper) basketMultiplierAt(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.basketMultiplierAtTime(ctx, sdk.UnwrapSDKContext(ctx).BlockTime())
}

// basketMultiplierAtTime aplica o +10% a.a. composto NUM INSTANTE concreto.
// É isto que permite preçar a cunhagem na data do PAGAMENTO: o mesmo recibo dá
// sempre o mesmo LUG, independentemente de quando é submetido (sem opção grátis
// sobre o salto de aniversário).
func (k Keeper) basketMultiplierAtTime(ctx context.Context, at time.Time) (sdkmath.LegacyDec, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	// §13.170 Grupo D — a âncora do epoch é derivada (bloco 0), não um param.
	startTime := k.chainStartTime(sdkCtx)

	years := 0
	if at.After(startTime) {
		elapsed := at.Sub(startTime)
		years = int(math.Floor(float64(elapsed) / float64(basketYearDuration)))
	}

	// mult = 3 × 1.10^years (integer power, deterministic)
	mult := basketMult
	for i := 0; i < years; i++ {
		mult = mult.Mul(basketAnnualGrowth)
	}
	return mult, nil
}

// usdParityFromBasketRates computes LUG = (CHF+EUR+GBP)/3 × current multiplier.
// The floor is proportional: 0.75 × multiplier (§13.48/§13.52). No upper cap.
func (k Keeper) usdParityFromBasketRates(ctx context.Context, chfUSD, eurUSD, gbpUSD sdkmath.LegacyDec) (sdkmath.LegacyDec, error) {
	return k.usdParityFromBasketRatesAt(ctx, sdk.UnwrapSDKContext(ctx).BlockTime(), chfUSD, eurUSD, gbpUSD)
}

// usdParityFromBasketRatesAt computes the parity with the multiplier of `at`
// (the payment date for the mining path).
func (k Keeper) usdParityFromBasketRatesAt(ctx context.Context, at time.Time, chfUSD, eurUSD, gbpUSD sdkmath.LegacyDec) (sdkmath.LegacyDec, error) {
	if chfUSD.IsNil() || eurUSD.IsNil() || gbpUSD.IsNil() ||
		chfUSD.LTE(sdkmath.LegacyZeroDec()) || eurUSD.LTE(sdkmath.LegacyZeroDec()) || gbpUSD.LTE(sdkmath.LegacyZeroDec()) {
		return sdkmath.LegacyZeroDec(), fmt.Errorf("oracle fx legs must be positive")
	}
	mult, err := k.basketMultiplierAtTime(ctx, at)
	if err != nil {
		return sdkmath.LegacyZeroDec(), err
	}
	avg := chfUSD.Add(eurUSD).Add(gbpUSD).QuoInt64(3)
	parity := avg.Mul(mult)

	// Anti-crash sanity check: floor 0.75 × multiplier (tracks the multiplier).
	minParity := sdkmath.LegacyMustNewDecFromStr("0.75").Mul(mult)
	if parity.LT(minParity) {
		return sdkmath.LegacyZeroDec(), fmt.Errorf(
			"computed usd parity below floor: got=%s min=%s",
			parity.String(),
			minParity.String(),
		)
	}
	return parity, nil
}

// staticUSDParityFallback returns the C1 semi-static parity using current multiplier.
func (k Keeper) staticUSDParityFallback(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.usdParityFromBasketRates(ctx, defaultCHFUSD, defaultEURUSD, defaultGBPUSD)
}

// computeUSDParityFromBasket is the legacy C1 entrypoint (tests).
func (k Keeper) computeUSDParityFromBasket(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.staticUSDParityFallback(ctx)
}

// --- Wrappers exportados para testes (package keeper_test) ---

// ComputeUSDParityFromBasketForTest exposes computeUSDParityFromBasket to external tests.
func (k Keeper) ComputeUSDParityFromBasketForTest(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.computeUSDParityFromBasket(ctx)
}

// BasketMultiplierAtForTest exposes basketMultiplierAt to external tests.
func (k Keeper) BasketMultiplierAtForTest(ctx context.Context) (sdkmath.LegacyDec, error) {
	return k.basketMultiplierAt(ctx)
}

// UsdParityFromBasketRatesForTest exposes usdParityFromBasketRates to external tests.
func (k Keeper) UsdParityFromBasketRatesForTest(ctx context.Context, chfUSD, eurUSD, gbpUSD sdkmath.LegacyDec) (sdkmath.LegacyDec, error) {
	return k.usdParityFromBasketRates(ctx, chfUSD, eurUSD, gbpUSD)
}

func validateOracleFxLeg(label string, d sdkmath.LegacyDec) error {
	if d.IsNil() || d.LTE(sdkmath.LegacyZeroDec()) {
		return fmt.Errorf("%s must be positive", label)
	}
	if d.LT(minFxLegUSD) || d.GT(maxFxLegUSD) {
		return fmt.Errorf("%s out of bounds [%s,%s]", label, minFxLegUSD.String(), maxFxLegUSD.String())
	}
	return nil
}

// validateOraclePerUsdLeg valida as pernas cotadas em UNIDADES POR 1 USD (BRL,
// CAD). A banda é estreita de propósito: um valor invertido ou no fundo de uma
// banda larga (0.05) sobre-cunharia dezenas de vezes, e a chave do feeder é a
// única autorizada a escrever.
func validateOraclePerUsdLeg(label string, d sdkmath.LegacyDec) error {
	if d.IsNil() || d.LTE(sdkmath.LegacyZeroDec()) {
		return fmt.Errorf("%s must be positive", label)
	}
	if d.LT(minPerUsdLeg) || d.GT(maxPerUsdLeg) {
		return fmt.Errorf("%s out of bounds [%s,%s]", label, minPerUsdLeg.String(), maxPerUsdLeg.String())
	}
	return nil
}

// --- Preço na data do PAGAMENTO (revisão do Zealynx) ------------------------
//
// A cunhagem tem de depender do pagamento, não do momento da submissão. Sem
// isto, um recibo é uma opção gratuita durante 365 dias: espera-se pelo dia com
// a cesta mais baixa e mina-se antes do aniversário da chain, quando o
// multiplicador salta +10% (1/1.1 ⇒ ~9% menos LUG no bloco seguinte).
//
// O keeper guarda um snapshot por DIA DE FIXING (a data que a taxa é publicada
// para) e preça tudo na data atestada:
//   - taxas da cesta (CHF/EUR/GBP) da fixing em vigor;
//   - multiplicador calculado no instante do pagamento;
//   - taxas BRL/USD e CAD/USD da fixing em vigor.
//
// PTAX não publica fins de semana/feriados: a procura recua até 7 dias, para um
// pagamento de sábado usar a última taxa publicada (convenção PTAX).
//
// Um recibo datado D usa a última fixing publicada ANTES de D — nunca a de D.
// Como um dia só pode ser escrito durante esse mesmo dia (ver
// UpdateUsdOracleBasket) e nunca é reescrito, o conjunto de candidatos para D
// está congelado antes das 00:00 de D: nenhuma escrita tardia, backfill ou retry
// pode mudar o que um recibo já resolveu.

const (
	oracleDayLayout       = "2006-01-02"
	maxOracleLookbackDays = 7

	// Guarda contra uma escrita má — chave do updater comprometida ou bug do
	// feeder. A mediana das fontes protege de UMA fonte errada; não protege de
	// uma escrita errada que chega à chain. Como o dia é imutável e os recibos
	// seguintes resolvem para ele, o dano de uma fixing errada dura a janela
	// inteira (365 dias).
	//
	// O limite é por dia decorrido desde a fixing anterior (para tolerar um
	// feeder que esteve parado) e tem um tecto. Uma fixing que salte mais do que
	// isto é recusada: o dia fica vazio e a mineração recua para a anterior
	// (fail-closed), em vez de cunhar ao preço errado.
	maxFixingMovePerDay = "0.10" // 10% por dia
	maxFixingMoveTotal  = "0.50" // tecto: 50%
	maxFixingPrevSearch = 30     // dias para trás à procura da fixing anterior
)

var (
	maxFixingMovePerDayDec = sdkmath.LegacyMustNewDecFromStr(maxFixingMovePerDay)
	maxFixingMoveTotalDec  = sdkmath.LegacyMustNewDecFromStr(maxFixingMoveTotal)
)

// OracleDayKey devolve a chave canónica do dia UTC ("2026-10-22").
func OracleDayKey(unix int64) string {
	return time.Unix(unix, 0).UTC().Format(oracleDayLayout)
}

// UsdOracleSnapshotForPaymentDay devolve a fixing em vigor no início do dia do
// pagamento — a última publicada ESTRITAMENTE antes de D (convenção PTAX).
// Fail-closed: sem nenhuma fixing nos 7 dias anteriores, a mineração pára.
func (k Keeper) UsdOracleSnapshotForPaymentDay(ctx context.Context, paymentDate int64) (types.UsdOracleSnapshot, error) {
	base := time.Unix(paymentDate, 0).UTC()
	if base.Unix() <= 0 {
		return types.UsdOracleSnapshot{}, fmt.Errorf("payment_date is required")
	}

	for i := 1; i <= maxOracleLookbackDays; i++ {
		day := base.AddDate(0, 0, -i).Format(oracleDayLayout)
		snap, found, err := k.oracleSnapshotForDay(ctx, day)
		if err != nil {
			return types.UsdOracleSnapshot{}, err
		}
		if found {
			return snap, nil
		}
	}
	return types.UsdOracleSnapshot{}, fmt.Errorf(
		"no usd oracle snapshot for %s (or the %d days before): mining is PAUSED until the oracle has a fixing for that window",
		base.Format(oracleDayLayout), maxOracleLookbackDays)
}

// oracleSnapshotForDay resolve um dia de fixing. A tabela embutida é a verdade
// para a janela que cobre (é compilada, logo igual em todos os nós); o store só
// serve os dias posteriores a essa janela. Na primeira utilização de um dia da
// tabela, o valor fica registado on-chain, para auditoria.
func (k Keeper) oracleSnapshotForDay(ctx context.Context, day string) (types.UsdOracleSnapshot, bool, error) {
	snap, found, err := k.oracleFixingForDay(ctx, day)
	if err != nil || !found {
		return snap, found, err
	}
	if _, covered, err := oracleHistoryForDay(day); err != nil {
		return types.UsdOracleSnapshot{}, false, err
	} else if covered {
		has, err := k.UsdOracleDaily.Has(ctx, day)
		if err != nil {
			return types.UsdOracleSnapshot{}, false, err
		}
		if !has {
			if err := k.UsdOracleDaily.Set(ctx, day, snap); err != nil {
				return types.UsdOracleSnapshot{}, false, err
			}
		}
	}
	return snap, true, nil
}

// oracleFixingForDay é a versão READ-ONLY da resolução: tabela primeiro, depois
// o store. Não fixa nada — é usada pela guarda de movimento, que não deve
// escrever estado.
func (k Keeper) oracleFixingForDay(ctx context.Context, day string) (types.UsdOracleSnapshot, bool, error) {
	if snap, ok, err := oracleHistoryForDay(day); err != nil {
		return types.UsdOracleSnapshot{}, false, err
	} else if ok {
		return snap, true, nil
	}

	snap, err := k.UsdOracleDaily.Get(ctx, day)
	if err == nil {
		return snap, true, nil
	}
	if !errors.Is(err, collections.ErrNotFound) {
		return types.UsdOracleSnapshot{}, false, err
	}
	return types.UsdOracleSnapshot{}, false, nil
}

// oracleFixingBefore devolve a fixing mais recente ANTES de `day`, e o dia dela.
func (k Keeper) oracleFixingBefore(ctx context.Context, day string) (types.UsdOracleSnapshot, string, bool, error) {
	d, err := time.Parse(oracleDayLayout, day)
	if err != nil {
		return types.UsdOracleSnapshot{}, "", false, fmt.Errorf("invalid oracle day %q: %w", day, err)
	}
	for i := 1; i <= maxFixingPrevSearch; i++ {
		key := d.AddDate(0, 0, -i).Format(oracleDayLayout)
		snap, ok, err := k.oracleFixingForDay(ctx, key)
		if err != nil {
			return types.UsdOracleSnapshot{}, "", false, err
		}
		if ok {
			return snap, key, true, nil
		}
	}
	return types.UsdOracleSnapshot{}, "", false, nil
}

// lastWrittenFixing devolve a última fixing ESCRITA (dia + valores). É a âncora
// da guarda de movimento — ao contrário de oracleFixingBefore, não tem janela.
func (k Keeper) lastWrittenFixing(ctx context.Context) (types.UsdOracleSnapshot, string, bool, error) {
	day, err := k.LastFixingDay.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.UsdOracleSnapshot{}, "", false, nil
		}
		return types.UsdOracleSnapshot{}, "", false, err
	}
	snap, err := k.LastFixingSnapshot.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return types.UsdOracleSnapshot{}, "", false,
				fmt.Errorf("last fixing day is %s but the snapshot is missing: state is inconsistent", day)
		}
		return types.UsdOracleSnapshot{}, "", false, err
	}
	return snap, day, true, nil
}

// checkFixingMove recusa uma fixing que salte mais do que o limite em relação à
// fixing anterior. Sem fixing anterior (primeira de sempre), não há comparação.
func (k Keeper) checkFixingMove(ctx context.Context, day string, next types.UsdOracleSnapshot) error {
	prev, prevDay, ok, err := k.lastWrittenFixing(ctx)
	if err != nil {
		return err
	}
	if !ok {
		// Primeira escrita depois do upgrade: o item ainda não existe. Recua-se
		// para a procura por dia (tabela → store), sem migração.
		prev, prevDay, ok, err = k.oracleFixingBefore(ctx, day)
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
	}

	d, err := time.Parse(oracleDayLayout, day)
	if err != nil {
		return fmt.Errorf("invalid oracle day %q: %w", day, err)
	}
	p, err := time.Parse(oracleDayLayout, prevDay)
	if err != nil {
		return fmt.Errorf("invalid previous oracle day %q: %w", prevDay, err)
	}
	days := int64(d.Sub(p).Hours() / 24)
	if days < 1 {
		days = 1
	}
	limit := maxFixingMovePerDayDec.MulInt64(days)
	if limit.GT(maxFixingMoveTotalDec) {
		limit = maxFixingMoveTotalDec
	}

	legs := []struct {
		name string
		prev sdkmath.LegacyDec
		next sdkmath.LegacyDec
	}{
		{"chf_usd", prev.ChfUsd, next.ChfUsd},
		{"eur_usd", prev.EurUsd, next.EurUsd},
		{"gbp_usd", prev.GbpUsd, next.GbpUsd},
		{"brl_usd", prev.BrlUsd, next.BrlUsd},
		{"cad_usd", prev.CadUsd, next.CadUsd},
	}
	for _, leg := range legs {
		if leg.prev.IsNil() || !leg.prev.IsPositive() || leg.next.IsNil() {
			continue
		}
		move := leg.next.Sub(leg.prev).Abs().Quo(leg.prev)
		if move.GT(limit) {
			return fmt.Errorf(
				"%s would move %s%% from the fixing of %s (%s), limit is %s%% over %d day(s): refusing to write a fixing this far from the previous one",
				leg.name,
				move.MulInt64(100).TruncateInt().String(),
				prevDay,
				leg.prev.String(),
				limit.MulInt64(100).TruncateInt().String(),
				days,
			)
		}
	}
	return nil
}

// usdParityFromBasketRatesAt is called by the mining path with the snapshot of
// the fixing in force on the payment day (see `UsdOracleSnapshotForPaymentDay`).
