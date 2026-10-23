package keeper_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"cosmossdk.io/core/address"
	sdkmath "cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	addresscodec "github.com/cosmos/cosmos-sdk/codec/address"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"luggom/api/reclaimverify"
	"luggom/x/luggom/keeper"
	module "luggom/x/luggom/module"
	"luggom/x/luggom/types"
)

type bankMintCall struct {
	module string
	coins  sdk.Coins
}

type bankSendCall struct {
	module string
	to     sdk.AccAddress
	coins  sdk.Coins
}

type trackingBankKeeper struct {
	mints  []bankMintCall
	sends  []bankSendCall
	supply sdk.Coin // configurable total uluggom supply (PoolOpen trigger, §13.84)
	// spendableByAddr simula os saldos por endereço (usado pelo drip dos
	// reservatórios — §13.119, que lê SpendableCoins do módulo).
	spendableByAddr map[string]sdk.Coins
	// failSendTo força a FALHA do envio para estes endereços — para testar que
	// cada fatia da mineração é fail-closed (DV M1).
	failSendTo map[string]bool
}

func (m *trackingBankKeeper) SpendableCoins(_ context.Context, addr sdk.AccAddress) sdk.Coins {
	if m.spendableByAddr != nil {
		if c, ok := m.spendableByAddr[addr.String()]; ok {
			return c
		}
	}
	return nil
}

func (m *trackingBankKeeper) MintCoins(_ context.Context, moduleName string, amt sdk.Coins) error {
	m.mints = append(m.mints, bankMintCall{module: moduleName, coins: amt})
	return nil
}

func (m *trackingBankKeeper) SendCoinsFromModuleToAccount(_ context.Context, moduleName string, recipientAddr sdk.AccAddress, amt sdk.Coins) error {
	if m.failSendTo[recipientAddr.String()] {
		return fmt.Errorf("mock: envio forçado a falhar para %s", recipientAddr)
	}
	dup := make(sdk.AccAddress, len(recipientAddr))
	copy(dup, recipientAddr)
	m.sends = append(m.sends, bankSendCall{module: moduleName, to: dup, coins: amt})
	return nil
}

func (m *trackingBankKeeper) SendCoinsFromModuleToModule(_ context.Context, moduleName, recipientModule string, amt sdk.Coins) error {
	// Registra no mesmo ledger, resolvendo o módulo destino para o seu endereço
	// (é por onde saem as fatias de módulo: pre-sale, reservatórios, POOL, cofre
	// OTC, fundador-em-vesting, fee_collector). Respeita `failSendTo` para que o
	// teste de fail-closed (DV M1) continue a cobrir este caminho.
	recipient := authtypes.NewModuleAddress(recipientModule)
	if m.failSendTo[recipient.String()] {
		return fmt.Errorf("mock: envio forçado a falhar para %s", recipient)
	}
	m.sends = append(m.sends, bankSendCall{
		module: moduleName,
		to:     recipient,
		coins:  amt,
	})
	return nil
}

func (m *trackingBankKeeper) SendCoinsFromAccountToModule(context.Context, sdk.AccAddress, string, sdk.Coins) error {
	return nil
}

func (m *trackingBankKeeper) GetSupply(_ context.Context, _ string) sdk.Coin {
	if m.supply.IsNil() || m.supply.IsZero() {
		return sdk.NewCoin("uluggom", sdkmath.ZeroInt())
	}
	return m.supply
}

type mineFixture struct {
	ctx          context.Context
	keeper       keeper.Keeper
	addressCodec address.Codec
	bank         *trackingBankKeeper
}

// testNotaryKeyHex — chave DEV do "notário" dos testes (32 bytes de 0x22).
// §13.170 Grupo B: a fase TLSNotary é uma CONSTANTE de código (2) — deixou de
// haver fase 1. Por isso os testes passaram a ASSINAR a atestação com esta chave
// e a registar o endereço dela em `api_verifier_address`.
const testNotaryKeyHex = "2222222222222222222222222222222222222222222222222222222222222222"

