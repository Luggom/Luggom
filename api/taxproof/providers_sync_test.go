package taxproof

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/require"

	"luggom/api/reclaimverify"
	govtypes "luggom/x/luggom/types"
)

// A whitelist de bancos vive em DOIS sítios:
//   - off-chain: `reclaimverify.KnownProviders()` (ID + domínio, usado no host check)
//   - on-chain:  `types.DefaultBankProviders()` (gov 67%, usado no portão do /mine/tax)
//
// O comentário do segundo diz "mirrors" o primeiro, mas **nada o verifica** — a
// cada banco novo há risco de deriva silenciosa: o banco é aceite numa camada e
// recusado na outra (403/1208 ou host check falhado), de forma difícil de
// diagnosticar. Este teste falha na divergência.
func TestBankProviderListsDoNotDrift(t *testing.T) {
	api := make([]string, 0, 20)
	for _, p := range reclaimverify.KnownProviders() {
		api = append(api, p.ID)
	}
	chain := govtypes.DefaultBankProviders()

	sort.Strings(api)
	sort.Strings(chain)
	require.Equal(t, chain, api,
		"as listas de bancos divergiram (reclaimverify.KnownProviders vs types.DefaultBankProviders)")
}
