package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	"github.com/cosmos/cosmos-sdk/std"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txv1beta1 "github.com/cosmos/cosmos-sdk/types/tx"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	txauth "github.com/cosmos/cosmos-sdk/x/auth/tx"
	"github.com/cosmos/gogoproto/proto"

	"luggom/x/luggom/types"
)

// Fluxo de mineração com custódia do minerador ("OPÇÃO 1", §13.97 / F2 §4):
//
//	POST /api/v1/mine/tax           → verifica a prova e devolve a tx NÃO assinada
//	POST /api/v1/mine/tax/broadcast → recebe a tx assinada e SÓ faz broadcast
//
// O `miner_address` é a carteira do utilizador (nunca uma chave da API), pelo que
// o LUG cai directamente na carteira — não existe `bank send` de encaminhamento.
// A seed nunca sai do aparelho: o cliente assina o sign-doc localmente.
const (
	// mineTaxGasLimit é o gás fixo da tx de mineração (§13.98 pattern: evita `auto`).
	mineTaxGasLimit = 1_000_000
	// mineTaxGasPriceUluggom é o preço mínimo de gás da chain (§13.87: 1uluggom
	// fecha txs 100% grátis). A taxa é gás × preço — abaixo disto a tx é rejeitada.
	mineTaxGasPriceUluggom = 1
	// mineTaxFeeUluggom é a taxa da tx em uluggom: 1e6 uluggom = 0.000000000001 LUG
	// (gás 1e6 × 1uluggom/gás), igual ao que a API pagava antes. É irrisória, mas
	// NÃO pode ser zero: a chain rejeita taxa abaixo do mínimo.
	//
	// Nota: agora é o MINERADOR que a paga (antes era a chave da API). Como a
	// assinatura exige uma conta on-chain existente, a carteira nova tem de
	// receber primeiro um valor mínimo — ver a ativação no handler.
	mineTaxFeeUluggom = mineTaxGasLimit * mineTaxGasPriceUluggom
)

var (
	txEncOnce sync.Once
	txEnc     client.TxConfig
	txReg     codectypes.InterfaceRegistry
	txEncErr  error

	sdkCfgOnce sync.Once
)

// setupSDKConfig fixa os prefixos bech32 do SDK (luggom...). É necessário para
// validar endereços e para a chave pública produzir o mesmo endereço do minerador.
// Chamar uma vez no arranque (e nos testes) antes de montar txs.
func setupSDKConfig(prefix string) {
	sdkCfgOnce.Do(func() {
		if strings.TrimSpace(prefix) == "" {
			prefix = defaultBech32Prefix
		}
		cfg := sdk.GetConfig()
		cfg.SetBech32PrefixForAccount(prefix, prefix+"pub")
		cfg.SetBech32PrefixForValidator(prefix+"valoper", prefix+"valoperpub")
		cfg.SetBech32PrefixForConsensusNode(prefix+"valcons", prefix+"valconspub")
	})
}

// clientTxConfig constrói (uma vez) um encoding config mínimo — std + crypto +
// luggom — suficiente para serializar MsgMineTaxPayment em TxBody/AuthInfo/SignDoc.
func clientTxConfig() (client.TxConfig, error) {
	txEncOnce.Do(func() {
		ir := codectypes.NewInterfaceRegistry()
		std.RegisterInterfaces(ir)
		cryptocodec.RegisterInterfaces(ir)
		types.RegisterInterfaces(ir)
		var cdc codec.Codec = codec.NewProtoCodec(ir)
		txReg = ir
		txEnc = txauth.NewTxConfig(cdc, txauth.DefaultSignModes)
	})
	return txEnc, txEncErr
}

// clientTxRegistry expõe o interface registry usado pelo TxConfig (para unpack de Any).
func clientTxRegistry() (codectypes.InterfaceRegistry, error) {
	if _, err := clientTxConfig(); err != nil {
		return nil, err
	}
	return txReg, nil
}

// accountNumberSequence consulta a chain pelo número de conta e sequência.
// A sequência é a que a tx assinada deve declarar no sign-doc.
func (s *Server) accountNumberSequence(address string) (uint64, uint64, error) {
	found, accNum, seq, err := s.queryAccount(address)
	if err != nil {
		return 0, 0, err
	}
	if !found {
		return 0, 0, fmt.Errorf("account %s does not exist on-chain (is it funded?)", address)
	}
	return accNum, seq, nil
}

