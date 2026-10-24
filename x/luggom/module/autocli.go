package luggom

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	"luggom/x/luggom/types"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: types.Query_serviceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Shows the parameters of the module",
				},
				{
					RpcMethod: "OracleFixing",
					Use:       "oracle-fixing [day]",
					Short:     "Fixing of a day (YYYY-MM-DD), embedded history first, then the store",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "day"},
					},
				},
				// this line is used by ignite scaffolding # autocli/query
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service:              types.Msg_serviceDesc.ServiceName,
			EnhanceCustomCommand: true, // only required if you want to use the custom command
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "UpdateParams",
					Skip:      true, // skipped because authority gated
				},
				// MsgMineUSD: subcomando `mine-usd` em GetTxCmd (cli_tx.go); não está no FileResolver do autocli.
				// this line is used by ignite scaffolding # autocli/tx
			},
		},
	}
}
