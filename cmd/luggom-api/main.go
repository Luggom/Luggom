// luggom-api: API HTTP para o APK (Fase B TLSNotary).
// Expõe POST /api/v1/mine/tax (MsgMineTaxPayment — Fase B TLSNotary),
// GET /api/v1/reclaim/providers (bancos suportados TLSNotary),
// GET /api/v1/chain/info (metadados), GET /api/v1/gov/luggom (governança),
// GET /api/v1/balance/:address, GET /api/v1/status.
// Uso: luggom-api [--port 8082] [--home ~/.luggom] [--bin ./build/luggomd]
package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/api/escrowlab"
	"luggom/api/reclaimverify"
	"luggom/api/taxnature"
	"luggom/api/taxproof"
	"luggom/api/uspipeline"
	"luggom/x/luggom/types"
)

// 8082: padrão local + emulador Android (APK aponta para 10.0.2.2:8082).
const defaultPort = "8082"
const defaultChainID = "luggom-test"
const defaultCosmosBip44Path = "m/44'/118'/0'/0/0"
const defaultBech32Prefix = "luggom"
const defaultDenomBase = "uluggom"

// defaultDevVerifierKeyHex é a chave privada secp256k1 de DESENVOLVIMENTO (32 bytes
// de 0x11). Pública e conhecida — serve só para a chain local; em mainnet tem de
// ser substituída por LUGGOM_API_VERIFIER_KEY_HEX.
const defaultDevVerifierKeyHex = "1111111111111111111111111111111111111111111111111111111111111111"