// testChainID é o chain-id da fixture. Entra no digest v4: a atestação vale
// para esta chain e não para outra (não se replay-a de testnet para mainnet).
const testChainID = "luggom-test"

var testNotaryAddress = func() string {
	addr, err := reclaimverify.AttestationSignerAddress(testNotaryKeyHex)
	if err != nil {
		panic(err)
	}
	return addr
}()

// devParamsForTest devolve os params com a chave dev do notário registada.
func devParamsForTest() types.Params {
	p := types.DefaultParams()
	p.ApiVerifierAddress = testNotaryAddress
	// §13.146 (review multi-agente): com founder_address. A mineração passou a ser
	// FAIL-CLOSED sem ele — antes, o endereço vazio saltava o envio em silêncio e
	// os 5% de cada mineração ficavam presos no módulo, com a tx a dizer sucesso.
	codec := addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix())
	if addr, err := codec.BytesToString(bytes.Repeat([]byte{0x77}, 20)); err == nil {
		p.FounderAddress = addr
	}
	return p
}

// taxPaymentMsgForTest constrói um MsgMineTaxPayment VÁLIDO para a fixture — o
// caminho de mineração COM prova que substitui o MsgMine/MsgMineUSD desactivados
// (§13.146 A). A fase TLSNotary fica em 1 (atestação aceite por comprimento; ver
// devParamsForTest) e o valor é calculado para render `lug` LUG à paridade do
// snapshot da fixture ((1,10+1,18+1,37)/3 × 3 = 3,65 USD/LUG).
//
// `unique` tem de ser DISTINTO por mineração: o proof_commitment e o payment_id
// são anti-replay — repeti-los é, por desenho, um replay.
func taxPaymentMsgForTest(t *testing.T, f *mineFixture, miner, unique string, lug int64) *types.MsgMineTaxPayment {
	t.Helper()
	sum := sha256.Sum256([]byte("proof-" + unique))
	msg := &types.MsgMineTaxPayment{
		MinerAddress:    miner,
		PaymentAmount:        sdkmath.LegacyNewDec(lug).Mul(sdkmath.LegacyMustNewDecFromStr("3.65")),
		TaxEntity:       "irs",
		CurrencyCode:    "USD",
		ProofCommitment: hex.EncodeToString(sum[:]),
		// Pseudónimos (privacidade on-chain): a chain exige 64 hex nos dois campos.
		PaymentId:     types.HashPaymentID("pay-" + unique),
		PaymentDate:   sdk.UnwrapSDKContext(f.ctx).BlockTime().Unix(),
		PayerTaxId:    types.HashTaxID("123456789"),
		TaxNatureHash: strings.Repeat("b", 64),
		ProviderId:    "viacredi",
	}
	// §13.170 Grupo B: fase 2 — a atestação tem de ser ASSINADA pelo notário.
	signAttestationForTest(t, msg)
	return msg
}

// signAttestationForTest (re)assina a msg com a chave dev do "notário",
// recalculando o digest a partir dos campos ATUAIS da msg. É preciso sempre que
// um teste MUTA campos depois de construir a msg.
func signAttestationForTest(t *testing.T, msg *types.MsgMineTaxPayment) {
	t.Helper()
	sig, err := reclaimverify.SignClaims(testNotaryKeyHex, types.AttestationFields{
		MinerAddress:    msg.MinerAddress,
		ChainID:         testChainID,
		ProofCommitment: msg.ProofCommitment,
		PaymentID:       msg.PaymentId,
		PaymentAmount:        msg.PaymentAmount.String(),
		TaxEntity:       msg.TaxEntity,
		CurrencyCode:    msg.CurrencyCode,
		PaymentDate:     msg.PaymentDate,
		TaxNatureHash:   msg.TaxNatureHash,
		PayerTaxID:      msg.PayerTaxId,
		ProviderID:      msg.ProviderId,
	})
	require.NoError(t, err)
	msg.ApiAttestation = sig
}

