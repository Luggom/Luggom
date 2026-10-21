package keeper

import (
	"context"
	"strings"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	sdk "github.com/cosmos/cosmos-sdk/types"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// SDKIterateProposalVotes expõe a iteração de votos para extensões Luggom (tally ponderado).
func (k Keeper) SDKIterateProposalVotes(
	ctx context.Context,
	proposalID uint64,
	cb func(key collections.Pair[uint64, sdk.AccAddress], vote v1.Vote) (bool, error),
) error {
	rng := collections.NewPrefixedPairRange[uint64, sdk.AccAddress](proposalID)
	return k.Votes.Walk(ctx, rng, cb)
}

// SDKRemoveVote remove um voto após o tally (mesmo contrato do SDK default).
func (k Keeper) SDKRemoveVote(ctx context.Context, key collections.Pair[uint64, sdk.AccAddress]) error {
	return k.Votes.Remove(ctx, key)
}

// SDKIterateDelegations delegações de um votante.
func (k Keeper) SDKIterateDelegations(
	ctx context.Context,
	delegator sdk.AccAddress,
	fn func(index int64, delegation stakingtypes.DelegationI) (stop bool),
) error {
	return k.sk.IterateDelegations(ctx, delegator, fn)
}

// SDKValidatorAddressCodec expõe o codec de endereço de validador.
func (k Keeper) SDKValidatorAddressCodec() address.Codec {
	return k.sk.ValidatorAddressCodec()
}

// SDKAuthAddressCodec expõe o codec de conta.
func (k Keeper) SDKAuthAddressCodec() address.Codec {
	return k.authKeeper.AddressCodec()
}

func trimBech32(s string) string {
	return strings.TrimSpace(s)
}
