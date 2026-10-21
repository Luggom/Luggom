package types

import (
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterInterfaces(registrar codectypes.InterfaceRegistry) {
	registrar.RegisterImplementations((*sdk.Msg)(nil),
		&MsgUpdateParams{},
		// ⚠️ MsgMine/MsgMineUSD estão DESACTIVADAS (§13.146 A): os handlers
		// recusam-nas (ErrMiningMsgDisabled). Continuam a ser registadas aqui — e
		// o `msgservice.RegisterMsgServiceDesc` abaixo volta a registá-las a partir
		// do descritor do proto, pelo que removê-las daqui seria cosmético. A
		// remoção definitiva do tipo é do Bloco 2 (regenerar o proto).
		&MsgMine{},
		&MsgMineUSD{},
		&MsgMineTaxPayment{},
		&MsgUpdateUsdOracleBasket{},
		&MsgLockSavings{},
		&MsgUnlockSavings{},
		// FASE 5 — governança de listas operacionais (§13.42)
		&MsgAddTaxEntity{},
		&MsgRemoveTaxEntity{},
		&MsgAddBankProvider{},
		&MsgRemoveBankProvider{},
		&MsgAddPoolAsset{},
		&MsgRemovePoolAsset{},
		// FASE 6A — Pool nativa (AMM, §13.76)
		&MsgAddLiquidity{},
		&MsgRemoveLiquidity{},
		&MsgSwap{},
		// FASE 6B — Escrow P2P on-chain (§13.76/§13.24)
		&MsgCreateEscrow{},
		&MsgAcceptEscrow{},
		&MsgCancelEscrow{},
		// BLOCO D — pre-sale on-chain (§13.132 §4)
		&MsgBuyPreSale{},
		&MsgDistributePreSaleProceeds{},
	)
	msgservice.RegisterMsgServiceDesc(registrar, &_Msg_serviceDesc)
}
