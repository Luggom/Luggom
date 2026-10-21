package keeper

import (
	"context"

	"luggom/x/luggom/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// Mine implements the MsgMine handler.
//
// ⛔ DESACTIVADO (§13.146 A). Reproduzido no review adversarial: esta msg cunhava
// com ZERO prova — só whitelist de CNPJ + comprimentos mínimos —, sem assinatura,
// sem atestação e ignorando a fase (1.000.000 LUG com um hash inventado). A chain
// não precisa dela: o caminho com prova (TLSNotary + notário) é o MsgMineTaxPayment.
//
// A porta fica FECHADA aqui. O tipo continua decodificável (o descritor do proto
// ainda o lista) até o Bloco 2 regenerar o proto; até lá, é este gate que garante
// que a msg NÃO mina.
func (k msgServer) Mine(ctx context.Context, msg *types.MsgMine) (*types.MsgMineResponse, error) {
	return nil, types.ErrMiningMsgDisabled.Wrap("MsgMine")
}

// MineUSD implements the MsgMineUSD handler — desactivado pela mesma razão do Mine
// (§13.146 A). Ver o comentário acima.
func (k msgServer) MineUSD(ctx context.Context, msg *types.MsgMineUSD) (*types.MsgMineUSDResponse, error) {
	return nil, types.ErrMiningMsgDisabled.Wrap("MsgMineUSD")
}

// MineTaxPayment implements the MsgMineTaxPayment handler.
func (k msgServer) MineTaxPayment(ctx context.Context, msg *types.MsgMineTaxPayment) (*types.MsgMineTaxPaymentResponse, error) {
	return k.Keeper.MineTaxPayment(ctx, msg)
}

// UpdateUsdOracleBasket implements dynamic USD basket oracle updates (Etapa C2).
func (k msgServer) UpdateUsdOracleBasket(ctx context.Context, msg *types.MsgUpdateUsdOracleBasket) (*types.MsgUpdateUsdOracleBasketResponse, error) {
	return k.Keeper.UpdateUsdOracleBasket(ctx, msg)
}

// LockSavings implements the MsgLockSavings handler (Bolsão — §13.64/§13.65).
func (k msgServer) LockSavings(ctx context.Context, msg *types.MsgLockSavings) (*types.MsgLockSavingsResponse, error) {
	return k.Keeper.LockSavings(ctx, msg)
}

// UnlockSavings implements the MsgUnlockSavings handler (early exit).
func (k msgServer) UnlockSavings(ctx context.Context, msg *types.MsgUnlockSavings) (*types.MsgUnlockSavingsResponse, error) {
	return k.Keeper.UnlockSavings(ctx, msg)
}

// FASE 6A — Pool nativa (AMM, §13.76)
func (k msgServer) AddLiquidity(ctx context.Context, msg *types.MsgAddLiquidity) (*types.MsgAddLiquidityResponse, error) {
	return k.Keeper.AddLiquidity(ctx, msg)
}

func (k msgServer) RemoveLiquidity(ctx context.Context, msg *types.MsgRemoveLiquidity) (*types.MsgRemoveLiquidityResponse, error) {
	return k.Keeper.RemoveLiquidity(ctx, msg)
}

func (k msgServer) Swap(ctx context.Context, msg *types.MsgSwap) (*types.MsgSwapResponse, error) {
	return k.Keeper.Swap(ctx, msg)
}

// FASE 6B — Escrow P2P on-chain (§13.76/§13.24)
func (k msgServer) CreateEscrow(ctx context.Context, msg *types.MsgCreateEscrow) (*types.MsgCreateEscrowResponse, error) {
	return k.Keeper.CreateEscrow(ctx, msg)
}

func (k msgServer) AcceptEscrow(ctx context.Context, msg *types.MsgAcceptEscrow) (*types.MsgAcceptEscrowResponse, error) {
	return k.Keeper.AcceptEscrow(ctx, msg)
}

func (k msgServer) CancelEscrow(ctx context.Context, msg *types.MsgCancelEscrow) (*types.MsgCancelEscrowResponse, error) {
	return k.Keeper.CancelEscrow(ctx, msg)
}

// BLOCO D — pre-sale on-chain (§13.132 §4).
func (k msgServer) BuyPreSale(ctx context.Context, msg *types.MsgBuyPreSale) (*types.MsgBuyPreSaleResponse, error) {
	return k.Keeper.BuyPreSale(ctx, msg)
}
