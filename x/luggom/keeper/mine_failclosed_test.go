package keeper_test

import (
	"testing"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

// DV M1 (achado do parecer do DV Master, 2026-09-18): as fatias da mineração
// são **FAIL-CLOSED**. Antes, um envio que falhasse — **Fundador, Staking
// Reservoir, ATOM Reservoir ou POOL** — era **engolido** (`_ = err`) e a tx
// reportava **SUCESSO** com o LUG preso no módulo. A fatia do pre-sale tinha o
// mesmo bug e foi corrigida no §13.136; estas quatro passaram a falhar alto.
//
// O teste força a falha de cada envio (no mock) e exige que a MINERAÇÃO FALHE.
// §13.146 A: mina-se pelo caminho COM prova (MsgMineTaxPayment) — as msgs legadas
// MsgMine/MsgMineUSD foram desactivadas.
func TestMiningSplit_FailClosedQuandoUmEnvioFalha(t *testing.T) {
	casos := []struct {
		nome string
		addr string
	}{
		{"staking reservoir", authtypes.NewModuleAddress(types.StakingReservoirModuleName).String()},
		{"atom reservoir", authtypes.NewModuleAddress(types.AtomReservoirModuleName).String()},
		{"pool treasury", authtypes.NewModuleAddress(types.PoolTreasuryModuleName).String()},
	}

	for _, tc := range casos {
		t.Run(tc.nome, func(t *testing.T) {
			f := initMineFixture(t)
			f.bank.failSendTo = map[string]bool{tc.addr: true}
			require.NoError(t, f.keeper.Params.Set(f.ctx, devParamsForTest()))

			minerStr, _ := makeAddr(t, f.addressCodec, 0x41)
			_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "failclosed-"+tc.nome, 100))
			require.Error(t, err, "um envio falhado TEM de fazer a mineração falhar (a fatia não pode ficar presa em silêncio)")
			require.Contains(t, err.Error(), "failed to fund")
		})
	}
}

// §13.146 (review multi-agente, agente 1): o founder_address VAZIO também é
// fail-closed. Antes, o `!= ""` saltava o envio em silêncio — e o VAZIO é o
// default dos params, logo uma chain com o genesis por completar cunhava 5% de
// CADA mineração para dentro do módulo, com a tx a reportar sucesso (e o recibo
// a reportar o fundador como pago). O teste do M1 não o apanhava porque só
// exercitava o endereço INVÁLIDO.
func TestMiningSplit_FounderAddressVazioFalha(t *testing.T) {
	f := initMineFixture(t)

	params := devParamsForTest()
	params.FounderAddress = "" // como vem um genesis por completar
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	minerStr, _ := makeAddr(t, f.addressCodec, 0x43)
	_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "founder-vazio", 100))
	require.Error(t, err, "sem founder_address a mineração TEM de falhar (senão os 5% ficam presos em silêncio)")
	require.Contains(t, err.Error(), "founder_address")
}

// O Fundador é o quarto caso: o envio só acontece com `founder_address` definido,
// e um endereço INVÁLIDO também tem de falhar (antes, era saltado em silêncio).
func TestMiningSplit_FounderAddressInvalidoFalha(t *testing.T) {
	f := initMineFixture(t)

	params := devParamsForTest()
	params.FounderAddress = "isto-nao-e-um-endereco"
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	minerStr, _ := makeAddr(t, f.addressCodec, 0x42)
	_, err := f.keeper.MineTaxPayment(f.ctx, taxPaymentMsgForTest(t, f, minerStr, "founder-invalido", 100))
	require.Error(t, err, "founder_address inválido TEM de falhar (senão os 5% ficam no módulo em silêncio)")
	require.Contains(t, err.Error(), "founder_address")
}
