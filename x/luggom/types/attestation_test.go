package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/stretchr/testify/require"
)

// TestAttestationInterop_RustSignature — vetor produzido pelo NOTÁRIO em Rust
// (k256 `sign_prehash_recoverable`, formato `recid||r||s`). Garante que a
// canonização e o formato batem entre linguagens: a assinatura do Rust tem de
// ser recuperável pelo keeper, e o payload Go tem de ser IDÊNTICO ao do Rust.
//
// Regenerar o vetor: `cargo test --release interop_vector -- --nocapture`
// em tlsn-verifier/servers/verifier.
func TestAttestationInterop_RustSignature(t *testing.T) {
	const (
		// O valor é canónico do Go (LegacyDec.String() → 18 casas); é este payload
		// que o extrator Go produz e que o notário assina.
		// v4 (2026-10-22): domínio novo + `miner_address` e `chain_id` a seguir ao
		// domínio, na ordem do BuildAttestationPayload. O pagador é vazio — daí o
		// "\n\n" antes do banco.
		rustPayload = "luggom-attestation/v4\nluggom1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5n0k6\nluggom-test\ncommit-abc\npay-1\n240.500000000000000000\nbr:receita_federal\nBRL\n1770000000\nnature-hash-abc\n\nviacredi"
		// header 0x20 = 27 + recid(1) + 4 (chave comprimida) — formato decred/Bitcoin.
		rustSig = "20342658f42eb2dc44f97139b747c1b83136e440cba458cb026956dd68af02d4486de78b6d3cba736309c5f4839d626a35f454ebb5089642535ba861c87bc4bf66"
	)

	// 1) A canonização do Go tem de produzir EXATAMENTE o payload do Rust.
	fields := AttestationFields{
		MinerAddress:    "luggom1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5n0k6",
		ChainID:         "luggom-test",
		ProofCommitment: "commit-abc",
		PaymentID:       "pay-1",
		PaymentAmount:        "240.500000000000000000",
		TaxEntity:       "br:receita_federal",
		CurrencyCode:    "BRL",
		PaymentDate:     1_770_000_000,
		TaxNatureHash:   "nature-hash-abc",
		PayerTaxID:      "",
		ProviderID:      "viacredi",
	}
	require.Equal(t, rustPayload, BuildAttestationPayload(fields),
		"o payload canónico do Go tem de ser idêntico ao do Rust")

	// 2) A assinatura do Rust tem de ser recuperável e dar o endereço da chave 0x11.
	key := secp256k1.PrivKeyFromBytes(bytes.Repeat([]byte{0x11}, 32))
	expected, err := AddressFromCompressedPubKey(key.PubKey().SerializeCompressed())
	require.NoError(t, err)

	digest := sha256.Sum256([]byte(rustPayload))
	got, err := RecoverAttestationSigner(rustSig, digest[:])
	require.NoError(t, err)
	require.Equal(t, expected, got, "a assinatura do notário Rust tem de recuperar o endereço do notário")
}

func testAttestationKey() *secp256k1.PrivateKey {
	return secp256k1.PrivKeyFromBytes(bytes.Repeat([]byte{0x11}, 32))
}

func signAttestation(t *testing.T, key *secp256k1.PrivateKey, digest []byte) string {
	t.Helper()
	return hex.EncodeToString(ecdsa.SignCompact(key, digest, true))
}

// baseFields são campos de referência para os testes.
func baseFields() AttestationFields {
	return AttestationFields{
		MinerAddress:    "luggom1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5n0k6",
		ChainID:         "luggom-test",
		ProofCommitment: "commit-abc",
		PaymentID:       "pay-1",
		PaymentAmount:        "240.50",
		TaxEntity:       "br:receita_federal",
		CurrencyCode:    "BRL",
		PaymentDate:     1_770_000_000,
		TaxNatureHash:   "nature-hash-abc",
		PayerTaxID:      "",
	}
}

func digestHex(f AttestationFields) string {
	return hex.EncodeToString(BuildAttestationDigest(f))
}

// TestAttestationDigest_DeterministicAndCanonical: o digest é estável e
// canonicaliza o decimal (a API e a chain têm de chegar ao mesmo byte-string).
func TestAttestationDigest_DeterministicAndCanonical(t *testing.T) {
	d1 := digestHex(baseFields())

	f := baseFields()
	f.PaymentAmount = "240.5"
	f.CurrencyCode = "brl"
	require.Equal(t, d1, digestHex(f), "240.50/240.5 e brl/BRL têm de dar o MESMO digest")

	g := baseFields()
	g.PaymentAmount = "240.51"
	require.NotEqual(t, d1, digestHex(g), "um valor diferente muda o digest")
}