type Config struct {
	Port    string
	Home    string
	Bin     string
	ChainID string
	Keyring string
	Node    string
	// GrpcURL: endereço gRPC do nó (ex.: 127.0.0.1:9090), se exposto — CosmJS/grpcurl.
	GrpcURL string
	// RestURL: LCD / gRPC-Gateway (ex.: http://127.0.0.1:1317), se activo.
	RestURL string
	// Bech32Prefix: prefixo de conta (alinhado a app.AccountAddressPrefix).
	Bech32Prefix string
	// DataProvider: rótulo informativo devolvido em /api/v1/chain/info (mock | sandbox | production).
	DataProvider string
	// EscrowLab: Etapa G2 — expõe /api/v1/escrow/lab/* (livro em memória; LUGGOM_ESCROW_LAB=1).
	EscrowLab bool
	// ReclaimMockMode (Fase B): se true, verificação de proof é mock (padrão: true).
	ReclaimMockMode bool
	// ApiVerifierKeyHex: chave privada secp256k1 (32 bytes, hex) que assina a
	// attestation de mineração. O endereço dela tem de estar em
	// `params.api_verifier_address` para a chain (fase 2) aceitar a prova.
	//
	// Em Fase B (§13.121) com NotaryURL definido, esta chave NÃO é usada para
	// assinar: o notário assina e a API só relaya. `api_verifier_address` passa
	// a ser o endereço do NOTÁRIO.
	ApiVerifierKeyHex string
	// NotaryURL: base URL do notário TLSNotary (ex.: http://127.0.0.1:7047). Se
	// definido, a API busca a atestação assinada pelo notário e usa OS CAMPOS
	// DELA — deixa de poder inventar o valor (Fecha a derivação confiada à API).
	NotaryURL string
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loadConfig() Config {
	home := getEnv("LUGGOM_HOME", os.ExpandEnv("$HOME/.luggom"))
	bin := getEnv("LUGGOMD_BIN", "./build/luggomd")
	return Config{
		Port:    getEnv("LUGGOM_API_PORT", defaultPort),
		Home:    home,
		Bin:     bin,
		ChainID: getEnv("LUGGOM_CHAIN_ID", defaultChainID),
		Keyring: getEnv("LUGGOM_KEYRING", "test"),
		// Nó RPC que o CLI usará para falar com a chain.
		// Por padrão, assume o nó local padrão do luggomd.
		Node:              getEnv("LUGGOM_NODE", "tcp://127.0.0.1:26657"),
		GrpcURL:           strings.TrimSpace(getEnv("LUGGOM_GRPC_URL", "")),
		RestURL:           strings.TrimSpace(getEnv("LUGGOM_REST_URL", "")),
		Bech32Prefix:      getEnv("LUGGOM_BECH32_PREFIX", defaultBech32Prefix),
		DataProvider:      strings.TrimSpace(getEnv("LUGGOM_DATA_PROVIDER", "")),
		EscrowLab:         strings.TrimSpace(getEnv("LUGGOM_ESCROW_LAB", "")) == "1",
		ReclaimMockMode:   strings.TrimSpace(getEnv("LUGGOM_RECLAIM_MOCK", "")) != "0",
		// Default = chave DEV conhecida (32 bytes de 0x11). NUNCA usar em mainnet:
		// sobrescrever com LUGGOM_API_VERIFIER_KEY_HEX e registar o endereço em
		// params.api_verifier_address.
		ApiVerifierKeyHex: strings.TrimSpace(getEnv("LUGGOM_API_VERIFIER_KEY_HEX", defaultDevVerifierKeyHex)),
		NotaryURL:         strings.TrimSpace(getEnv("LUGGOM_NOTARY_URL", "")),
	}
}

func (c Config) dataProviderLabel() string {
	if c.DataProvider == "" {
		return "mock"
	}
	return c.DataProvider
}

// rpcHTTPEndpoint converte tcp://host:26657 em http://host:26657 para clientes HTTP JSON-RPC (CosmJS, etc.).
func rpcHTTPEndpoint(node string) string {
	n := strings.TrimSpace(node)
	if n == "" {
		return ""
	}
	if strings.HasPrefix(n, "tcp://") {
		return "http://" + strings.TrimPrefix(n, "tcp://")
	}
	return n
}

// ChainInfoResponse metadados para o APK / wallets (Etapa E1 + G1 tokenomics).
type ChainInfoResponse struct {
	ChainID        string `json:"chain_id"`
	RpcAddress     string `json:"rpc_address"`        // como em LUGGOM_NODE (ex.: tcp://127.0.0.1:26657)
	RpcHTTPURL     string `json:"rpc_http_url"`       // URL HTTP para /status, txs, etc.
	GrpcURL        string `json:"grpc_url,omitempty"` // se LUGGOM_GRPC_URL definido
	RestURL        string `json:"rest_url,omitempty"` // LCD / gateway se LUGGOM_REST_URL definido
	Bech32Prefix   string `json:"bech32_prefix"`
	DenomBase      string `json:"denom_base"`
	DenomDisplay   string `json:"denom_display"`
	CosmosCoinType int    `json:"cosmos_coin_type"`
	HdPathDefault  string `json:"hd_path_default"`
	DataProvider   string `json:"data_provider"`
	LuggomAPIPort  string `json:"luggom_api_port"`
	Note           string `json:"note"`
	// Tokenomics (Etapa G1): snapshot derivado de `query luggom params` (pool_active).
	Tokenomics           *uspipeline.TokenomicsSnapshot `json:"tokenomics,omitempty"`
	TokenomicsQueryError string                         `json:"tokenomics_query_error,omitempty"`
	// EscrowLab (Etapa G2): metadados quando LUGGOM_ESCROW_LAB=1.
	EscrowLab *escrowlab.ChainSnapshot `json:"escrow_lab,omitempty"`
	// UsdPerLugParity é a paridade de referência da cesta em USD/LUG
	// (CHF+EUR+GBP)/3 × 3, +5% a.a. composto — §13.132. É INFORMATIVA (custo de
	// mineração); o aviso "abaixo do valor justo" saiu com o Farol (§13.128 §7).
	UsdPerLugParity float64 `json:"usd_per_lug_parity,omitempty"`
}

// Server estado HTTP partilhado (escrow lab + meta das txs de mineração).
type Server struct {
	cfg    Config
	escrow *escrowlab.Store

	// pendingMu protege pendingMeta: meta do extrato guardada no passo 1
	// (/mine/tax) e fixada ao txhash no passo 2 (/mine/tax/broadcast).
	pendingMu   sync.Mutex
	pendingMeta map[string]MineTaxMeta
}

// storePendingMineMeta guarda a meta do extrato à espera do txhash (chave: payment_id).
func (s *Server) storePendingMineMeta(paymentID string, meta MineTaxMeta) {
	pid := strings.TrimSpace(paymentID)
	if pid == "" {
		return
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	if s.pendingMeta == nil {
		s.pendingMeta = map[string]MineTaxMeta{}
	}
	s.pendingMeta[pid] = meta
}

// takePendingMineMeta devolve (e remove) a meta pendente de um payment_id.
func (s *Server) takePendingMineMeta(paymentID string) (MineTaxMeta, bool) {
	pid := strings.TrimSpace(paymentID)
	if pid == "" {
		return MineTaxMeta{}, false
	}
	s.pendingMu.Lock()
	defer s.pendingMu.Unlock()
	meta, ok := s.pendingMeta[pid]
	if ok {
		delete(s.pendingMeta, pid)
	}
	return meta, ok
}

// usdPerLugParityC1 calcula a paridade USD/LUG da cesta estática (C1).
// Fórmula: (CHF/USD + EUR/USD + GBP/USD) / 3 × multiplicador
// Multiplicador = 3 no lançamento, +5% a.a. composto (§13.132; era 6 no §13.116).
// Valores default (C1 estático): CHF=1.10, EUR=1.18, GBP=1.37
// Paridade resultante ≈ 3,65 USD/LUG.
// Fase C2 (oráculo dinâmico) substitui este valor na mainnet.
func usdPerLugParityC1() float64 {
	return basketParityUSDLug()
}

func (s *Server) handleChainInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := ChainInfoResponse{
		ChainID:         s.cfg.ChainID,
		RpcAddress:      s.cfg.Node,
		RpcHTTPURL:      rpcHTTPEndpoint(s.cfg.Node),
		GrpcURL:         s.cfg.GrpcURL,
		RestURL:         s.cfg.RestURL,
		Bech32Prefix:    s.cfg.Bech32Prefix,
		DenomBase:       defaultDenomBase,
		DenomDisplay:    "LUG",
		CosmosCoinType:  118,
		HdPathDefault:   defaultCosmosBip44Path,
		DataProvider:    s.cfg.dataProviderLabel(),
		LuggomAPIPort:   s.cfg.Port,
		UsdPerLugParity: usdPerLugParityC1(),
		Note:            "E1+G1: configure LUGGOM_NODE para o host alcançável pelo cliente (ex.: emulador Android: IP da máquina ou proxy, não só 127.0.0.1 no dispositivo). Tokenomics exige nó RPC acessível.",
	}
	if tok, err := s.queryTokenomicsFromNode(); err != nil {
		resp.TokenomicsQueryError = err.Error()
	} else {
		t := tok
		resp.Tokenomics = &t
	}
	if s.cfg.EscrowLab {
		snap := escrowlab.NewChainSnapshot()
		resp.EscrowLab = &snap
	}
	writeJSON(w, http.StatusOK, resp)
}

// MineTaxPaymentRequest corpo do POST /api/v1/mine/tax (Fase B — TLSNotary).
type MineTaxPaymentRequest struct {
	// TlsnProof = bundle da proof REAL (prover UniFFI no APK, §13.58 passo 2).
	// É a ÚNICA forma aceite: o caminho legacy `reclaim_proof` (JSON não
	// verificado) foi removido — era forjável e invalidava o portão da mineração.
	TlsnProof   *TlsnProofPayload `json:"tlsn_proof"`
	ProviderId  string            `json:"provider_id"`  // "chase", "itau", etc.
	DestAddress string            `json:"dest_address"` // carteira do minerador (luggom1...)
	// PublicKey é a chave pública secp256k1 comprimida do minerador (base64 ou hex)
	// ou o proto Any (base64). Entra no AuthInfo da tx não assinada — a API nunca
	// vê a seed nem a chave privada.
	PublicKey string `json:"public_key"`
}

// TlsnProofPayload é a proof real TLSNotary enviada pelo APK após o
// `proveFinalize` (transcript atestado + corpo revelado por selective disclosure).
type TlsnProofPayload struct {
	SessionID    string `json:"session_id"`
	ServerName   string `json:"server_name"`
	Status       int    `json:"status"`
	ResponseBody string `json:"response_body"` // corpo atestado (HTML/texto do comprovante)
	Transcript   struct {
		Sent string `json:"sent"` // base64
		Recv string `json:"recv"` // base64
	} `json:"transcript"`
}

// MineTaxPaymentResponse resposta de POST /api/v1/mine/tax (tx NÃO assinada) e de
// POST /api/v1/mine/tax/broadcast (resultado on-chain).
type MineTaxPaymentResponse struct {
	TxHash      string `json:"tx_hash,omitempty"`
	AmountMined string `json:"amount_mined,omitempty"`
	PaymentAmount    string `json:"payment_amount,omitempty"`
	TaxEntity   string `json:"tax_entity,omitempty"`
	PaymentId   string `json:"payment_id,omitempty"`
	Error       string `json:"error,omitempty"`
	Code        int    `json:"code,omitempty"`
	// Material de assinatura local (só no passo 1 — /mine/tax).
	Unsigned *UnsignedTx `json:"unsigned,omitempty"`
}

// SendRequest corpo do POST /api/v1/send
type SendRequest struct {
	FromKey       string `json:"from_key"`       // nome da chave no keyring (ex: validator)
	ToAddress     string `json:"to_address"`     // endereço destino
	AmountUluggom string `json:"amount_uluggom"` // quantidade em uluggom (sem denom)
}

// SendResponse resposta do POST /api/v1/send
type SendResponse struct {
	TxHash string `json:"tx_hash,omitempty"`
	Error  string `json:"error,omitempty"`
	Code   int    `json:"code,omitempty"`
}

// BroadcastRequest: assinatura local (opção A). O APK envia a tx já assinada.
type BroadcastRequest struct {
	TxBytes string `json:"tx_bytes"`       // base64 do tx assinado
	Mode    string `json:"mode,omitempty"` // "sync", "async" (default: "sync")
}

// BackupEmailRequest: pedido de envio do ficheiro .luggom-backup por email.
type BackupEmailRequest struct {
	Email         string `json:"email"`
	CiphertextB64 string `json:"ciphertext_b64"`
	LuggomAddress string `json:"luggom_address"`
	ConfirmCode   string `json:"confirm_code,omitempty"` // código de 6 dígitos
}

// backupRateLimit: mapa em memória para rate-limit (5/hora/endereço).
var backupRateLimit = struct {
	mu      sync.Mutex
	buckets map[string][]time.Time
}{buckets: make(map[string][]time.Time)}

// backupConfirmCodes: códigos de confirmação pendentes.
var backupConfirmCodes = struct {
	mu    sync.Mutex
	codes map[string]string // email → code
}{codes: make(map[string]string)}

// TransactionApiEntry: formato esperado pelo APK para preencher o extrato.
// (veja ApiClient.getTransactions e ExtractScreen)
type TransactionApiEntry struct {
	Type          string      `json:"type"` // "received" | "sent" | "mined"
	FromAddress   string      `json:"from_address"`
	AmountUluggom string      `json:"amount_uluggom"`
	Timestamp     string      `json:"timestamp"`
	TxHash        string      `json:"tx_hash"`
	MineMeta      MineTaxMeta `json:"mine_meta,omitempty"`
}

type TransactionsApiResponse struct {
	Transactions []TransactionApiEntry `json:"transactions"`
}

func (s *Server) handleTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	address := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/transactions/"), "/")
	if address == "" {
		http.Error(w, "missing address", http.StatusBadRequest)
		return
	}

	// O `q txs` do Cosmos não aceita `OR` na query (retorna "Usage"). Então
	// rodamos DUAS queries (recebimentos + envios) e concatenamos, deduplicando
	// por txhash. Também mudamos de `q txs` (que pode ter problemas com a
	// query string) para a forma com `--query` dupla via duas execuções.
	type txResult struct {
		TxHash    string `json:"txhash"`
		Height    string `json:"height"`
		Timestamp string `json:"timestamp"`
		Code      int    `json:"code"`
		Events    []struct {
			Type       string `json:"type"`
			Attributes []struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			} `json:"attributes"`
		} `json:"events"`
		Logs []struct {
			Events []struct {
				Type       string `json:"type"`
				Attributes []struct {
					Key   string `json:"key"`
					Value string `json:"value"`
				} `json:"attributes"`
			} `json:"events"`
		} `json:"logs"`
	}

	runQuery := func(query string) ([]txResult, error) {
		args := []string{
			"q", "txs",
			"--query", query,
			"--home", s.cfg.Home,
			"--node", s.cfg.Node,
			"--limit", "20",
			"-o", "json",
		}
		cmd := exec.Command(s.cfg.Bin, args...)
		cmd.Env = os.Environ()
		out, err := cmd.CombinedOutput()
		if err != nil {
			return nil, err
		}
		var res struct {
			Txs []txResult `json:"txs"`
		}
		if err := json.Unmarshal(out, &res); err != nil {
			return nil, err
		}
		return res.Txs, nil
	}

	received, err := runQuery(fmt.Sprintf("transfer.recipient='%s'", address))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, TransactionsApiResponse{Transactions: []TransactionApiEntry{}})
		return
	}
	sent, err := runQuery(fmt.Sprintf("transfer.sender='%s'", address))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, TransactionsApiResponse{Transactions: []TransactionApiEntry{}})
		return
	}
	// Concatena e deduplica por txhash.
	seen := map[string]bool{}
	txsResult := struct {
		Txs []txResult
	}{Txs: []txResult{}}
	for _, tx := range append(received, sent...) {
		if seen[tx.TxHash] {
			continue
		}
		seen[tx.TxHash] = true
		txsResult.Txs = append(txsResult.Txs, tx)
	}

	toUluggom := func(v string) string {
		v = strings.TrimSpace(v)
		if strings.HasSuffix(v, "uluggom") {
			return strings.TrimSuffix(v, "uluggom")
		}
		return v
	}

	extractFromEvents := func(events []struct {
		Type       string `json:"type"`
		Attributes []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"attributes"`
	}, wantRecipient string) (fromAddress string, amountUluggom string, isMine bool, meta MineTaxMeta) {
		for _, ev := range events {
			if ev.Type == "mine" || ev.Type == "luggom.mine" {
				isMine = true
				// Reconstrói a meta a partir do evento on-chain (independe do map
				// em memória, que zera ao reiniciar a API). bank/session não estão
				// no evento; esses vêm do map quando disponível.
				for _, attr := range ev.Attributes {
					switch attr.Key {
					case "payment_amount":
						meta.FiatAmount = attr.Value
					case "currency_code":
						meta.Currency = attr.Value
					case "tax_entity":
						meta.TaxEntity = attr.Value
					}
				}
			}
			if ev.Type != "transfer" {
				continue
			}
			var sender string
			var recipient string
			var amount string
			for _, attr := range ev.Attributes {
				switch attr.Key {
				case "sender":
					sender = attr.Value
				case "recipient":
					recipient = attr.Value
				case "amount":
					amount = attr.Value
				}
			}
			// Query já garante recipient, mas vamos ser defensivos.
			if recipient != "" && recipient != wantRecipient {
				continue
			}
			if sender != "" && amount != "" {
				return sender, toUluggom(amount), isMine, meta
			}
		}
		return "", "", isMine, meta
	}

	entries := make([]TransactionApiEntry, 0, len(txsResult.Txs))
	for _, tx := range txsResult.Txs {
		fromAddr, amount, isMine, eventMeta := extractFromEvents(tx.Events, address)
		if fromAddr == "" || amount == "" {
			for _, logEntry := range tx.Logs {
				if fromAddr != "" && amount != "" {
					break
				}
				var mineFlag bool
				fromAddr, amount, mineFlag, eventMeta = extractFromEvents(logEntry.Events, address)
				if mineFlag {
					isMine = true
				}
			}
		}
		if fromAddr == "" || amount == "" {
			continue
		}

		txType := "received"
		if isMine {
			txType = "mined"
		} else if fromAddr == address {
			txType = "sent"
		} else if fromAddr == s.serviceAccountAddress() {
			// Forward da mineração (conta de serviço → usuário): é uma mineração.
			txType = "mined"
		}

		entry := TransactionApiEntry{
			Type:          txType,
			FromAddress:   fromAddr,
			AmountUluggom: amount,
			Timestamp:     tx.Timestamp,
			TxHash:        tx.TxHash,
		}
		// Prioridade: (1) meta exata do map em memória (bank/session); (2) meta
		// reconstruída do evento on-chain (valor/moeda/entidade); senão vazio.
		if meta, ok := s.getMineTaxMeta(tx.TxHash); ok {
			entry.MineMeta = meta
		} else if eventMeta.FiatAmount != "" || eventMeta.TaxEntity != "" {
			entry.MineMeta = eventMeta
		}
		entries = append(entries, entry)
	}

	writeJSON(w, http.StatusOK, TransactionsApiResponse{Transactions: entries})
}

func (s *Server) resolveAddress(keyName string) (string, error) {
	cmd := exec.Command(s.cfg.Bin, "keys", "show", keyName, "-a", "--keyring-backend", "test", "--home", s.cfg.Home)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// queryTxCode consulta a tx após o broadcast e retorna o `code` real de
// inclusão no bloco. O broadcast sync do CLI retorna antes da inclusão, então
// o code real (ex: 1202 anti-replay) só aparece no query posterior.
func (s *Server) queryTxCode(txHash string) (int, error) {
	var out []byte
	var queryErr error
	for i := 0; i < 10; i++ {
		args := []string{
			"query", "tx",
			"--type", "hash", txHash,
			"--home", s.cfg.Home,
			"--node", s.cfg.Node,
			"-o", "json",
		}
		cmd := exec.Command(s.cfg.Bin, args...)
		cmd.Env = os.Environ()
		out, queryErr = cmd.CombinedOutput()
		if queryErr == nil {
			break
		}
		// Ainda não indexada: aguarda e tenta de novo.
		time.Sleep(1 * time.Second)
	}
	if queryErr != nil {
		return 1, fmt.Errorf("query tx failed: %w (%s)", queryErr, strings.TrimSpace(string(out)))
	}
	var txResult struct {
		Code int `json:"code"`
		Logs []struct {
			Code int `json:"code"`
		} `json:"logs"`
	}
	if err := json.Unmarshal(out, &txResult); err != nil {
		return 1, fmt.Errorf("parse tx json: %w", err)
	}
	code := txResult.Code
	if code == 0 {
		for _, l := range txResult.Logs {
			if l.Code != 0 {
				code = l.Code
				break
			}
		}
	}
	return code, nil
}

func (s *Server) handleBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	address := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/balance/"), "/")
	if address == "" {
		http.Error(w, "missing address", http.StatusBadRequest)
		return
	}
	args := []string{
		"query", "bank", "balances", address,
		"--home", s.cfg.Home,
		"--node", s.cfg.Node,
		"-o", "json",
	}
	cmd := exec.Command(s.cfg.Bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": string(out)})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(out)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	args := []string{"status", "--home", s.cfg.Home, "--node", s.cfg.Node}
	cmd := exec.Command(s.cfg.Bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": string(out)})
		return
	}
	// status retorna texto; opcionalmente parsear ou devolver raw
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.Encode(map[string]string{"status_output": strings.TrimSpace(string(out))})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req SendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SendResponse{Error: "invalid json: " + err.Error()})
		return
	}
	if req.FromKey == "" || req.ToAddress == "" || req.AmountUluggom == "" {
		writeJSON(w, http.StatusBadRequest, SendResponse{Error: "missing required: from_key, to_address, amount_uluggom"})
		return
	}
	log.Printf("send request received from_key=%s to_address=%s amount_uluggom=%s", req.FromKey, req.ToAddress, req.AmountUluggom)

	amountArg := req.AmountUluggom + "uluggom"
	args := []string{
		"tx", "bank", "send",
		req.FromKey, req.ToAddress, amountArg,
		"--from", req.FromKey,
		"--keyring-backend", "test",
		"--home", s.cfg.Home,
		"--chain-id", s.cfg.ChainID,
		"--gas", "1000000", "--gas-prices", "1uluggom",
		"-y",
		"-o", "json",
	}
	cmd := exec.Command(s.cfg.Bin, args...)
	cmd.Env = os.Environ()
	out, runErr := cmd.CombinedOutput()

	// Extrai o JSON final.
	outStr := strings.TrimSpace(string(out))
	if idx := strings.LastIndex(outStr, "{"); idx >= 0 {
		outStr = outStr[idx:]
	}
	var result struct {
		TxHash string `json:"txhash"`
		Code   int    `json:"code"`
	}
	if err := json.Unmarshal([]byte(outStr), &result); err != nil {
		if runErr != nil {
			writeJSON(w, http.StatusInternalServerError, SendResponse{
				Error: fmt.Sprintf("exec: %v\n%s", runErr, string(out)),
				Code:  1,
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, SendResponse{
			Error: fmt.Sprintf("parse output: %v (raw: %s)", err, strings.TrimSpace(string(out))),
			Code:  1,
		})
		return
	}
	if result.Code != 0 {
		log.Printf("send failed code=%d txhash=%s", result.Code, result.TxHash)
		writeJSON(w, http.StatusBadRequest, SendResponse{
			TxHash: result.TxHash,
			Code:   result.Code,
			Error:  strings.TrimSpace(string(out)),
		})
		return
	}
	log.Printf("send success txhash=%s from_key=%s to_address=%s amount_uluggom=%s", result.TxHash, req.FromKey, req.ToAddress, req.AmountUluggom)
	writeJSON(w, http.StatusOK, SendResponse{TxHash: result.TxHash, Code: 0})
}

// handleAccount retorna account_number e sequence para o endereço (necessário p/ construir tx).
func (s *Server) handleAccount(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	address := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/account/"), "/")
	if address == "" {
		http.Error(w, "missing address", http.StatusBadRequest)
		return
	}
	args := []string{
		"query", "auth", "account", address,
		"--home", s.cfg.Home,
		"--node", s.cfg.Node,
		"-o", "json",
	}
	cmd := exec.Command(s.cfg.Bin, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": string(out)})
		return
	}

	var result struct {
		AccountNumber int `json:"account_number"`
		Sequence      int `json:"sequence"`
	}
	var raw struct {
		Account struct {
			BaseAccount struct {
				AccountNumber string `json:"account_number"`
				Sequence      string `json:"sequence"`
			} `json:"base_account"`
		} `json:"account"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		// fallback: try top-level fields
		var flat struct {
			AccountNumber string `json:"account_number"`
			Sequence      string `json:"sequence"`
		}
		if err2 := json.Unmarshal(out, &flat); err2 != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "parse: " + err.Error()})
			return
		}
		result.AccountNumber, _ = strconv.Atoi(flat.AccountNumber)
		result.Sequence, _ = strconv.Atoi(flat.Sequence)
	} else {
		result.AccountNumber, _ = strconv.Atoi(raw.Account.BaseAccount.AccountNumber)
		result.Sequence, _ = strconv.Atoi(raw.Account.BaseAccount.Sequence)
	}

	writeJSON(w, http.StatusOK, result)
}

