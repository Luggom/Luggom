package keeper

import (
	"context"
	"errors"
	"fmt"

	"cosmossdk.io/collections"
	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"

	"luggom/x/luggom/types"
)

// MVP 2.0 mint split — 3 FASES, todas a 100% (§13.132, 2026-09-17).
// SUBSTITUI as percentagens do §13.117 e do §13.128.
//
//	A Fase 2 é DEFINITIVA (pós pool/IBC) e imutável. Gatilho da Fase 1 → 2 é a
//	abertura da pool/IBC por decisão do fundador (params.pool_active, ~12M
//	minerados). Entre o fim da pre-sale (10M) e a abertura da pool há uma
//	TRANSIÇÃO: o pre-sale e a POOL encerram e as fatias vão para os
//	reservatórios (staking/ATOM a 30 cada) — nada fica órfão.
//
//	TABELA VIGENTE (§13.132), em percentagem:
//	  Componente           Fase 1   Transição   Fase 2
//	  Minerador              30       30          30
//	  Pre-sale               35        0           0
//	  OTC LOCKED             —         0          42
//	  Staking Reservoir     7,5       30          10
//	  ATOM Reservoir        7,5       30          10
//	  Fundador                5        5           5
//	  Validadores             3        3           3
//	  Gov Reserve             2        2           0
//	  POOL                   10        0           0
//	  Total                 100      100         100
//
//	§13.132: a POOL (10%) é DOTACAO ÚNICA de liquidez — um carve-out que corre
//	só na Fase 1 (10% × 10M = 1M LUG) e encerra na Transição; NÃO é fatia
//	contínua. Na Fase 2 a Gov Reserve MANTÉM-SE a 2% (§13.153), dividida em
//	0,5% para o notariado + 1,5% de tesouraria (§13.154); o OTC desce a 40%.
//
//	O OTC LOCKED é contínuo e SEM TECTO: 40% de toda a mineração, enquanto
//	existir mineração. O cofre só vende LUG por BTC (nunca PAXG), em lotes
//	mínimos de 100k LUG, sempre ao preço da cesta daquele dia e ano. O BTC
//	recebido é LASTRO PERMANENTE — nunca sai, nunca é queimado, nunca é trocado
//	por LUG; só migra de custódia com 87% e só para custódia igual ou mais forte
//	(imutável + inconfiscável), que hoje não existe ⇒ na prática congelado.
//	O LUG do cofre só sai COMPRADO, nunca por voto: não é tesouraria, é parede
//	de oferta.
//
// §13.64: a fatia ATOM vai SEMPRE para o Atom Reservoir (luggom_atom_reservoir).
// GovReserve está REACTIVADA (2026-09-10): 2% para parcerias futuras (Open Finance etc.).
const (
	bpsDenom = 10_000

	// Constantes em todas as fases.
	bpsFounder   = 500 // 5% → fundador (multisig 2-de-3)
	bpsValidator = 300 // 3% → fee_collector/x-distribution (proporcional ao stake)

	// Gov Reserve (TESOURARIA) — POR FASE. 2% na Fase 1 e na Transição (§13.132);
	// na Fase 2 mantém-se a 2% mas PARTIDA (§13.153/§13.154): 1,5% fica aqui,
	// discricionário e só gasto por voto, e 0,5% vai para a reserva do notariado.
	bpsGovPhase1     = 200
	bpsGovTransition = 200
	bpsGovPhase2     = 150 // 1,5% → tesouraria (só por voto)

	// Reserva do NOTARIADO — 0,5% na Fase 2 (§13.154). Conta SEPARADA da
	// tesouraria: o dinheiro dos notários não pode ser alcançado pelo mesmo voto
	// que gasta marketing. Só existe na Fase 2 (na Fase 1 o notário é o próprio
	// arranque, sem partilha).
	bpsNotaryPhase2 = 50 // 0,5% → reserva do notariado

	// Fatia POOL — POR FASE (§13.132): 10% só na Fase 1, como DOTACAO ÚNICA de
	// liquidez (10% × 10M = 1M LUG) que ENCERRA na Transição. Não é fatia
	// contínua. Mantê-la global sobre-alocaria as fases seguintes.
	bpsPoolPhase1     = 1_000
	bpsPoolTransition = 0
	bpsPoolPhase2     = 0

	// Fase 1 — pré-pool com pre-sale activa (<10M minerados).
	bpsMinerPhase1   = 3_000 // 30%   → minerador
	bpsPreSalePhase1 = 3_500 // 35%   → conta de pre-sale (3,5M LUG × $0,10 = $350k)
	bpsStakingPhase1 = 750   // 7,5%  → Staking Reservoir
	bpsAtomPhase1    = 750   // 7,5%  → Atom Reservoir

	// Transição — pré-pool, pre-sale e POOL encerradas (≥10M, pool ainda fechada):
	// as fatias do pre-sale e da POOL vão para os reservatórios (30% cada).
	bpsMinerTransition   = 3_000 // 30% → minerador
	bpsStakingTransition = 3_000 // 30% → Staking Reservoir
	bpsAtomTransition    = 3_000 // 30% → Atom Reservoir

	// Fase 2 — DEFINITIVA (pool/IBC abertos). Permanente e imutável.
	// §13.132 com a correção do §13.154: o OTC fica a 40% (não 42), porque 2%
	// voltam à Gov Reserve — 1,5 tesouraria + 0,5 notariado. Fundamento da fatia
	// grande do OTC: a fatia do minerador é float FORÇADO a vender; o cofre nunca
	// é forçado (só sai COMPRADO à cesta, em lotes de 100k).
	bpsMinerPhase2   = 3_000 // 30% → minerador
	bpsOtcPhase2     = 4_000 // 40% → cofre OTC LOCKED (era 42; §13.154 devolveu 2% à Gov)
	bpsStakingPhase2 = 1_000 // 10% → Staking Reservoir
	bpsAtomPhase2    = 1_000 // 10% → Atom Reservoir

	// bpsAtomBonusCap é o teto flat pago aos locks qualificados ATOM (§13.85, pós-pool).
	bpsAtomBonusCap = 700
)

