package types

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsHash64(t *testing.T) {
	valid := strings.Repeat("ab", 32)
	require.True(t, IsHash64(valid))
	require.True(t, IsHash64(strings.ToUpper(valid)))
	require.False(t, IsHash64(""))
	require.False(t, IsHash64(valid[:63]))
	require.False(t, IsHash64(valid+"00"))
	require.False(t, IsHash64(strings.Repeat("zz", 32)))
	require.False(t, IsHash64("E82639451202607152106LZ1QCIXXYE6")) // E2E cru
}

func TestHashTaxID_Normalizes(t *testing.T) {
	require.Equal(t, HashTaxID("12.345.678/0001-90"), HashTaxID("12345678000190"))
	require.Equal(t, HashTaxID("br:12345678000190"), HashTaxID("12345678000190"))
	require.Equal(t, HashTaxID("12-3456789"), HashTaxID("123456789"))
	require.Equal(t, "", HashTaxID(""))
	require.True(t, IsHash64(HashTaxID("12345678000190")))
}

func TestHashPaymentID_Normalizes(t *testing.T) {
	require.Equal(t, HashPaymentID("  abc123  "), HashPaymentID("ABC123"))
	require.Equal(t, "", HashPaymentID("   "))
	require.True(t, IsHash64(HashPaymentID("E82639451202607152106LZ1QCIXXYE6")))
}

// TestIsGovernmentEntityHash_CoversMaskedIDs: um recibo que mascara parte do
// CNPJ deixa visível um trecho; o hash desse trecho tem de continuar a bater.
func TestIsGovernmentEntityHash_CoversMaskedIDs(t *testing.T) {
	const saoPaulo = "46392130000118" // prefeitura de São Paulo (na blacklist default)
	gov := DefaultGovernmentEntities()

	require.True(t, IsGovernmentEntityHash(HashTaxID(saoPaulo), gov), "ID completo")
	require.True(t, IsGovernmentEntityHash(HashTaxID("3921300001"), gov), "trecho mascarado")

	require.False(t, IsGovernmentEntityHash(HashTaxID("12345678000190"), gov))
	require.False(t, IsGovernmentEntityHash("", gov))
	require.False(t, IsGovernmentEntityHash("nao-e-hash", gov), "não-hash falha fechado")
}

// TestIsHashOfWhitelistedEntity_CoversMaskedIDs: o fallback cobre municípios que
// nem estão na blacklist pequena (aqui, Açailândia/MA).
func TestIsHashOfWhitelistedEntity_CoversMaskedIDs(t *testing.T) {
	const acailandia = "07000263000108"

	require.True(t, IsHashOfWhitelistedEntity(HashTaxID(acailandia)))
	require.True(t, IsHashOfWhitelistedEntity(HashTaxID("0002630001")))
	require.False(t, IsHashOfWhitelistedEntity(HashTaxID("12345678000190")))
	require.False(t, IsHashOfWhitelistedEntity("nao-e-hash"))
}
