package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"
)

const testFixingDay = "2026-10-22"

func dec(s string) sdkmath.LegacyDec { return sdkmath.LegacyMustNewDecFromStr(s) }

func rates(chf, eur, gbp, brl, cad string) Rates {
	return Rates{
		Date:   testFixingDay,
		ChfUsd: dec(chf),
		EurUsd: dec(eur),
		GbpUsd: dec(gbp),
		BrlUsd: dec(brl),
		CadUsd: dec(cad),
	}
}

func TestMedian(t *testing.T) {
	// ímpar ⇒ o do meio
	require.True(t, median([]sdkmath.LegacyDec{dec("3"), dec("1"), dec("2")}).Equal(dec("2")))
	// par ⇒ média dos dois do meio (é o caso "1 fonte caiu")
	require.True(t, median([]sdkmath.LegacyDec{dec("2"), dec("1")}).Equal(dec("1.5")))
	// uma só
	require.True(t, median([]sdkmath.LegacyDec{dec("2")}).Equal(dec("2")))
}

func TestAggregate(t *testing.T) {
	three := []Rates{
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
		rates("1.12", "1.19", "1.38", "5.06", "1.41"),
		rates("1.11", "1.20", "1.39", "5.03", "1.42"),
	}
	got, err := Aggregate(three, 2, dec("0.05"))
	require.NoError(t, err)
	require.True(t, got.ChfUsd.Equal(dec("1.11")), "mediana CHF: %s", got.ChfUsd)
	require.True(t, got.EurUsd.Equal(dec("1.19")), "mediana EUR: %s", got.EurUsd)
	require.True(t, got.GbpUsd.Equal(dec("1.38")), "mediana GBP: %s", got.GbpUsd)
	require.True(t, got.BrlUsd.Equal(dec("5.03")), "mediana BRL: %s", got.BrlUsd)
	require.True(t, got.CadUsd.Equal(dec("1.41")), "mediana CAD: %s", got.CadUsd)
	require.Equal(t, testFixingDay, got.Date)

	// A fixing date do agregado é a MAIS ANTIGA: só se escreve o dia que todas
	// as fontes publicaram.
	lagging := []Rates{
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
	}
	lagging[1].Date = "2026-10-21"
	got, err = Aggregate(lagging, 2, dec("0.05"))
	require.NoError(t, err)
	require.Equal(t, "2026-10-21", got.Date)

	// 1 fonte caiu ⇒ mediana das outras 2 (média dos dois)
	two := []Rates{
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
		rates("1.12", "1.20", "1.39", "5.10", "1.42"),
	}
	got, err = Aggregate(two, 2, dec("0.05"))
	require.NoError(t, err)
	require.True(t, got.ChfUsd.Equal(dec("1.11")))
	require.True(t, got.EurUsd.Equal(dec("1.19")))
	require.True(t, got.GbpUsd.Equal(dec("1.38")))
	require.True(t, got.BrlUsd.Equal(dec("5.05")))

	// só UMA fonte ⇒ NÃO submete (mediana de 1 não é robusta)
	_, err = Aggregate(two[:1], 2, dec("0.05"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "NÃO submeter")

	// zero fontes ⇒ NÃO submete
	_, err = Aggregate(nil, 2, dec("0.05"))
	require.Error(t, err)

	// fonte sem fixing date ⇒ NÃO submete
	semData := []Rates{
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
	}
	semData[0].Date = ""
	_, err = Aggregate(semData, 2, dec("0.05"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "fixing date")

	// fontes em DESACORDO numa perna ⇒ NÃO submete
	discord := []Rates{
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
		rates("1.90", "1.18", "1.37", "5.00", "1.40"),
	}
	_, err = Aggregate(discord, 2, dec("0.05"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "desvio")

	// perna inválida (zero) ⇒ NÃO submete
	_, err = Aggregate([]Rates{
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
		rates("0", "1.18", "1.37", "5.00", "1.40"),
	}, 2, dec("0.05"))
	require.Error(t, err)

	// BRL a zero (fonte que não publicou) ⇒ NÃO submete
	_, err = Aggregate([]Rates{
		rates("1.10", "1.18", "1.37", "5.00", "1.40"),
		rates("1.10", "1.18", "1.37", "0", "1.40"),
	}, 2, dec("0.05"))
	require.Error(t, err)
}

func TestHTTPSourceDynamicPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": testFixingDay,
			"data": []any{
				map[string]any{"amount": "1.10"}, // chf (string)
				map[string]any{"amount": 1.18},   // eur (número)
				map[string]any{"amount": "1.37"}, // gbp
				map[string]any{"amount": "5.04"}, // brl
				map[string]any{"amount": "1.40"}, // cad
			},
		})
	}))
	defer srv.Close()

	s, err := newHTTPSource(sourceConfig{
		Name: "fake", URL: srv.URL,
		Paths: map[string]string{
			"date": "date",
			"chf":  "data.0.amount",
			"eur":  "data.1.amount",
			"gbp":  "data.2.amount",
			"brl":  "data.3.amount",
			"cad":  "data.4.amount",
		},
	})
	require.NoError(t, err)

	r, err := s.Rates(context.Background())
	require.NoError(t, err)
	require.Equal(t, testFixingDay, r.Date)
	require.True(t, r.ChfUsd.Equal(dec("1.10")))
	require.True(t, r.EurUsd.Equal(dec("1.18")))
	require.True(t, r.GbpUsd.Equal(dec("1.37")))
	require.True(t, r.BrlUsd.Equal(dec("5.04")))
	require.True(t, r.CadUsd.Equal(dec("1.40")))
}

