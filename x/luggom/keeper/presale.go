package keeper

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"luggom/x/luggom/types"
)

// PRE-SALE ON-CHAIN (Bloco D, §13.132 §4).
//
// A venda: o comprador paga em `usdt` e recebe uma posição de vesting LINEAR de
// 3 meses. O LUG vendido está em `luggom_presale` (que acumula os 35% da Fase 1)
// e só sai à medida que veste — `RunPreSaleVesting`, no BeginBlock.
//
// Preço ($0,10), cap (3,5M LUG) e duração (3 meses) são CONSTANTES COMPILADAS
// (types/presale.go): fora do alcance da governança, como a cesta ×3. O que a
// governança decide é só a DISTRIBUIÇÃO do USDT arrecadado
// (`MsgDistributePreSaleProceeds`), porque ele fica numa conta de módulo.

// preSaleLugDenom é o denom do LUG em unidades base.
const preSaleLugDenom = "uluggom"

// getPreSaleInt lê um Item[sdkmath.Int], tratando "não existe" como zero.
func (k Keeper) getPreSaleInt(ctx context.Context, item collections.Item[sdkmath.Int]) (sdkmath.Int, error) {
	v, err := item.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return sdkmath.ZeroInt(), nil
		}
		return sdkmath.ZeroInt(), err
	}
	return v, nil
}

// BuyPreSale vende LUG do pre-sale: o comprador paga `usdt_amount` (unidades
// base de `usdt`) e recebe `PreSaleLugForUSDT(usdt_amount)` numa posição de
// vesting linear de 3 meses. O LUG NÃO sai já — fica em `luggom_presale` e é
// libertado pelo RunPreSaleVesting.
func (k Keeper) BuyPreSale(ctx context.Context, msg *types.MsgBuyPreSale) (*types.MsgBuyPreSaleResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	// 1. Só na Fase 1 (pre-sale activa: < 10M LUG minerados).
	if !k.preSaleActive(sdkCtx) {
		return nil, types.ErrPreSaleClosed.Wrap("pre-sale ended at 10M LUG mined")
	}

	// 2. Mínimo por compra (evita posições-dust no BeginBlock).
	if msg.UsdtAmount.IsNil() || !msg.UsdtAmount.IsPositive() {
		return nil, types.ErrPreSaleBelowMinimum.Wrap("usdt_amount must be positive")
	}
	if msg.UsdtAmount.LT(types.PreSaleMinUSDT()) {
		return nil, types.ErrPreSaleBelowMinimum.Wrapf("%s < %s", msg.UsdtAmount, types.PreSaleMinUSDT())
	}

	// 3. Conversão ao preço fixo (truncada para baixo).
	lugOut := types.PreSaleLugForUSDT(msg.UsdtAmount)
	if !lugOut.IsPositive() {
		return nil, types.ErrPreSaleBelowMinimum.Wrap("purchase rounds to 0 LUG")
	}

	// 4. Cap de 3,5M LUG.
	committed, err := k.getPreSaleInt(sdkCtx, k.PreSaleCommitted)
	if err != nil {
		return nil, err
	}
	if committed.Add(lugOut).GT(types.PreSaleCapLug()) {
		return nil, types.ErrPreSaleCapReached.Wrapf(
			"sold %s + %s > cap %s", committed, lugOut, types.PreSaleCapLug())
	}

	// 5. Backing (fail-closed): a custódia tem de cobrir o que já está prometido e
	// ainda não vestiu, mais esta compra. Nunca prometer LUG que não existe.
	released, err := k.getPreSaleInt(sdkCtx, k.PreSaleReleased)
	if err != nil {
		return nil, err
	}
	preSaleAddr := authtypes.NewModuleAddress(types.PreSaleModuleName)
	custody := k.bankKeeper.SpendableCoins(sdkCtx, preSaleAddr).AmountOf(preSaleLugDenom)
	owed := committed.Sub(released).Add(lugOut)
	if custody.LT(owed) {
		return nil, types.ErrPreSaleNotBacked.Wrapf("custody %s < owed %s", custody, owed)
	}

	// 6. USDT do comprador → conta do pre-sale.
	buyerAddr, err := k.addressCodec.StringToBytes(msg.Buyer)
	if err != nil {
		return nil, fmt.Errorf("invalid buyer address: %w", err)
	}
	usdt := sdk.NewCoins(sdk.NewCoin(types.PreSaleUsdtDenom, msg.UsdtAmount))
	if err := k.bankKeeper.SendCoinsFromAccountToModule(sdkCtx, buyerAddr, types.PreSaleModuleName, usdt); err != nil {
		return nil, fmt.Errorf("pre-sale: failed to receive USDT: %w", err)
	}

	// 7. Posição de vesting.
	id, err := k.PreSalePositionsSeq.Next(sdkCtx)
	if err != nil {
		return nil, err
	}
	now := sdkCtx.BlockTime().Unix()
	pos := types.PreSalePosition{
		Buyer:       msg.Buyer,
		TotalLug:    lugOut,
		ReleasedLug: sdkmath.ZeroInt(),
		StartTime:   now,
		EndTime:     now + types.PreSaleVestingSeconds(),
	}
	if err := k.PreSalePositions.Set(sdkCtx, id, pos); err != nil {
		return nil, err
	}
	if err := k.PreSaleCommitted.Set(sdkCtx, committed.Add(lugOut)); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(types.EventTypePreSale,
		sdk.NewAttribute("action", "buy"),
		sdk.NewAttribute("position_id", strconv.FormatUint(id, 10)),
		sdk.NewAttribute("buyer", msg.Buyer),
		sdk.NewAttribute("usdt_amount", msg.UsdtAmount.String()),
		sdk.NewAttribute("lug_amount", lugOut.String()),
		sdk.NewAttribute("unlock_time", strconv.FormatInt(pos.EndTime, 10)),
	))

	return &types.MsgBuyPreSaleResponse{LugAmount: lugOut, UnlockTime: pos.EndTime}, nil
}

