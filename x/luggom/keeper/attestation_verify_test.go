package keeper_test

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

// attestationMsg constrói um MsgMineTaxPayment mínimo para exercitar a verificação.
func attestationMsg(attestation string) *types.MsgMineTaxPayment {
	return &types.MsgMineTaxPayment{
		MinerAddress:    "luggom1qqqqqqqqqqqqqqqqqqqqqqqqqqqqqqqq5n0k6",
		PaymentAmount:        sdkmath.LegacyMustNewDecFromStr("240.50"),
		TaxEntity:       "br:receita_federal",
		CurrencyCode:    "BRL",
		ProofCommitment: "commit-abc",
		PaymentId:       "pay-1",
		PaymentDate:     1_770_000_000,
		TaxNatureHash:   "nature-hash-abc",
		ProviderId:      "viacredi", // banco na whitelist on-chain (§13.131 A4)
		ApiAttestation:  attestation,
	}
}

func signForMsg(t *testing.T, key *secp256k1.PrivateKey, msg *types.MsgMineTaxPayment) string {
	t.Helper()
	digest := types.BuildAttestationDigest(types.AttestationFields{
		MinerAddress:    msg.MinerAddress,
		ChainID:         testChainID,
		ProofCommitment: msg.ProofCommitment,
		PaymentID:       msg.PaymentId,
		PaymentAmount:        msg.PaymentAmount.String(),
		TaxEntity:       msg.TaxEntity,
		CurrencyCode:    msg.CurrencyCode,
		PaymentDate:     msg.PaymentDate,
		TaxNatureHash:   msg.TaxNatureHash,
		PayerTaxID:      msg.PayerTaxId,
		ProviderID:      msg.ProviderId,
	})
	return hex.EncodeToString(ecdsa.SignCompact(key, digest, true))
}

func setPhase2(t *testing.T, f *mineFixture, verifierAddr string) {
	t.Helper()
	params := types.DefaultParams()
	// §13.170 Grupo B: a fase é uma constante (2) — já não se escolhe.
	params.ApiVerifierAddress = verifierAddr
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
}

// TestVerifyApiAttestation_Phase2_AcceptsValid: assinatura do attestor correto passa.
func TestVerifyApiAttestation_Phase2_AcceptsValid(t *testing.T) {
	f := initMineFixture(t)
	key := secp256k1.PrivKeyFromBytes(bytes.Repeat([]byte{0x11}, 32))
	signer, err := types.AddressFromCompressedPubKey(key.PubKey().SerializeCompressed())
	require.NoError(t, err)
	setPhase2(t, f, signer)

	msg := attestationMsg(signForMsg(t, key, attestationMsg("")))
	require.NoError(t, f.keeper.VerifyApiAttestation(f.ctx, msg))
}