// Uma fonte em baixo NÃO derruba o feeder: as restantes continuam e a mediana é
// das que responderam — é a regra "1 oráculo caiu" da decisão.
func TestFetchAll_IsolaFonteEmBaixo(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer bad.Close()

	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"date": testFixingDay,
			"chf":  "1.10", "eur": "1.18", "gbp": "1.37", "brl": "5.04", "cad": "1.40",
		})
	}))
	defer goodSrv.Close()

	paths := map[string]string{
		"date": "date", "chf": "chf", "eur": "eur", "gbp": "gbp", "brl": "brl", "cad": "cad",
	}
	badSrc, err := newHTTPSource(sourceConfig{Name: "bad", URL: bad.URL, Paths: paths})
	require.NoError(t, err)
	goodSrc, err := newHTTPSource(sourceConfig{Name: "good", URL: goodSrv.URL, Paths: paths})
	require.NoError(t, err)

	got := fetchAll(context.Background(), []Source{badSrc, goodSrc}, func(string, ...any) {})
	require.Len(t, got, 1, "só a fonte que respondeu deve contar")
	require.True(t, got[0].ChfUsd.Equal(dec("1.10")))
}

func TestLoadSources_FailClosed(t *testing.T) {
	_, err := loadSources(nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "fail-closed")

	// fonte sem caminhos ⇒ inválida à construção
	_, err = loadSources([]sourceConfig{{Name: "x", URL: "http://x"}})
	require.Error(t, err)

	// fonte sem a fixing date ⇒ inválida (não se escreve um dia sem data)
	_, err = loadSources([]sourceConfig{{
		Name: "x", URL: "http://x",
		Paths: map[string]string{"chf": "a", "eur": "b", "gbp": "c", "brl": "d", "cad": "e"},
	}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "date")
}

func TestReadSourcesFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.json")
	require.NoError(t, os.WriteFile(p,
		[]byte(`[{"name":"a","url":"http://a","paths":{"chf":"x","eur":"y","gbp":"z"}}]`), 0o600))

	cfgs, err := readSourcesFile(p)
	require.NoError(t, err)
	require.Len(t, cfgs, 1)
	require.Equal(t, "a", cfgs[0].Name)

	// sem ficheiro ⇒ erro (não há fontes por omissão)
	_, err = readSourcesFile("")
	require.Error(t, err)
}

// A-5 (hard test): piso rígido de 2 fontes. O feeder NÃO pode aceitar
// `-min-sources` abaixo de 2 — um oráculo de uma só fonte é manipulável.
func TestValidateMinSources_HardFloor(t *testing.T) {
	for _, n := range []int{-1, 0, 1} {
		require.Error(t, validateMinSources(n), "min-sources=%d devia ser RECUSADO", n)
	}
	for _, n := range []int{2, 3, 5} {
		require.NoError(t, validateMinSources(n), "min-sources=%d devia ser aceite", n)
	}
}
