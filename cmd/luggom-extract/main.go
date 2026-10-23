// luggom-extract — extrator PARTILHADO (§13.121, Fase B).
//
// O notário (Rust) invoca este binário sobre o transcript que ele próprio
// verificou, e recebe de volta:
//   - os campos canónicos (AttestationFields), e
//   - o PAYLOAD exato a assinar (types.BuildAttestationPayload).
//
// O notário faz sha256(payload) e assina com a chave secp256k1 dele; a API só
// RELAYA. Assim a derivação (extração do valor) passa a estar dentro do domínio
// de confiança do notário — uma API comprometida deixa de poder inflacionar.
//
// Reutiliza o MESMO código que a API usava (taxproof + taxnature): uma só
// implementação, sem risco de drift entre duas linguagens.
//
// Uso: escreve um JSON em stdin, lê um JSON de stdout.
//
//	{"server_name":"...","miner_address":"luggom1...","chain_id":"luggom-test","transcript_recv_b64":"..."}
//	→ {"fields":{...},"payload":"..."}  |  {"error":"..."}
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/api/reclaimverify"
	"luggom/api/taxnature"
	"luggom/api/taxproof"
	"luggom/x/luggom/types"
)

// init fixa o prefixo bech32 da chain, senão o endereço sairia `cosmos1…`.
func init() {
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount("luggom", "luggompub")
	cfg.SetBech32PrefixForValidator("luggomvaloper", "luggomvaloperpub")
	cfg.SetBech32PrefixForConsensusNode("luggomvalcons", "luggomvalconspub")
}

type request struct {
	ServerName string `json:"server_name"`
	// MinerAddress e ChainID entram no digest v4: o destinatário da cunhagem e
	// a chain onde a atestação vale. São obrigatórios (fail-closed).
	MinerAddress      string `json:"miner_address"`
	ChainID           string `json:"chain_id"`
	TranscriptRecvB64 string `json:"transcript_recv_b64"`
}

type response struct {
	Fields  *types.AttestationFields `json:"fields,omitempty"`
	Payload string                   `json:"payload,omitempty"`
	Error   string                   `json:"error,omitempty"`
}

func main() {
	// Modo utilitário: derivar o endereço do notário a partir da chave privada
	// (é o valor que vai em `params.api_verifier_address`).
	//   luggom-extract --print-address <hex32>
	if len(os.Args) >= 3 && os.Args[1] == "--print-address" {
		addr, err := reclaimverify.AttestationSignerAddress(os.Args[2])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(addr)
		return
	}

	var req request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fail(fmt.Errorf("invalid input json: %w", err))
	}
	fields, payload, err := extract(req.ServerName, req.MinerAddress, req.ChainID, req.TranscriptRecvB64)
	if err != nil {
		fail(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(response{Fields: fields, Payload: payload}); err != nil {
		fmt.Fprintf(os.Stderr, "encode: %v\n", err)
		os.Exit(1)
	}
}

func fail(err error) {
	_ = json.NewEncoder(os.Stdout).Encode(response{Error: err.Error()})
	os.Exit(1)
}

// extract resolve o banco a partir do server_name atestado, corre o extrator e
// devolve os campos canónicos + o payload a assinar.
func extract(serverName, minerAddress, chainID, recvB64 string) (*types.AttestationFields, string, error) {
	if strings.TrimSpace(minerAddress) == "" {
		return nil, "", fmt.Errorf("miner_address is empty (v4 requires the recipient in the signed payload)")
	}
	if strings.TrimSpace(chainID) == "" {
		return nil, "", fmt.Errorf("chain_id is empty (v4 binds the attestation to a chain)")
	}
	providerID, err := resolveProvider(serverName)
	if err != nil {
		return nil, "", err
	}
	if strings.TrimSpace(recvB64) == "" {
		return nil, "", fmt.Errorf("transcript_recv_b64 is empty")
	}

	claims, proofCommitment, err := taxproof.VerifyTlsnProof(providerID, taxproof.Bundle{
		SessionID:  "notary",
		ServerName: serverName,
		Transcript: taxproof.Transcript{Recv: recvB64},
	})
	if err != nil {
		return nil, "", fmt.Errorf("tlsn proof validation: %w", err)
	}

	natureFrag, err := taxnature.Extract(providerID, taxnature.Claims{
		ProviderID:    providerID,
		Recipient:     claims.RecipientAccount,
		RecipientName: claims.RecipientName,
		Payer:         claims.PayerAccount,
		PayerTaxID:    claims.PayerTaxID,
		Description:   claims.Description,
	})
	if err != nil {
		return nil, "", fmt.Errorf("payment is not a tax collection: %w", err)
	}

	fields := &types.AttestationFields{
		MinerAddress:    strings.TrimSpace(minerAddress),
		ChainID:         strings.TrimSpace(chainID),
		ProofCommitment: proofCommitment,
		// Pseudónimos: nem o E2E do PIX nem o CNPJ/EIN do pagador vão para a
		// chain em claro. A chain exige 64 hex (IsHash64), por isso o valor cru
		// não pode ser reintroduzido por um cliente modificado.
		PaymentID:     types.HashPaymentID(claims.TransactionID),
		PaymentAmount: claims.PaymentAmount,
		TaxEntity:     claims.TaxEntityMatch,
		CurrencyCode:  claims.PaymentCurrency,
		PaymentDate:   claims.PaymentDate,
		TaxNatureHash: taxnature.Hash(natureFrag),
		PayerTaxID:    types.HashTaxID(claims.PayerTaxID),
		// O banco resolvido a partir do server_name ATESTADO (§13.131 A4): vai no
		// payload assinado, para a chain poder aplicar a whitelist de bancos.
		ProviderID: providerID,
	}
	if fields.PaymentID == "" {
		return nil, "", fmt.Errorf("cannot mine without a payment id (the attested receipt has none)")
	}
	return fields, types.BuildAttestationPayload(*fields), nil
}

// resolveProvider encontra o banco cujo domínio corresponde ao server_name
// atestado. Fail-closed: sem correspondência → erro.
func resolveProvider(serverName string) (string, error) {
	host := strings.ToLower(strings.TrimSpace(serverName))
	host = strings.TrimSuffix(host, ".")
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	if host == "" {
		return "", fmt.Errorf("proof has no server_name")
	}
	for _, p := range reclaimverify.KnownProviders() {
		if p.Domain == "" {
			continue
		}
		d := strings.ToLower(p.Domain)
		if host == d || strings.HasSuffix(host, "."+d) {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("server_name %q does not match any known bank", serverName)
}
