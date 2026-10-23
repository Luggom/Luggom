package keeper

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	sdkmath "cosmossdk.io/math"

	"luggom/x/luggom/types"
)

// oracle_history.json is 366 days of published fixing rates, generated once by
// cmd/gen-oracle-history and embedded in the binary.
//
// Why embedded: the price of a receipt is a function of its payment date, and
// that must be identical on every node, forever. If the chain fetched rates at
// runtime two nodes could price the same receipt differently and fork. If it
// trusted only what the feeder wrote, a receipt dated before the upgrade would
// have no rate to resolve to.
//
// The table is authoritative for the days it covers. The feeder only covers the
// days after it.
//
//go:embed oracle_history.json
var oracleHistoryJSON []byte

type oracleHistoryEntry struct {
	Date   string `json:"date"`
	ChfUsd string `json:"chf_usd"`
	EurUsd string `json:"eur_usd"`
	GbpUsd string `json:"gbp_usd"`
	BrlUsd string `json:"brl_usd"`
	CadUsd string `json:"cad_usd"`
}

type oracleHistoryDoc struct {
	Start string               `json:"start"`
	End   string               `json:"end"`
	Days  []oracleHistoryEntry `json:"days"`
}

var (
	oracleHistoryOnce  sync.Once
	oracleHistoryDays  map[string]types.UsdOracleSnapshot
	oracleHistoryErr   error
	oracleHistoryStart string
	oracleHistoryEnd   string
)

// loadOracleHistory parseia a tabela UMA vez. Qualquer defeito é registado como
// erro (nunca um panic): um ficheiro mal formado tem de falhar alto e claro na
// primeira utilização, não desaparecer em silêncio nem derrubar um handler de
// consenso.
func loadOracleHistory() {
	var doc oracleHistoryDoc
	if err := json.Unmarshal(oracleHistoryJSON, &doc); err != nil {
		oracleHistoryErr = fmt.Errorf("oracle_history.json: %w", err)
		return
	}
	if len(doc.Days) == 0 {
		oracleHistoryErr = fmt.Errorf("oracle_history.json: no days")
		return
	}

	days := make(map[string]types.UsdOracleSnapshot, len(doc.Days))
	var prev time.Time
	for i, e := range doc.Days {
		day, err := time.Parse(oracleDayLayout, e.Date)
		if err != nil {
			oracleHistoryErr = fmt.Errorf("oracle_history.json: bad date %q: %w", e.Date, err)
			return
		}
		if i > 0 && !day.Equal(prev.AddDate(0, 0, 1)) {
			oracleHistoryErr = fmt.Errorf(
				"oracle_history.json: gap between %s and %s", prev.Format(oracleDayLayout), e.Date)
			return
		}
		snap, err := parseHistorySnapshot(e)
		if err != nil {
			oracleHistoryErr = fmt.Errorf("oracle_history.json: %s: %w", e.Date, err)
			return
		}
		days[e.Date] = snap
		prev = day
	}

	if doc.Start != doc.Days[0].Date || doc.End != doc.Days[len(doc.Days)-1].Date {
		oracleHistoryErr = fmt.Errorf(
			"oracle_history.json: start/end (%s..%s) do not match the days (%s..%s)",
			doc.Start, doc.End, doc.Days[0].Date, doc.Days[len(doc.Days)-1].Date)
		return
	}

	oracleHistoryDays = days
	oracleHistoryStart = doc.Start
	oracleHistoryEnd = doc.End
}

func parseHistorySnapshot(e oracleHistoryEntry) (types.UsdOracleSnapshot, error) {
	vals := map[string]string{
		"chf_usd": e.ChfUsd, "eur_usd": e.EurUsd, "gbp_usd": e.GbpUsd,
		"brl_usd": e.BrlUsd, "cad_usd": e.CadUsd,
	}
	dec := make(map[string]sdkmath.LegacyDec, len(vals))
	for name, raw := range vals {
		d, err := sdkmath.LegacyNewDecFromStr(raw)
		if err != nil {
			return types.UsdOracleSnapshot{}, fmt.Errorf("bad %s %q: %w", name, raw, err)
		}
		if !d.IsPositive() {
			return types.UsdOracleSnapshot{}, fmt.Errorf("%s must be positive, got %s", name, raw)
		}
		dec[name] = d
	}

	day, err := time.Parse(oracleDayLayout, e.Date)
	if err != nil {
		return types.UsdOracleSnapshot{}, err
	}

	return types.UsdOracleSnapshot{
		ChfUsd: dec["chf_usd"],
		EurUsd: dec["eur_usd"],
		GbpUsd: dec["gbp_usd"],
		BrlUsd: dec["brl_usd"],
		CadUsd: dec["cad_usd"],
		// A taxa foi conhecimento público até ao fim do seu dia de fixing. É isto
		// que a torna utilizável por um recibo datado do dia seguinte.
		UpdatedAt: day.AddDate(0, 0, 1).Unix() - 1,
	}, nil
}

// oracleHistoryForDay returns the embedded fixing of a day, if the table covers it.
func oracleHistoryForDay(day string) (types.UsdOracleSnapshot, bool, error) {
	oracleHistoryOnce.Do(loadOracleHistory)
	if oracleHistoryErr != nil {
		return types.UsdOracleSnapshot{}, false, oracleHistoryErr
	}
	snap, ok := oracleHistoryDays[day]
	return snap, ok, nil
}

// OracleHistoryWindowForTest exposes the covered window (inclusive).
func OracleHistoryWindowForTest() (string, string, error) {
	oracleHistoryOnce.Do(loadOracleHistory)
	if oracleHistoryErr != nil {
		return "", "", oracleHistoryErr
	}
	return oracleHistoryStart, oracleHistoryEnd, nil
}

// OracleHistoryDaysForTest expõe uma CÓPIA da tabela (o cache partilhado não
// pode ser mutado por quem chama).
func OracleHistoryDaysForTest() (map[string]types.UsdOracleSnapshot, error) {
	oracleHistoryOnce.Do(loadOracleHistory)
	if oracleHistoryErr != nil {
		return nil, oracleHistoryErr
	}
	out := make(map[string]types.UsdOracleSnapshot, len(oracleHistoryDays))
	for k, v := range oracleHistoryDays {
		out[k] = v
	}
	return out, nil
}