// findStringFields procura recursivamente os primeiros valores (como string) de
// cada chave pedida num JSON aninhado. Devolve as chaves pela ordem pedida.
func findStringFields(v interface{}, keys ...string) []string {
	want := map[string]bool{}
	for _, k := range keys {
		want[k] = true
	}
	found := map[string]string{}

	var walk func(node interface{})
	walk = func(node interface{}) {
		switch n := node.(type) {
		case map[string]interface{}:
			for k, val := range n {
				if want[k] {
					if _, ok := found[k]; !ok {
						switch s := val.(type) {
						case string:
							found[k] = s
						case float64:
							found[k] = strconv.FormatInt(int64(s), 10)
						}
					}
				}
				walk(val)
			}
		case []interface{}:
			for _, item := range n {
				walk(item)
			}
		}
	}
	walk(v)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, found[k])
	}
	return out
}

// parsePublicKey descodifica a chave pública secp256k1 comprimida enviada pelo
// cliente. Aceita a chave crua (33 bytes, base64 ou hex) ou um proto Any (base64).
func parsePublicKey(raw string) (cryptotypes.PubKey, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return nil, fmt.Errorf("missing public_key")
	}

	// 1) proto Any (base64).
	if bz, err := base64.StdEncoding.DecodeString(v); err == nil {
		var any codectypes.Any
		if err := proto.Unmarshal(bz, &any); err == nil && any.TypeUrl != "" && len(any.Value) > 0 {
			if reg, err := clientTxRegistry(); err == nil {
				var pk cryptotypes.PubKey
				if err := reg.UnpackAny(&any, &pk); err == nil && pk != nil {
					if bz := pk.Bytes(); isCompressedSecp256k1(bz) {
						return pk, nil
					}
				}
			}
		}
		// 2) chave crua comprimida (33 bytes).
		if isCompressedSecp256k1(bz) {
			return &secp256k1.PubKey{Key: bz}, nil
		}
	}

	// 3) hex.
	if bz, err := hex.DecodeString(strings.TrimPrefix(strings.TrimPrefix(v, "0x"), "0X")); err == nil && isCompressedSecp256k1(bz) {
		return &secp256k1.PubKey{Key: bz}, nil
	}

	return nil, fmt.Errorf("public_key must be a compressed secp256k1 key (33 bytes) in base64/hex, or a proto Any in base64")
}

// isCompressedSecp256k1 verifica a forma de uma chave pública comprimida
// (33 bytes, prefixo 0x02/0x03). A validação criptográfica final é a da chain,
// que rejeita a assinatura se a chave não corresponder ao endereço.
func isCompressedSecp256k1(bz []byte) bool {
	return len(bz) == 33 && (bz[0] == 0x02 || bz[0] == 0x03)
}

// pubKeyMatchesAddress indica se a chave pública corresponde ao endereço.
//
// ⚠️ Compara BYTES. `pubKey.Address()` devolve os 20 bytes crus e o seu
// `.String()` é HEX (não bech32) — comparar strings rejeitaria sempre. Este bug
// existiu até ao E2E de 2026-09-10 (o teste unitário não o apanhava porque
// chamava o construtor da tx directamente, sem passar pelo handler).
func pubKeyMatchesAddress(pubKey cryptotypes.PubKey, addr sdk.AccAddress) bool {
	if pubKey == nil || addr == nil {
		return false
	}
	return sdk.AccAddress(pubKey.Address()).Equals(addr)
}

// UnsignedTx é o material devolvido ao cliente para assinatura local.
type UnsignedTx struct {
	// TxBytes é o TxRaw (protobuf, base64) ainda sem assinaturas.
	TxBytes string `json:"tx_bytes"`
	// BodyBytes e AuthInfoBytes são os componentes exactos do TxRaw.
	BodyBytes     string `json:"body_bytes"`
	AuthInfoBytes string `json:"auth_info_bytes"`
	// SignDocBytes é o que o cliente MUST hashear (SHA-256) e assinar com
	// secp256k1. É a fonte de verdade — não reconstruir a partir da tx.
	SignDocBytes string `json:"sign_doc_bytes"`
	// SignDocSHA256 é o digest que o ECDSA assina (conveniência).
	SignDocSHA256 string `json:"sign_doc_sha256"`
	SignerAddress string `json:"signer_address"`
	AccountNumber uint64 `json:"account_number"`
	Sequence      uint64 `json:"sequence"`
	ChainID       string `json:"chain_id"`
}