// miningSplit é a alocação de uma mineração. Todos os valores em uluggom.
type miningSplit struct {
	Phase         types.OtcPhase
	Miner         sdkmath.Int
	PreSale       sdkmath.Int
	OtcVault      sdkmath.Int
	Staking       sdkmath.Int
	Atom          sdkmath.Int
	Founder       sdkmath.Int
	Validator     sdkmath.Int
	GovReserve    sdkmath.Int
	NotaryReserve sdkmath.Int
	Pool          sdkmath.Int
}

// Total soma todas as fatias do split.
func (s miningSplit) Total() sdkmath.Int {
	return s.Miner.
		Add(s.PreSale).
		Add(s.OtcVault).
		Add(s.Staking).
		Add(s.Atom).
		Add(s.Founder).
		Add(s.Validator).
		Add(s.GovReserve).
		Add(s.NotaryReserve).
		Add(s.Pool)
}

// splitBps devolve os bps variáveis por fase (miner, pre-sale, OTC, staking,
// ATOM, gov, notariado, POOL). Gov, notariado e POOL são por fase desde o
// §13.132/§13.154.
func splitBps(phase types.OtcPhase) (miner, preSale, otc, staking, atom, gov, notary, pool int64) {
	switch phase {
	case types.OtcPhase1:
		return bpsMinerPhase1, bpsPreSalePhase1, 0, bpsStakingPhase1, bpsAtomPhase1, bpsGovPhase1, 0, bpsPoolPhase1
	case types.OtcPhaseTransition:
		return bpsMinerTransition, 0, 0, bpsStakingTransition, bpsAtomTransition, bpsGovTransition, 0, bpsPoolTransition
	default: // Fase 2 (definitiva)
		return bpsMinerPhase2, 0, bpsOtcPhase2, bpsStakingPhase2, bpsAtomPhase2, bpsGovPhase2, bpsNotaryPhase2, bpsPoolPhase2
	}
}

