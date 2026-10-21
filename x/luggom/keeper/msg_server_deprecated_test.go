package keeper_test

import (
	"testing"

	sdkmath "cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/keeper"
	"luggom/x/luggom/types"
)

// §13.146 A — as msgs legadas de mineração (MsgMine/MsgMineUSD) cunhavam com
// ZERO prova (reproduzido no review adversarial: 1.000.000 LUG com um hash
// INVENTADO, sem assinatura, sem atestação e ignorando a fase). A porta passa a
// estar FECHADA no handler.
//
// Este teste pina isso contra o conteúdo da msg: seja qual for, nenhuma das duas
// cunha. Se alguém voltar a ligá-las ao keeper, este teste cai.
func TestLegacyMineMsgsAreDisabled(t *testing.T) {
	f := initMineFixture(t)
	ms := keeper.NewMsgServerImpl(f.keeper)

	minerStr, _ := makeAddr(t, f.addressCodec, 0x61)

	_, err := ms.Mine(f.ctx, &types.MsgMine{
		MinerAddress:       minerStr,
		BrlAmount:           sdkmath.LegacyMustNewDecFromStr("123456"), // ~10.000 LUG
		DestinationCnpj:        "12345678000199",                          // §13.170: whitelist_tax_ids foi APAGADO
		PixReceiptHash: "hash-inventado-pelo-atacante",
		IptuIpvaException:    true,
	})
	require.ErrorIs(t, err, types.ErrMiningMsgDisabled, "MsgMine TEM de estar fechada (§13.146 A)")
	require.Empty(t, f.bank.mints, "MsgMine não pode cunhar NADA")

	_, err = ms.MineUSD(f.ctx, &types.MsgMineUSD{
		MinerAddress: minerStr,
		PaymentAmount:     sdkmath.LegacyMustNewDecFromStr("1000000"),
		TaxEntity:    "irs",
		DocumentHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SsnHash:      "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		ProofIrs:     "proof-inventado",
	})
	require.ErrorIs(t, err, types.ErrMiningMsgDisabled, "MsgMineUSD TEM de estar fechada (§13.146 A)")
	require.Empty(t, f.bank.mints, "MsgMineUSD não pode cunhar NADA")
}