// buildUnsignedMineTaxTx monta o MsgMineTaxPayment e serializa TxBody, AuthInfo
// e SignDoc (SIGN_MODE_DIRECT) — sem assinar. O signer é a carteira do
// utilizador (miner_address), cuja chave pública entra no AuthInfo.
func (s *Server) buildUnsignedMineTaxTx(args broadcastMineTaxArgs, signerAddr string, pubKey cryptotypes.PubKey, accNum, seq uint64) (*UnsignedTx, error) {
	cfg, err := clientTxConfig()
	if err != nil {
		return nil, err
	}

	paymentAmount, err := sdkmath.LegacyNewDecFromStr(args.PaymentAmount)
	if err != nil {
		return nil, fmt.Errorf("payment_amount %q: %w", args.PaymentAmount, err)
	}

	msg := &types.MsgMineTaxPayment{
		MinerAddress:    signerAddr,
		PaymentAmount:        paymentAmount,
		TaxEntity:       args.TaxEntity,
		CurrencyCode:    args.CurrencyCode,
		ProofCommitment: args.ProofCommitment,
		PaymentId:       args.PaymentId,
		ApiAttestation:  args.ApiAttestation,
		PaymentDate:     args.PaymentDate,
		PayerTaxId:      args.PayerTaxId,
		TaxNatureHash:   args.TaxNatureHash,
		ProviderId:      args.ProviderId,
	}

	builder := cfg.NewTxBuilder()
	if err := builder.SetMsgs(msg); err != nil {
		return nil, fmt.Errorf("set msgs: %w", err)
	}
	builder.SetGasLimit(mineTaxGasLimit)
	builder.SetFeeAmount(sdk.NewCoins(sdk.NewCoin("uluggom", sdkmath.NewInt(mineTaxFeeUluggom))))
	builder.SetMemo("")
	// A assinatura entra depois (pelo cliente): aqui fica vazia, com a pubkey.
	if err := builder.SetSignatures(signingtypes.SignatureV2{
		PubKey:   pubKey,
		Data:     &signingtypes.SingleSignatureData{SignMode: signingtypes.SignMode_SIGN_MODE_DIRECT},
		Sequence: seq,
	}); err != nil {
		return nil, fmt.Errorf("set signatures: %w", err)
	}

	txRawBz, err := cfg.TxEncoder()(builder.GetTx())
	if err != nil {
		return nil, fmt.Errorf("encode tx: %w", err)
	}
	var raw txv1beta1.TxRaw
	if err := proto.Unmarshal(txRawBz, &raw); err != nil {
		return nil, fmt.Errorf("decode txraw: %w", err)
	}
	// A tx devolvida é genuinamente NÃO assinada: o SetSignatures acima só serve
	// para colocar a chave pública no AuthInfo. Remove-se o placeholder vazio.
	raw.Signatures = nil
	unsignedBz, err := proto.Marshal(&raw)
	if err != nil {
		return nil, fmt.Errorf("marshal unsigned tx: %w", err)
	}

	signDoc := &txv1beta1.SignDoc{
		BodyBytes:     raw.BodyBytes,
		AuthInfoBytes: raw.AuthInfoBytes,
		ChainId:       s.cfg.ChainID,
		AccountNumber: accNum,
	}
	signDocBz, err := proto.Marshal(signDoc)
	if err != nil {
		return nil, fmt.Errorf("marshal sign doc: %w", err)
	}
	digest := sha256.Sum256(signDocBz)

	return &UnsignedTx{
		TxBytes:       base64.StdEncoding.EncodeToString(unsignedBz),
		BodyBytes:     base64.StdEncoding.EncodeToString(raw.BodyBytes),
		AuthInfoBytes: base64.StdEncoding.EncodeToString(raw.AuthInfoBytes),
		SignDocBytes:  base64.StdEncoding.EncodeToString(signDocBz),
		SignDocSHA256: base64.StdEncoding.EncodeToString(digest[:]),
		SignerAddress: signerAddr,
		AccountNumber: accNum,
		Sequence:      seq,
		ChainID:       s.cfg.ChainID,
	}, nil
}

// MineTaxBroadcastRequest corpo do POST /api/v1/mine/tax/broadcast.
type MineTaxBroadcastRequest struct {
	// TxBytes é a tx ASSINADA localmente (TxRaw protobuf, base64).
	TxBytes string `json:"tx_bytes"`
	// PaymentId permite reencontrar a meta guardada no passo 1 (extrato).
	PaymentId string `json:"payment_id"`
	// Mode: "sync" (default) ou "async".
	Mode string `json:"mode,omitempty"`
}

