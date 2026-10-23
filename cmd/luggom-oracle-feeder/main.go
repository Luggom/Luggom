// luggom-oracle-feeder — o feeder do oráculo (§13.28 + §13.56).
//
// 1×/DIA: lê CHF/USD, EUR/USD, GBP/USD (USD por unidade) e BRL/USD, CAD/USD
// (unidades por 1 USD) de N fontes independentes, toma a **mediana por perna** e
// submete `MsgUpdateUsdOracleBasket` com a fixing date do dia.
//
// **Fail-closed, dos dois lados:**
//   - aqui: se responderem menos de `-min-sources` fontes, se as fontes
//     discordarem acima de `-max-spread`, ou se nenhuma publicar uma fixing para
//     hoje, **NÃO submete**;
//   - na chain: sem fixing na janela do pagamento a mineração **PÁRA**
//     (`oracle_usd.go`).
//
// ⚠️ As fontes são **CONFIGURAÇÃO** (`-sources-file`), não constantes: o Chainlink
// e a Band são **on-chain por natureza** (EVM / chain próprio) e precisam de um
// adapter por RPC que ainda não existe. Ver `sources.example.json`.
//
// Uso:
//
//	luggom-oracle-feeder -sources-file sources.json -once -dry-run
//	luggom-oracle-feeder -sources-file sources.json -interval 24h \
//	  -bin ./build/luggomd -home ~/.luggom -chain-id luggom -keyring test \
//	  -from oracle -node tcp://127.0.0.1:26657
//
// systemd/cron: usar `-once` e agendar 1×/dia.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	sdkmath "cosmossdk.io/math"
)

func main() {
	var (
		sourcesFile = flag.String("sources-file", "", "ficheiro JSON com as fontes (obrigatório)")
		bin         = flag.String("bin", "./build/luggomd", "binário luggomd que submete a tx")
		home        = flag.String("home", "", "--home do luggomd")
		chainID     = flag.String("chain-id", "", "--chain-id")
		keyring     = flag.String("keyring", "", "--keyring-backend")
		from        = flag.String("from", "", "--from (conta que assina)")
		node        = flag.String("node", "", "--node")
		interval    = flag.Duration("interval", 24*time.Hour, "cadência (§13.56: 1×/dia)")
		once        = flag.Bool("once", false, "corre uma vez e sai (cron/systemd)")
		dryRun      = flag.Bool("dry-run", false, "calcula e imprime SEM submeter tx")
		minSources  = flag.Int("min-sources", 2, "mínimo de fontes a responder (1 cai ⇒ mediana das outras 2)")
		maxSpreadS  = flag.String("max-spread", "0.02", "desvio máximo entre fontes por perna (2% — assumido)")
		fees        = flag.String("fees", "5000000000000000uluggom",
			"fee da tx — a taxa fixa de movimento é 0,005 LUG (§13.87); o gas price mínimo (1uluggom) fica coberto")
	)
	flag.Parse()

	maxSpread, err := sdkmath.LegacyNewDecFromStr(*maxSpreadS)
	if err != nil {
		log.Fatalf("max-spread inválido: %v", err)
	}

	// A-5 (hard test): piso rígido de 2 fontes. Um oráculo alimentado por UMA só
	// fonte é manipulável — não é a "mediana de fontes independentes" da decisão.
	if err := validateMinSources(*minSources); err != nil {
		log.Fatalf("%v", err)
	}

	cfgs, err := readSourcesFile(*sourcesFile)
	if err != nil {
		log.Fatalf("sources-file: %v", err)
	}
	srcs, err := loadSources(cfgs)
	if err != nil {
		log.Fatalf("%v", err)
	}
	log.Printf("feeder do oráculo: %d fonte(s), mínimo %d, spread máx %s, cadência %s",
		len(srcs), *minSources, maxSpread.String(), *interval)

	extra := buildTxArgs(*home, *chainID, *keyring, *from, *node)
	// A tx TEM de levar fee explícita: o ante exige a taxa fixa de 0,005 LUG
	// (§13.87) MAIS o gas price mínimo (1uluggom). Sem `--fees` a chain responde
	// "insufficient fees; got: required: 200000uluggom".
	extra = append(extra, "--fees", *fees)

	runOnce := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()

		rates := fetchAll(ctx, srcs, log.Printf)
		agg, err := Aggregate(rates, *minSources, maxSpread)
		if err != nil {
			return err
		}
		log.Printf("MEDIANA: %s", agg.String())

		// A fixing date tem de ser a de hoje (UTC). Num fim de semana/feriado as
		// fontes reportam a última publicação e NÃO se escreve nada: a mineração
		// usa a fixing anterior (convenção PTAX). Escrever a taxa de sexta com a
		// data de sábado seria inventar uma fixing que não existe.
		today := time.Now().UTC().Format("2006-01-02")
		if agg.Date != today {
			log.Printf("sem fixing publicada hoje (fontes dizem %s, hoje é %s) — NÃO submetido", agg.Date, today)
			return nil
		}

		if *dryRun {
			log.Printf("dry-run: NÃO submetido (seria rate_date=%s chf=%s eur=%s gbp=%s brl=%s cad=%s)",
				agg.Date, agg.ChfUsd, agg.EurUsd, agg.GbpUsd, agg.BrlUsd, agg.CadUsd)
			return nil
		}
		return submit(*bin, extra, agg)
	}

	if *once {
		if err := runOnce(); err != nil {
			log.Fatalf("falhou: %v", err)
		}
		return
	}

	for {
		if err := runOnce(); err != nil {
			// Um ciclo falhado NÃO derruba o feeder: a próxima hora/dia tenta
			// de novo, e a chain pausa a mineração entretanto (fail-closed).
			log.Printf("ciclo falhou: %v", err)
		}
		time.Sleep(*interval)
	}
}

