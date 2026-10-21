package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	sdkmath "cosmossdk.io/math"

	"luggom/api/uspipeline"
)

// preSaleThresholdUluggom é o fim da Fase 1: 10M LUG em uluggom (§13.117).
var preSaleThresholdUluggom = func() sdkmath.Int {
	v, ok := sdkmath.NewIntFromString("10000000000000000000000000")
	if !ok {
		panic("bad preSaleThresholdUluggom")
	}
	return v
}()

// parsePoolActiveFromQueryParamsJSON extrai pool_active e atom_bonus_active da saída JSON.
func parsePoolActiveFromQueryParamsJSON(raw []byte) (poolActive bool, atomBonusActive bool, err error) {
	var root map[string]interface{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return false, false, fmt.Errorf("json root: %w", err)
	}
	p, ok := root["params"].(map[string]interface{})
	if !ok {
		return false, false, fmt.Errorf("missing params object")
	}
	if v, ok := p["pool_active"].(bool); ok {
		poolActive = v
	} else if v, ok := p["poolActive"].(bool); ok {
		poolActive = v
	}
	if v, ok := p["atom_bonus_active"].(bool); ok {
		atomBonusActive = v
	} else if v, ok := p["atomBonusActive"].(bool); ok {
		atomBonusActive = v
	}
	return poolActive, atomBonusActive, nil
}

// queryPoolActiveFromNode reads pool_active from the on-chain params flag
// (§13.117 — a pool/IBC abre por decisão do fundador, NÃO por um marco de supply;
// o antigo gate de 50M está extinto). Corre `luggomd query luggom params` e lê
// params.pool_active.
func (s *Server) queryPoolActiveFromNode() (bool, error) {
	params, err := s.queryParamsFromNode()
	if err != nil {
		return false, err
	}
	if v, ok := params["pool_active"].(bool); ok {
		return v, nil
	}
	if v, ok := params["poolActive"].(bool); ok {
		return v, nil
	}
	return false, nil
}

// queryTokenomicsFromNode corre `luggomd query luggom params -o json` e devolve o snapshot G1.
func (s *Server) queryTokenomicsFromNode() (uspipeline.TokenomicsSnapshot, error) {
	args := []string{
		"query", "luggom", "params",
		"-o", "json",
		"--node", s.cfg.Node,
		"--home", s.cfg.Home,
	}
	cmd := exec.Command(s.cfg.Bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return uspipeline.TokenomicsSnapshot{}, fmt.Errorf("query params: %w — %s", err, strings.TrimSpace(string(out)))
	}
	_, atomBonusActive, err := parsePoolActiveFromQueryParamsJSON(out)
	if err != nil {
		return uspipeline.TokenomicsSnapshot{}, err
	}
	poolActive, err := s.queryPoolActiveFromNode()
	if err != nil {
		// Fallback: params flag (kept for genesis compat) if the node query fails.
		poolActive, _, _ = parsePoolActiveFromQueryParamsJSON(out)
	}
	// §13.132: a Fase 1 termina quando o supply total atinge 10M. A partir daí,
	// enquanto a pool estiver fechada, vigora a transição — minerador 30% +
	// staking 30% + ATOM 30% + fundador 5% + validadores 3% + gov 2% (sem
	// pre-sale e sem POOL).
	preSaleActive := false
	if supply, err := s.queryLugSupplyFromNode(); err == nil {
		preSaleActive = supply.LT(preSaleThresholdUluggom)
	}
	return uspipeline.BuildTokenomicsSnapshot(poolActive, preSaleActive, atomBonusActive), nil
}

// queryParamsFromNode runs `luggomd query luggom params -o json` and returns the raw
// params JSON object (map). FASE 5.2: used to read the on-chain bank_providers whitelist.
func (s *Server) queryParamsFromNode() (map[string]interface{}, error) {
	args := []string{
		"query", "luggom", "params",
		"-o", "json",
		"--node", s.cfg.Node,
		"--home", s.cfg.Home,
	}
	cmd := exec.Command(s.cfg.Bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("query params: %w — %s", err, strings.TrimSpace(string(out)))
	}
	var root map[string]interface{}
	if err := json.Unmarshal(out, &root); err != nil {
		return nil, fmt.Errorf("json root: %w", err)
	}
	p, ok := root["params"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("missing params object")
	}
	return p, nil
}

// onChainBankProviders reads the bank_providers whitelist from on-chain params.
// NÃO há fallback: uma falha de leitura devolve erro e os chamadores RECUSAM
// (fail-closed, A-4 do hard test) — a whitelist on-chain é a autoritativa.
func (s *Server) onChainBankProviders() (map[string]bool, error) {
	p, err := s.queryParamsFromNode()
	if err != nil {
		return nil, err
	}
	providers := map[string]bool{}
	switch v := p["bank_providers"].(type) {
	case []interface{}:
		for _, item := range v {
			if s, ok := item.(string); ok {
				providers[s] = true
			}
		}
	case []string:
		for _, s := range v {
			providers[s] = true
		}
	}
	return providers, nil
}
