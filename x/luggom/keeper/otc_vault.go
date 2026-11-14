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

// Split ÚNICO e permanente, a 100%:
//
//	Componente             %
//	Minerador             30
//	Luggom Business Core   5   (directo, sem cliff)
//	OTC LOCKED            40   (contínuo, sem tecto)
//	Staking Reservoir     7,5
//	ATOM Reservoir        7,5
//	Fundador               5   (cliff de 24 meses)
//	Validadores            3
//	Gov Reserve            1,5 (tesouraria)
//	Notariado              0,5 (reserva separada)
//	POOL                   —   (sem dotação do protocolo)
//	Total                 100
//
// A governança é founder-led até 10M LUG minerados; a partir daí é 1 LUG em
// staking = 1 voto (DAO). O split não muda com o supply.
//
// A POOL não tem dotação: a LLC pode abrir uma pool como LP normal (AddLiquidity)
// quando a governança ligar `pool_active`. Sem pool, o OTC LOCKED é a única via
// de compra do protocolo: só BTC via IBC, lotes de 100k LUG, ao preço da cesta.
//
// O OTC LOCKED é contínuo e SEM TECTO: 40% de toda a mineração. O BTC recebido é
// LASTRO PERMANENTE — nunca sai, nunca é queimado, nunca é trocado por LUG; só
// migra de custódia com 87% e só para custódia igual ou mais forte (imutável +
// inconfiscável), que hoje não existe ⇒ na prática congelado. O LUG do cofre só
// sai COMPRADO, nunca por voto: não é tesouraria, é parede de oferta.
//
// §13.64: a fatia ATOM vai SEMPRE para o Atom Reservoir (luggom_atom_reservoir).
const (
	bpsDenom = 10_000

	bpsMiner        = 3_000 // 30% → minerador
	bpsBusinessCore = 500   // 5%  → Luggom Business Core (directo, 3/5)
	bpsOtc          = 4_000 // 40% → cofre OTC LOCKED (contínuo, sem tecto)
	bpsStaking      = 750   // 7,5% → Staking Reservoir
	bpsAtom         = 750   // 7,5% → Atom Reservoir
	bpsFounder      = 500   // 5%  → fundador (multisig 2-de-3, cliff 24m)
	bpsValidator    = 300   // 3%  → fee_collector/x-distribution (por stake)
	bpsGov          = 150   // 1,5% → tesouraria (só por voto)
	bpsNotary       = 50    // 0,5% → reserva do notariado (conta separada)
	bpsPool         = 0     // sem dotação de POOL no protocolo

	// bpsAtomBonusCap é o teto flat pago aos locks qualificados ATOM (§13.85).
	bpsAtomBonusCap = 700
)

// miningSplit é a alocação de uma mineração. Todos os valores em uluggom.
type miningSplit struct {
	Miner         sdkmath.Int
	BusinessCore  sdkmath.Int
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
		Add(s.BusinessCore).
		Add(s.OtcVault).
		Add(s.Staking).
		Add(s.Atom).
		Add(s.Founder).
		Add(s.Validator).
		Add(s.GovReserve).
		Add(s.NotaryReserve).
		Add(s.Pool)
}

// computeMiningSplit calcula a alocação de `amount`. A poeira (resto da divisão
// inteira) vai para o minerador, pelo que Total() == amount.
func computeMiningSplit(amount sdkmath.Int) miningSplit {
	calc := func(bps int64) sdkmath.Int {
		if bps <= 0 {
			return sdkmath.ZeroInt()
		}
		return amount.MulRaw(bps).QuoRaw(bpsDenom)
	}

	s := miningSplit{
		Miner:         calc(bpsMiner),
		BusinessCore:  calc(bpsBusinessCore),
		OtcVault:      calc(bpsOtc),
		Staking:       calc(bpsStaking),
		Atom:          calc(bpsAtom),
		Founder:       calc(bpsFounder),
		Validator:     calc(bpsValidator),
		GovReserve:    calc(bpsGov),
		NotaryReserve: calc(bpsNotary),
		Pool:          calc(bpsPool),
	}
	s.Miner = s.Miner.Add(amount.Sub(s.Total())) // poeira → minerador
	return s
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
// enquanto existir mineração o cofre recebe 40%.
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

	// Luggom Business Core — 5% directo para a carteira 3/5, sem cliff.
	// Fail-closed: sem endereço compilado, a mineração aborta.
	if split.BusinessCore.IsPositive() {
		if types.BusinessCoreAddress == "" {
			return fmt.Errorf("business core address is not configured: refusing to mint the business core slice (%s would be stranded in the module)", split.BusinessCore)
		}
		businessCoreAddr, err := k.addressCodec.StringToBytes(types.BusinessCoreAddress)
		if err != nil {
			return fmt.Errorf("business core address invalid: %w", err)
		}
		coinsBusinessCore := sdk.NewCoins(sdk.NewCoin("uluggom", split.BusinessCore))
		if err := k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, businessCoreAddr, coinsBusinessCore); err != nil {
			return fmt.Errorf("failed to fund business core: %w", err)
		}
	}

	// Fundador (5%). Fail-closed (DV M1): um endereço inválido ou um envio que
	// falhe deixava os 5% presos no módulo com a tx a reportar SUCESSO.
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

	// Reserva do notariado (0,5%) — conta SEPARADA da tesouraria.
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

	// Sem dotação de POOL no protocolo: a LLC pode abrir uma pool como LP.
	if split.Pool.IsPositive() {
		coinsPool := sdk.NewCoins(sdk.NewCoin("uluggom", split.Pool))
		if err := k.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, types.PoolTreasuryModuleName, coinsPool); err != nil {
			return fmt.Errorf("failed to fund pool treasury: %w", err)
		}
	}

	// OTC LOCKED (40% contínuo) — fail-closed.
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
		sdk.NewAttribute("tax_business_core", split.BusinessCore.String()),
		sdk.NewAttribute("tax_otc_vault", split.OtcVault.String()),
		sdk.NewAttribute("tax_founder", split.Founder.String()),
		sdk.NewAttribute("tax_validators", split.Validator.String()),
		sdk.NewAttribute("tax_gov_reserve", split.GovReserve.String()),
		sdk.NewAttribute("tax_notary_reserve", split.NotaryReserve.String()),
		sdk.NewAttribute("tax_distribution", split.Staking.String()),
		sdk.NewAttribute("tax_pool_treasury", split.Pool.String()),
		sdk.NewAttribute("tax_atom_reservoir", split.Atom.String()),
	}
}

// SellOtcVaultBatch valida uma venda do cofre (lote mínimo 100k LUG, só BTC) e
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

// ComputeMiningSplitForTest expõe computeMiningSplit a testes externos.
func ComputeMiningSplitForTest(amount sdkmath.Int) miningSplit {
	return computeMiningSplit(amount)
}