// handleMineTaxBroadcast (4.2) recebe a tx já assinada pelo cliente e SÓ faz
// broadcast. A chave privada nunca passa por aqui.
func (s *Server) handleMineTaxBroadcast(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req MineTaxBroadcastRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: "invalid json: " + err.Error(), Code: 1})
		return
	}
	if strings.TrimSpace(req.TxBytes) == "" {
		writeJSON(w, http.StatusBadRequest, MineTaxPaymentResponse{Error: "missing tx_bytes", Code: 1})
		return
	}

	txHash, code, err := s.broadcastSignedTx(req.TxBytes, req.Mode)
	log.Printf("mine/tax/broadcast: txhash=%s code=%d err=%v txbytes_len=%d", txHash, code, err, len(req.TxBytes))

	// ⚠️ O broadcast é `sync`: devolve `code 0` quando a tx é ACEITE NO MEMPOOL,
	// mesmo que venha a ser REJEITADA no bloco (ex.: anti-replay 1202). Sem esta
	// consulta, a API devolvia sucesso a uma mineração que falhou — bug
	// encontrado no E2E de 2026-09-10. O code real só aparece no `query tx`.
	if err == nil && code == 0 && txHash != "" {
		if realCode, qErr := s.queryTxCode(txHash); qErr == nil && realCode != 0 {
			err = fmt.Errorf("tx failed on-chain (code=%d)", realCode)
			code = realCode
		}
	}

	if err != nil || code != 0 {
		httpStatus := http.StatusInternalServerError
		userCode := 1
		msg := "broadcast failed"
		if err != nil {
			msg = err.Error()
		}
		switch code {
		case 1202:
			httpStatus = http.StatusConflict
			userCode = 1202
			msg = "mineração negada: comprovante já utilizado (anti-replay). Cada comprovante só minera uma vez — use um comprovante novo."
		case 1206:
			httpStatus = http.StatusForbidden
			userCode = 1206
			msg = "mineração negada: entidades governamentais não podem minerar LUG (transferência EIN/CNPJ bloqueada)."
		case 1207:
			httpStatus = http.StatusBadRequest
			userCode = 1207
			msg = "mineração negada: comprovante fora da whitelist fiscal (não é arrecadação de imposto)."
		case 1208:
			httpStatus = http.StatusForbidden
			userCode = 1208
			msg = "mineração negada: instituição financeira fora da whitelist on-chain."
		}
		writeJSON(w, httpStatus, MineTaxPaymentResponse{TxHash: txHash, Error: msg, Code: userCode})
		return
	}

	amountMinedLug, err := s.queryMineAmountMined(txHash)
	if err != nil {
		log.Printf("query mine amount_mined (%s): %v", txHash, err)
	}

	// Fixa a meta do extrato ao txhash real (guardada pendente no passo 1).
	if meta, ok := s.takePendingMineMeta(strings.TrimSpace(req.PaymentId)); ok {
		s.storeMineTaxMeta(txHash, meta)
	}

	log.Printf("mine/tax broadcast ok txhash=%s", txHash)
	writeJSON(w, http.StatusOK, MineTaxPaymentResponse{
		TxHash:      txHash,
		AmountMined: amountMinedLug,
		Code:        0,
	})
}