// handleBroadcast recebe tx_bytes assinados do APK (opção A) e faz broadcast.
func (s *Server) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, SendResponse{Error: "invalid json: " + err.Error(), Code: 1})
		return
	}
	if req.TxBytes == "" {
		writeJSON(w, http.StatusBadRequest, SendResponse{Error: "missing required: tx_bytes", Code: 1})
		return
	}

	mode := "sync"
	if m := strings.TrimSpace(req.Mode); m == "async" {
		mode = "async"
	}

	// O CLI Cosmos `tx broadcast` espera um CAMINHO para um arquivo com a tx
	// (não a string base64). O APK envia `tx_bytes` em base64; decodificamos e
	// escrevemos num arquivo temporário para passar como [file_path].
	rawTx, err := base64.StdEncoding.DecodeString(req.TxBytes)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, SendResponse{Error: "invalid tx_bytes base64: " + err.Error(), Code: 1})
		return
	}
	txFile, err := os.CreateTemp("", "luggom-tx-*.tx")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, SendResponse{Error: "create temp tx file: " + err.Error(), Code: 1})
		return
	}
	txFilePath := txFile.Name()
	defer os.Remove(txFilePath)
	if _, err := txFile.Write(rawTx); err != nil {
		txFile.Close()
		writeJSON(w, http.StatusInternalServerError, SendResponse{Error: "write temp tx file: " + err.Error(), Code: 1})
		return
	}
	if err := txFile.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, SendResponse{Error: "close temp tx file: " + err.Error(), Code: 1})
		return
	}

	args := []string{
		"tx", "broadcast", txFilePath,
		"--home", s.cfg.Home,
		"--node", s.cfg.Node,
		"-o", "json",
	}
	if mode == "async" {
		args = append(args, "--broadcast-mode", "async")
	} else {
		args = append(args, "--broadcast-mode", "block")
	}

	cmd := exec.Command(s.cfg.Bin, args...)
	cmd.Env = os.Environ()
	out, runErr := cmd.CombinedOutput()

	outStr := strings.TrimSpace(string(out))
	if idx := strings.LastIndex(outStr, "{"); idx >= 0 {
		outStr = outStr[idx:]
	}
	var result struct {
		TxHash string `json:"txhash"`
		Code   int    `json:"code"`
	}
	if err := json.Unmarshal([]byte(outStr), &result); err != nil {
		if runErr != nil {
			writeJSON(w, http.StatusInternalServerError, SendResponse{
				Error: fmt.Sprintf("broadcast: %v\n%s", runErr, string(out)),
				Code:  1,
			})
			return
		}
		writeJSON(w, http.StatusInternalServerError, SendResponse{
			Error: fmt.Sprintf("parse output: %v (raw: %s)", err, outStr),
			Code:  1,
		})
		return
	}
	if result.Code != 0 {
		log.Printf("broadcast tx failed code=%d txhash=%s", result.Code, result.TxHash)
		writeJSON(w, http.StatusBadRequest, SendResponse{
			TxHash: result.TxHash,
			Code:   result.Code,
			Error:  "tx failed on-chain (code=" + strconv.Itoa(result.Code) + ")",
		})
		return
	}

	log.Printf("broadcast success txhash=%s", result.TxHash)
	writeJSON(w, http.StatusOK, SendResponse{TxHash: result.TxHash, Code: 0})
}

