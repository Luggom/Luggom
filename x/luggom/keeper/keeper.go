package keeper

import (
	"context"
	"fmt"
	"time"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/address"
	corestore "cosmossdk.io/core/store"
	sdkmath "cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"luggom/x/luggom/types"
)

type Keeper struct {
	storeService   corestore.KVStoreService
	cdc            codec.Codec
	addressCodec   address.Codec
	authority      []byte
	bankKeeper     types.BankKeeper
	stakingKeeper  types.StakingKeeper
	slashingKeeper types.SlashingKeeper

	Schema                  collections.Schema
	Params                  collections.Item[types.Params]
	UsdOracleSnapshot       collections.Item[types.UsdOracleSnapshot]
	UsedDocumentHashes      collections.KeySet[string]
	UsedProofIrsTokens      collections.KeySet[string]
	UsedPaymentIDs          collections.KeySet[string]
	MiningCapDailyBuckets   collections.Map[string, sdkmath.Int]
	SavingsLocks            collections.Map[uint64, types.SavingsLock]
	SavingsLocksSeq         collections.Sequence
	SavingsLocksByDepositor collections.KeySet[collections.Pair[string, uint64]]
	LastDripEpoch           collections.Item[string]
	// ATOM provisionado por depositor — qualificador da camada ATOM (§13.119).
	AtomLocks collections.Map[string, types.AtomLock]
	// FASE 6A — Pool nativa (AMM, §13.76)
	PoolPairs       collections.Map[string, types.PoolPair]
	LpPositions     collections.Map[collections.Pair[string, string], types.LpPosition]
	PoolLpSharesSeq collections.Sequence
	// FASE 6B — Escrow P2P on-chain (§13.76/§13.24)
	EscrowTrades collections.Map[uint64, types.EscrowTrade]
	EscrowSeq    collections.Sequence
	// OTC LOCKED vault (§13.132) — acumula o LUG da Fase 2 (40% contínuo, sem tecto).
	OtcVault collections.Item[types.OtcVault]
	// BLOCO D — pre-sale on-chain (§13.132 §4): posições de vesting + contadores.
	PreSalePositions    collections.Map[uint64, types.PreSalePosition]
	PreSalePositionsSeq collections.Sequence
	PreSaleCommitted    collections.Item[sdkmath.Int]
	PreSaleReleased     collections.Item[sdkmath.Int]
	PreSaleVestingEpoch collections.Item[string]
	// §13.171 — cliff do Fundador: inicio do relogio + total retido.
	FounderVestingStart collections.Item[uint64]
	FounderVestingHeld  collections.Item[sdkmath.Int]
	// §13.170 Grupo D — âncora do epoch anual (bloco 0), derivada e não governável.
	ChainStartTime collections.Item[uint64]
}

func NewKeeper(
	storeService corestore.KVStoreService,
	cdc codec.Codec,
	addressCodec address.Codec,
	authority []byte,
	bankKeeper types.BankKeeper,
	stakingKeeper types.StakingKeeper,
	slashingKeeper types.SlashingKeeper,
) Keeper {
	if _, err := addressCodec.BytesToString(authority); err != nil {
		panic(fmt.Sprintf("invalid authority address %s: %s", authority, err))
	}

	sb := collections.NewSchemaBuilder(storeService)

	k := Keeper{
		storeService:   storeService,
		cdc:            cdc,
		addressCodec:   addressCodec,
		authority:      authority,
		bankKeeper:     bankKeeper,
		stakingKeeper:  stakingKeeper,
		slashingKeeper: slashingKeeper,

		Params:                  collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		UsdOracleSnapshot:       collections.NewItem(sb, types.UsdOracleSnapshotKey, "usd_oracle_snapshot", codec.CollValue[types.UsdOracleSnapshot](cdc)),
		UsedDocumentHashes:      collections.NewKeySet(sb, types.UsedDocumentHashesPrefix, "used_doc_hashes", collections.StringKey),
		UsedProofIrsTokens:      collections.NewKeySet(sb, types.UsedProofIrsTokensPrefix, "used_proof_irs", collections.StringKey),
		UsedPaymentIDs:          collections.NewKeySet(sb, types.UsedPaymentIDsPrefix, "used_payment_ids", collections.StringKey),
		MiningCapDailyBuckets:   collections.NewMap(sb, types.MiningCapDailyBucketsPrefix, "mining_cap_daily_buckets", collections.StringKey, sdk.IntValue),
		SavingsLocks:            collections.NewMap(sb, types.SavingsLocksPrefix, "savings_locks", collections.Uint64Key, codec.CollValue[types.SavingsLock](cdc)),
		SavingsLocksSeq:         collections.NewSequence(sb, types.SavingsLockSequenceKey, "savings_lock_seq"),
		SavingsLocksByDepositor: collections.NewKeySet(sb, types.SavingsLocksByDepositorPrefix, "savings_locks_by_depositor", collections.PairKeyCodec(collections.StringKey, collections.Uint64Key)),
		LastDripEpoch:           collections.NewItem(sb, types.LastDripEpochKey, "last_drip_epoch", collections.StringValue),
		AtomLocks:               collections.NewMap(sb, types.AtomLocksPrefix, "atom_locks", collections.StringKey, codec.CollValue[types.AtomLock](cdc)),
		// FASE 6A — Pool nativa (AMM, §13.76)
		PoolPairs:       collections.NewMap(sb, types.PoolPairPrefix, "pool_pairs", collections.StringKey, codec.CollValue[types.PoolPair](cdc)),
		LpPositions:     collections.NewMap(sb, types.LpPositionPrefix, "lp_positions", collections.PairKeyCodec(collections.StringKey, collections.StringKey), codec.CollValue[types.LpPosition](cdc)),
		PoolLpSharesSeq: collections.NewSequence(sb, types.PoolLpSharesSeqPrefix, "pool_lp_shares_seq"),
		// FASE 6B — Escrow P2P on-chain (§13.76/§13.24)
		EscrowTrades: collections.NewMap(sb, types.EscrowTradePrefix, "escrow_trades", collections.Uint64Key, codec.CollValue[types.EscrowTrade](cdc)),
		EscrowSeq:    collections.NewSequence(sb, types.EscrowSequenceKey, "escrow_seq"),
		// OTC LOCKED vault (§13.117)
		OtcVault: collections.NewItem(sb, types.OtcVaultKey, "otc_vault", codec.CollValue[types.OtcVault](cdc)),
		// BLOCO D — pre-sale on-chain (§13.132 §4)
		PreSalePositions:    collections.NewMap(sb, types.PreSalePositionsPrefix, "presale_positions", collections.Uint64Key, codec.CollValue[types.PreSalePosition](cdc)),
		PreSalePositionsSeq: collections.NewSequence(sb, types.PreSalePositionsSeqPrefix, "presale_positions_seq"),
		PreSaleCommitted:    collections.NewItem(sb, types.PreSaleCommittedKey, "presale_committed", sdk.IntValue),
		PreSaleReleased:     collections.NewItem(sb, types.PreSaleReleasedKey, "presale_released", sdk.IntValue),
		PreSaleVestingEpoch: collections.NewItem(sb, types.PreSaleVestingEpochKey, "presale_vesting_epoch", collections.StringValue),
		// §13.171 — cliff do Fundador
		FounderVestingStart: collections.NewItem(sb, types.FounderVestingStartKey, "founder_vesting_start", collections.Uint64Value),
		FounderVestingHeld:  collections.NewItem(sb, types.FounderVestingHeldKey, "founder_vesting_held", sdk.IntValue),
		ChainStartTime:      collections.NewItem(sb, types.ChainStartTimeKey, "chain_start_time", collections.Uint64Value),
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}
	k.Schema = schema

	return k
}