// RunPreSaleVesting liberta o LUG vestido para cada comprador (BeginBlock).
//
// Corre uma vez por DIA UTC (não por bloco): iterar todas as posições a cada
// bloco seria O(n)/bloco. As escritas são aplicadas depois de fechar o iterador.
func (k Keeper) RunPreSaleVesting(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	now := sdkCtx.BlockTime()
	day := now.UTC().Format("20060102")

	last, err := k.PreSaleVestingEpoch.Get(sdkCtx)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return err
	}
	if last == day {
		return nil // já processado hoje
	}

	released, err := k.getPreSaleInt(sdkCtx, k.PreSaleReleased)
	if err != nil {
		return err
	}

	type due struct {
		id  uint64
		pos types.PreSalePosition
	}
	var todo []due

	iter, err := k.PreSalePositions.Iterate(sdkCtx, nil)
	if err != nil {
		return err
	}
	for ; iter.Valid(); iter.Next() {
		id, err := iter.Key()
		if err != nil {
			_ = iter.Close()
			return err
		}
		pos, err := iter.Value()
		if err != nil {
			_ = iter.Close()
			return err
		}
		already := pos.ReleasedLug
		if already.IsNil() {
			already = sdkmath.ZeroInt()
		}
		if pos.VestedAt(now.Unix()).Sub(already).IsPositive() {
			todo = append(todo, due{id: id, pos: pos})
		}
	}
	if err := iter.Close(); err != nil {
		return err
	}

	for _, d := range todo {
		already := d.pos.ReleasedLug
		if already.IsNil() {
			already = sdkmath.ZeroInt()
		}
		vested := d.pos.VestedAt(now.Unix())
		delta := vested.Sub(already)
		if !delta.IsPositive() {
			continue
		}
		buyerAddr, err := k.addressCodec.StringToBytes(d.pos.Buyer)
		if err != nil {
			return fmt.Errorf("pre-sale vesting: invalid buyer %q: %w", d.pos.Buyer, err)
		}
		coins := sdk.NewCoins(sdk.NewCoin(preSaleLugDenom, delta))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(sdkCtx, types.PreSaleModuleName, buyerAddr, coins); err != nil {
			return fmt.Errorf("pre-sale vesting: release to %s failed: %w", d.pos.Buyer, err)
		}
		d.pos.ReleasedLug = vested
		if err := k.PreSalePositions.Set(sdkCtx, d.id, d.pos); err != nil {
			return err
		}
		released = released.Add(delta)
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(types.EventTypePreSale,
			sdk.NewAttribute("action", "vesting_release"),
			sdk.NewAttribute("position_id", strconv.FormatUint(d.id, 10)),
			sdk.NewAttribute("buyer", d.pos.Buyer),
			sdk.NewAttribute("lug_amount", delta.String()),
		))
	}

	if err := k.PreSaleReleased.Set(sdkCtx, released); err != nil {
		return err
	}
	return k.PreSaleVestingEpoch.Set(sdkCtx, day)
}

// DistributePreSaleProceeds move o USDT acumulado no pre-sale para um destino
// (tesouraria da pool, marketing). Gated pela authority: um module account
// não pode ser gasto por um utilizador com um send normal — sem esta Msg o
// dinheiro ficaria preso (§13.135).
func (k msgServer) DistributePreSaleProceeds(ctx context.Context, msg *types.MsgDistributePreSaleProceeds) (*types.MsgDistributePreSaleProceedsResponse, error) {
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
	coins := sdk.NewCoins(sdk.NewCoin(types.PreSaleUsdtDenom, msg.Amount))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.PreSaleModuleName, dest, coins); err != nil {
		return nil, fmt.Errorf("failed to distribute pre-sale proceeds: %w", err)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(types.EventTypePreSale,
		sdk.NewAttribute("action", "distribute"),
		sdk.NewAttribute("destination", msg.Destination),
		sdk.NewAttribute("amount", msg.Amount.String()),
	))
	return &types.MsgDistributePreSaleProceedsResponse{}, nil
}