// TestAttestationDigest_CoversAllFields — A1.4. Cada campo que a chain lê tem de
// alterar o digest; senão uma API comprometida poderia manipulá-lo.
func TestAttestationDigest_CoversAllFields(t *testing.T) {
	base := digestHex(baseFields())

	mutations := map[string]func(*AttestationFields){
		// v4: o destinatário da cunhagem e a chain em que a atestação vale.
		"miner_address": func(f *AttestationFields) {
			f.MinerAddress = "luggom1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
		},
		"chain_id":         func(f *AttestationFields) { f.ChainID = "luggom-mainnet" },
		"proof_commitment": func(f *AttestationFields) { f.ProofCommitment = "outro" },
		"payment_id":       func(f *AttestationFields) { f.PaymentID = "outro" },
		"payment_amount":        func(f *AttestationFields) { f.PaymentAmount = "999" },
		"tax_entity":       func(f *AttestationFields) { f.TaxEntity = "irs" },
		"currency_code":    func(f *AttestationFields) { f.CurrencyCode = "USD" },
		// estes dois eram os que faltavam antes do A1.4:
		"payment_date":   func(f *AttestationFields) { f.PaymentDate = 1 },
		"tax_nature_hash": func(f *AttestationFields) { f.TaxNatureHash = "outro-hash" },
		"payer_tax_id":   func(f *AttestationFields) { f.PayerTaxID = "12345678000199" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			f := baseFields()
			mutate(&f)
			require.NotEqual(t, base, digestHex(f), "%s tem de estar coberto pelo digest", name)
		})
	}
}

// TestAttestationSignRecover: a assinatura recupera o endereço do assinante.
func TestAttestationSignRecover(t *testing.T) {
	key := testAttestationKey()
	expected, err := AddressFromCompressedPubKey(key.PubKey().SerializeCompressed())
	require.NoError(t, err)

	digest := BuildAttestationDigest(baseFields())
	sig := signAttestation(t, key, digest)

	require.NoError(t, VerifyAttestationSignature(sig, digest, expected))

	got, err := RecoverAttestationSigner(sig, digest)
	require.NoError(t, err)
	require.Equal(t, expected, got)
}

// TestAttestationVerify_RejectsTampered: claims alteradas invalidam a assinatura.
func TestAttestationVerify_RejectsTampered(t *testing.T) {
	key := testAttestationKey()
	signer, err := AddressFromCompressedPubKey(key.PubKey().SerializeCompressed())
	require.NoError(t, err)

	digest := BuildAttestationDigest(baseFields())
	sig := signAttestation(t, key, digest)

	// Valor inflacionado.
	f := baseFields()
	f.PaymentAmount = "240500.00"
	require.Error(t, VerifyAttestationSignature(sig, BuildAttestationDigest(f), signer))

	// Transcript inventado (outro proof_commitment).
	g := baseFields()
	g.ProofCommitment = "commit-INVENTADO"
	require.Error(t, VerifyAttestationSignature(sig, BuildAttestationDigest(g), signer))

	// Data falsificada para contornar a janela de 365 dias (A1.4).
	h := baseFields()
	h.PaymentDate = 0
	require.Error(t, VerifyAttestationSignature(sig, BuildAttestationDigest(h), signer))

	// Hash de natureza fiscal inventado (A1.4).
	i := baseFields()
	i.TaxNatureHash = "hash-inventado"
	require.Error(t, VerifyAttestationSignature(sig, BuildAttestationDigest(i), signer))

	// v4: outra carteira a receber a cunhagem — é o roubo que o miner_address
	// no digest fecha (quem visse a tx no mempool reenviava-a para si).
	j := baseFields()
	j.MinerAddress = "luggom1zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"
	require.Error(t, VerifyAttestationSignature(sig, BuildAttestationDigest(j), signer))

	// v4: a mesma atestação noutra chain (testnet → mainnet) tem de falhar.
	k := baseFields()
	k.ChainID = "luggom-mainnet"
	require.Error(t, VerifyAttestationSignature(sig, BuildAttestationDigest(k), signer))

	// Outro signatário (atacante com a sua própria chave).
	attacker := secp256k1.PrivKeyFromBytes(bytes.Repeat([]byte{0x22}, 32))
	attackerSig := signAttestation(t, attacker, digest)
	require.Error(t, VerifyAttestationSignature(attackerSig, digest, signer),
		"a assinatura de outra chave não pode ser aceite como sendo do attestor")
}

// TestAttestationVerify_RejectsMalformed: assinaturas malformadas falham fechado.
func TestAttestationVerify_RejectsMalformed(t *testing.T) {
	key := testAttestationKey()
	signer, err := AddressFromCompressedPubKey(key.PubKey().SerializeCompressed())
	require.NoError(t, err)
	digest := BuildAttestationDigest(baseFields())

	require.Error(t, VerifyAttestationSignature("", digest, signer), "assinatura vazia")
	require.Error(t, VerifyAttestationSignature("zzzz", digest, signer), "hex inválido")
	require.Error(t, VerifyAttestationSignature("aabb", digest, signer), "comprimento errado")
	require.Error(t, VerifyAttestationSignature(hex.EncodeToString(bytes.Repeat([]byte{0}, 65)), digest, signer),
		"65 bytes de zeros não é uma assinatura válida")
	require.Error(t, VerifyAttestationSignature(signAttestation(t, key, digest), digest, ""),
		"endereço esperado vazio tem de falhar (fail-closed)")
}