// GetAuthority returns the module's authority.
func (k Keeper) GetAuthority() []byte {
	return k.authority
}

// GetApiVerifierAddress returns the expected attestation signer from on-chain params.
func (k Keeper) GetApiVerifierAddress(ctx context.Context) string {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return ""
	}
	return params.ApiVerifierAddress
}

// IsTaxEntityWhitelisted checks the on-chain TLSNotary whitelist.
// Whitelist é governado por params on-chain — requer 67% de aprovação para editar.
func (k Keeper) IsTaxEntityWhitelisted(ctx context.Context, entity string) (bool, error) {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return false, fmt.Errorf("failed to get params: %w", err)
	}
	return types.IsTaxEntityWhitelisted(entity, params.TaxEntityWhitelist), nil
}

// GetTotalSupply returns the total supply of a denom via the bank keeper.
func (k Keeper) GetTotalSupply(ctx context.Context, denom string) sdk.Coin {
	return k.bankKeeper.GetSupply(ctx, denom)
}

// preSaleThreshold is the supply threshold that ENDS the pre-sale phase and
// starts the transition: 10M LUG in uluggom base units (10_000_000 × 10^18),
// §13.132. Deterministic, on-chain.
//
// NOTA: compara com o SUPPLY TOTAL (não só o minerado) — o genesis entra na conta.
var preSaleThreshold = func() sdkmath.Int {
	v, ok := sdkmath.NewIntFromString("10000000000000000000000000")
	if !ok {
		panic("bad preSaleThreshold")
	}
	return v
}()

// preSaleActive reports whether the pre-sale phase is still running
// (total supply < 10M LUG, §13.132). Enquanto for verdade corre a Fase 1:
// minerador 30% + pre-sale 35% + POOL 10% + staking 7,5% + ATOM 7,5% + fundador
// 5% + validadores 3% + gov 2%.
//
// Depois dos 10M a pre-sale encerra (a fatia vai a 0), a POOL encerra, e o
// staking/ATOM sobem para 30% cada até a pool/IBC abrir — é a **Transição**.
// A Fase 2 definitiva (OTC 40%) só começa com a pool aberta (params.pool_active).
func (k Keeper) preSaleActive(ctx context.Context) bool {
	supply := k.GetTotalSupply(ctx, "uluggom")
	return supply.Amount.LT(preSaleThreshold)
}

// PoolOpen reports whether the pool is open — now a GOVERNABLE flag (params.pool_active),
// NOT a supply threshold. §13.114: the former 50M supply gate is EXTINCT; the pool opens
// after the pre-sale, at the founder/governance discretion.
func (k Keeper) PoolOpen(ctx context.Context) bool {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return false
	}
	return params.PoolActive
}

// GetTlsnotaryPhase devolve a fase de verificação — §13.170 Grupo B: a fase é
// uma CONSTANTE de código (2), já não um param. A fase 1 (trust-API, forjável)
// deixou de existir; uma proposta de governança já não a pode ligar.
func (k Keeper) GetTlsnotaryPhase(_ context.Context) uint32 {
	return types.TlsnotaryPhaseFrozen
}

// chainStartTime devolve a âncora do epoch anual (bloco 0). Fixa-se uma vez, no
// genesis; se por alguma razão estiver a 0, fixa-se ao bloco atual.
func (k Keeper) chainStartTime(ctx sdk.Context) time.Time {
	v, err := k.ChainStartTime.Get(ctx)
	if err == nil && v != 0 {
		return time.Unix(int64(v), 0).UTC()
	}
	now := ctx.BlockTime().Unix()
	_ = k.ChainStartTime.Set(ctx, uint64(now))
	return time.Unix(now, 0).UTC()
}