// TestVerifyApiAttestation_Phase2_RejectsForged: o cenário do P0 — um comprovante
// INVENTADO (ou assinado por outra chave) tem de ser REJEITADO.
func TestVerifyApiAttestation_Phase2_RejectsForged(t *testing.T) {
	f := initMineFixture(t)
	legit := secp256k1.PrivKeyFromBytes(bytes.Repeat([]byte{0x11}, 32))
	signer, err := types.AddressFromCompressedPubKey(legit.PubKey().SerializeCompressed())
	require.NoError(t, err)
	setPhase2(t, f, signer)

	// (1) Atacante assina com a própria chave → endereço diferente → rejeitado.
	attacker := secp256k1.PrivKeyFromBytes(bytes.Repeat([]byte{0x22}, 32))
	forged := attestationMsg(signForMsg(t, attacker, attestationMsg("")))
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, forged), types.ErrInvalidAttestation)

	// (2) Assinatura legítima mas claims adulteradas (valor inflacionado) → rejeitado.
	validSig := signForMsg(t, legit, attestationMsg(""))
	tampered := attestationMsg(validSig)
	tampered.PaymentAmount = sdkmath.LegacyMustNewDecFromStr("240500.00")
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, tampered), types.ErrInvalidAttestation)

	// (3) Transcript inventado (outro proof_commitment) com a assinatura antiga → rejeitado.
	otherCommitment := attestationMsg(validSig)
	otherCommitment.ProofCommitment = "commit-INVENTADO"
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, otherCommitment), types.ErrInvalidAttestation)

	// (4) Atestação lixo → rejeitado.
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, attestationMsg("deadbeef")), types.ErrInvalidAttestation)

	// (5) A1.4 — falsificar a DATA para contornar a janela de 365 dias (1205).
	fakeDate := attestationMsg(validSig)
	fakeDate.PaymentDate = 0
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, fakeDate), types.ErrInvalidAttestation)

	// (6) A1.4 — inventar o HASH de natureza fiscal (contornar o 1207).
	fakeNature := attestationMsg(validSig)
	fakeNature.TaxNatureHash = "hash-inventado"
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, fakeNature), types.ErrInvalidAttestation)

	// (7) A4 (§13.131) — TROCAR O BANCO: a prova é de `viacredi` e o atacante
	// declara outro provider (ou um fora da whitelist). Como o provider_id está
	// no digest assinado pelo notário, a assinatura deixa de bater.
	swapped := attestationMsg(validSig)
	swapped.ProviderId = "itau"
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, swapped), types.ErrInvalidAttestation)

	// (8) A4 — esvaziar o provider_id também invalida (vai no digest).
	emptied := attestationMsg(validSig)
	emptied.ProviderId = ""
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, emptied), types.ErrInvalidAttestation)

	// (9) v4 — trocar o destinatário da cunhagem. Era o ataque do mempool: ver a
	// msg, reenviá-la a partir de outra carteira e ficar com o LUG.
	otherMiner, _ := makeAddr(t, f.addressCodec, 0x33)
	stolen := attestationMsg(validSig)
	stolen.MinerAddress = otherMiner
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, stolen), types.ErrInvalidAttestation)

	// (10) v4 — a mesma atestação noutra chain (replay testnet → mainnet).
	otherChain := sdk.UnwrapSDKContext(f.ctx).WithChainID("luggom-mainnet")
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(otherChain, attestationMsg(validSig)),
		types.ErrInvalidAttestation)
}

// TestVerifyApiAttestation_Phase2_FailsClosedWithoutVerifier: sem endereço
// configurado, a fase 2 rejeita em vez de aceitar (era o furo original).
func TestVerifyApiAttestation_Phase2_FailsClosedWithoutVerifier(t *testing.T) {
	f := initMineFixture(t)
	key := secp256k1.PrivKeyFromBytes(bytes.Repeat([]byte{0x11}, 32))
	setPhase2(t, f, "") // sem attestor configurado

	msg := attestationMsg(signForMsg(t, key, attestationMsg("")))
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, msg), types.ErrInvalidAttestation)
}

// TestVerifyApiAttestation_Phase1IsGone: §13.170 Grupo B — a fase deixou de ser
// um param e está CONGELADA em 2. A "fase 1" (atestação aceite só pelo
// comprimento, forjável por qualquer um) DEIXOU de existir: uma atestação de 64
// caracteres é agora RECUSADA. É o ganho de segurança da classificação A–E.
func TestVerifyApiAttestation_Phase1IsGone(t *testing.T) {
	f := initMineFixture(t)
	require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

	require.Equal(t, uint32(2), f.keeper.GetTlsnotaryPhase(f.ctx),
		"a fase tem de estar congelada em 2 (não há fase 1)")

	long := attestationMsg(strings.Repeat("a", 64))
	require.ErrorIs(t, f.keeper.VerifyApiAttestation(f.ctx, long), types.ErrInvalidAttestation,
		"a fase 1 desapareceu: uma atestação só por comprimento tem de ser recusada")
}
