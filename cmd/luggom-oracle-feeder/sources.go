package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
)

// Source é uma fonte INDEPENDENTE de taxas FX. A decisão (§13.28) nomeia três
// (Chainlink + Band + Pyth), mas o feeder NÃO assume nenhuma em particular:
// recebe-as por CONFIGURAÇÃO, porque duas das três são **on-chain por natureza**
// (o Chainlink Data Feeds vive em EVM; a Band num chain próprio) e não têm um
// HTTP simples — precisam de um adapter por RPC, que ainda não existe.
type Source interface {
	Name() string
	// Rates devolve as três pernas, ou erro ("esta fonte caiu"). Um erro NÃO
	// aborta o feeder: as restantes continuam e a mediana é das que responderam.
	Rates(ctx context.Context) (Rates, error)
}

// sourceConfig é uma fonte declarada no ficheiro de configuração.
type sourceConfig struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	// Paths mapeia a perna ("chf" | "eur" | "gbp") para um caminho pontilhado no
	// JSON da resposta (ex.: "EUR_USD" ou "data.0.amount"). Assim o MESMO
	// httpSource serve qualquer API, sem um cliente escrito por fornecedor.
	Paths map[string]string `json:"paths"`
}

// httpSource lê uma fonte por HTTP e extrai as três pernas por caminho.
type httpSource struct {
	name   string
	url    string
	paths  map[string]string
	client *http.Client
}

func newHTTPSource(c sourceConfig) (*httpSource, error) {
	if c.Name == "" || c.URL == "" {
		return nil, fmt.Errorf("fonte sem nome ou url")
	}
	for _, leg := range []string{"chf", "eur", "gbp"} {
		if c.Paths[leg] == "" {
			return nil, fmt.Errorf("fonte %q sem caminho para a perna %q", c.Name, leg)
		}
	}
	return &httpSource{
		name:   c.Name,
		url:    c.URL,
		paths:  c.Paths,
		client: &http.Client{Timeout: 15 * time.Second},
	}, nil
}

func (s *httpSource) Name() string { return s.name }

func (s *httpSource) Rates(ctx context.Context) (Rates, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url, nil)
	if err != nil {
		return Rates{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return Rates{}, fmt.Errorf("%s: %w", s.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Rates{}, fmt.Errorf("%s: http %d", s.name, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Rates{}, fmt.Errorf("%s: %w", s.name, err)
	}
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return Rates{}, fmt.Errorf("%s: json: %w", s.name, err)
	}

	var out Rates
	for leg, path := range s.paths {
		raw, err := lookupDotPath(doc, path)
		if err != nil {
			return Rates{}, fmt.Errorf("%s: perna %s: %w", s.name, leg, err)
		}
		v, err := sdkmath.LegacyNewDecFromStr(raw)
		if err != nil {
			return Rates{}, fmt.Errorf("%s: perna %s: %q não é decimal", s.name, leg, raw)
		}
		switch leg {
		case "chf":
			out.ChfUsd = v
		case "eur":
			out.EurUsd = v
		case "gbp":
			out.GbpUsd = v
		}
	}
	return out, nil
}

// lookupDotPath resolve "a.b.0.c" num documento JSON já decodificado e devolve o
// valor como string (números e strings são aceites — as APIs variam).
func lookupDotPath(doc any, path string) (string, error) {
	cur := doc
	for _, part := range strings.Split(path, ".") {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[part]
			if !ok {
				return "", fmt.Errorf("caminho %q não existe", path)
			}
			cur = v
		case []any:
			i, err := strconv.Atoi(part)
			if err != nil || i < 0 || i >= len(node) {
				return "", fmt.Errorf("índice %q inválido em %q", part, path)
			}
			cur = node[i]
		default:
			return "", fmt.Errorf("caminho %q: %q não é objecto nem lista", path, part)
		}
	}
	switch v := cur.(type) {
	case string:
		return strings.TrimSpace(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case json.Number:
		return v.String(), nil
	default:
		return "", fmt.Errorf("caminho %q: tipo %T não é número nem string", path, cur)
	}
}

// loadSources constrói as fontes a partir da configuração.
func loadSources(cfgs []sourceConfig) ([]Source, error) {
	if len(cfgs) == 0 {
		return nil, fmt.Errorf(
			"nenhuma fonte configurada — fail-closed: sem fontes não se submete nada (usa -sources-file)")
	}
	out := make([]Source, 0, len(cfgs))
	for _, c := range cfgs {
		s, err := newHTTPSource(c)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// fetchAll consulta todas as fontes em série e devolve as que responderam. As
// que falharem são registadas e **descartadas** — é assim que a decisão trata
// "1 oráculo caiu": a mediana passa a ser das outras.
func fetchAll(ctx context.Context, srcs []Source, logf func(string, ...any)) []Rates {
	var out []Rates
	for _, s := range srcs {
		r, err := s.Rates(ctx)
		if err != nil {
			logf("fonte %s FALHOU: %v", s.Name(), err)
			continue
		}
		logf("fonte %s: %s", s.Name(), r.String())
		out = append(out, r)
	}
	return out
}
