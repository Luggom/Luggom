package luggom

import (
	"fmt"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	"github.com/spf13/cobra"

	"luggom/x/luggom/types"
)

// NewLockSavingsCliCmd returns the cobra command for MsgLockSavings (Reservoir — §13.64/§13.65).
func NewLockSavingsCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock-savings [amount] [tranche-days]",
		Short: "Lock LUG in the savings account (Bolsão) for 90/180/365 days",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			amount, ok := math.NewIntFromString(args[0])
			if !ok {
				return fmt.Errorf("invalid amount: %s", args[0])
			}
			var tranche uint32
			if _, err := fmt.Sscanf(args[1], "%d", &tranche); err != nil {
				return fmt.Errorf("invalid tranche-days: %s", args[1])
			}
			msg := &types.MsgLockSavings{
				Depositor:   clientCtx.GetFromAddress().String(),
				Amount:      amount,
				TrancheDays: tranche,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// NewUnlockSavingsCliCmd returns the cobra command for MsgUnlockSavings (early exit).
func NewUnlockSavingsCliCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unlock-savings [lock-id]",
		Short: "Cancel a savings lock before maturity (early exit — principal returns, pending yield stays)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}
			var lockID uint64
			if _, err := fmt.Sscanf(args[0], "%d", &lockID); err != nil {
				return fmt.Errorf("invalid lock-id: %s", args[0])
			}
			msg := &types.MsgUnlockSavings{
				Depositor: clientCtx.GetFromAddress().String(),
				LockId:    lockID,
			}
			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}
	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
