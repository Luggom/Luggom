package reclaimverify

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"

	"luggom/x/luggom/types"
)

// SignClaims produz a atestação assinada da mineração (P0, §13.97).
//
// Substitui o HMAC-SHA256 simétrico antigo (não verificável on-chain) por uma
// **assinatura ECDSA secp256k1 compacta (65 bytes, hex)** sobre o digest canónico
// (`types.BuildAttestationDigest`). A chain recupera a chave pública da assinatura
// e compara o endereço com `params.api_verifier_address`.
//
// `privKeyHex` é a chave privada secp256k1 (32 bytes em hex) do attestor.
//
// `fields` tem de conter TODOS os campos que afetam o resultado (A1.4) — o mesmo
// digest que o keeper reconstrói a partir da msg.
func SignClaims(privKeyHex string, fields types.AttestationFields) (string, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(privKeyHex))
	if err != nil {
		return "", fmt.Errorf("attestation private key is not valid hex: %w", err)
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("attestation private key must be 32 bytes (64 hex chars), got %d", len(raw))
	}

	priv := secp256k1.PrivKeyFromBytes(raw)

	digest := types.BuildAttestationDigest(fields)
	sig := ecdsa.SignCompact(priv, digest, true /* compressed pubkey recovery */)
	return hex.EncodeToString(sig), nil
}

// AttestationSignerAddress devolve o endereço bech32 do attestor (para colocar em
// `params.api_verifier_address`). Evita derivar a chave "à mão" no operador.
func AttestationSignerAddress(privKeyHex string) (string, error) {
	raw, err := hex.DecodeString(strings.TrimSpace(privKeyHex))
	if err != nil {
		return "", fmt.Errorf("attestation private key is not valid hex: %w", err)
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("attestation private key must be 32 bytes (64 hex chars), got %d", len(raw))
	}
	priv := secp256k1.PrivKeyFromBytes(raw)
	pub := priv.PubKey().SerializeCompressed()

	// Reutiliza a derivação do keeper (recupera o endereço a partir de uma
	// assinatura válida) seria indireto; aqui derivamos via o tipo do SDK.
	return types.AddressFromCompressedPubKey(pub)
}
