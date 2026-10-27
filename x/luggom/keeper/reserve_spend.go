package keeper

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"luggom/x/luggom/types"
)

const (
	// §16 — um payee novo só recebe depois disto. Sem o atraso, uma única
	// proposta podia adicionar o endereço (MsgUpdateParams) e pagá-lo
	// (MsgSpendFromReserve) na mesma execução: o x/gov corre as mensagens por
	// ordem. Com o atraso, "a governança muda quem, não para onde" é verdade.
	// Piso do atraso. O valor efectivo é derivado do x/gov em notaryPayeeDelay.
	notaryPayeeDelayFloor = 7 * 24 * time.Hour

	// Tecto por janela: no máximo 25% do saldo da reserva em cada 7 dias.
	// Contém uma sequência de propostas que, sem ele, esvaziaria a reserva.
	notarySpendWindowSeconds = int64(7 * 24 * 60 * 60)
	notarySpendWindowCapBps  = int64(2_500)
)

// SpendFromReserve gasta a tesouraria (gov) ou a reserva do notariado.
//
// Uma conta de módulo não tem chave e não pode assinar um `MsgSend`: sem esta
// mensagem o saldo das duas reservas acumulava para sempre (§14). A authority é
// o x/gov — é o único caminho de saída.
//
// A tesouraria é discricionária (qualquer destino). A reserva do notariado NÃO:
// o destino tem de constar em `params.notary_payees`, ter entrado há pelo menos
// 7 dias, e cada janela de 7 dias gasta no máximo 25% do saldo. É isso que
// mantém a promessa documentada no whitepaper — a governança muda QUEM é
// elegível para atestar, não PARA ONDE vai o orçamento.
func (k msgServer) SpendFromReserve(ctx context.Context, msg *types.MsgSpendFromReserve) (*types.MsgSpendFromReserveResponse, error) {
	if err := k.authorityOk(ctx, msg.Authority); err != nil {
		return nil, err
	}
	if msg.Amount.IsNil() || !msg.Amount.IsPositive() {
		return nil, fmt.Errorf("amount must be positive")
	}
	dest, err := k.addressCodec.StringToBytes(msg.Destination)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}
	// Forma canónica (a lista é validada assim): evita que uma variante de
	// maiúsculas na lista nunca case com o destino.
	canonical, err := k.addressCodec.BytesToString(dest)
	if err != nil {
		return nil, fmt.Errorf("invalid destination address: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime().Unix()

	var module string
	switch msg.Reserve {
	case types.ReserveGov:
		module = types.GovReserveModuleName

	case types.ReserveNotary:
		module = types.NotaryReserveModuleName

		params, err := k.Params.Get(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to read params: %w", err)
		}
		listed := false
		for _, payee := range params.NotaryPayees {
			if payee == canonical {
				listed = true
				break
			}
		}
		if !listed {
			return nil, fmt.Errorf(
				"destination %s is not in notary_payees: the notary budget is not discretionary", canonical)
		}

		// Atraso: entrar na lista não torna o endereço elegível.
		addedAt, err := k.NotaryPayeeAddedAt.Get(ctx, canonical)
		if err != nil {
			if errors.Is(err, collections.ErrNotFound) {
				return nil, fmt.Errorf(
					"destination %s has no entry time: adding it to notary_payees does not make it eligible immediately",
					canonical)
			}
			return nil, err
		}
		delay, err := k.notaryPayeeDelay(ctx)
		if err != nil {
			return nil, err
		}
		delaySecs := int64(delay / time.Second)
		if now < addedAt+delaySecs {
			daysLeft := (addedAt + delaySecs - now + 86_399) / 86_400
			return nil, fmt.Errorf(
				"destination %s is eligible in %d day(s): the notary budget pays who is eligible, not when it moves",
				canonical, daysLeft)
		}

	default:
		return nil, fmt.Errorf("unknown reserve %q (want %q or %q)", msg.Reserve, types.ReserveGov, types.ReserveNotary)
	}

	// Tecto por janela, só na reserva do notariado. O saldo é lido da conta de
	// módulo — o tecto é uma fracção do que lá está, não um número fixo. Nota:
	// é uma janela ANCORADA no primeiro gasto, não uma janela móvel; na pior das
	// hipóteses saem 50% do saldo em 7 dias (fim de uma janela + início da
	// seguinte). Contém a velocidade, não o esgotamento.
	var windowFrom int64
	var windowSpent sdkmath.Int
	if msg.Reserve == types.ReserveNotary {
		balance := k.bankKeeper.SpendableCoins(ctx, authtypes.NewModuleAddress(module)).AmountOf("uluggom")
		capAmount := balance.MulRaw(notarySpendWindowCapBps).QuoRaw(10_000)
		if capAmount.IsZero() {
			// Reserva minúscula: o tecto proporcional arredondaria para zero e o
			// pó ficaria preso para sempre.
			capAmount = balance
		}
		if msg.Amount.GT(capAmount) {
			return nil, fmt.Errorf(
				"amount %s exceeds the per-window cap of %s (25%% of the reserve)",
				msg.Amount, capAmount)
		}

		windowFrom, err = k.NotarySpendWindowFrom.Get(ctx)
		if err != nil {
			if !errors.Is(err, collections.ErrNotFound) {
				return nil, err
			}
			windowFrom = now
		}
		spentStr, err := k.NotarySpendWindowSpent.Get(ctx)
		if err != nil && !errors.Is(err, collections.ErrNotFound) {
			return nil, err
		}
		windowSpent = sdkmath.ZeroInt()
		if spentStr != "" {
			v, ok := sdkmath.NewIntFromString(spentStr)
			if !ok {
				return nil, fmt.Errorf("corrupt notary spend window amount %q", spentStr)
			}
			windowSpent = v
		}
		if now-windowFrom >= notarySpendWindowSeconds {
			windowFrom, windowSpent = now, sdkmath.ZeroInt()
		}
		if windowSpent.Add(msg.Amount).GT(capAmount) {
			return nil, fmt.Errorf(
				"window cap reached: %s already spent this window, %s requested, cap %s",
				windowSpent, msg.Amount, capAmount)
		}
	}

	coins := sdk.NewCoins(sdk.NewCoin("uluggom", msg.Amount))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, module, dest, coins); err != nil {
		return nil, fmt.Errorf("failed to spend from the %s reserve: %w", msg.Reserve, err)
	}

	if msg.Reserve == types.ReserveNotary {
		if err := k.NotarySpendWindowFrom.Set(ctx, windowFrom); err != nil {
			return nil, err
		}
		if err := k.NotarySpendWindowSpent.Set(ctx, windowSpent.Add(msg.Amount).String()); err != nil {
			return nil, err
		}
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(types.EventTypeReserveSpend,
		sdk.NewAttribute("reserve", msg.Reserve),
		sdk.NewAttribute("reserve_module", module),
		sdk.NewAttribute("destination", msg.Destination),
		sdk.NewAttribute("amount", msg.Amount.String()),
	))

	return &types.MsgSpendFromReserveResponse{}, nil
}

// notaryPayeeDelay devolve o atraso mínimo entre entrar na lista e poder receber.
//
// Deriva do x/gov, não de um múltiplo: uma remoção só começa a votar quando
// atinge o `min_deposit`, e o x/gov executa a proposta aprovada no EndBlock em
// que a votação termina — a execução não acrescenta tempo. O pior caso é
// `max_deposit_period + voting_period + 1 dia` (para alguém notar o add e
// submeter). A via expedita pode ser mais rápida, mas dimensiona-se pela normal:
// uma expedita que falhe o limiar passa a normal e demora um `voting_period`
// inteiro. Piso de 7 dias.
func (k Keeper) notaryPayeeDelay(ctx context.Context) (time.Duration, error) {
	deposit, voting, err := k.govPeriods(ctx)
	if err != nil {
		return 0, fmt.Errorf("cannot read gov periods: %w", err)
	}
	delay := deposit + voting + 24*time.Hour
	if delay < notaryPayeeDelayFloor {
		delay = notaryPayeeDelayFloor
	}
	return delay, nil
}
