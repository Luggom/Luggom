package types_test

import (
	"testing"

	"luggom/x/luggom/types"

	"github.com/stretchr/testify/require"
)

func TestGenesisState_Validate(t *testing.T) {
	tests := []struct {
		desc     string
		genState *types.GenesisState
		valid    bool
	}{
		{
			desc:     "default is valid",
			genState: types.DefaultGenesis(),
			valid:    true,
		},
		{
			desc:     "empty genesis invalid (params must be set)",
			genState: &types.GenesisState{},
			valid:    false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.genState.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				// §13.170: `parity_mvp` foi apagado; o primeiro erro de um genesis
				// vazio passa a ser o dos pesos do tally.
				require.Contains(t, err.Error(), "gov vote weights")
			}
		})
	}
}
