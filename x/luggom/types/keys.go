package types

import "cosmossdk.io/collections"

const (
	// ModuleName defines the module name
	ModuleName = "luggom"

	// LLCTreasuryModuleName deprecated (LLC DAO dissolved 2026-06-25). Kept for proto compat;
	// no longer receives mint. Pre-upgrade balance should be migrated by an upgrade handler.
	LLCTreasuryModuleName = "luggom_llc_treasury"

	// PoolTreasuryModuleName receives the POOL share — a one-time 10% liquidity
	// carve-out that runs ONLY in Fase 1 and ends at the Transition (§13.132 §1).
	PoolTreasuryModuleName = "luggom_pool_treasury"

	// PreSaleModuleName holds the pre-sale LUG during Fase 1 (§13.132 §4): it
	// receives the 35% share of each mint (<10M mined) and pays out the LUG sold
	// in the $350k raise (3,5M LUG × $0,10) as the 3-month linear vesting
	// releases. It also holds the USDT proceeds (moved out by the authority-gated
	// MsgDistributePreSaleProceeds).
	PreSaleModuleName = "luggom_presale"

	// AtomReservoirModuleName (Atom Reservoir — §13.64) ALWAYS receives the ATOM share
	// (11% pre / 14% post). It accumulates and pays the flat caps to qualified holders (Phase 2);
	// the excess stays as a cushion for low-mining months. It is LUG's savings account.
	AtomReservoirModuleName = "luggom_atom_reservoir"

	// StakingReservoirModuleName (Staking Reservoir — §13.56/§13.64) receives the
	// base staking share (11% constant). It feeds the monthly drip paid to LUG locks
	// (savings account). Separate from native PoS (0.005 LUG/tx + 8% validators in x/distribution).
	StakingReservoirModuleName = "luggom_staking_reservoir"

	// GovReserveModuleName is the protocol Treasury. §13.132 reactivated it at 2%
	// (Fase 1 and Transição) after it had been dropped in §13.64. From §13.153/
	// §13.154 it also stays at 2% in Fase 2 — but split, see below.
	//
	// It is DISCRETIONARY money: it only leaves by governance vote (security,
	// marketing, implementation, Open Finance). It is the opposite of the OTC
	// vault, which is never spent.
	GovReserveModuleName = "luggom_gov_reserve"

	// NotaryReserveModuleName holds the notary share (0,5% of each mint in Fase 2,
	// §13.154). It is a SEPARATE account from the Treasury on purpose: the money
	// that pays the notaries must not be reachable by the same vote that spends
	// marketing. The notaries are drawn by lot from a pool of volunteer validators
	// (§13.152), and paying them has to be automatic — a failed vote cannot be
	// allowed to stop mining.
	NotaryReserveModuleName = "luggom_notary_reserve"

	// OtcVaultModuleName is the OTC LOCKED vault (§13.132). It receives the Phase 2
	// share (40% of each mint, no cap) while mining exists. The LUG is sold for BTC
	// at the basket price (min lot 100k LUG); BTC only enters via IBC, so settlement
	// is deferred. The BTC received is PERMANENT collateral — it never leaves the
	// vault. It is NOT a spendable treasury.
	OtcVaultModuleName = "luggom_otc_vault"

	// FounderVestingModuleName retem a fatia de 5% do Fundador ate ao cliff de 24
	// meses passar (cliff PURO, §13.171). Aos 24 meses liberta TODO o acumulado
	// para founder_address; dai em diante os 5% entram directos. Sem permissoes.
	FounderVestingModuleName = "luggom_founder_vesting"

	// StoreKey defines the primary module store key
	StoreKey = ModuleName

	// GovModuleName duplicates the gov module's name to avoid a dependency with x/gov.
	// It should be synced with the gov module's name if it is ever changed.
	// See: https://github.com/cosmos/cosmos-sdk/blob/v0.52.0-beta.2/x/gov/types/keys.go#L9
	GovModuleName = "gov"
)

// ParamsKey is the prefix to retrieve all Params
var ParamsKey = collections.NewPrefix("p_luggom")

// UsedDocumentHashesPrefix is the prefix for storing hashes of documents already used for mining (prevents double-spend).
var UsedDocumentHashesPrefix = collections.NewPrefix("used_hash_")

// UsedProofIrsTokensPrefix stores backend proof handles already consumed (anti-replay for MsgMineUSD).
var UsedProofIrsTokensPrefix = collections.NewPrefix("used_proof_")

// UsedPaymentIDsPrefix stores payment ids already consumed (anti-replay for MsgMineTaxPayment).
var UsedPaymentIDsPrefix = collections.NewPrefix("used_payid_")

// UsdOracleSnapshotKey stores the last committed USD basket FX rates (Etapa C2).
var UsdOracleSnapshotKey = collections.NewPrefix("usd_ora_snap")