// handleBackupEmail — Passo 10: envia backup cifrado por email.
func (s *Server) handleBackupEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BackupEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json: " + err.Error()})
		return
	}

	email := strings.TrimSpace(strings.ToLower(req.Email))
	if email == "" || !strings.Contains(email, "@") {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email inválido"})
		return
	}
	if req.LuggomAddress == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "luggom_address obrigatório"})
		return
	}

	emailHash := fmt.Sprintf("%x", sha256.Sum256([]byte(email)))[:16]
	// §13.146 (review multi-agente): cortar só se houver o que cortar. Antes era
	// `req.LuggomAddress[:12]` direto — e um endereço com menos de 12 caracteres
	// fazia PANIC (slice bounds out of range) num endpoint público.
	addrHint := req.LuggomAddress
	if len(addrHint) > 12 {
		addrHint = addrHint[:12] + "..."
	}

	if !checkBackupRateLimit(email) {
		log.Printf("backup-email rate-limit hit email_hash=%s", emailHash)
		writeJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "rate-limit: 5 emails/hora por endereço",
		})
		return
	}

	if req.ConfirmCode == "" {
		code := fmt.Sprintf("%06d", rand.Intn(1000000))
		backupConfirmCodes.mu.Lock()
		backupConfirmCodes.codes[email] = code
		backupConfirmCodes.mu.Unlock()
		log.Printf("backup-email confirm code email_hash=%s addr=%s code=%s", emailHash, addrHint, code)
		writeJSON(w, http.StatusOK, map[string]string{
			"status":       "confirm_code_sent",
			"confirm_code": code,
		})
		return
	}

	backupConfirmCodes.mu.Lock()
	expected := backupConfirmCodes.codes[email]
	if expected == "" || req.ConfirmCode != expected {
		backupConfirmCodes.mu.Unlock()
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "código inválido"})
		return
	}
	delete(backupConfirmCodes.codes, email)
	backupConfirmCodes.mu.Unlock()

	if len(req.CiphertextB64) > 350000 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "ficheiro > 256 KB"})
		return
	}

	smtpDisabled := os.Getenv("LUGGOM_SMTP_DISABLED") == "1"
	from := getEnv("LUGGOM_SMTP_FROM", "backup@luggom.local")
	smtpHost := getEnv("LUGGOM_SMTP_HOST", "localhost")
	smtpPort := getEnv("LUGGOM_SMTP_PORT", "25")
	emlDir := getEnv("LUGGOM_EML_DIR", os.TempDir())

	subject := "Luggom — backup cifrado da sua carteira"
	body := "Backup da carteira " + req.LuggomAddress + ".\r\n\r\n" +
		"Ficheiro anexo: backup.luggom-backup (cifrado).\r\n" +
		"O servidor nunca ve a sua senha nem as suas 24 palavras.\r\n"

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n"+
		"MIME-Version: 1.0\r\n"+
		"Content-Type: multipart/mixed; boundary=\"luggom1\"\r\n\r\n"+
		"--luggom1\r\nContent-Type: text/plain; charset=utf-8\r\n\r\n%s\r\n"+
		"--luggom1\r\nContent-Type: application/octet-stream\r\n"+
		"Content-Transfer-Encoding: base64\r\n"+
		"Content-Disposition: attachment; filename=\"backup.luggom-backup\"\r\n\r\n"+
		"%s\r\n"+
		"--luggom1--\r\n",
		from, email, subject, body, req.CiphertextB64)

	if smtpDisabled {
		emlFile := filepath.Join(emlDir, fmt.Sprintf("luggom-backup-%s-%d.eml", emailHash, time.Now().Unix()))
		if err := os.WriteFile(emlFile, []byte(msg), 0644); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "write .eml failed"})
			return
		}
		log.Printf("backup-email .eml saved %s email_hash=%s", emlFile, emailHash)
	} else {
		auth := smtp.PlainAuth("", getEnv("LUGGOM_SMTP_USER", ""), getEnv("LUGGOM_SMTP_PASS", ""), smtpHost)
		if err := smtp.SendMail(smtpHost+":"+smtpPort, auth, from, []string{email}, []byte(msg)); err != nil {
			log.Printf("backup-email SMTP failed: %v email_hash=%s", err, emailHash)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "SMTP send failed"})
			return
		}
		log.Printf("backup-email SMTP sent email_hash=%s", emailHash)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "sent"})
}

