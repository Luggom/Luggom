package types

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
)

// Atestação assinada pelo attestor (o notário TLSNotary / a API) — §13.97, P0.
//
// O problema que isto fecha: hoje o `api_attestation` era um HMAC SIMÉTRICO
// (não verificável on-chain) e a chain só conferia o comprimento. Qualquer um
// podia minerar com um transcript INVENTADO.
//
// A assinatura é **ECDSA secp256k1 em formato compacto (65 bytes: recovery || r || s)**,
// o que permite à chain **recuperar a chave pública** do assinante a partir da
// assinatura e do digest — e comparar o endereço resultante com
// `params.api_verifier_address`. Não é preciso guardar a pubkey em params nem
// regenerar o proto.

// AttestationDomain separa o domínio da assinatura (evita reutilizar a mesma
// assinatura noutro contexto). v2: o digest passou a cobrir TODOS os campos.
// v3: acrescentado o ProviderID (o banco atestado) — §13.131 achado A4: sem ele
// no digest, uma API comprometida podia trocar o banco e a chain não dava por isso.
// v4: acrescentados o MinerAddress e o ChainID. Sem o minerador no digest, quem
// visse a MsgMineTaxPayment no mempool reenviava a mesma atestação com a sua
// carteira e ficava com o LUG (o pagador real apanhava ErrProofAlreadyUsed).
// Sem o chain-id, uma atestação da testnet podia ser minerada outra vez na
// mainnet se a chave do notário fosse reutilizada.
const AttestationDomain = "luggom-attestation/v4"

// AttestationFields são TODOS os campos da mineração que afetam o resultado.
//
// ⚠️ TÊM de estar TODOS no digest. Qualquer campo que a chain leia da msg e que
// fique FORA da assinatura pode ser manipulado por uma API comprometida sem
// quebrar a assinatura (A1.4, §13.121): `payment_date` contornaria a janela de
// 365 dias (1205) e `tax_nature_hash` contornaria o portão de natureza fiscal
// (1207); `provider_id` contornaria a whitelist de bancos (A4, §13.131);
// `miner_address` permitiria roubar a cunhagem (v4).
type AttestationFields struct {
	MinerAddress    string
	ChainID         string
	ProofCommitment string
	PaymentID       string
	PaymentAmount   string
	TaxEntity       string
	CurrencyCode    string
	PaymentDate     int64
	TaxNatureHash   string
	PayerTaxID      string
	ProviderID      string
}

// BuildAttestationDigest constroi o digest assinado (SHA-256) a partir dos campos
// canónicos da mineração. O MESMO digest é reconstruído on-chain a partir do
// MsgMineTaxPayment — qualquer divergência invalida a assinatura.
//
// `paymentAmount` é canonicalizado (LegacyDec) para o lado da API e o lado da chain
// chegarem ao mesmo byte-string independentemente da formatação de entrada.
// BuildAttestationPayload devolve a string canónica EXATA a assinar. É a única
// fonte de verdade da canonicalização — o notário assina este payload (via o
// extrator partilhado) e a chain reconstrói-o a partir da msg.
func BuildAttestationPayload(f AttestationFields) string {
	return strings.Join([]string{
		AttestationDomain,
		strings.TrimSpace(f.MinerAddress),
		strings.TrimSpace(f.ChainID),
		strings.TrimSpace(f.ProofCommitment),
		strings.TrimSpace(f.PaymentID),
		canonicalDecimal(f.PaymentAmount),
		strings.TrimSpace(f.TaxEntity),
		strings.ToUpper(strings.TrimSpace(f.CurrencyCode)),
		strconv.FormatInt(f.PaymentDate, 10),
		strings.TrimSpace(f.TaxNatureHash),
		strings.TrimSpace(f.PayerTaxID),
		strings.TrimSpace(f.ProviderID),
	}, "\n")
}

// BuildAttestationDigest devolve sha256 do payload canónico.
func BuildAttestationDigest(f AttestationFields) []byte {
	sum := sha256.Sum256([]byte(BuildAttestationPayload(f)))
	return sum[:]
}

// canonicalDecimal normaliza um decimal (ex.: "240.50" e "240.5" → "240.5").
// Se não for um decimal válido, devolve a string original (trim).
func canonicalDecimal(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	d, err := sdkmath.LegacyNewDecFromStr(s)
	if err != nil {
		return s
	}
	return d.String()
}

// RecoverAttestationSigner recupera o endereço bech32 do assinante a partir da
// assinatura compacta (hex, 65 bytes) e do digest. Devolve erro se a assinatura
// for malformada ou irrecuperável (→ a chain rejeita).
func RecoverAttestationSigner(signatureHex string, digest []byte) (string, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(signatureHex))
	if err != nil {
		return "", fmt.Errorf("attestation signature is not valid hex: %w", err)
	}
	if len(raw) != 65 {
		return "", fmt.Errorf("attestation signature must be 65 bytes (recovery||r||s), got %d", len(raw))
	}
	if len(digest) != sha256.Size {
		return "", fmt.Errorf("attestation digest must be %d bytes, got %d", sha256.Size, len(digest))
	}

	pub, _, err := ecdsa.RecoverCompact(raw, digest)
	if err != nil {
		return "", fmt.Errorf("cannot recover attestation signer: %w", err)
	}

	return AddressFromCompressedPubKey(pub.SerializeCompressed())
}

// AddressFromCompressedPubKey devolve o endereço bech32 Cosmos de uma chave
// pública secp256k1 comprimida (33 bytes): ripemd160(sha256(pubkey)).
func AddressFromCompressedPubKey(compressed []byte) (string, error) {
	if len(compressed) != 33 {
		return "", fmt.Errorf("compressed secp256k1 pubkey must be 33 bytes, got %d", len(compressed))
	}
	addrBytes := (&secp256k1.PubKey{Key: compressed}).Address()
	addr, err := sdk.Bech32ifyAddressBytes(sdk.GetConfig().GetBech32AccountAddrPrefix(), addrBytes)
	if err != nil {
		return "", fmt.Errorf("cannot encode address: %w", err)
	}
	return addr, nil
}

// VerifyAttestationSignature verifica que a assinatura foi produzida pela chave
// cujo endereço é `expectedAddress` (bech32). Fail-closed em qualquer erro.
func VerifyAttestationSignature(signatureHex string, digest []byte, expectedAddress string) error {
	if strings.TrimSpace(expectedAddress) == "" {
		return fmt.Errorf("expected verifier address is empty")
	}
	got, err := RecoverAttestationSigner(signatureHex, digest)
	if err != nil {
		return err
	}
	if got != strings.TrimSpace(expectedAddress) {
		return fmt.Errorf("attestation signer %s does not match the expected verifier %s", got, expectedAddress)
	}
	return nil
}