// computeMiningSplit calcula a alocação de `amount` na fase indicada. A poeira
// (resto da divisão inteira) vai para o minerador, pelo que Total() == amount.
func computeMiningSplit(amount sdkmath.Int, phase types.OtcPhase) miningSplit {
	minerBps, preSaleBps, otcBps, stakingBps, atomBps, govBps, notaryBps, poolBps := splitBps(phase)

	calc := func(bps int64) sdkmath.Int {
		if bps <= 0 {
			return sdkmath.ZeroInt()
		}
		return amount.MulRaw(bps).QuoRaw(bpsDenom)
	}

	s := miningSplit{
		Phase:         phase,
		Miner:         calc(minerBps),
		PreSale:       calc(preSaleBps),
		OtcVault:      calc(otcBps),
		Staking:       calc(stakingBps),
		Atom:          calc(atomBps),
		Founder:       calc(bpsFounder),
		Validator:     calc(bpsValidator),
		GovReserve:    calc(govBps),
		NotaryReserve: calc(notaryBps),
		Pool:          calc(poolBps),
	}
	s.Miner = s.Miner.Add(amount.Sub(s.Total())) // poeira → minerador
	return s
}

// miningPhase resolve o regime vigente (§13.117):
//
//	pre-sale activa (<10M minerados)     → Fase 1
//	pre-sale encerrada e pool fechada    → Transição
//	pool/IBC abertos                     → Fase 2 (definitiva)
func (k Keeper) miningPhase(ctx context.Context) (types.OtcPhase, error) {
	if k.preSaleActive(ctx) {
		return types.OtcPhase1, nil
	}
	if !k.PoolOpen(ctx) {
		return types.OtcPhaseTransition, nil
	}
	return types.OtcPhase2, nil
}

// otcVaultAccumulated devolve o LUG acumulado no cofre (0 se ainda não existe estado).
func (k Keeper) otcVaultAccumulated(ctx context.Context) (sdkmath.Int, error) {
	vault, err := k.OtcVault.Get(ctx)
	if err != nil {
		if errors.Is(err, collections.ErrNotFound) {
			return sdkmath.ZeroInt(), nil
		}
		return sdkmath.ZeroInt(), err
	}
	if vault.AccumulatedLug.IsNil() {
		return sdkmath.ZeroInt(), nil
	}
	return vault.AccumulatedLug, nil
}

// OtcVaultAccumulated devolve o LUG acumulado no cofre OTC (para queries/testes).
func (k Keeper) OtcVaultAccumulated(ctx context.Context) (sdkmath.Int, error) {
	return k.otcVaultAccumulated(ctx)
}

// addToOtcVault regista o LUG enviado para o cofre. Não há marco nem tecto:
// enquanto existir mineração a Fase 2 alimenta o cofre com 40% (§13.132).
func (k Keeper) addToOtcVault(ctx context.Context, amount sdkmath.Int) error {
	if amount.IsNil() || !amount.IsPositive() {
		return nil
	}

	vault, err := k.OtcVault.Get(ctx)
	if err != nil && !errors.Is(err, collections.ErrNotFound) {
		return err
	}
	before := sdkmath.ZeroInt()
	if !vault.AccumulatedLug.IsNil() {
		before = vault.AccumulatedLug
	}

	vault.AccumulatedLug = before.Add(amount)
	if err := k.OtcVault.Set(ctx, vault); err != nil {
		return fmt.Errorf("failed to update OTC vault: %w", err)
	}
	return nil
}

