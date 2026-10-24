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

func TestMoveLimit(t *testing.T) {
	require.True(t, moveLimit(1).Equal(dec("0.10")))
	require.True(t, moveLimit(3).Equal(dec("0.30")))
	require.True(t, moveLimit(10).Equal(dec("0.50")), "o tecto é 50%")
}

// O feeder tem de escrever o passo limitado na direcção do alvo — não deixar a
// chain sem escrita (o que congelaria a fixing errada e poria a mineração em
// pausa quando o caminho de volta excedesse o tecto).
func TestClampTowardsLimit(t *testing.T) {
	prev := fixingSample{
		ChfUsd: dec("1.0"), EurUsd: dec("1.0"), GbpUsd: dec("1.0"),
		BrlUsd: dec("5.0"), CadUsd: dec("1.4"),
	}

	// Alvo 10× abaixo: escreve 5.0 − 10% = 4.5.
	down := rates("1.0", "1.0", "1.0", "0.5", "1.4")
	out := clampTowardsLimit(prev, down, moveLimit(1), func(string, ...any) {})
	require.True(t, out.BrlUsd.Equal(dec("4.5")), "veio %s", out.BrlUsd)
	require.True(t, out.ChfUsd.Equal(dec("1.0")), "uma perna dentro do limite não é tocada")

	// Alvo acima: 1.0 + 10% = 1.1.
	up := rates("2.0", "1.0", "1.0", "5.0", "1.4")
	out = clampTowardsLimit(prev, up, moveLimit(1), func(string, ...any) {})
	require.True(t, out.ChfUsd.Equal(dec("1.1")), "veio %s", out.ChfUsd)

	// Alguns dias de gap: o limite escala (30%) e o passo é maior.
	out = clampTowardsLimit(prev, down, moveLimit(3), func(string, ...any) {})
	require.True(t, out.BrlUsd.Equal(dec("3.5")), "5.0 − 30%% = 3.5, veio %s", out.BrlUsd)
}

func TestFetchFixingParsesQueryJSON(t *testing.T) {
	dir := t.TempDir()

	bin := filepath.Join(dir, "fake.sh")
	script := `#!/bin/sh
printf '%s\n' '{"day":"2026-10-23","found":true,"chf_usd":"1.216","eur_usd":"1.120","gbp_usd":"1.338","brl_usd":"5.0377","cad_usd":"1.4004"}'
`
	require.NoError(t, os.WriteFile(bin, []byte(script), 0o755))

	fx, ok, err := fetchFixing(bin, "", "", "2026-10-23")
	require.NoError(t, err)
	require.True(t, ok)
	require.True(t, fx.BrlUsd.Equal(dec("5.0377")))
	require.True(t, fx.ChfUsd.Equal(dec("1.216")))

	nf := filepath.Join(dir, "nf.sh")
	require.NoError(t, os.WriteFile(nf, []byte("#!/bin/sh\nprintf '%s\\n' '{\"found\":false}'\n"), 0o755))
	_, ok, err = fetchFixing(nf, "", "", "2020-01-01")
	require.NoError(t, err)
	require.False(t, ok)
}

// Invariante que o keeper aplica: |passo| / anterior <= limite. O Mul arredonda
// para 18 casas e pode cair 1 ulp acima — este teste é o que impede que o
// walk-back fique preso num valor que a chain recusa.
func TestClampStepRespectsLimit(t *testing.T) {
	limit := moveLimit(1)
	// Inclui o caso que o revisor reproduziu (18 casas, canto de arredondamento).
	vals := []string{
		"0.684197719688542826", "5.0377", "1.216485", "0.001",
		"12345.678901234567890123", "3.333333333333333333",
	}
	for _, s := range vals {
		p := dec(s)
		for _, v := range []sdkmath.LegacyDec{p.Mul(dec("1.5")), p.Mul(dec("0.5")), p.Mul(dec("10"))} {
			out := clampStep(p, v, limit)
			require.True(t, out.IsPositive(), "passo não positivo para %s", p)
			require.False(t, out.Sub(p).Abs().Quo(p).GT(limit),
				"passo %s de %s excede o limite %s (alvo %s)", out, p, limit, v)
		}
	}
}
