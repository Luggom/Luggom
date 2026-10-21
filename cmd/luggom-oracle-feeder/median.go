package main

import (
	"fmt"
	"sort"

	sdkmath "cosmossdk.io/math"
)

// Rates são as três pernas da cesta (cotação USD por 1 unidade da moeda).
type Rates struct {
	ChfUsd sdkmath.LegacyDec
	EurUsd sdkmath.LegacyDec
	GbpUsd sdkmath.LegacyDec
}

func (r Rates) String() string {
	return fmt.Sprintf("CHF/USD=%s EUR/USD=%s GBP/USD=%s", r.ChfUsd, r.EurUsd, r.GbpUsd)
}

// median devolve a mediana de uma lista NÃO vazia: ímpar ⇒ o elemento do meio;
// par ⇒ a média dos dois do meio. É exactamente o que a decisão pede no caso
// "1 fonte caiu ⇒ mediana das outras 2" (§13.28).
func median(vals []sdkmath.LegacyDec) sdkmath.LegacyDec {
	s := make([]sdkmath.LegacyDec, len(vals))
	copy(s, vals)
	sort.Slice(s, func(i, j int) bool { return s[i].LT(s[j]) })

	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return s[n/2-1].Add(s[n/2]).QuoInt64(2)
}

// spread relativo de uma perna: (max − min) ÷ média. Devolve zero com <2 valores.
func spread(vals []sdkmath.LegacyDec) sdkmath.LegacyDec {
	if len(vals) < 2 {
		return sdkmath.LegacyZeroDec()
	}
	mn, mx := vals[0], vals[0]
	sum := sdkmath.LegacyZeroDec()
	for _, v := range vals {
		if v.LT(mn) {
			mn = v
		}
		if v.GT(mx) {
			mx = v
		}
		sum = sum.Add(v)
	}
	avg := sum.QuoInt64(int64(len(vals)))
	if !avg.IsPositive() {
		return sdkmath.LegacyZeroDec()
	}
	return mx.Sub(mn).Quo(avg)
}

// Aggregate aplica a REGRA DE ACEITAÇÃO das fontes (§13.28/§13.56):
//
//  1. responderem MENOS de `minSources` fontes ⇒ **erro** (não submeter: uma
//     leitura só não é robusta — a mediana de 1 é a própria leitura);
//  2. o desvio entre fontes numa perna exceder `maxSpread` ⇒ **erro** (não
//     submeter: as fontes não concordam, algo está errado);
//  3. caso contrário ⇒ **mediana por perna**.
//
// Fail-closed em todos os caminhos: o feeder prefere NÃO submeter a submeter
// dados em que não confia; a chain, sem snapshot fresco, pausa a mineração.
func Aggregate(rs []Rates, minSources int, maxSpread sdkmath.LegacyDec) (Rates, error) {
	if minSources < 1 {
		minSources = 1
	}
	if len(rs) < minSources {
		return Rates{}, fmt.Errorf(
			"apenas %d fonte(s) responderam (mínimo %d) — NÃO submeter", len(rs), minSources)
	}

	legs := []struct {
		nome string
		get  func(Rates) sdkmath.LegacyDec
	}{
		{"CHF/USD", func(r Rates) sdkmath.LegacyDec { return r.ChfUsd }},
		{"EUR/USD", func(r Rates) sdkmath.LegacyDec { return r.EurUsd }},
		{"GBP/USD", func(r Rates) sdkmath.LegacyDec { return r.GbpUsd }},
	}

	out := Rates{}
	for _, leg := range legs {
		vals := make([]sdkmath.LegacyDec, 0, len(rs))
		for _, r := range rs {
			v := leg.get(r)
			if v.IsNil() || !v.IsPositive() {
				return Rates{}, fmt.Errorf("perna %s inválida numa fonte (%s)", leg.nome, v)
			}
			vals = append(vals, v)
		}
		if sp := spread(vals); sp.GT(maxSpread) {
			return Rates{}, fmt.Errorf(
				"perna %s com desvio de %s entre fontes (máx %s) — NÃO submeter",
				leg.nome, sp.String(), maxSpread.String())
		}
		m := median(vals)
		switch leg.nome {
		case "CHF/USD":
			out.ChfUsd = m
		case "EUR/USD":
			out.EurUsd = m
		case "GBP/USD":
			out.GbpUsd = m
		}
	}
	return out, nil
}