// distributeMiningSplit materializa o split: minta o total e envia cada fatia.
// O envio para o cofre OTC é FAIL-CLOSED (um erro aborta a tx) porque a
// contabilidade do cofre tem de acompanhar o saldo do módulo.
func (k Keeper) distributeMiningSplit(ctx context.Context, params types.Params, split miningSplit, minerAddr sdk.AccAddress) error {
	coinsToMint := sdk.NewCoins(sdk.NewCoin("uluggom", split.Total()))
	if err := k.bankKeeper.MintCoins(ctx, types.ModuleName, coinsToMint); err != nil {
		return fmt.Errorf("failed to mint coins: %w", err)
	}

	coinsToMiner := sdk.NewCoins(sdk.NewCoin("uluggom", split.Miner))
	if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, minerAddr, coinsToMiner); err != nil {
		return fmt.Errorf("failed to send coins to miner: %w", err)
	}

	// Pre-sale (só Fase 1). FAIL-CLOSED: o pre-sale on-chain (§13.132 §4) vende a
	// partir desta conta — se o envio falhasse em silêncio, a venda ficaria sem
	// lastro. Ao contrário das outras fatias, esta não pode ser engolida.
	if split.PreSale.IsPositive() {
		coinsPreSale := sdk.NewCoins(sdk.NewCoin("uluggom", split.PreSale))
		// §13.172 — creditar o MÓDULO com ModuleToModule (e não o endereço com
		// ModuleToAccount): o segundo cria uma BaseAccount no endereço do módulo e
		// o `GetModuleAccount` seguinte (AccountToModule/ModuleToModule) faz
		// PANIC ("account is not a module account"). Bug apanhado no T-CHAIN-10.
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.PreSaleModuleName, coinsPreSale); err != nil {
			return fmt.Errorf("failed to fund pre-sale account: %w", err)
		}
	}

	// Fundador (5%). Fail-closed (DV M1): um endereço inválido ou um envio que
	// falhe deixava os 5% presos no módulo com a tx a reportar SUCESSO.
	//
	// §13.146 (review multi-agente): o endereço VAZIO também é fail-closed. Antes,
	// o `!= ""` saltava o envio em silêncio — e o vazio é o DEFAULT dos params,
	// logo uma chain com o genesis por completar cunhava os 5% para dentro do
	// módulo a cada mineração, com a tx a dizer sucesso (e o recibo a reportar o
	// fundador como pago).
	if split.Founder.IsPositive() {
		if params.FounderAddress == "" {
			return fmt.Errorf("founder_address is not configured: refusing to mint the founder slice (%s would be stranded in the module)", split.Founder)
		}
		founderAddr, err := k.addressCodec.StringToBytes(params.FounderAddress)
		if err != nil {
			return fmt.Errorf("founder_address inválido: %w", err)
		}
		coinsToFounder := sdk.NewCoins(sdk.NewCoin("uluggom", split.Founder))
		// §13.171 — cliff PURO de 24 meses: antes do cliff a fatia fica retida em
		// `luggom_founder_vesting`; depois do cliff vai directa para o multisig.
		if k.founderCliffPassed(sdk.UnwrapSDKContext(ctx)) {
			if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, founderAddr, coinsToFounder); err != nil {
				return fmt.Errorf("failed to fund founder: %w", err)
			}
		} else {
			if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.FounderVestingModuleName, coinsToFounder); err != nil {
				return fmt.Errorf("failed to retain founder slice: %w", err)
			}
			if err := k.addFounderVestingHeld(sdk.UnwrapSDKContext(ctx), split.Founder); err != nil {
				return err
			}
		}
	}

	// Staking Reservoir — alimenta o drip mensal dos locks de LUG. Fail-closed (DV M1).
	if split.Staking.IsPositive() {
		coinsToReservoir := sdk.NewCoins(sdk.NewCoin("uluggom", split.Staking))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.StakingReservoirModuleName, coinsToReservoir); err != nil {
			return fmt.Errorf("failed to fund staking reservoir: %w", err)
		}
	}

	// Validadores → fee_collector, que x/distribution reparte pelo stake.
	// NÃO usar SendCoinsFromModuleToAccount para o módulo distribution: esse
	// endereço está em blockAccAddrs e o bank keeper rejeita o envio
	// (`is not allowed to receive funds`) — os coins ficariam presos.
	if split.Validator.IsPositive() {
		coinsToValidators := sdk.NewCoins(sdk.NewCoin("uluggom", split.Validator))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, authtypes.FeeCollectorName, coinsToValidators); err != nil {
			return fmt.Errorf("failed to fund validator rewards: %w", err)
		}
	}

	// Gov reserve (tesouraria) — só sai por decisão de governança.
	if split.GovReserve.IsPositive() {
		coinsGovReserve := sdk.NewCoins(sdk.NewCoin("uluggom", split.GovReserve))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.GovReserveModuleName, coinsGovReserve); err != nil {
			return fmt.Errorf("failed to fund gov reserve: %w", err)
		}
	}

	// Reserva do notariado (0,5% na Fase 2, §13.154) — conta SEPARADA da
	// tesouraria, para que o dinheiro dos notários não possa ser gasto pela
	// mesma votação que gasta marketing. Fail-closed.
	if split.NotaryReserve.IsPositive() {
		coinsNotaryReserve := sdk.NewCoins(sdk.NewCoin("uluggom", split.NotaryReserve))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.NotaryReserveModuleName, coinsNotaryReserve); err != nil {
			return fmt.Errorf("failed to fund notary reserve: %w", err)
		}
	}

	// Atom Reservoir — savings account (§13.64). Fail-closed (DV M1).
	if split.Atom.IsPositive() {
		coinsAtomReservoir := sdk.NewCoins(sdk.NewCoin("uluggom", split.Atom))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.AtomReservoirModuleName, coinsAtomReservoir); err != nil {
			return fmt.Errorf("failed to fund atom reservoir: %w", err)
		}
	}

	// POOL treasury (10% — dotação única de liquidez, só na Fase 1; §13.132 §1).
	// Fail-closed (DV M1).
	if split.Pool.IsPositive() {
		coinsPool := sdk.NewCoins(sdk.NewCoin("uluggom", split.Pool))
		// §13.172 — se a POOL tiver endereço próprio (params) é conta normal; senão
		// credita-se o MÓDULO (ModuleToModule) para não criar BaseAccount.
		// §13.170 Grupo A — o override por params (`pool_treasury_address`) foi
		// apagado: a POOL vai sempre para o module account.
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.PoolTreasuryModuleName, coinsPool); err != nil {
			return fmt.Errorf("failed to fund pool treasury: %w", err)
		}
	}

	// OTC LOCKED (Fase 2, 40% contínuo — §13.132) — fail-closed.
	if split.OtcVault.IsPositive() {
		coinsOtc := sdk.NewCoins(sdk.NewCoin("uluggom", split.OtcVault))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.OtcVaultModuleName, coinsOtc); err != nil {
			return fmt.Errorf("failed to fund OTC vault: %w", err)
		}
		if err := k.addToOtcVault(ctx, split.OtcVault); err != nil {
			return err
		}
	}

	return nil
}

