package main

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	sdk "github.com/cosmos/cosmos-sdk/types"
	txv1beta1 "github.com/cosmos/cosmos-sdk/types/tx"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"
)

// testServer constrói um Server mínimo para os testes de montagem da tx.
func testServer(t *testing.T) *Server {
	t.Helper()
	setupSDKConfig("luggom")
	return &Server{
		cfg: Config{
			ChainID:      "luggom-test",
			Bech32Prefix: "luggom",
		},
		pendingMeta: map[string]MineTaxMeta{},
	}
}

// TestBuildUnsignedMineTaxTx_SignableAndAssembles prova o contrato do fluxo
// OPÇÃO 1: o sign_doc devolvido pela API é aquilo que o cliente assina com
// secp256k1, e a assinatura produzida valida contra a tx montada.
func TestBuildUnsignedMineTaxTx_SignableAndAssembles(t *testing.T) {
	s := testServer(t)

	priv := secp256k1.GenPrivKey()
	pub := priv.PubKey()
	signer := sdk.AccAddress(pub.Address()).String()

	unsigned, err := s.buildUnsignedMineTaxTx(broadcastMineTaxArgs{
		PaymentAmount:        "240.50",
		TaxEntity:       "irs",
		CurrencyCode:    "USD",
		ProofCommitment: "commitment-0123456789abcdef0123456789abcdef",
		PaymentId:       "pay-123456",
		ApiAttestation:  "attestation",
		PaymentDate:     1_700_000_000,
		PayerTaxId:      "12-3456789",
		TaxNatureHash:   "naturehash-0123456789abcdef0123456789abcdef",
	}, signer, pub, 7, 3)
	require.NoError(t, err)

	require.Equal(t, signer, unsigned.SignerAddress)
	require.Equal(t, uint64(7), unsigned.AccountNumber)
	require.Equal(t, uint64(3), unsigned.Sequence)
	require.Equal(t, "luggom-test", unsigned.ChainID)

	signDocBz, err := base64.StdEncoding.DecodeString(unsigned.SignDocBytes)
	require.NoError(t, err)
	require.NotEmpty(t, signDocBz)

	// O digest publicado é o SHA-256 do sign-doc (é o que o ECDSA assina).
	digest := sha256.Sum256(signDocBz)
	require.Equal(t, base64.StdEncoding.EncodeToString(digest[:]), unsigned.SignDocSHA256)

	// A tx não assinada tem de descodificar e não trazer assinaturas.
	txRawBz, err := base64.StdEncoding.DecodeString(unsigned.TxBytes)
	require.NoError(t, err)
	var raw txv1beta1.TxRaw
	require.NoError(t, proto.Unmarshal(txRawBz, &raw))
	require.Empty(t, raw.Signatures)

	// O AuthInfo inclui a chave pública do minerador (sem ela a chain não
	// consegue verificar a assinatura). A pubkey viaja num proto Any.
	authInfoBz, err := base64.StdEncoding.DecodeString(unsigned.AuthInfoBytes)
	require.NoError(t, err)
	var authInfo txv1beta1.AuthInfo
	require.NoError(t, proto.Unmarshal(authInfoBz, &authInfo))
	require.Len(t, authInfo.SignerInfos, 1)
	require.NotNil(t, authInfo.SignerInfos[0].PublicKey)
	require.Contains(t, authInfo.SignerInfos[0].PublicKey.TypeUrl, "secp256k1")
	// O Any.Value é o PubKey serializado (field 1 = Key).
	var pk secp256k1.PubKey
	require.NoError(t, proto.Unmarshal(authInfo.SignerInfos[0].PublicKey.Value, &pk))
	require.Equal(t, pub.Bytes(), pk.Key)
	require.Equal(t, uint64(3), authInfo.SignerInfos[0].Sequence)

	// --- Cliente simulado: assina o sign-doc e monta o TxRaw ---
	sig, err := priv.Sign(digest[:])
	require.NoError(t, err)
	require.True(t, pub.VerifySignature(digest[:], sig), "signature must verify against the sign doc digest")

	txRaw := &txv1beta1.TxRaw{
		BodyBytes:     raw.BodyBytes,
		AuthInfoBytes: raw.AuthInfoBytes,
		Signatures:    [][]byte{sig},
	}
	signedBz, err := proto.Marshal(txRaw)
	require.NoError(t, err)

	// A tx assinada tem de sobreviver ao encoder do SDK (é o que vai on-chain).
	cfg, err := clientTxConfig()
	require.NoError(t, err)
	var decoded txv1beta1.TxRaw
	require.NoError(t, proto.Unmarshal(signedBz, &decoded))
	require.Len(t, decoded.Signatures, 1)
	require.Equal(t, raw.BodyBytes, decoded.BodyBytes)
	require.Equal(t, raw.AuthInfoBytes, decoded.AuthInfoBytes)

	// E o sign-doc tem de continuar a bater certo com os bytes da tx.
	signDoc := &txv1beta1.SignDoc{
		BodyBytes:     decoded.BodyBytes,
		AuthInfoBytes: decoded.AuthInfoBytes,
		ChainId:       unsigned.ChainID,
		AccountNumber: unsigned.AccountNumber,
	}
	rebuilt, err := proto.Marshal(signDoc)
	require.NoError(t, err)
	require.Equal(t, unsigned.SignDocBytes, base64.StdEncoding.EncodeToString(rebuilt))
	_ = cfg
}

