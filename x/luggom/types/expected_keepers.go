package types

import (
	"context"

	"cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// AuthKeeper defines the expected interface for the Auth module.
type AuthKeeper interface {
	AddressCodec() address.Codec
	GetAccount(context.Context, sdk.AccAddress) sdk.AccountI // only used for simulation
	// Methods imported from account should be defined here
}

// BankKeeper defines the expected interface for the Bank module.
type BankKeeper interface {
	SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins
	MintCoins(context.Context, string, sdk.Coins) error
	SendCoinsFromModuleToAccount(context.Context, string, sdk.AccAddress, sdk.Coins) error
	SendCoinsFromModuleToModule(context.Context, string, string, sdk.Coins) error
	SendCoinsFromAccountToModule(context.Context, sdk.AccAddress, string, sdk.Coins) error
	GetSupply(context.Context, string) sdk.Coin
	// Methods imported from bank should be defined here
}

// StakingKeeper defines the expected interface for the Staking module (FASE 3 —
// validator cap 2× média and timestamp slashing, §13.24/§13.25).
type StakingKeeper interface {
	TotalBondedTokens(context.Context) (sdkmath.Int, error)
	GetBondedValidatorsByPower(context.Context) ([]stakingtypes.Validator, error)
	GetValidator(context.Context, sdk.ValAddress) (stakingtypes.Validator, error)
	GetValidatorByConsAddr(context.Context, sdk.ConsAddress) (stakingtypes.Validator, error)
	GetLastValidatorPower(context.Context, sdk.ValAddress) (int64, error)
	Jail(context.Context, sdk.ConsAddress) error
	// Methods imported from staking should be defined here
}

// SlashingKeeper defines the expected interface for the Slashing module (FASE 3).
type SlashingKeeper interface {
	GetValidatorSigningInfo(context.Context, sdk.ConsAddress) (slashingtypes.ValidatorSigningInfo, error)
	IterateValidatorSigningInfos(context.Context, func(sdk.ConsAddress, slashingtypes.ValidatorSigningInfo) (stop bool)) error
	// Methods imported from slashing should be defined here
}

// ParamSubspace defines the expected Subspace interface for parameters.
type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