func checkBackupRateLimit(email string) bool {
	backupRateLimit.mu.Lock()
	defer backupRateLimit.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-1 * time.Hour)
	times := backupRateLimit.buckets[email]

	var recent []time.Time
	for _, t := range times {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 5 {
		backupRateLimit.buckets[email] = recent
		return false
	}
	recent = append(recent, now)
	backupRateLimit.buckets[email] = recent
	return true
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// MineTaxMeta guarda metadados da mineração para enriquecer o extrato (Carteira).
type MineTaxMeta struct {
	FiatAmount  string `json:"fiat_amount,omitempty"`
	Currency   string `json:"currency,omitempty"`
	TaxEntity  string `json:"tax_entity,omitempty"`
	Bank       string `json:"bank,omitempty"`
	Session    string `json:"session,omitempty"`
	DestWallet string `json:"dest_address,omitempty"`
}

// Server fields para meta das txs de mineração. Persistido em arquivo JSON
// (sobrevive a reinícios da API — o map em memória sozinho perdia as metas ao
// reiniciar, fazendo o extrato exibir dados errados/incompletos).
// mineTaxMetaMap é populado no handleMineTaxPayment e lido no handleTransactions.
var (
	mineTaxMetaMu   sync.Mutex
	mineTaxMetaMap  = map[string]MineTaxMeta{}
	mineTaxMetaFile = "" // definido em setupMineTaxMetaFile (caminho do arquivo)
)

// setupMineTaxMetaFile define o caminho do arquivo de persistência e carrega o
// map a partir dele (se existir). Chamado na inicialização do servidor.
func (s *Server) setupMineTaxMetaFile() {
	if s.cfg.Home == "" {
		return
	}
	mineTaxMetaFile = filepath.Join(s.cfg.Home, "luggom-minemeta.json")
	data, err := os.ReadFile(mineTaxMetaFile)
	if err != nil {
		return // arquivo ainda não existe (primeira vez)
	}
	mineTaxMetaMu.Lock()
	defer mineTaxMetaMu.Unlock()
	_ = json.Unmarshal(data, &mineTaxMetaMap)
}

// saveMineTaxMeta persiste o map em disco (best-effort; erros só logam).
func (s *Server) saveMineTaxMeta() {
	if mineTaxMetaFile == "" {
		return
	}
	mineTaxMetaMu.Lock()
	data, err := json.MarshalIndent(mineTaxMetaMap, "", "  ")
	mineTaxMetaMu.Unlock()
	if err != nil {
		return
	}
	if err := os.WriteFile(mineTaxMetaFile, data, 0o600); err != nil {
		log.Printf("warn: save mineTaxMeta file: %v", err)
	}
}

func (s *Server) storeMineTaxMeta(txHash string, meta MineTaxMeta) {
	if txHash == "" {
		return
	}
	mineTaxMetaMu.Lock()
	mineTaxMetaMap[txHash] = meta
	mineTaxMetaMu.Unlock()
	s.saveMineTaxMeta()
}

func (s *Server) getMineTaxMeta(txHash string) (MineTaxMeta, bool) {
	mineTaxMetaMu.Lock()
	defer mineTaxMetaMu.Unlock()
	m, ok := mineTaxMetaMap[txHash]
	return m, ok
}

// snapshotMineTaxMeta devolve uma cópia do map de metas (para iteração segura).
func (s *Server) snapshotMineTaxMeta() map[string]MineTaxMeta {
	mineTaxMetaMu.Lock()
	defer mineTaxMetaMu.Unlock()
	out := make(map[string]MineTaxMeta, len(mineTaxMetaMap))
	for k, v := range mineTaxMetaMap {
		out[k] = v
	}
	return out
}

// serviceAccountAddress devolve o endereço da conta de serviço (keyring) da API.
func (s *Server) serviceAccountAddress() string {
	addr, err := s.resolveAddress(s.cfg.Keyring)
	if err != nil {
		return ""
	}
	return addr
}

// --- TLSNotary / Reclaim Protocol (Fase B) ---

// notaryRequiredError diz se a mineração pode ser atestada. §13.145 (A5/A10):
// fail-closed — sem notário configurado, a API NÃO assina nada (ver o uso em
// handleMineTaxPayment). Extraído para função própria para ser testável: o
// handler não tem cobertura HTTP.
func notaryRequiredError(notaryURL string) error {
	if strings.TrimSpace(notaryURL) == "" {
		return fmt.Errorf("notary not configured: mining requires a TLSNotary notary (set LUGGOM_NOTARY_URL)")
	}
	return nil
}

// handleMineTaxPayment sync: POST /api/v1/mine/tax
func (s *Server) handleMineTaxPayment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("mine/tax: read body: %v", err)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: err.Error(), Code: 1})
		return
	}
	var req MineTaxPaymentRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		log.Printf("mine/tax: invalid json: %v (raw: %.200s)", err, string(raw))
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: "invalid json: " + err.Error(), Code: 1})
		return
	}
	log.Printf("mine/tax: req provider=%q dest=%q has_tlsn_proof=%v pubkey_len=%d", req.ProviderId, req.DestAddress, req.TlsnProof != nil, len(req.PublicKey))
	if req.TlsnProof == nil || req.ProviderId == "" {
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{
			Error: "missing tlsn_proof and provider_id (o bundle TLSNotary real é obrigatório)",
			Code:  1,
		})
		return
	}
	if req.DestAddress == "" {
		log.Printf("mine/tax: missing dest_address")
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: "missing dest_address", Code: 1})
		return
	}

	// 1. Verify proof — SÓ o bundle REAL TLSNotary (prover UniFFI, §13.58 passo 2).
	//    As claims são extraídas do CORPO ATESTADO (selective disclosure) e o
	//    anti-replay ancora no hash do transcript criptográfico.
	//
	//    O caminho legacy `reclaim_proof` (JSON) foi REMOVIDO: era `json.Unmarshal`
	//    sem qualquer verificação criptográfica, portanto forjável por qualquer um.
	//    Como a API é o único guardião (assina a attestation que a chain confia),
	//    esse caminho invalidava o portão da mineração e o "1 prova = 1 conta".
	claims, proofCommitment, err := taxproof.VerifyTlsnProof(req.ProviderId, taxproof.Bundle{
		SessionID:    req.TlsnProof.SessionID,
		ServerName:   req.TlsnProof.ServerName,
		Status:       req.TlsnProof.Status,
		ResponseBody: req.TlsnProof.ResponseBody,
		Transcript: taxproof.Transcript{
			Sent: req.TlsnProof.Transcript.Sent,
			Recv: req.TlsnProof.Transcript.Recv,
		},
	})
	if err != nil {
		log.Printf("mine/tax: VerifyTlsnProof: %v", err)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: "tlsn proof validation: " + err.Error(), Code: 1})
		return
	}

	// 1a. FASE 5.2: provider must be in the ON-CHAIN bank_providers whitelist.
	// (gov-67% editable — a removed bank is cut at the root.)
	//
	// Fail-closed (A-4, hard test): se a query on-chain FALHAR, não se salta o
	// teste — recusa-se. Antes, uma falha de leitura deixava passar qualquer
	// `provider_id` (defesa em profundidade perdida); a chain ainda recusaria com
	// 1215, mas a API não deve construir uma tx que já sabe que vai falhar.
	onChain, err := s.onChainBankProviders()
	if err != nil {
		log.Printf("mine/tax: onChainBankProviders: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, MineTaxPaymentResponse{
			Error: "cannot verify bank provider whitelist (on-chain params unavailable) — refusing to mine (fail-closed)",
			Code:  1,
		})
		return
	}
	if !onChain[req.ProviderId] {
		writeJSON(w, http.StatusForbidden, MineTaxPaymentResponse{
			Error: "bank provider not whitelisted on-chain: " + req.ProviderId,
			Code:  1208,
		})
		return
	}

	// 1b. Tax-nature extraction (anti-cycle, §13.34): the proof must demonstrate
	// a real tax collection (DARF/IPTU/IPVA, property tax bill, CRA BN+RT).
	// A common PIX/transfer to a whitelisted CNPJ/EIN fails here → no hash →
	// the chain rejects the tx with ErrMissingTaxNature (1207).
	natureFrag, err := taxnature.Extract(req.ProviderId, taxnature.Claims{
		ProviderID:    req.ProviderId,
		Recipient:     claims.RecipientAccount,
		RecipientName: claims.RecipientName,
		Payer:         claims.PayerAccount,
		PayerTaxID:    claims.PayerTaxID,
		Description:   claims.Description, // tax-nature fragment (barcode/PIX arrecadação/bill) — FASE 4
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{
			Error: "payment is not a tax collection: " + err.Error(),
			Code:  1207,
		})
		return
	}
	taxNatureHash := taxnature.Hash(natureFrag)

	// 3. Resolve os campos canónicos (A1.4: o digest cobre TODOS os campos).
	//
	//    FASE B (§13.121): se o notário estiver configurado, é ELE que extrai e
	//    assina — a API busca a atestação por `session_id` e usa OS CAMPOS DO
	//    NOTÁRIO, deixando de poder inventar o valor. A assinatura que vai na tx
	//    é a do notário; `params.api_verifier_address` é o endereço dele.
	//
	//    Sem notário (dev local), mantém-se o caminho antigo: a API assina.
	fields := types.AttestationFields{
		// v4: o destinatário e a chain entram no digest. Aqui só servem para a
		// comparação do proof_commitment abaixo — os campos que vão na tx são os
		// do notário (bloco seguinte).
		MinerAddress:    req.DestAddress,
		ChainID:         s.cfg.ChainID,
		ProofCommitment: proofCommitment,
		PaymentID:       claims.TransactionID,
		PaymentAmount:        claims.PaymentAmount,
		TaxEntity:       claims.TaxEntityMatch,
		CurrencyCode:    claims.PaymentCurrency,
		PaymentDate:     claims.PaymentDate,
		TaxNatureHash:   taxNatureHash,
		PayerTaxID:      claims.PayerTaxID,
		// O banco vai no digest (§13.131 A4): a chain aplica a whitelist on-chain
		// sobre este valor, e o notário assina-o — a API não o pode trocar depois.
		ProviderID: req.ProviderId,
	}

	// §13.145 (A5/A10): fail-closed — sem notário, NÃO se mina.
	//
	// A API a assinar o que ela própria extraiu de um transcript que o CLIENTE
	// enviou não prova nada: o cliente controla aqueles bytes, logo podia inventar
	// o comprovante. Era este o caminho que tornava a mineração arbitrária quando
	// faltava configuração. A atestação tem de vir do notário — que a extraiu da
	// sessão que ELE verificou.
	if err := notaryRequiredError(s.cfg.NotaryURL); err != nil {
		log.Printf("mine/tax: notaryRequiredError: %v", err)
		writeJSON(w, http.StatusServiceUnavailable, MineTaxPaymentResponse{Error: err.Error(), Code: 1})
		return
	}

	nFields, nSig, err := s.fetchNotaryAttestation(req.TlsnProof.SessionID)
	if err != nil {
		log.Printf("mine/tax: notary attestation: %v", err)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{
			Error: "notary attestation: " + err.Error(),
			Code:  1,
		})
		return
	}
	// O proof_commitment do notário tem de ser o mesmo do transcript submetido
	// — garante que a atestação corresponde à prova que o cliente enviou.
	if nFields.ProofCommitment != fields.ProofCommitment {
		log.Printf("mine/tax: proof_commitment mismatch: notary=%s submitted=%s", nFields.ProofCommitment, fields.ProofCommitment)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{
			Error: "notary attestation does not match the submitted proof (proof_commitment mismatch)",
			Code:  1,
		})
		return
	}
	// v4: a atestação nomeia o destinatário da cunhagem e a chain onde vale.
	// Se não baterem com a tx que estamos a montar, a chain recusa a assinatura
	// — recusa-se aqui, com um erro claro, em vez de devolver uma tx morta.
	if strings.TrimSpace(nFields.MinerAddress) != strings.TrimSpace(req.DestAddress) {
		log.Printf("mine/tax: notary miner_address mismatch: notary=%s dest=%s", nFields.MinerAddress, req.DestAddress)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{
			Error: "notary attestation was signed for a different miner_address (v4 binds the recipient)",
			Code:  1,
		})
		return
	}
	if strings.TrimSpace(nFields.ChainID) != strings.TrimSpace(s.cfg.ChainID) {
		log.Printf("mine/tax: notary chain_id mismatch: notary=%s api=%s", nFields.ChainID, s.cfg.ChainID)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{
			Error: "notary attestation was signed for a different chain_id (v4 binds the chain)",
			Code:  1,
		})
		return
	}
	fields = *nFields
	apiAttestation := nSig

	// 3b. Block government-to-government mining (anti-inflation)
	if fields.PayerTaxID != "" && isGovernmentPayer(fields.PayerTaxID) {
		writeJSON(w, http.StatusForbidden, MineTaxPaymentResponse{
			Error: "government entities cannot mine LUG (EIN/CNPJ-to-EIN/CNPJ transfer blocked)",
			Code:  1206,
		})
		return
	}

	// 4. Monta a tx NÃO assinada (OPÇÃO 1, §13.97): o `miner_address` é a
	//    carteira do utilizador, que assina localmente. A API nunca vê a seed
	//    nem a chave privada — só a chave pública (para o AuthInfo).
	pubKey, err := parsePublicKey(req.PublicKey)
	if err != nil {
		log.Printf("mine/tax: parsePublicKey: %v", err)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: err.Error(), Code: 1})
		return
	}
	signerAddr, err := sdk.AccAddressFromBech32(req.DestAddress)
	if err != nil {
		log.Printf("mine/tax: dest_address: %v", err)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: "dest_address: " + err.Error(), Code: 1})
		return
	}
	// Comparar por BYTES (ver pubKeyMatchesAddress): `pubKey.Address()` devolve os
	// 20 bytes crus e o seu .String() é HEX, não bech32.
	if !pubKeyMatchesAddress(pubKey, signerAddr) {
		log.Printf("mine/tax: public_key does not match dest_address (pubkey addr=%x, dest=%s)", pubKey.Address(), signerAddr.String())
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{
			Error: "public_key does not match dest_address (the tx would be rejected on-chain)",
			Code:  1,
		})
		return
	}

	// 4b. Ativação da carteira (D1–D7): a conta tem de existir e ter saldo para a
	//     taxa, senão não há `account_number` para assinar e a tx falharia ao
	//     cobrar. Só aqui — DEPOIS de a prova real ter sido validada acima — é
	//     que o pó sai da reserva, para o abuso ficar preso a uma prova válida.
	//     (O anti-replay garante 1 prova = 1 ativação; ver usado em /broadcast.)
	if err := s.ensureMinerAccount(signerAddr.String(), clientIPFromRequest(r)); err != nil {
		log.Printf("mine/tax: ensureMinerAccount: %v", err)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: err.Error(), Code: 1})
		return
	}

	accNum, seq, err := s.accountNumberSequence(signerAddr.String())
	if err != nil {
		log.Printf("mine/tax: accountNumberSequence: %v", err)
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: err.Error(), Code: 1})
		return
	}

	unsigned, err := s.buildUnsignedMineTaxTx(broadcastMineTaxArgs{
		PaymentAmount:        fields.PaymentAmount,
		TaxEntity:       fields.TaxEntity,
		CurrencyCode:    fields.CurrencyCode,
		ProofCommitment: fields.ProofCommitment,
		PaymentId:       fields.PaymentID,
		ApiAttestation:  apiAttestation,
		PaymentDate:     fields.PaymentDate,
		PayerTaxId:      fields.PayerTaxID,
		TaxNatureHash:   fields.TaxNatureHash,
		ProviderId:      fields.ProviderID,
	}, signerAddr.String(), pubKey, accNum, seq)
	if err != nil {
		log.Printf("mine/tax: buildUnsignedMineTaxTx: %v", err)
		writeJSON(w, http.StatusInternalServerError, MineTaxPaymentResponse{Error: "build tx: " + err.Error(), Code: 1})
		return
	}

	// A meta só pode ser gravada pelo txhash, que só existe no passo 2.
	// Guarda-a pendente pelo payment_id (anti-replay garante unicidade).
	s.storePendingMineMeta(fields.PaymentID, MineTaxMeta{
		FiatAmount:  fields.PaymentAmount,
		Currency:   fields.CurrencyCode,
		TaxEntity:  fields.TaxEntity,
		Bank:       claims.BankIdentity,
		Session:    req.ProviderId,
		DestWallet: signerAddr.String(),
	})

	writeJSON(w, http.StatusOK, MineTaxPaymentResponse{
		PaymentAmount:  fields.PaymentAmount,
		TaxEntity: fields.TaxEntity,
		PaymentId: fields.PaymentID,
		Code:      0,
		Unsigned:  unsigned,
	})
}

