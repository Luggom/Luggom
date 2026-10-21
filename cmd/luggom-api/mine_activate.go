package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	sdkmath "cosmossdk.io/math"
)

// Ativação da carteira do minerador.
//
// Uma carteira acabada de criar não existe on-chain, e o Cosmos exige conta
// para assinar (`GetSignerAcc`) e para pagar a taxa (`DeductFeeDecorator`).
// Sem isto, o primeiro mine falharia sempre no passo 1.
//
// A API envia um pó (que cria a conta e cobre a taxa) DEPOIS de validar a prova
// TLSNotary — o abuso fica preso a uma prova real. O pó é irrelevante em valor:
// activationDustUluggom = 1e7 uluggom = 0.00000001 LUG.
//
// ⚠️ Custo real para a API: sendo um MsgSend, o ante handler cobra também a taxa
// fixa de movimento de LUG (0.005 LUG, §13.87). Isto é por carteira nova — é uma
// decisão de política para a mainnet (ver checkpoint).
const (
	// activationDustUluggom é o pó enviado ao endereço novo: 10× a taxa da
	// mineração, para a conta existir e sobrar margem para a taxa.
	activationDustUluggom = 10 * mineTaxFeeUluggom
	// activationMaxPerIPPerHour limita ativações por IP (anti-spam de endereços).
	activationMaxPerIPPerHour = 5
)

// --- Persistência das ativações (uma por endereço, para sempre) ---

type activationStore struct {
	mu   sync.Mutex
	file string
	set  map[string]bool
	// rate limita por IP.
	rateMu sync.Mutex
	rate   map[string][]time.Time
}

var activations = &activationStore{set: map[string]bool{}, rate: map[string][]time.Time{}}

// setupActivationStore define o ficheiro e carrega as ativações já feitas.
func (s *Server) setupActivationStore() {
	if s.cfg.Home == "" {
		return
	}
	activations.mu.Lock()
	defer activations.mu.Unlock()
	activations.file = filepath.Join(s.cfg.Home, "luggom-activated.json")

	data, err := os.ReadFile(activations.file)
	if err != nil {
		return // primeira vez
	}
	_ = json.Unmarshal(data, &activations.set)
}

func (s *Server) saveActivations() {
	if activations.file == "" {
		return
	}
	activations.mu.Lock()
	data, err := json.MarshalIndent(activations.set, "", "  ")
	activations.mu.Unlock()
	if err != nil {
		return
	}
	if err := os.WriteFile(activations.file, data, 0o600); err != nil {
		log.Printf("warn: save activations file: %v", err)
	}
}

func (s *Server) activationAlreadySent(addr string) bool {
	activations.mu.Lock()
	defer activations.mu.Unlock()
	return activations.set[addr]
}

func (s *Server) markActivated(addr string) {
	activations.mu.Lock()
	activations.set[addr] = true
	activations.mu.Unlock()
	s.saveActivations()
}

// activationRateAllowed aplica o limite por IP (janela de 1h).
func activationRateAllowed(ip string) bool {
	activations.rateMu.Lock()
	defer activations.rateMu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)
	recent := make([]time.Time, 0, len(activations.rate[ip]))
	for _, t := range activations.rate[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= activationMaxPerIPPerHour {
		activations.rate[ip] = recent
		return false
	}
	activations.rate[ip] = append(recent, now)
	return true
}

// --- Queries ---

// queryAccount devolve se a conta existe e, se sim, número e sequência.
// Distingue "não existe" (found=false, err=nil) de uma falha real de query.
func (s *Server) queryAccount(address string) (found bool, accNum, seq uint64, err error) {
	args := []string{
		"query", "auth", "account", address,
		"--home", s.cfg.Home,
		"--node", s.cfg.Node,
		"-o", "json",
	}
	out, runErr := exec.Command(s.cfg.Bin, args...).CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if runErr != nil {
		// A chain responde "not found" para uma conta que nunca recebeu nada.
		low := strings.ToLower(outStr)
		if strings.Contains(low, "not found") || strings.Contains(low, "does not exist") {
			return false, 0, 0, nil
		}
		return false, 0, 0, fmt.Errorf("query account %s: %w: %s", address, runErr, outStr)
	}

	var probe map[string]interface{}
	if err := json.Unmarshal(out, &probe); err != nil {
		return false, 0, 0, fmt.Errorf("parse account: %w", err)
	}
	fields := findStringFields(probe, "account_number", "sequence")
	if fields[0] == "" {
		// O proto3 omite o zero: uma conta com `account_number = 0` (a 1ª conta
		// da chain, tipicamente o fundador) não traz o campo no JSON. Se o
		// objeto devolvido tem mesmo um endereço, a conta existe e o número
		// é 0 — não é "conta inexistente". (Bug encontrado no happy path de
		// 2026-09-11: o fundador não conseguia minerar.)
		if addr := findStringFields(probe, "address"); addr[0] != "" {
			fields[0] = "0"
		} else {
			return false, 0, 0, fmt.Errorf("account %s: account_number not found", address)
		}
	}
	accNum, err = parseUint(fields[0])
	if err != nil {
		return false, 0, 0, fmt.Errorf("account_number %q: %w", fields[0], err)
	}
	if fields[1] != "" {
		if seq, err = parseUint(fields[1]); err != nil {
			return false, 0, 0, fmt.Errorf("sequence %q: %w", fields[1], err)
		}
	}
	return true, accNum, seq, nil
}

func parseUint(s string) (uint64, error) {
	var v uint64
	if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &v); err != nil {
		return 0, err
	}
	return v, nil
}

