package uspipeline

// TokenomicsSnapshot mirrors the tokenomics split. The AUTHORITY is
// x/luggom/keeper/otc_vault.go — this file is a copy for the API/APK.
//
// ✅ ALINHADO em 2026-09-19 com o keeper (§13.132 + §13.154). Esteve
// desactualizado desde o §13.132 (dizia miner 50% / OTC 15%) e era servido ao
// APK por GET /api/v1/chain/info — ou seja, o APK mostrava números que a chain
// não pagava. O teste vizinho só verificava que somava 100% dentro desta cópia,
// pelo que nunca apanhou a divergência.
//
// 2 FASES + TRANSIÇÃO (§13.117), com os números do §13.132:
//   - Fase 1 (<10M minerados, pre-sale activa): miner 30% + pre-sale 35%,
//     staking 7,5%, ATOM 7,5%, POOL 10% (dotação única), gov 2%.
//   - Transição (≥10M, pool ainda fechada): pre-sale e POOL encerram; as fatias
//     vão para staking e ATOM (30% cada).
//   - Fase 2 (pool/IBC abertos, DEFINITIVA e imutável): miner 30%,
//     OTC LOCKED 40%, staking 10%, ATOM 10%, tesouraria 1,5%, notariado 0,5%.
//
// §13.154: a Gov Reserve mantém 2% na Fase 2, partida em 1,5% de tesouraria
// (só gasta por voto) + 0,5% de reserva do notariado (conta separada).
//
// The pool opens via a governable flag (params.pool_active), NOT a supply threshold.
// Etapa G1: expose these fields to the APK via GET /api/v1/chain/info.
type TokenomicsSnapshot struct {
	PoolActive              bool   `json:"pool_active"`
	PreSaleActive           bool   `json:"pre_sale_active"`
	PrePoolBoostOperational bool   `json:"pre_pool_boost_operational"`
	MinerBps                int    `json:"miner_bps"`
	PreSaleBps              int    `json:"pre_sale_bps"`
	LlcTreasuryBps          int    `json:"llc_treasury_bps"` // deprecated: LLC dissolved — always 0
	FounderBps              int    `json:"founder_bps"`
	PosMintFromMiningBps    int    `json:"pos_mint_from_mining_bps"`
	PoolTreasuryMintBps     int    `json:"pool_treasury_mint_bps"`
	ValidatorFixedBps       int    `json:"validator_fixed_bps"`
	GovReserveBps           int    `json:"gov_reserve_bps"`    // 1,5% na Fase 2 (tesouraria)
	NotaryReserveBps        int    `json:"notary_reserve_bps"` // 0,5% na Fase 2 (§13.154)
	AtomBonusBps            int    `json:"atom_bonus_bps"`
	OtcVaultBps             int    `json:"otc_vault_bps"`
	AtomBonusCapBps         int    `json:"atom_bonus_cap_bps"` // §13.85: flat cap PAID to ATOM-qualified locks (7% post-pool)
	AtomBonusActive         bool   `json:"atom_bonus_active"`
	Reference               string `json:"reference"`
}

// Os bps abaixo ESPELHAM o bloco de constantes de
// x/luggom/keeper/otc_vault.go (§13.132 + §13.154). Manter as duas cópias em
// sincronia — são pacotes separados (API vs chain) sem ligação em compilação.
// TestTokenomicsSplitSumsTo100Percent só valida a coerência INTERNA desta
// cópia; a autoridade continua a ser o keeper.
const (
	// Fase 1 — pré-pool, com a pre-sale activa (<10M minerados).
	bpsMinerPhase1   = 3_000 // 30%
	bpsPreSalePhase1 = 3_500 // 35%
	bpsStakingPhase1 = 750   // 7,5%
	bpsAtomPhase1    = 750   // 7,5%
	bpsGovPhase1     = 200   // 2%
	bpsPoolPhase1    = 1_000 // 10% — dotação única de liquidez, encerra na Transição

	// Transição — pre-sale e POOL encerradas (≥10M, pool ainda fechada).
	bpsMinerTransition   = 3_000 // 30%
	bpsStakingTransition = 3_000 // 30%
	bpsAtomTransition    = 3_000 // 30%
	bpsGovTransition     = 200   // 2%

	// Fase 2 — DEFINITIVA (pool/IBC abertos). Permanente e imutável.
	bpsMinerPhase2   = 3_000 // 30%
	bpsOtcPhase2     = 4_000 // 40% → OTC LOCKED (era 42; §13.154 devolveu 2% à Gov)
	bpsStakingPhase2 = 1_000 // 10%
	bpsAtomPhase2    = 1_000 // 10%
	bpsGovPhase2     = 150   // 1,5% → tesouraria (só gasta por voto)
	bpsNotaryPhase2  = 50    // 0,5% → reserva do notariado (conta separada)

	// Constantes em todas as fases.
	bpsFounder      = 500 // 5%  → fundador
	bpsValidator    = 300 // 3%  → fee_collector/x-distribution
	bpsAtomBonusCap = 700 // 7% flat cap paid to ATOM-qualified locks (§13.85)
)

// BuildTokenomicsSnapshot computes the snapshot for the current regime.
// §13.64: the ATOM share ALWAYS goes to the Atom Reservoir.
func BuildTokenomicsSnapshot(poolActive bool, preSaleActive bool, atomBonusActive bool) TokenomicsSnapshot {
	s := TokenomicsSnapshot{
		PoolActive:              poolActive,
		PreSaleActive:           preSaleActive,
		PrePoolBoostOperational: !poolActive,
		LlcTreasuryBps:          0, // LLC dissolved
		FounderBps:              bpsFounder,
		ValidatorFixedBps:       bpsValidator,
		AtomBonusCapBps:         bpsAtomBonusCap,
		AtomBonusActive:         atomBonusActive,
	}

	switch {
	case poolActive: // Fase 2 — definitiva
		s.MinerBps = bpsMinerPhase2
		s.PosMintFromMiningBps = bpsStakingPhase2
		s.AtomBonusBps = bpsAtomPhase2
		s.OtcVaultBps = bpsOtcPhase2
		s.GovReserveBps = bpsGovPhase2
		s.NotaryReserveBps = bpsNotaryPhase2
		s.Reference = "Fase 2 (definitiva) — §13.132/§13.154: miner 30%, OTC LOCKED 40% (contínuo, sem tecto), staking 10%, ATOM 10%, tesouraria 1,5%, notariado 0,5%; fundador 5%, validadores 3%"
	case preSaleActive: // Fase 1
		s.MinerBps = bpsMinerPhase1
		s.PreSaleBps = bpsPreSalePhase1
		s.PosMintFromMiningBps = bpsStakingPhase1
		s.AtomBonusBps = bpsAtomPhase1
		s.GovReserveBps = bpsGovPhase1
		s.PoolTreasuryMintBps = bpsPoolPhase1
		s.Reference = "Fase 1 (pré-pool) — §13.132: miner 30% + pre-sale 35%, staking 7,5%, ATOM 7,5%, POOL 10% (dotação única); fundador 5%, validadores 3%, gov 2%"
	default: // Transição
		s.MinerBps = bpsMinerTransition
		s.PosMintFromMiningBps = bpsStakingTransition
		s.AtomBonusBps = bpsAtomTransition
		s.GovReserveBps = bpsGovTransition
		s.Reference = "Transição (pré-pool, pre-sale encerrada) — §13.132: miner 30%, staking 30%, ATOM 30%, gov 2%; sem OTC e sem POOL"
	}
	return s
}
