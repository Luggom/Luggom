package types

import (
	sdkmath "cosmossdk.io/math"
	proto "github.com/cosmos/gogoproto/proto"
)

// SavingsLock represents a LUG lock in the Reservoir (savings account — §13.64/§13.65).
// Principal + yield return AUTOMATICALLY to the wallet at lock maturity (§13.38).
type SavingsLock struct {
	// Id is the global lock identifier (auto-increment).
	Id uint64 `protobuf:"varint,1,opt,name=id,json=id,proto3" json:"id"`
	// Depositor is the address that locked.
	Depositor string `protobuf:"bytes,2,opt,name=depositor,json=depositor,proto3" json:"depositor"`
	// Amount is the locked principal (uluggom).
	Amount sdkmath.Int `protobuf:"bytes,3,opt,name=amount,json=amount,proto3,customtype=cosmossdk.io/math.Int" json:"amount"`
	// TrancheDays: 90, 180 or 365.
	TrancheDays uint32 `protobuf:"varint,4,opt,name=tranche_days,json=trancheDays,proto3" json:"tranche_days"`
	// StartHeight is the height at which the lock started.
	StartHeight int64 `protobuf:"varint,5,opt,name=start_height,json=startHeight,proto3" json:"start_height"`
	// StartTime is the Unix seconds when the lock started (for time-based yield).
	StartTime int64 `protobuf:"varint,6,opt,name=start_time,json=startTime,proto3" json:"start_time"`
	// UnlockHeight is the height at which the lock matures (principal + yield released automatically).
	UnlockHeight int64 `protobuf:"varint,7,opt,name=unlock_height,json=unlockHeight,proto3" json:"unlock_height"`
	// YieldAccumulated is the yield already credited to this lock (uluggom).
	YieldAccumulated sdkmath.Int `protobuf:"bytes,8,opt,name=yield_accumulated,json=yieldAccumulated,proto3,customtype=cosmossdk.io/math.Int" json:"yield_accumulated"`
	// StakingYieldAccumulated is the part of the yield that came from the Staking
	// Reservoir. Kept separately so an EARLY unlock can return it to its source
	// instead of leaving it orphaned in the module escrow (§13.119).
	StakingYieldAccumulated sdkmath.Int `protobuf:"bytes,9,opt,name=staking_yield_accumulated,json=stakingYieldAccumulated,proto3,customtype=cosmossdk.io/math.Int" json:"staking_yield_accumulated"`
	// AtomYieldAccumulated is the part of the yield that came from the Atom Reservoir.
	AtomYieldAccumulated sdkmath.Int `protobuf:"bytes,10,opt,name=atom_yield_accumulated,json=atomYieldAccumulated,proto3,customtype=cosmossdk.io/math.Int" json:"atom_yield_accumulated"`
}

func (m *SavingsLock) Reset()         { *m = SavingsLock{} }
func (m *SavingsLock) String() string { return proto.CompactTextString(m) }
func (*SavingsLock) ProtoMessage()    {}
func (*SavingsLock) Descriptor() ([]byte, []int) {
	return nil, nil
}

func init() {
	proto.RegisterType((*SavingsLock)(nil), "luggom.luggom.v1.SavingsLock")
}
