package types

import (
	sdkmath "cosmossdk.io/math"
)

// PRE-SALE ON-CHAIN (Bloco D, §13.132 §4).
//
// Venda de 3.500.000 LUG a $0,10 com vesting LINEAR de 3 meses. O comprador paga
// em `usdt` (denom nativo, 6 casas); o LUG fica em `luggom_presale` (que acumula
// os 35% da Fase 1) e sai à medida que veste.
//
// Preço, cap e duração são CONSTANTES COMPILADAS — tal como a cesta ×3 e o lote
// de 100k (§13.134 §3): um `const` não é param nenhum, logo fica fora
// do alcance da governança. É a mesma garantia do "o LUG só sai comprado à cesta".
const (
	// PreSaleUsdtDenom é o denom aceite na venda.
	PreSaleUsdtDenom = "usdt"

	// preSalePriceUSD é o preço unitário da venda (USD/LUG).
	preSalePriceUSD = "0.10"

	// preSaleVestingSeconds é a duração do vesting LINEAR: 3 meses ≈ 90 dias.
	preSaleVestingSeconds = int64(90 * 24 * 60 * 60)

	// preSaleMinUSDTBase é o mínimo por compra: 10 USDT (6 casas). Evita o spam
	// de posições-dust, que incharia a iteração do BeginBlock.
	preSaleMinUSDTBase = "10000000"

	// preSaleCapLugBase é o máximo vendável: 3.500.000 LUG (= 35% × 10M minerados).
	preSaleCapLugBase = "3500000000000000000000000"
)

var (
	preSalePrice   = sdkmath.LegacyMustNewDecFromStr(preSalePriceUSD)
	preSaleCapLug  = mustPreSaleInt(preSaleCapLugBase)
	preSaleMinUSDT = mustPreSaleInt(preSaleMinUSDTBase)

	// preSalePriceScale = 10^(18−6): converte reservas brutas usdt/lug em USD/LUG.
	preSalePriceScale = sdkmath.NewInt(1_000_000_000_000)
)

func mustPreSaleInt(s string) sdkmath.Int {
	v, ok := sdkmath.NewIntFromString(s)
	if !ok {
		panic("bad presale constant: " + s)
	}
	return v
}

// PreSalePriceUSD devolve o preço unitário da venda (USD/LUG).
func PreSalePriceUSD() sdkmath.LegacyDec { return preSalePrice }

// PreSaleCapLug devolve o máximo vendável, em uluggom.
func PreSaleCapLug() sdkmath.Int { return preSaleCapLug }

// PreSaleMinUSDT devolve o mínimo por compra, em unidades base de `usdt`.
func PreSaleMinUSDT() sdkmath.Int { return preSaleMinUSDT }

// PreSaleVestingSeconds devolve a duração do vesting linear.
func PreSaleVestingSeconds() int64 { return preSaleVestingSeconds }

// PreSaleLugForUSDT converte o pagamento em `usdt` (unidades base, 6 casas) no
// LUG comprado (uluggom, 18 casas), TRUNCANDO para baixo — o protocolo nunca dá
// mais do que o preço. Fórmula: lug = usdt × 10^12 ÷ preço.
func PreSaleLugForUSDT(usdt sdkmath.Int) sdkmath.Int {
	if usdt.IsNil() || !usdt.IsPositive() {
		return sdkmath.ZeroInt()
	}
	return sdkmath.LegacyNewDecFromInt(usdt).
		Mul(sdkmath.LegacyNewDecFromInt(preSalePriceScale)).
		Quo(preSalePrice).
		TruncateInt()
}