// notaryAttestationResponse é o que o notário devolve em GET /attestation.
type notaryAttestationResponse struct {
	ServerName string                  `json:"server_name"`
	Fields     types.AttestationFields `json:"fields"`
	Signature  string                  `json:"signature"`
	Error      string                  `json:"error"`
}

// fetchNotaryAttestation busca a atestação assinada pelo notário (§13.121 Fase B).
// Fail-closed: qualquer falha → erro → a mineração é rejeitada.
func (s *Server) fetchNotaryAttestation(sessionID string) (*types.AttestationFields, string, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, "", fmt.Errorf("missing session_id")
	}
	endpoint := strings.TrimRight(s.cfg.NotaryURL, "/") + "/attestation?sessionId=" + url.QueryEscape(sessionID)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("cannot reach notary at %s: %w", s.cfg.NotaryURL, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("notary returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var out notaryAttestationResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, "", fmt.Errorf("bad notary response: %w", err)
	}
	if out.Error != "" {
		return nil, "", fmt.Errorf("%s", out.Error)
	}
	if strings.TrimSpace(out.Signature) == "" {
		return nil, "", fmt.Errorf("notary returned no signature")
	}
	return &out.Fields, out.Signature, nil
}

// handleReclaimProviders: GET /api/v1/reclaim/providers
// FASE 5.2: providers are filtered by the ON-CHAIN bank_providers whitelist
// (gov-67% editable). Falls back to the full hardcoded list if the chain query
// fails (the APK still needs a list during bootstrap/mock).
func (s *Server) handleReclaimProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	all := reclaimverify.KnownProviders()
	onChain, err := s.onChainBankProviders()
	if err != nil {
		// A-4 (hard test): fail-closed. Antes caía na lista COMPLETA hardcoded,
		// anunciando bancos que a governança pode ter removido on-chain.
		log.Printf("on-chain bank_providers query failed — refusing the list (fail-closed): %v", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]interface{}{
			"error": "cannot read the on-chain bank_providers whitelist (fail-closed)",
		})
		return
	}
	filtered := make([]reclaimverify.ProviderInfo, 0, len(all))
	for _, p := range all {
		if onChain[p.ID] {
			filtered = append(filtered, p)
		}
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"providers": filtered})
}

