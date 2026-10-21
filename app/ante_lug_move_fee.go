package app

import (
	"context"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"

	luggomtypes "luggom/x/luggom/types"
)

// lugMovementFlatFeeUluggom is 0.005 LUG in base units (18 decimals).
// §13.87 (2026-08-30): reduced from 0.02 LUG — at ~$7.90/LUG, 0.02 LUG ≈ US$0.158/tx
// (≈ R$0.80) was too expensive; 0.005 LUG ≈ US$0.04/tx (≈ R$0.20) keeps the
// anti-spam barrier at a sane level for users.
var lugMovementFlatFeeUluggom = sdkmath.NewInt(5_000_000_000_000_000)

func lugMovementFeeCoins() sdk.Coins {
	return sdk.NewCoins(sdk.NewCoin(sdk.DefaultBondDenom, lugMovementFlatFeeUluggom))
}

// dustReserveAddress é a ÚNICA conta isenta da taxa fixa de movimento (§13.87).
//
// Porquê: ativar uma carteira nova custa um envio (MsgSend) do pó que cria a
// conta. Sem isenção, esse envio pagaria 0,005 LUG de taxa — 500 milhões de
// vezes o valor enviado (o pó é 1e-11 LUG), e a reserva de 20M carteiras
// passaria de 0,000204 LUG para 100.000 LUG. Ver as decisões do DUST (D1–D9) no
// checkpoint03.
//
// ⚠️ Porquê ESTREITA (só esta conta): isentar qualquer envio que crie conta
// tornaria criar contas ~25 mil milhões× mais barato (DoS de estado). E criar a
// conta no ante seria pior — o estado do ante persiste mesmo com a msg a falhar
// (baseapp.go faz msCache.Write() antes de correr a msg) → contas grátis sem
// prova nenhuma.
//
// ⚠️ Compilada e não em params: a taxa acima também o é, e o `buf` está quebrado
// (adicionar um campo a Params é trabalho do Bloco 2). Na mainnet este endereço
// muda (outra seed) → deve migrar para `params.dust_reserve_address` (governável)
// quando o proto for regenerado. Até lá, é config de rede tal como a taxa.
//
// Valor por defeito = conta GGOM da chain local (a chave que a API usa). Se não
// corresponder a nenhuma conta da rede, a isenção simplesmente não se aplica
// (falha segura: todos pagam).
var dustReserveAddress = "luggom16ffvkvwz7nxekjx22xe4fcj2ysc4sg73du0nna"

// SetDustReserveAddress define a conta isenta (vazio = ninguém isento).
func SetDustReserveAddress(addr string) {
	dustReserveAddress = addr
}

// isDustReserveSender indica se addr é a conta isenta da taxa fixa.
func isDustReserveSender(addr sdk.AccAddress) bool {
	if dustReserveAddress == "" {
		return false
	}
	reserve, err := sdk.AccAddressFromBech32(dustReserveAddress)
	if err != nil {
		return false
	}
	return reserve.Equals(addr)
}

func firstNativeLugSender(tx sdk.Tx) (sdk.AccAddress, bool) {
	denom := sdk.DefaultBondDenom
	for _, msg := range tx.GetMsgs() {
		switch m := msg.(type) {
		case *banktypes.MsgSend:
			if m.Amount.AmountOf(denom).IsPositive() {
				addr, err := sdk.AccAddressFromBech32(m.FromAddress)
				if err != nil {
					continue
				}
				return addr, true
			}
		case *banktypes.MsgMultiSend:
			for _, in := range m.Inputs {
				if in.Coins.AmountOf(denom).IsPositive() {
					addr, err := sdk.AccAddressFromBech32(in.Address)
					if err != nil {
						continue
					}
					return addr, true
				}
			}
		case *luggomtypes.MsgSwap:
			// FASE 6A: swap moves LUG (either side of the pair) — charge the flat fee.
			addr, err := sdk.AccAddressFromBech32(m.Trader)
			if err != nil {
				continue
			}
			return addr, true
		}
	}
	return nil, false
}

type lugMovementFeeBank interface {
	SendCoins(ctx context.Context, from, to sdk.AccAddress, amt sdk.Coins) error
}

// wrapAnteHandlerWithLugMovementFee charges a flat 0.005 LUG to the first uluggom sender
// in the tx (bank MsgSend / MsgMultiSend), after the default ante (gas fees already paid).
// A conta da reserva de pó (dustReserveAddress) é ISENTA — é ela que cria as contas
// dos mineradores novos com o pó (ver o comentário de dustReserveAddress).
// Simulate skips the charge.
func wrapAnteHandlerWithLugMovementFee(bk lugMovementFeeBank, inner sdk.AnteHandler) sdk.AnteHandler {
	if inner == nil {
		return nil
	}
	fee := lugMovementFeeCoins()
	feeCol := authtypes.NewModuleAddress(authtypes.FeeCollectorName)

	return func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		newCtx, err := inner(ctx, tx, simulate)
		if err != nil {
			return newCtx, err
		}
		if simulate {
			return newCtx, nil
		}
		from, ok := firstNativeLugSender(tx)
		if !ok {
			return newCtx, nil
		}
		// Reserva de pó: isenta (é operação de sistema, não movimento de LUG do
		// utilizador). Sem isto, ativar uma carteira custaria 0,005 LUG por um
		// envio de 0,00000000001 LUG.
		if isDustReserveSender(from) {
			return newCtx, nil
		}
		if err := bk.SendCoins(newCtx, from, feeCol, fee); err != nil {
			return newCtx, sdkerrors.ErrInsufficientFunds.Wrapf("lug movement fee (0.005 LUG): %v", err)
		}
		return newCtx, nil
	}
}
