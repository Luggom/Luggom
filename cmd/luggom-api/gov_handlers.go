package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"luggom/api/govmeta"
)

// Etapa H1: GET /api/v1/gov/luggom — agrega `query luggom params`, `query gov params` e política canónica (GOVERNANCE_MVP15).

func (s *Server) queryLuggomParamsRaw() ([]byte, error) {
	args := []string{
		"query", "luggom", "params",
		"-o", "json",
		"--node", s.cfg.Node,
		"--home", s.cfg.Home,
	}
	cmd := exec.Command(s.cfg.Bin, args...)
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func (s *Server) queryGovParamsRaw() ([]byte, error) {
	args := []string{
		"query", "gov", "params",
		"-o", "json",
		"--node", s.cfg.Node,
		"--home", s.cfg.Home,
	}
	cmd := exec.Command(s.cfg.Bin, args...)
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func parseJSONObject(raw []byte) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Server) handleGovLuggom(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var luggomMap map[string]interface{}
	var luggomErr string
	if raw, err := s.queryLuggomParamsRaw(); err != nil {
		luggomErr = fmt.Sprintf("%v — %s", err, strings.TrimSpace(string(raw)))
	} else if m, err := parseJSONObject(raw); err != nil {
		luggomErr = fmt.Sprintf("parse luggom params json: %v", err)
	} else {
		luggomMap = m
	}
	var cosmosGov map[string]interface{}
	var cosmosErr string
	if raw, err := s.queryGovParamsRaw(); err != nil {
		cosmosErr = fmt.Sprintf("%v — %s", err, strings.TrimSpace(string(raw)))
	} else if m, err := parseJSONObject(raw); err != nil {
		cosmosErr = fmt.Sprintf("parse gov params json: %v", err)
	} else {
		cosmosGov = m
	}
	summary := govmeta.BuildSummary(luggomMap, cosmosGov, cosmosErr)
	if luggomErr != "" {
		summary = summary.WithLuggomQueryError(luggomErr)
	}
	writeJSON(w, http.StatusOK, summary)
}
