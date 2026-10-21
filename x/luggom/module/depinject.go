package luggom

import (
	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/depinject/appconfig"
	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"

	"luggom/x/luggom/govtally"
	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

var _ depinject.OnePerModuleType = AppModule{}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (AppModule) IsOnePerModuleType() {}

func init() {
	appconfig.Register(
		&types.Module{},
		appconfig.Provide(ProvideModule),
	)
}

type ModuleInputs struct {
	depinject.In

	Config       *types.Module
	StoreService store.KVStoreService
	Cdc          codec.Codec
	AddressCodec address.Codec

	AuthKeeper types.AuthKeeper
	BankKeeper types.BankKeeper
	// FASE 3 (validadores/consenso): staking + slashing para cap 2× média e
	// slashing por timestamp.
	StakingKeeper  types.StakingKeeper
	SlashingKeeper types.SlashingKeeper
}

type ModuleOutputs struct {
	depinject.Out

	LuggomKeeper keeper.Keeper
	Module       appmodule.AppModule
	// Optional input do módulo x/gov (Cosmos SDK depinject): tally ponderado Luggom.
	CustomCalculateVoteResultsAndVotingPowerFn govkeeper.CalculateVoteResultsAndVotingPowerFn
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	// default to governance authority if not provided
	authority := authtypes.NewModuleAddress(types.GovModuleName)
	if in.Config.Authority != "" {
		authority = authtypes.NewModuleAddressOrBech32Address(in.Config.Authority)
	}
	k := keeper.NewKeeper(
		in.StoreService,
		in.Cdc,
		in.AddressCodec,
		authority,
		in.BankKeeper,
		in.StakingKeeper,
		in.SlashingKeeper,
	)
	m := NewAppModule(in.Cdc, k, in.AuthKeeper, in.BankKeeper)
	tallyFn := govtally.NewWeightedTallyFn(&k)

	return ModuleOutputs{
		LuggomKeeper: k,
		Module:       m,
		CustomCalculateVoteResultsAndVotingPowerFn: tallyFn,
	}
}