// validateMinSources impõe o PISO de 2 fontes (§13.28/§13.139, A-5 do hard test):
// a decisão exige "≥2 fontes independentes" — com uma só, a mediana É a leitura,
// logo o feeder passaria a ser um ponto único de manipulação do oráculo.
func validateMinSources(n int) error {
	if n < 2 {
		return fmt.Errorf(
			"min-sources tem de ser >= 2 (recebido %d): um oráculo de uma só fonte é manipulável (§13.139)", n)
	}
	return nil
}

func readSourcesFile(path string) ([]sourceConfig, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf(
			"obrigatório: as fontes são configuração, não constantes (ver sources.example.json)")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfgs []sourceConfig
	if err := json.Unmarshal(raw, &cfgs); err != nil {
		return nil, fmt.Errorf("json: %w", err)
	}
	return cfgs, nil
}

func buildTxArgs(home, chainID, keyring, from, node string) []string {
	var a []string
	if home != "" {
		a = append(a, "--home", home)
	}
	if chainID != "" {
		a = append(a, "--chain-id", chainID)
	}
	if keyring != "" {
		a = append(a, "--keyring-backend", keyring)
	}
	if from != "" {
		a = append(a, "--from", from)
	}
	if node != "" {
		a = append(a, "--node", node)
	}
	return a
}

// submit passa a mediana ao luggomd (o mesmo padrão que a API usa para assinar):
// o feeder NÃO tem lógica de chain, só entrega a tx.
func submit(bin string, extra []string, r Rates) error {
	args := []string{
		"tx", "luggom", "update-usd-oracle-basket",
		"--rate-date", r.Date,
		"--chf-usd", r.ChfUsd.String(),
		"--eur-usd", r.EurUsd.String(),
		"--gbp-usd", r.GbpUsd.String(),
		"--brl-usd", r.BrlUsd.String(),
		"--cad-usd", r.CadUsd.String(),
		"--yes",
	}
	args = append(args, extra...)
	cmd := exec.Command(bin, args...)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Idempotência: se o dia já foi escrito (retry na mesma janela, ou uma
		// execução anterior que chegou à chain), isto é SUCESSO — não é falha.
		if strings.Contains(string(out), "oracle days are immutable") ||
			strings.Contains(string(out), "already written") {
			log.Printf("fixing de %s já estava escrita — nada a fazer", r.Date)
			return nil
		}
		return fmt.Errorf("luggomd tx falhou: %w — %s", err, strings.TrimSpace(string(out)))
	}
	log.Printf("submetido: %s", strings.TrimSpace(string(out)))
	return nil
}