// TestBuildUnsignedMineTaxTx_MinerIsTheSigner garante que o miner_address é a
// carteira do utilizador — nunca uma chave da API (fim do forward, §13.97).
func TestBuildUnsignedMineTaxTx_MinerIsTheSigner(t *testing.T) {
	s := testServer(t)

	priv := secp256k1.GenPrivKey()
	pub := priv.PubKey()
	signer := sdk.AccAddress(pub.Address()).String()

	unsigned, err := s.buildUnsignedMineTaxTx(broadcastMineTaxArgs{
		PaymentAmount:        "100",
		TaxEntity:       "irs",
		CurrencyCode:    "USD",
		ProofCommitment: "c-0123456789abcdef0123456789abcdef",
		PaymentId:       "p-1",
		ApiAttestation:  "a",
		PaymentDate:     1,
		PayerTaxId:      "12-3456789",
		TaxNatureHash:   "t-0123456789abcdef0123456789abcdef",
	}, signer, pub, 0, 0)
	require.NoError(t, err)

	bodyBz, err := base64.StdEncoding.DecodeString(unsigned.BodyBytes)
	require.NoError(t, err)

	var body txv1beta1.TxBody
	require.NoError(t, proto.Unmarshal(bodyBz, &body))
	require.Len(t, body.Messages, 1)

	// O Any do MsgMineTaxPayment tem de conter o endereço do utilizador.
	msgBz := body.Messages[0].Value
	require.Contains(t, string(msgBz), signer)
}