func initMineFixture(t *testing.T) *mineFixture {
	t.Helper()

	encCfg := moduletestutil.MakeTestEncodingConfig(module.AppModule{})
	addressCodec := addresscodec.NewBech32Codec(sdk.GetConfig().GetBech32AccountAddrPrefix())
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	storeService := runtime.NewKVStoreService(storeKey)
	ctx := testutil.DefaultContextWithDB(t, storeKey, storetypes.NewTransientStoreKey("transient_test")).Ctx
	ctx = ctx.WithChainID(testChainID)

	authority := authtypes.NewModuleAddress(types.GovModuleName)
	bank := &trackingBankKeeper{}

	k := keeper.NewKeeper(
		storeService,
		encCfg.Codec,
		addressCodec,
		authority,
		bank,
		&mockStakingKeeper{},
		&mockSlashingKeeper{},
	)

	// §13.146 (review multi-agente): a fixture configura um founder_address — a
	// mineração passou a ser FAIL-CLOSED sem ele (antes cunhava os 5% para dentro
	// do módulo em silêncio, com a tx a reportar sucesso).
	fixtureParams := types.DefaultParams()
	founderStr, err := addressCodec.BytesToString(bytes.Repeat([]byte{0x77}, 20))
	require.NoError(t, err)
	fixtureParams.FounderAddress = founderStr
	fixtureParams.ApiVerifierAddress = testNotaryAddress
	require.NoError(t, k.Params.Set(ctx, fixtureParams))

	// §13.171 — por omissao a fixture corre DEPOIS do cliff (o Fundador recebe
	// directo, como antes desta feature); os testes do vesting sobrepoem isto.
	require.NoError(t, k.FounderVestingStart.Set(ctx, uint64(1)))

	// A mineração é FAIL-CLOSED sem oráculo fresco (§13.28/§6b/§13.56): a fixture
	// fornece um snapshot como o feeder faz em produção, senão nenhum teste que
	// minere passaria. Quem quiser exercitar o fail-closed apaga-o ou envelhece-o.
	require.NoError(t, k.UsdOracleSnapshot.Set(ctx, types.UsdOracleSnapshot{
		ChfUsd:      sdkmath.LegacyMustNewDecFromStr("1.10"),
		EurUsd:      sdkmath.LegacyMustNewDecFromStr("1.18"),
		GbpUsd:      sdkmath.LegacyMustNewDecFromStr("1.37"),
		BlockHeight: 1,
		UpdatedAt:   sdk.UnwrapSDKContext(ctx).BlockTime().Unix(),
	}))

	return &mineFixture{
		ctx:          ctx,
		keeper:       k,
		addressCodec: addressCodec,
		bank:         bank,
	}
}

func makeAddr(t *testing.T, codec address.Codec, b byte) (string, sdk.AccAddress) {
	t.Helper()
	raw := bytes.Repeat([]byte{b}, 20)
	addrStr, err := codec.BytesToString(raw)
	require.NoError(t, err)
	return addrStr, sdk.AccAddress(raw)
}

func sentToAmount(bank *trackingBankKeeper, target sdk.AccAddress) sdkmath.Int {
	total := sdkmath.ZeroInt()
	for _, s := range bank.sends {
		if bytes.Equal(s.to, target) {
			total = total.Add(s.coins.AmountOf("uluggom"))
		}
	}
	return total
}