// miningSplitAttributes devolve os atributos comuns do evento de mineração.
func miningSplitAttributes(split miningSplit) []sdk.Attribute {
	return []sdk.Attribute{
		sdk.NewAttribute("amount_mined", split.Miner.String()),
		sdk.NewAttribute("tax_presale", split.PreSale.String()),
		sdk.NewAttribute("tax_otc_vault", split.OtcVault.String()),
		sdk.NewAttribute("tax_founder", split.Founder.String()),
		sdk.NewAttribute("tax_validators", split.Validator.String()),
		sdk.NewAttribute("tax_gov_reserve", split.GovReserve.String()),
		sdk.NewAttribute("tax_notary_reserve", split.NotaryReserve.String()),
		sdk.NewAttribute("tax_distribution", split.Staking.String()),
		sdk.NewAttribute("tax_pool_treasury", split.Pool.String()),
		sdk.NewAttribute("tax_atom_reservoir", split.Atom.String()),
		sdk.NewAttribute("otc_phase", split.Phase.String()),
	}
}

// SellOtcVaultBatch valida uma venda do cofre (lote mínimo 10k LUG, só BTC) e
// fail-closa: o BTC só entra via IBC (§13.117), pelo que a liquidação fica
// desligada até o rail do IBC abrir. O preço é a cesta. O BTC recebido é lastro
// permanente — nunca sai, nem se troca por LUG.
func (k Keeper) SellOtcVaultBatch(_ context.Context, assetDenom string, lugAmount sdkmath.Int) error {
	if err := types.ValidateOtcSale(assetDenom, lugAmount); err != nil {
		return err
	}
	return types.ErrOtcIbcNotOpen
}

// --- Wrappers exportados para testes (package keeper_test) ---

// MiningPhaseForTest expõe miningPhase a testes externos.
func (k Keeper) MiningPhaseForTest(ctx context.Context) (types.OtcPhase, error) {
	return k.miningPhase(ctx)
}

// ComputeMiningSplitForTest expõe computeMiningSplit a testes externos.
func ComputeMiningSplitForTest(amount sdkmath.Int, phase types.OtcPhase) miningSplit {
	return computeMiningSplit(amount, phase)
}
