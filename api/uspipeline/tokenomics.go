package uspipeline

// TokenomicsSnapshot mirrors the single permanent split. The AUTHORITY is
// x/luggom/keeper/otc_vault.go — this file is a copy for the API/APK.
//
// Split ÚNICO e permanente (2026-11-13): miner 30%, LBC 5%, OTC LOCKED 40%,
// staking 7,5%, ATOM 7,5%, fundador 5% (cliff 24m), validadores 3%, gov 1,5%,
// notariado 0,5%, POOL 0.
//
// O split NÃO muda com o supply, com a pool nem com a fase de governança; só
// o peso do voto muda aos 10M (founder-led → 1 LUG = 1 voto).
//
// Etapa G1: expose these fields to the APK via GET /api/v1/chain/info.
type TokenomicsSnapshot struct {
	PoolActive              bool   `json:"pool_active"`
	PreSaleActive           bool   `json:"pre_sale_active"` // informativo; a pré-venda está desligada
	PrePoolBoostOperational bool   `json:"pre_pool_boost_operational"`
	MinerBps                int    `json:"miner_bps"`
	BusinessCoreBps         int    `json:"business_core_bps"`
	PreSaleBps              int    `json:"pre_sale_bps"`     // sempre 0
	LlcTreasuryBps          int    `json:"llc_treasury_bps"` // deprecated: LLC dissolved — always 0
	FounderBps              int    `json:"founder_bps"`
	PosMintFromMiningBps    int    `json:"pos_mint_from_mining_bps"`
	PoolTreasuryMintBps     int    `json:"pool_treasury_mint_bps"` // sempre 0
	ValidatorFixedBps       int    `json:"validator_fixed_bps"`
	GovReserveBps           int    `json:"gov_reserve_bps"`
	NotaryReserveBps        int    `json:"notary_reserve_bps"`
	AtomBonusBps            int    `json:"atom_bonus_bps"`
	OtcVaultBps             int    `json:"otc_vault_bps"`
	AtomBonusCapBps         int    `json:"atom_bonus_cap_bps"` // §13.85: flat cap pago aos locks ATOM qualificados
	AtomBonusActive         bool   `json:"atom_bonus_active"`
	Reference               string `json:"reference"`
}

// Os bps abaixo ESPELHAM o bloco de constantes de
// x/luggom/keeper/otc_vault.go. Manter as duas cópias em sincronia — são
// pacotes separados (API vs chain) sem ligação em compilação.
const (
	bpsMiner        = 3_000 // 30% → minerador
	bpsBusinessCore = 500   // 5%  → LBC (direto, sem cliff)
	bpsOtc          = 4_000 // 40% → OTC LOCKED (contínuo, sem tecto)
	bpsStaking      = 750   // 7,5% → Staking Reservoir
	bpsAtom         = 750   // 7,5% → Atom Reservoir
	bpsFounder      = 500   // 5%  → fundador (cliff 24m)
	bpsValidator    = 300   // 3%  → fee_collector/x-distribution
	bpsGov          = 150   // 1,5% → tesouraria
	bpsNotary       = 50    // 0,5% → reserva do notariado
	bpsPool         = 0     // sem dotação de POOL

	bpsAtomBonusCap = 700 // 7% flat cap pago aos locks qualificados ATOM (§13.85)
)

const tokenomicsReference = "Split único e permanente — miner 30%, LBC 5%, OTC LOCKED 40% (contínuo, sem tecto), staking 7,5%, ATOM 7,5%, fundador 5% (cliff 24m), validadores 3%, gov 1,5%, notariado 0,5%; POOL 0"

// BuildTokenomicsSnapshot computes the snapshot. O split NÃO muda com pool,
// pré-venda ou governança; poolActive e preSaleActive alimentam apenas os flags.
func BuildTokenomicsSnapshot(poolActive bool, preSaleActive bool, atomBonusActive bool) TokenomicsSnapshot {
	return TokenomicsSnapshot{
		PoolActive:              poolActive,
		PreSaleActive:           preSaleActive,
		PrePoolBoostOperational: !poolActive,
		MinerBps:                bpsMiner,
		BusinessCoreBps:         bpsBusinessCore,
		PreSaleBps:              0,
		LlcTreasuryBps:          0,
		FounderBps:              bpsFounder,
		PosMintFromMiningBps:    bpsStaking,
		PoolTreasuryMintBps:     bpsPool,
		ValidatorFixedBps:       bpsValidator,
		GovReserveBps:           bpsGov,
		NotaryReserveBps:        bpsNotary,
		AtomBonusBps:            bpsAtom,
		OtcVaultBps:             bpsOtc,
		AtomBonusCapBps:         bpsAtomBonusCap,
		AtomBonusActive:         atomBonusActive,
		Reference:               tokenomicsReference,
	}
}