type broadcastMineTaxArgs struct {
	PaymentAmount        string
	TaxEntity       string
	CurrencyCode    string
	ProofCommitment string
	PaymentId       string
	ApiAttestation  string
	PaymentDate     int64
	PayerTaxId      string
	TaxNatureHash   string
	// ProviderId é o BANCO atestado (§13.131 A4) — a chain verifica-o contra a
	// whitelist on-chain; a API só relaya o que o notário assinou.
	ProviderId string
}

// registerRoutes regista as rotas da API. Extraído do main() para ser testável.
// Regressão do hard test: `/api/v1/send_mnemonic` (A-1 — aceitava uma seed
// BIP39 sem autenticação) foi REMOVIDO e não deve voltar.
func registerRoutes(mux *http.ServeMux, s *Server) {
	mux.HandleFunc("/api/v1/mine/tax", s.handleMineTaxPayment)
	mux.HandleFunc("/api/v1/mine/tax/broadcast", s.handleMineTaxBroadcast)
	mux.HandleFunc("/api/v1/reclaim/providers", s.handleReclaimProviders)
	mux.HandleFunc("/api/v1/chain/info", s.handleChainInfo)
	mux.HandleFunc("/api/v1/gov/luggom", s.handleGovLuggom)
	mux.HandleFunc("/api/v1/balance/", s.handleBalance)
	mux.HandleFunc("/api/v1/account/", s.handleAccount)
	mux.HandleFunc("/api/v1/status", s.handleStatus)
	mux.HandleFunc("/api/v1/send", s.handleSend)
	mux.HandleFunc("/api/v1/broadcast", s.handleBroadcast)
	mux.HandleFunc("/api/v1/transactions/", s.handleTransactions)
	mux.HandleFunc("/api/v1/escrow/lab/status", s.handleEscrowLabStatus)
	mux.HandleFunc("/api/v1/escrow/lab/offers", s.handleEscrowLabOffersList)
	mux.HandleFunc("/api/v1/escrow/lab/offers/create", s.handleEscrowLabOffersCreate)
	mux.HandleFunc("/api/v1/escrow/lab/offers/accept", s.handleEscrowLabOffersAccept)
	mux.HandleFunc("/api/v1/escrow/lab/offers/cancel", s.handleEscrowLabOffersCancel)
	mux.HandleFunc("/api/v1/notify/backup-email", s.handleBackupEmail)
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
}