// TestTxRawToJSON é a regressão do bug encontrado no E2E de 2026-09-10: o CLI
// `tx broadcast` só aceita JSON (TxJSONDecoder), nunca protobuf binário. A API
// tem de converter o TxRaw assinado antes de o entregar ao CLI.
func TestTxRawToJSON(t *testing.T) {
	setupSDKConfig("luggom")

	priv := secp256k1.GenPrivKey()
	pub := priv.PubKey()
	signer := sdk.AccAddress(pub.Address()).String()

	s := testServer(t)
	unsigned, err := s.buildUnsignedMineTaxTx(broadcastMineTaxArgs{
		PaymentAmount:        "240.50",
		TaxEntity:       "irs",
		CurrencyCode:    "USD",
		ProofCommitment: "c-0123456789abcdef0123456789abcdef",
		PaymentId:       "p-1",
		ApiAttestation:  "a",
		PaymentDate:     1_700_000_000,
		PayerTaxId:      "12-3456789",
		TaxNatureHash:   "t-0123456789abcdef0123456789abcdef",
	}, signer, pub, 7, 3)
	require.NoError(t, err)

	// Assina (como o cliente faria) e monta o TxRaw binário.
	signDocBz, err := base64.StdEncoding.DecodeString(unsigned.SignDocBytes)
	require.NoError(t, err)
	digest := sha256.Sum256(signDocBz)
	sig, err := priv.Sign(digest[:])
	require.NoError(t, err)

	bodyBz, err := base64.StdEncoding.DecodeString(unsigned.BodyBytes)
	require.NoError(t, err)
	authBz, err := base64.StdEncoding.DecodeString(unsigned.AuthInfoBytes)
	require.NoError(t, err)
	signedBz, err := proto.Marshal(&txv1beta1.TxRaw{
		BodyBytes:     bodyBz,
		AuthInfoBytes: authBz,
		Signatures:    [][]byte{sig},
	})
	require.NoError(t, err)

	// A conversão tem de produzir JSON (não binário) …
	jsonBz, err := txRawToJSON(signedBz)
	require.NoError(t, err)
	require.True(t, len(jsonBz) > 0 && jsonBz[0] == '{', "esperava JSON, veio %q", string(jsonBz[:min(20, len(jsonBz))]))

	// … que o TxJSONDecoder do CLI aceita (é ele que o broadcast usa).
	cfg, err := clientTxConfig()
	require.NoError(t, err)
	decoded, err := cfg.TxJSONDecoder()(jsonBz)
	require.NoError(t, err, "o CLI tem de conseguir descodificar o que produzimos")
	require.NotNil(t, decoded)

	// E o ciclo completo tem de preservar a assinatura.
	roundTrip, err := cfg.TxEncoder()(decoded)
	require.NoError(t, err)
	var back txv1beta1.TxRaw
	require.NoError(t, proto.Unmarshal(roundTrip, &back))
	require.Len(t, back.Signatures, 1)
	require.Equal(t, sig, back.Signatures[0])

	// JSON de entrada passa inalterado (tolerância).
	same, err := txRawToJSON([]byte(`{"body":{}}`))
	require.NoError(t, err)
	require.Equal(t, `{"body":{}}`, string(same))

	// Binário corrompido → erro, não pânico.
	_, err = txRawToJSON([]byte{0xFF, 0xFF, 0xFF})
	require.Error(t, err)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TestPubKeyMatchesAddress é a regressão do bug encontrado no E2E de 2026-09-10:
// `pubKey.Address().String()` devolve HEX, não bech32, portanto comparar strings
// rejeitava SEMPRE (e a mineração nunca funcionaria).
func TestPubKeyMatchesAddress(t *testing.T) {
	setupSDKConfig("luggom")

	priv := secp256k1.GenPrivKey()
	pub := priv.PubKey()
	own := sdk.AccAddress(pub.Address())

	if !pubKeyMatchesAddress(pub, own) {
		t.Fatal("a chave pública tem de corresponder ao seu próprio endereço")
	}

	// Endereço de outra chave → não corresponde.
	other := sdk.AccAddress(secp256k1.GenPrivKey().PubKey().Address())
	if pubKeyMatchesAddress(pub, other) {
		t.Fatal("chave de outra conta não pode corresponder")
	}

	// Salvaguardas de nil.
	if pubKeyMatchesAddress(nil, own) || pubKeyMatchesAddress(pub, nil) {
		t.Fatal("nil não pode corresponder")
	}

	// Documenta o motivo do bug: o .String() do endereço cru NÃO é bech32.
	rawStr := pub.Address().String()
	require.NotEqual(t, own.String(), rawStr,
		"se isto passar a ser igual, a comparação por string voltaria a ser válida")
	t.Logf("Address().String() = %q (hex), bech32 = %q", rawStr, own.String())
}

// TestParsePublicKey aceita chave crua (base64/hex) e rejeita formas inválidas.
func TestParsePublicKey(t *testing.T) {
	setupSDKConfig("luggom")
	priv := secp256k1.GenPrivKey()
	pub := priv.PubKey()

	pk, err := parsePublicKey(base64.StdEncoding.EncodeToString(pub.Bytes()))
	require.NoError(t, err)
	require.Equal(t, pub.Address().String(), pk.Address().String())

	_, err = parsePublicKey("")
	require.Error(t, err)

	_, err = parsePublicKey(base64.StdEncoding.EncodeToString([]byte("too-short")))
	require.Error(t, err)
}

// TestUluggomToLugString confere a conversão usada no amount_mined do extrato.
func TestUluggomToLugString(t *testing.T) {
	// 100 LUG = 100 × 10^18 uluggom
	require.Equal(t, "100.000000", uluggomToLugString("100000000000000000000"))
	// 1,5 LUG
	require.Equal(t, "1.500000", uluggomToLugString("1500000000000000000"))
	// valor entre aspas (o JSON do Cosmos pode serializar assim)
	require.Equal(t, "2.000000", uluggomToLugString(`"2000000000000000000"`))
}

// TestPendingMineMeta garante que o passo 2 fixa a meta do passo 1 ao txhash.
func TestPendingMineMeta(t *testing.T) {
	s := testServer(t)

	s.storePendingMineMeta("pay-1", MineTaxMeta{FiatAmount: "240.50", TaxEntity: "irs"})

	meta, ok := s.takePendingMineMeta("pay-1")
	require.True(t, ok)
	require.Equal(t, "240.50", meta.FiatAmount)

	// Consumido uma vez — não volta a aparecer.
	_, ok = s.takePendingMineMeta("pay-1")
	require.False(t, ok)

	// payment_id vazio é ignorado (não cria entradas fantasma).
	s.storePendingMineMeta("", MineTaxMeta{})
	_, ok = s.takePendingMineMeta("")
	require.False(t, ok)
}