// UsdOracleDailyPrefix stores one snapshot per UTC day (key "YYYY-MM-DD"), so a
// mint can be priced at the ATTESTED PAYMENT DATE instead of the mining date.
// Sem isto, um recibo vira uma opção gratuita por 365 dias: espera-se pelo dia
// com a cesta mais baixa (e pelo dia antes do aniversário, quando o multiplicador
// salta +10%). Ver checkpoint §10 / revisão do Zealynx.
var UsdOracleDailyPrefix = collections.NewPrefix("usd_ora_day")

// MiningCapDailyBucketsPrefix stores the daily mining buckets for the 365d sliding window
// (annual maximum cap §13.49/§13.52: 15B year 0, −5% a.a.). Key: "mining_cap_" + YYYYMMDD.
var MiningCapDailyBucketsPrefix = collections.NewPrefix("mining_cap_")

// SavingsLocksPrefix stores savings locks by ID (Reservoir — §13.64/§13.65).
var SavingsLocksPrefix = collections.NewPrefix("sav_lock_")

// SavingsLocksByDepositorPrefix indexes savings locks by depositor address.
// Key: depositor_address + lock_id. Prefix must NOT be a prefix of SavingsLocksPrefix
// (collections rejects overlapping prefixes in the schema).
var SavingsLocksByDepositorPrefix = collections.NewPrefix("sav_dep_")

// SavingsLockSequenceKey stores the auto-increment ID for savings locks.
var SavingsLockSequenceKey = collections.NewPrefix("sav_seq_")

// LastDripEpochKey stores the last processed monthly drip epoch (YYYYMM) — §13.65 decision 4.
var LastDripEpochKey = collections.NewPrefix("drip_epoch")

// AtomLocksPrefix stores the ATOM provisioned by each depositor (qualifier for the
// ATOM layer of the savings yield, §13.119). Key: depositor address.
var AtomLocksPrefix = collections.NewPrefix("atom_lock_")

// FASE 6A — Pool nativa (AMM, §13.76)
// PoolPairPrefix stores the reserves of each LUG↔asset pair. Key: asset denom.
var PoolPairPrefix = collections.NewPrefix("pool_pair_")

// LpPositionPrefix stores LP positions by (depositor, asset). Key: depositor + asset.
var LpPositionPrefix = collections.NewPrefix("lp_pos_")

// PoolLpSharesSeqPrefix is the auto-increment ID for LP positions.
var PoolLpSharesSeqPrefix = collections.NewPrefix("lp_seq_")

// FASE 6B — Escrow P2P on-chain (§13.76/§13.24)
// EscrowTradePrefix stores P2P trades by ID.
var EscrowTradePrefix = collections.NewPrefix("escrow_tr_")

// EscrowSequenceKey is the auto-increment ID for escrow trades.
var EscrowSequenceKey = collections.NewPrefix("escrow_seq_")

// OtcVaultKey stores the OTC LOCKED vault state (accumulated LUG + retained assets, §13.117).
var OtcVaultKey = collections.NewPrefix("otc_vault")

// --- BLOCO D — pre-sale on-chain (§13.132 §4) ---

// PreSalePositionsPrefix stores the vesting positions by ID.
var PreSalePositionsPrefix = collections.NewPrefix("presale_pos_")

// PreSalePositionsSeqPrefix is the auto-increment ID for pre-sale positions.
var PreSalePositionsSeqPrefix = collections.NewPrefix("presale_seq_")

// PreSaleCommittedKey stores the total LUG sold (cap = PreSaleCapLug).
var PreSaleCommittedKey = collections.NewPrefix("presale_sold")

// PreSaleReleasedKey stores the total LUG already released to buyers.
var PreSaleReleasedKey = collections.NewPrefix("presale_released")

// PreSaleVestingEpochKey stores the last vesting day processed (YYYYMMDD, UTC).
var PreSaleVestingEpochKey = collections.NewPrefix("presale_epoch")

// ChainStartTimeKey stores the Unix time (UTC) of block 0 (genesis). É a âncora
// determinística do epoch anual (cesta e teto) — §13.170 Grupo D: derivada, não
// um knob governável.
var ChainStartTimeKey = collections.NewPrefix("chain_start_time")

// FounderVestingStartKey stores the Unix time (UTC) the founder cliff clock
// started — captured once, at genesis (block 0).
var FounderVestingStartKey = collections.NewPrefix("founder_vesting_start")

// FounderVestingHeldKey stores the total LUG retained for the founder and not
// yet released (mirrors PreSaleReleasedKey).
var FounderVestingHeldKey = collections.NewPrefix("founder_vesting_held")

// Event types
const (
	EventTypeMine           = "mine"
	EventTypePreSale        = "presale"
	EventTypeFounderVesting = "founder_vesting"
)