func main() {
	cfg := loadConfig()
	setupSDKConfig(cfg.Bech32Prefix) // prefixos bech32 luggom... (validação de endereços)
	s := &Server{
		cfg:         cfg,
		escrow:      escrowlab.NewStore(),
		pendingMeta: map[string]MineTaxMeta{},
	}
	s.setupMineTaxMetaFile() // carrega metas persistidas (extrato correto após reinício)
	s.setupActivationStore() // carrega as ativações já feitas (1 pó por carteira)
	// §13.123: em Fase B quem assina é o NOTÁRIO — `params.api_verifier_address`
	// tem de ser o endereço DELE. Sem notário configurado, assina a API (dev).
	if cfg.NotaryURL != "" {
		log.Printf("attestation: Fase B ativa — o NOTÁRIO assina (notary=%s).", cfg.NotaryURL)
		log.Printf("  → `params.api_verifier_address` tem de ser o endereço do NOTÁRIO: obtenha com `luggom-extract --print-address <LUGGOM_NOTARY_KEY_HEX>`")
	} else if addr, err := reclaimverify.AttestationSignerAddress(cfg.ApiVerifierKeyHex); err != nil {
		log.Printf("⚠️  attestation key not usable (chain phase 2 will reject): %v", err)
	} else {
		log.Printf("attestation: a API assina (sem notário) — endereço: %s", addr)
	}
	// Mineração com custódia do minerador (OPÇÃO 1): passo 1 devolve a tx não
	// assinada; passo 2 recebe a tx assinada localmente e só faz broadcast.
	registerRoutes(http.DefaultServeMux, s)
	addr := ":" + cfg.Port
	log.Printf("luggom-api listening on %s (home=%s bin=%s data_provider=%s escrow_lab=%v)", addr, cfg.Home, cfg.Bin, cfg.dataProviderLabel(), cfg.EscrowLab)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}

// isGovernmentPayer checks if the payer EIN/CNPJ/BN belongs to a government entity.
// Blocks government-to-government transfers from mining LUG (anti-inflation).
// Mirrors the on-chain rule (FASE 4-A..F):
//
//	(1) government blacklist (types.DefaultGovernmentEntities — same as params default)
//	(2) fallback: payer ID matches any whitelisted tax entity (tax_entity_ids.go)
func isGovernmentPayer(payerTaxID string) bool {
	normalized := types.NormalizeTaxID(payerTaxID)
	if normalized == "" {
		return false
	}
	// Layer 1: government blacklist (same default as params.GovernmentEntityBlacklist)
	if types.IsGovernmentEntity(normalized, types.DefaultGovernmentEntities()) {
		return true
	}
	// Layer 2: fallback — payer belongs to a whitelisted tax entity
	return types.IsIDOfWhitelistedEntity(normalized)
}
