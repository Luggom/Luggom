package keeper

import (
	"context"
	"fmt"
	"strings"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	v1 "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// LuggomCalculateVoteResultsAndVotingPower aplica pesos LLC / fundador / validadores (basis points)
// ao poder de voto delegado, seguindo a estrutura de defaultCalculateVoteResultsAndVotingPower.
func LuggomCalculateVoteResultsAndVotingPower(
	ctx context.Context,
	k Keeper,
	proposal v1.Proposal,
	validators map[string]v1.ValidatorGovInfo,
	wl, wf, wv math.LegacyDec,
	fundadorBech32, llcValoperBech32 string,
) (totalVotingPower math.LegacyDec, results map[v1.VoteOption]math.LegacyDec, err error) {
	totalVotingPower = math.LegacyZeroDec()

	results = make(map[v1.VoteOption]math.LegacyDec)
	results[v1.OptionYes] = math.LegacyZeroDec()
	results[v1.OptionAbstain] = math.LegacyZeroDec()
	results[v1.OptionNo] = math.LegacyZeroDec()
	results[v1.OptionNoWithVeto] = math.LegacyZeroDec()

	fundadorBech32 = trimBech32(fundadorBech32)
	llcValoperBech32 = trimBech32(llcValoperBech32)

	var founderValStr string
	if fundadorBech32 != "" {
		if fa, e := sdk.AccAddressFromBech32(fundadorBech32); e == nil {
			founderValStr = sdk.ValAddress(fa).String()
		}
	}

	votesToRemove := []collections.Pair[uint64, sdk.AccAddress]{}
	err = k.SDKIterateProposalVotes(ctx, proposal.Id, func(key collections.Pair[uint64, sdk.AccAddress], vote v1.Vote) (bool, error) {
		voter, err := k.SDKAuthAddressCodec().StringToBytes(vote.Voter)
		if err != nil {
			return false, err
		}

		valAddrStr, err := k.SDKValidatorAddressCodec().BytesToString(voter)
		if err != nil {
			return false, err
		}
		if val, ok := validators[valAddrStr]; ok {
			val.Vote = vote.Options
			validators[valAddrStr] = val
		}

		bucket := voterBucketDelegator(vote.Voter, fundadorBech32, llcValoperBech32, wl, wf, wv)

		err = k.SDKIterateDelegations(ctx, voter, func(index int64, delegation stakingtypes.DelegationI) (stop bool) {
			valAddrStr := delegation.GetValidatorAddr()

			if val, ok := validators[valAddrStr]; ok {
				val.DelegatorDeductions = val.DelegatorDeductions.Add(delegation.GetShares())
				validators[valAddrStr] = val

				votingPower := delegation.GetShares().MulInt(val.BondedTokens).Quo(val.DelegatorShares)
				votingPower = votingPower.Mul(bucket)

				for _, option := range vote.Options {
					weight, _ := math.LegacyNewDecFromStr(option.Weight)
					subPower := votingPower.Mul(weight)
					results[option.Option] = results[option.Option].Add(subPower)
				}
				totalVotingPower = totalVotingPower.Add(votingPower)
			}

			return false
		})
		if err != nil {
			return false, err
		}

		votesToRemove = append(votesToRemove, key)
		return false, nil
	})
	if err != nil {
		return math.LegacyZeroDec(), nil, fmt.Errorf("error while iterating delegations: %w", err)
	}

	for _, key := range votesToRemove {
		if err := k.SDKRemoveVote(ctx, key); err != nil {
			return math.LegacyDec{}, nil, fmt.Errorf("error while removing vote (%d/%s): %w", key.K1(), key.K2(), err)
		}
	}

	for valAddrStr, val := range validators {
		if len(val.Vote) == 0 {
			continue
		}

		sharesAfterDeductions := val.DelegatorShares.Sub(val.DelegatorDeductions)
		votingPower := sharesAfterDeductions.MulInt(val.BondedTokens).Quo(val.DelegatorShares)

		bucket := validatorSelfBucket(valAddrStr, founderValStr, llcValoperBech32, wl, wf, wv)
		votingPower = votingPower.Mul(bucket)

		for _, option := range val.Vote {
			weight, _ := math.LegacyNewDecFromStr(option.Weight)
			subPower := votingPower.Mul(weight)
			results[option.Option] = results[option.Option].Add(subPower)
		}
		totalVotingPower = totalVotingPower.Add(votingPower)
	}

	return totalVotingPower, results, nil
}

func voterBucketDelegator(voterBech32, fundadorBech32, llcValoper string, wl, wf, wv math.LegacyDec) math.LegacyDec {
	voterBech32 = trimBech32(voterBech32)
	if llcValoper != "" && voterBech32 == llcValoper {
		return wl
	}
	if fundadorBech32 != "" && voterBech32 == fundadorBech32 {
		if llcValoper == "" && wl.IsPositive() {
			return wl.Add(wf)
		}
		return wf
	}
	return wv
}

func validatorSelfBucket(valAddrStr, founderValStr, llcValoper string, wl, wf, wv math.LegacyDec) math.LegacyDec {
	valAddrStr = strings.TrimSpace(valAddrStr)
	if llcValoper != "" && valAddrStr == llcValoper {
		return wl
	}
	if founderValStr != "" && valAddrStr == founderValStr {
		if llcValoper == "" && wl.IsPositive() {
			return wl.Add(wf)
		}
		return wf
	}
	return wv
}