func TestMineSplit_ThreePhases(t *testing.T) {
	// §13.132 com a correção do §13.154: as fatias variáveis por regime.
	// founder/validators são constantes; a POOL (10%) existe só na Fase 1
	// (dotação única). A Gov Reserve mantém 2% na Fase 2 mas PARTIDA: 1,5% de
	// tesouraria + 0,5% de reserva do notariado (conta separada).
	tests := []struct {
		name       string
		mined10M   bool // supply ≥10M (fim da Fase 1)
		poolActive bool // pool/IBC abertos (Fase 2)
		minerBps   int64
		preSaleBps int64
		otcBps     int64
		stakingBps int64
		atomBps    int64
		govBps     int64
		notaryBps  int64
		poolBps    int64
	}{
		{
			name:       "fase 1 (<10M): miner 30 / pre-sale 35 / staking 7,5 / ATOM 7,5 / gov 2 / POOL 10",
			minerBps:   3000,
			preSaleBps: 3500,
			otcBps:     0,
			stakingBps: 750,
			atomBps:    750,
			govBps:     200,
			notaryBps:  0,
			poolBps:    1000,
		},
		{
			name:       "transição (≥10M, pool fechada): miner 30 / staking 30 / ATOM 30 / gov 2",
			mined10M:   true,
			minerBps:   3000,
			preSaleBps: 0,
			otcBps:     0,
			stakingBps: 3000,
			atomBps:    3000,
			govBps:     200,
			notaryBps:  0,
			poolBps:    0,
		},
		{
			name:       "fase 2 (pool aberta): miner 30 / OTC 40 / staking 10 / ATOM 10 / tesouraria 1,5 / notariado 0,5 (§13.154)",
			mined10M:   true,
			poolActive: true,
			minerBps:   3000,
			preSaleBps: 0,
			otcBps:     4000,
			stakingBps: 1000,
			atomBps:    1000,
			govBps:     150,
			notaryBps:  50,
			poolBps:    0,
		},
	}

	// Constantes em todas as fases (§13.132).
	const (
		founderBps   int64 = 500
		validatorBps int64 = 300
	)

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := initMineFixture(t)
			minerStr, minerAddr := makeAddr(t, f.addressCodec, 0x11)
			founderStr, founderAddr := makeAddr(t, f.addressCodec, 0x22)

			// §13.170 Grupo B: a fase é constante (2) e a atestação é ASSINADA
			// pela chave dev do notário (devParamsForTest). A fase OTC sai de supply+pool.
			params := devParamsForTest()
			params.FounderAddress = founderStr
			params.PoolActive = tc.poolActive
			require.NoError(t, f.keeper.Params.Set(f.ctx, params))

			// Fim da Fase 1 é derivado do supply minerado (preSaleActive, §13.117).
			if tc.mined10M {
				thr, ok := sdkmath.NewIntFromString("10000000000000000000000000") // 10M LUG
				require.True(t, ok)
				f.bank.supply = sdk.NewCoin("uluggom", thr)
			} else {
				f.bank.supply = sdk.NewCoin("uluggom", sdkmath.ZeroInt())
			}

			// 100 LUG: o valor da msg é derivado da paridade da fixture (3,65).
			msg := taxPaymentMsgForTest(t, f, minerStr, fmt.Sprintf("split-phase-%d", i), 100)

			resp, err := f.keeper.MineTaxPayment(f.ctx, msg)
			require.NoError(t, err)

			require.Len(t, f.bank.mints, 1)
			minted := f.bank.mints[0].coins.AmountOf("uluggom")
			require.True(t, minted.IsPositive())

			// As FATIAS são o que este teste afere; derivam do total efectivamente
			// cunhado (≈100 LUG). O valor exacto varia nos últimos wei porque a
			// paridade do oráculo é LegacyDec — não se fixa um total com casas.
			expectedTotal := minted

			expectedPreSale := expectedTotal.MulRaw(tc.preSaleBps).QuoRaw(10_000)
			expectedOtc := expectedTotal.MulRaw(tc.otcBps).QuoRaw(10_000)
			expectedFounder := expectedTotal.MulRaw(founderBps).QuoRaw(10_000)
			expectedValidatorFixed := expectedTotal.MulRaw(validatorBps).QuoRaw(10_000)
			expectedGovReserve := expectedTotal.MulRaw(tc.govBps).QuoRaw(10_000)
			expectedNotaryReserve := expectedTotal.MulRaw(tc.notaryBps).QuoRaw(10_000)
			expectedDistr := expectedTotal.MulRaw(tc.stakingBps).QuoRaw(10_000)
			expectedPool := expectedTotal.MulRaw(tc.poolBps).QuoRaw(10_000)
			expectedAtomReservoir := expectedTotal.MulRaw(tc.atomBps).QuoRaw(10_000)
			// O minerador absorve a POEIRA — é o que `computeMiningSplit` faz: não
			// é o 30% truncado, é o total MENOS as outras fatias.
			expectedMiner := expectedTotal
			for _, s := range []sdkmath.Int{
				expectedPreSale, expectedOtc, expectedFounder, expectedValidatorFixed,
				expectedGovReserve, expectedNotaryReserve, expectedDistr, expectedPool,
				expectedAtomReservoir,
			} {
				expectedMiner = expectedMiner.Sub(s)
			}

			// A fatia dos validadores sai para o fee_collector (o módulo
			// distribution é endereço bloqueado no bank keeper — a transferência
			// direta falha e os coins ficariam presos no módulo x/luggom).
			govReserveAddr := authtypes.NewModuleAddress(types.GovReserveModuleName)
			notaryReserveAddr := authtypes.NewModuleAddress(types.NotaryReserveModuleName)
			feeCollectorAddr := authtypes.NewModuleAddress(authtypes.FeeCollectorName)
			stakingReservoirAddr := authtypes.NewModuleAddress(types.StakingReservoirModuleName)
			poolAddr := authtypes.NewModuleAddress(types.PoolTreasuryModuleName)
			atomReservoirAddr := authtypes.NewModuleAddress(types.AtomReservoirModuleName)
			preSaleAddr := authtypes.NewModuleAddress(types.PreSaleModuleName)
			otcVaultAddr := authtypes.NewModuleAddress(types.OtcVaultModuleName)

			amtMiner := sentToAmount(f.bank, minerAddr)
			amtFounder := sentToAmount(f.bank, founderAddr)
			amtDistr := sentToAmount(f.bank, feeCollectorAddr)
			amtStakingReservoir := sentToAmount(f.bank, stakingReservoirAddr)
			amtPool := sentToAmount(f.bank, poolAddr)
			amtAtomReservoir := sentToAmount(f.bank, atomReservoirAddr)
			amtPreSale := sentToAmount(f.bank, preSaleAddr)
			amtGovReserve := sentToAmount(f.bank, govReserveAddr)
			amtNotaryReserve := sentToAmount(f.bank, notaryReserveAddr)
			amtOtc := sentToAmount(f.bank, otcVaultAddr)

			require.Equal(t, expectedFounder.String(), amtFounder.String())
			require.Equal(t, expectedDistr.String(), amtStakingReservoir.String())
			require.Equal(t, expectedValidatorFixed.String(), amtDistr.String())
			require.Equal(t, expectedGovReserve.String(), amtGovReserve.String())
			require.Equal(t, expectedNotaryReserve.String(), amtNotaryReserve.String())
			require.Equal(t, expectedPool.String(), amtPool.String())
			require.Equal(t, expectedAtomReservoir.String(), amtAtomReservoir.String())
			require.Equal(t, expectedPreSale.String(), amtPreSale.String())
			require.Equal(t, expectedOtc.String(), amtOtc.String())
			require.Equal(t, expectedMiner.String(), amtMiner.String())

			// Response should expose the same split components.
			require.Equal(t, expectedFounder.String(), resp.TaxRoot.String())
			require.Equal(t, expectedDistr.String(), resp.TaxStaking.String())
			require.Equal(t, expectedPool.String(), resp.TaxValidator.String())

			// O cofre acumula a fatia OTC (só a Fase 2 alimenta o cofre, sem tecto).
			vaultAfter, err := f.keeper.OtcVaultAccumulated(f.ctx)
			require.NoError(t, err)
			require.Equal(t, expectedOtc.String(), vaultAfter.String())

			// Invariant: todos os tokens minerados são alocados.
			sum := amtMiner.Add(amtPreSale).Add(amtOtc).Add(amtFounder).
				Add(amtStakingReservoir).Add(amtDistr).Add(amtGovReserve).
				Add(amtNotaryReserve).Add(amtPool).Add(amtAtomReservoir)
			require.Equal(t, minted.String(), sum.String())
		})
	}
}