// broadcastSignedTx faz broadcast de uma tx já assinada.
//
// ⚠️ O CLI `tx broadcast <ficheiro>` usa `TxJSONDecoder()` — aceita **só JSON**,
// nunca protobuf binário (ver deps/cosmos-sdk/x/auth/client/tx.go:ReadTxFromFile).
// É a origem do problema registado no §13.97 ("/broadcast só aceita JSON").
// O cliente assina e devolve TxRaw BINÁRIO (base64); aqui convertemos para o
// JSON canónico do Cosmos antes de escrever o ficheiro.
func (s *Server) broadcastSignedTx(txBytesB64, mode string) (txHash string, code int, err error) {
	rawTx, err := base64.StdEncoding.DecodeString(strings.TrimSpace(txBytesB64))
	if err != nil {
		return "", 1, fmt.Errorf("invalid tx_bytes base64: %w", err)
	}

	txJSON, err := txRawToJSON(rawTx)
	if err != nil {
		return "", 1, err
	}

	// O CLI espera um CAMINHO de arquivo, não o conteúdo.
	tmp, err := os.CreateTemp("", "luggom-minetax-*.json")
	if err != nil {
		return "", 1, fmt.Errorf("create temp tx: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(txJSON); err != nil {
		tmp.Close()
		return "", 1, fmt.Errorf("write temp tx: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", 1, fmt.Errorf("close temp tx: %w", err)
	}

	broadcastMode := "sync"
	if strings.TrimSpace(mode) == "async" {
		broadcastMode = "async"
	}
	args := []string{
		"tx", "broadcast", tmp.Name(),
		"--home", s.cfg.Home,
		"--node", s.cfg.Node,
		"--broadcast-mode", broadcastMode,
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
		Logs   []struct {
			Code int    `json:"code"`
			Log  string `json:"log"`
		} `json:"logs"`
		RawLog string `json:"raw_log"`
	}
	if err := json.Unmarshal([]byte(outStr), &result); err != nil {
		if runErr != nil {
			return "", 1, fmt.Errorf("broadcast: %w: %s", runErr, strings.TrimSpace(string(out)))
		}
		return "", 1, fmt.Errorf("parse broadcast output: %w (raw: %s)", err, outStr)
	}
	txCode := result.Code
	if txCode == 0 {
		for _, l := range result.Logs {
			if l.Code != 0 {
				txCode = l.Code
				break
			}
		}
	}
	if txCode != 0 {
		msg := strings.TrimSpace(result.RawLog)
		if msg == "" && len(result.Logs) > 0 {
			msg = result.Logs[0].Log
		}
		return result.TxHash, txCode, fmt.Errorf("tx failed (code=%d): %s", txCode, msg)
	}
	return result.TxHash, result.Code, nil
}

// txRawToJSON converte um TxRaw binário (assinado) para o JSON canónico do Cosmos,
// que é o único formato que o CLI `tx broadcast` aceita. Se o input já for JSON,
// devolve-o inalterado (tolerância a clientes que enviem JSON).
func txRawToJSON(rawTx []byte) ([]byte, error) {
	trimmed := bytes.TrimSpace(rawTx)
	if len(trimmed) > 0 && trimmed[0] == '{' {
		return trimmed, nil // já é JSON
	}

	cfg, err := clientTxConfig()
	if err != nil {
		return nil, err
	}
	tx, err := cfg.TxDecoder()(rawTx)
	if err != nil {
		return nil, fmt.Errorf("decode signed tx: %w", err)
	}
	jsonBz, err := cfg.TxJSONEncoder()(tx)
	if err != nil {
		return nil, fmt.Errorf("encode tx json: %w", err)
	}
	return jsonBz, nil
}

// queryMineAmountMined lê o atributo `amount_mined` do evento `mine` da tx e
// devolve-o em LUG (6 decimais). Sem forward, é esta a fonte do valor minerado.
func (s *Server) queryMineAmountMined(txHash string) (string, error) {
	args := []string{
		"query", "tx", "--type", "hash", txHash,
		"--home", s.cfg.Home,
		"--node", s.cfg.Node,
		"-o", "json",
	}
	var out []byte
	var err error
	for i := 0; i < 10; i++ {
		out, err = exec.Command(s.cfg.Bin, args...).CombinedOutput()
		if err == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	if err != nil {
		return "", fmt.Errorf("query tx: %w: %s", err, strings.TrimSpace(string(out)))
	}

	var parsed struct {
		Events []txEvent `json:"events"`
		Logs   []struct {
			Events []txEvent `json:"events"`
		} `json:"logs"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		return "", fmt.Errorf("parse tx: %w", err)
	}

	events := parsed.Events
	for _, l := range parsed.Logs {
		events = append(events, l.Events...)
	}
	for _, ev := range events {
		if ev.Type != "mine" {
			continue
		}
		for _, attr := range ev.Attributes {
			if attr.Key == "amount_mined" {
				return uluggomToLugString(attr.Value), nil
			}
		}
	}
	return "", fmt.Errorf("mine event with amount_mined not found in tx %s", txHash)
}

type txEvent struct {
	Type       string `json:"type"`
	Attributes []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"attributes"`
}

// uluggomToLugString converte uluggom (18 decimais, string) para LUG com 6 casas.
func uluggomToLugString(uluggom string) string {
	v := strings.TrimSpace(uluggom)
	// Alguns eventos vêm entre aspas (Cosmos JSON pode serializar assim).
	v = strings.Trim(v, `"`)
	ugInt, ok := new(big.Int).SetString(v, 10)
	if !ok {
		return uluggom
	}
	denom := new(big.Int).Exp(big.NewInt(10), big.NewInt(18), nil)
	whole := new(big.Int).Quo(ugInt, denom)
	frac := new(big.Int).Rem(ugInt, denom)
	fracScaled := new(big.Int).Quo(new(big.Int).Mul(frac, big.NewInt(1_000_000)), denom)
	return fmt.Sprintf("%s.%06d", whole.String(), fracScaled)
}
