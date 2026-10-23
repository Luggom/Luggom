// Command gen-oracle-history builds the embedded FX history used by the
// Luggom daily oracle (x/luggom/keeper/oracle_history.json).
//
// It is a DEVELOPMENT tool: it is run once, by hand, and its output is
// committed. The chain never fetches anything at runtime — two nodes with
// different rates would fork.
//
// Sources:
//   - Frankfurter (ECB reference rates) for CHF/EUR/GBP/CAD;
//   - BCB PTAX (Olinda API) for BRL — the closing rate, cotacaoVenda.
//
// Non-publication days (weekends, holidays) carry the last published rate
// forward, per leg, which is the PTAX convention.
//
// Usage:
//
//	go run ./cmd/gen-oracle-history -start 2025-10-22 -end 2026-10-22 \
//	    -out x/luggom/keeper/oracle_history.json
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"sort"
	"time"
)

const dayLayout = "2006-01-02"

type dayRates struct {
	Date   string `json:"date"`
	ChfUsd string `json:"chf_usd"`
	EurUsd string `json:"eur_usd"`
	GbpUsd string `json:"gbp_usd"`
	BrlUsd string `json:"brl_usd"`
	CadUsd string `json:"cad_usd"`
}

type historyFile struct {
	Start string     `json:"start"`
	End   string     `json:"end"`
	Days  []dayRates `json:"days"`
}

// quotes holds, per day, the raw upstream value (already in the convention the
// chain uses): USD per 1 unit for CHF/EUR/GBP, units per 1 USD for BRL/CAD.
type quotes map[string]map[string]*big.Rat

func main() {
	startS := flag.String("start", "", "first day to emit (YYYY-MM-DD, inclusive)")
	endS := flag.String("end", "", "last day to emit (YYYY-MM-DD, inclusive)")
	out := flag.String("out", "x/luggom/keeper/oracle_history.json", "output file")
	flag.Parse()

	start, err := time.Parse(dayLayout, *startS)
	must(err)
	end, err := time.Parse(dayLayout, *endS)
	must(err)
	if !end.After(start) {
		must(fmt.Errorf("end must be after start"))
	}

	// Fetch a wider window so the first emitted day has a carried-forward
	// value even when it is a weekend or a holiday.
	fetchFrom := start.AddDate(0, 0, -45)

	frank, err := fetchFrankfurter(fetchFrom, end)
	must(err)
	ptax, err := fetchPTAX(fetchFrom, end)
	must(err)

	legs := []string{"chf", "eur", "gbp", "cad", "brl"}
	carried := map[string]*big.Rat{}
	var days []dayRates

	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		key := d.Format(dayLayout)
		if q, ok := frank[key]; ok {
			for _, leg := range []string{"chf", "eur", "gbp", "cad"} {
				carried[leg] = q[leg]
			}
		}
		if q, ok := ptax[key]; ok {
			carried["brl"] = q["brl"]
		}
		for _, leg := range legs {
			if carried[leg] == nil {
				must(fmt.Errorf("no %s rate published on or before %s", leg, key))
			}
		}
		days = append(days, dayRates{
			Date:   key,
			ChfUsd: dec18(carried["chf"]),
			EurUsd: dec18(carried["eur"]),
			GbpUsd: dec18(carried["gbp"]),
			BrlUsd: dec18(carried["brl"]),
			CadUsd: dec18(carried["cad"]),
		})
	}

	sort.Slice(days, func(i, j int) bool { return days[i].Date < days[j].Date })

	buf, err := json.MarshalIndent(historyFile{
		Start: start.Format(dayLayout),
		End:   end.Format(dayLayout),
		Days:  days,
	}, "", " ")
	must(err)
	must(os.WriteFile(*out, append(buf, '\n'), 0o644))

	fmt.Printf("wrote %s: %d days (%s .. %s)\n", *out, len(days), days[0].Date, days[len(days)-1].Date)
}

// fetchFrankfurter returns, per published day, USD per 1 unit for CHF/EUR/GBP
// and units per 1 USD for CAD. Frankfurter quotes currencies per 1 USD, so the
// three European legs are inverted and CAD is not.
func fetchFrankfurter(from, to time.Time) (quotes, error) {
	url := fmt.Sprintf("https://api.frankfurter.dev/v1/%s..%s?base=USD&symbols=CHF,EUR,GBP,CAD",
		from.Format(dayLayout), to.Format(dayLayout))

	var doc struct {
		Rates map[string]map[string]json.Number `json:"rates"`
	}
	if err := getJSON(url, &doc); err != nil {
		return nil, fmt.Errorf("frankfurter: %w", err)
	}

	out := quotes{}
	for day, r := range doc.Rates {
		out[day] = map[string]*big.Rat{
			"chf": invert(r["CHF"]),
			"eur": invert(r["EUR"]),
			"gbp": invert(r["GBP"]),
			"cad": decimal(r["CAD"]),
		}
	}
	return out, nil
}

// fetchPTAX returns BRL per 1 USD (PTAX closing, cotacaoVenda) per business day.
func fetchPTAX(from, to time.Time) (quotes, error) {
	url := fmt.Sprintf(
		"https://olinda.bcb.gov.br/olinda/servico/PTAX/versao/v1/odata/CotacaoDolarPeriodo"+
			"(dataInicial=@dataInicial,dataFinalCotacao=@dataFinalCotacao)"+
			"?@dataInicial='%s'&@dataFinalCotacao='%s'&$top=10000&$format=json"+
			"&$select=cotacaoVenda,dataHoraCotacao",
		from.Format("01-02-2006"), to.Format("01-02-2006"))

	var doc struct {
		Value []struct {
			CotacaoVenda    json.Number `json:"cotacaoVenda"`
			DataHoraCotacao string      `json:"dataHoraCotacao"`
		} `json:"value"`
	}
	if err := getJSON(url, &doc); err != nil {
		return nil, fmt.Errorf("bcb ptax: %w", err)
	}

	out := quotes{}
	for _, row := range doc.Value {
		ts, err := time.Parse("2006-01-02 15:04:05.999999", row.DataHoraCotacao)
		if err != nil {
			return nil, fmt.Errorf("bcb ptax: bad dataHoraCotacao %q: %w", row.DataHoraCotacao, err)
		}
		out[ts.Format(dayLayout)] = map[string]*big.Rat{"brl": decimal(row.CotacaoVenda)}
	}
	return out, nil
}

func getJSON(url string, dst any) error {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func decimal(n json.Number) *big.Rat {
	r, ok := new(big.Rat).SetString(n.String())
	if !ok {
		panic(fmt.Sprintf("not a decimal: %q", n.String()))
	}
	return r
}

func invert(n json.Number) *big.Rat {
	return new(big.Rat).Inv(decimal(n))
}

// dec18 renders a rational with exactly 18 decimals, truncated (never rounded),
// which is the precision LegacyDec accepts.
func dec18(r *big.Rat) string {
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	scaled := new(big.Int).Mul(r.Num(), scale)
	scaled.Quo(scaled, r.Denom()) // truncates toward zero

	neg := scaled.Sign() < 0
	if neg {
		scaled.Neg(scaled)
	}
	s := scaled.String()
	if len(s) <= 18 {
		s = fmt.Sprintf("%019s", s)
	}
	out := s[:len(s)-18] + "." + s[len(s)-18:]
	if neg {
		out = "-" + out
	}
	return out
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
