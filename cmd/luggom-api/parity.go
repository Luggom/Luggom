package main

// Paridade da cesta (USD/LUG) e leitura do supply on-chain.
//
// O Farol (FPT — `fair_value = dívida dos EUA / supply × 1%`) foi ELIMINADO
// (§13.128 §7, "higiene do Farol"): o LUG é uma moeda de preço livre com
// referência ÚNICA na cesta, não uma promessa — e o aviso "X% abaixo do valor
// justo" saiu do APK. Com ele saíram o endpoint `/api/v1/fair-value` e o feeder
// da dívida americana (`api.fiscaldata.treasury.gov`).
//
// O que fica é a PARIDADE da cesta (custo de mineração), recalibrada no §13.132
// (multiplicador 3 — era 6 no §13.116).

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	sdkmath "cosmossdk.io/math"
)

// queryLugSupplyFromNode returns the on-chain uluggom supply (base units).
func (s *Server) queryLugSupplyFromNode() (sdkmath.Int, error) {
	args := []string{
		"query", "bank", "total",
		"-o", "json",
		"--node", s.cfg.Node,
		"--home", s.cfg.Home,
	}
	cmd := exec.Command(s.cfg.Bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return sdkmath.Int{}, fmt.Errorf("query bank total: %w — %s", err, strings.TrimSpace(string(out)))
	}
	var root map[string]interface{}
	if err := json.Unmarshal(out, &root); err != nil {
		return sdkmath.Int{}, fmt.Errorf("json root: %w", err)
	}
	supplyList, ok := root["supply"].([]interface{})
	if !ok {
		return sdkmath.Int{}, fmt.Errorf("missing supply array")
	}
	for _, item := range supplyList {
		coin, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		denom, _ := coin["denom"].(string)
		amountStr, _ := coin["amount"].(string)
		if denom != "uluggom" || amountStr == "" {
			continue
		}
		amount, ok := sdkmath.NewIntFromString(amountStr)
		if !ok {
			return sdkmath.Int{}, fmt.Errorf("bad supply amount %q", amountStr)
		}
		return amount, nil
	}
	return sdkmath.Int{}, fmt.Errorf("uluggom not in supply")
}

// basketParityUSDLug computes the mining basket parity (USD/LUG) with the same
// formula as the keeper (x/luggom/keeper/oracle_usd.go, §13.132):
//
//	LUG = (CHF/USD + EUR/USD + GBP/USD) / 3 × multiplicador(3, +5% a.a. composto)
//
// The dynamic oracle (MsgUpdateUsdOracleBasket, 1×/day feeder) overrides the FX
// legs on-chain; there is no public query for the committed snapshot, so this
// endpoint reports the deterministic static basket (the same fallback the
// keeper uses). The legacy `parity_mvp` param (12.34 BRL) is NOT used by the
// mining flow and is deliberately ignored here.
func basketParityUSDLug() float64 {
	const chfUSD = 1.10
	const eurUSD = 1.18
	const gbpUSD = 1.37
	// §13.132: multiplicador 3 (era 6 no §13.116).
	const mult = 3.0
	avg := (chfUSD + eurUSD + gbpUSD) / 3.0
	return avg * mult
}