// queryBalanceUluggom devolve o saldo em uluggom do endereço (0 se não existir).
func (s *Server) queryBalanceUluggom(address string) (sdkmath.Int, error) {
	args := []string{
		"query", "bank", "balances", address,
		"--home", s.cfg.Home,
		"--node", s.cfg.Node,
		"-o", "json",
	}
	out, runErr := exec.Command(s.cfg.Bin, args...).CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if runErr != nil {
		// Conta inexistente → saldo zero (não é erro fatal aqui).
		low := strings.ToLower(outStr)
		if strings.Contains(low, "not found") || strings.Contains(low, "does not exist") {
			return sdkmath.ZeroInt(), nil
		}
		return sdkmath.ZeroInt(), fmt.Errorf("query balance %s: %w: %s", address, runErr, outStr)
	}

	var parsed struct {
		Balances []struct {
			Denom  string `json:"denom"`
			Amount string `json:"amount"`
		} `json:"balances"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return sdkmath.ZeroInt(), fmt.Errorf("parse balance: %w", err)
	}
	for _, c := range parsed.Balances {
		if c.Denom != "uluggom" {
			continue
		}
		amt, ok := sdkmath.NewIntFromString(c.Amount)
		if !ok {
			return sdkmath.ZeroInt(), fmt.Errorf("saldo inválido %q", c.Amount)
		}
		return amt, nil
	}
	return sdkmath.ZeroInt(), nil
}

// --- Ativação ---

// ensureMinerAccount garante que o endereço do minerador consegue ASSINAR e
// PAGAR a taxa da mineração. Duas condições (D7):
//
//  1. a conta tem de existir on-chain — senão não há `account_number`/`sequence`
//     e a API não consegue sequer montar o sign-doc;
//  2. tem de ter saldo ≥ taxa, senão a tx falha na cobrança (`insufficient funds`).
//
// Verificar só a existência não bastava: uma conta que recebeu e gastou tudo
// existe mas não paga. Se faltar qualquer das duas, envia o pó (uma vez).
func (s *Server) ensureMinerAccount(address, clientIP string) error {
	found, _, _, err := s.queryAccount(address)
	if err != nil {
		return err
	}
	if found {
		balance, err := s.queryBalanceUluggom(address)
		if err != nil {
			return err
		}
		if balance.GTE(sdkmath.NewInt(mineTaxFeeUluggom)) {
			return nil // já consegue pagar: não toca na reserva
		}
		log.Printf("miner %s existe mas tem saldo %s < taxa %d; a enviar pó",
			address, balance.String(), mineTaxFeeUluggom)
	}

	// Já enviámos o pó mas continua sem conseguir pagar: não repetir
	// (evita drenar a reserva a cada pedido). Erro claro, com saída manual.
	if s.activationAlreadySent(address) {
		return fmt.Errorf("a conta %s já foi ativada uma vez mas não tem saldo para a taxa; "+
			"receba LUG nesse endereço ou contacte o suporte", address)
	}

	if !activationRateAllowed(clientIP) {
		return fmt.Errorf("demasiadas ativações a partir deste IP; tente mais tarde")
	}

	if err := s.sendActivationDust(address); err != nil {
		return fmt.Errorf("failed to activate miner account: %w", err)
	}

	// O broadcast é `sync`: regressa ANTES de a tx entrar no bloco. Sem esperar,
	// a consulta seguinte (account_number/sequence) diria que a conta ainda não
	// existe — a mineração falharia na primeira tentativa de cada carteira nova.
	// (Bug encontrado no E2E de 2026-09-10.)
	if err := s.waitForAccount(address, activationWaitTimeout); err != nil {
		return err
	}

	s.markActivated(address)
	log.Printf("activated miner account %s (dust=%duluggom)", address, activationDustUluggom)
	return nil
}

// activationWaitTimeout é quanto se espera pelo pó entrar em bloco.
const activationWaitTimeout = 20 * time.Second

// waitForAccount espera até a conta existir on-chain (ou esgotar o tempo).
func (s *Server) waitForAccount(address string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		found, _, _, err := s.queryAccount(address)
		if err != nil {
			return err
		}
		if found {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("o pó foi enviado mas a conta %s ainda não apareceu on-chain após %s", address, timeout)
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// sendActivationDust envia o pó da conta de serviço da API para o minerador.
func (s *Server) sendActivationDust(address string) error {
	args := []string{
		"tx", "bank", "send", s.cfg.Keyring, address,
		fmt.Sprintf("%duluggom", activationDustUluggom),
		"--keyring-backend", "test",
		"--home", s.cfg.Home,
		"--chain-id", s.cfg.ChainID,
		"--node", s.cfg.Node,
		"--gas", "200000", "--gas-prices", "1uluggom",
		"-y",
		"-o", "json",
	}
	out, runErr := exec.Command(s.cfg.Bin, args...).CombinedOutput()
	outStr := strings.TrimSpace(string(out))
	if idx := strings.LastIndex(outStr, "{"); idx >= 0 {
		outStr = outStr[idx:]
	}
	var result struct {
		TxHash string `json:"txhash"`
		Code   int    `json:"code"`
		RawLog string `json:"raw_log"`
	}
	if err := json.Unmarshal([]byte(outStr), &result); err != nil {
		if runErr != nil {
			return fmt.Errorf("%w: %s", runErr, strings.TrimSpace(string(out)))
		}
		return fmt.Errorf("parse output: %w (raw: %s)", err, outStr)
	}
	if result.Code != 0 {
		return fmt.Errorf("tx failed (code=%d): %s", result.Code, strings.TrimSpace(result.RawLog))
	}
	return nil
}

// clientIPFromRequest extrai o IP do cliente (respeita X-Forwarded-For).
func clientIPFromRequest(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		if i := strings.IndexByte(fwd, ','); i >= 0 {
			fwd = fwd[:i]
		}
		if ip := strings.TrimSpace(fwd); ip != "" {
			return ip
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
