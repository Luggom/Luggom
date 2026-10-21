package types

import (
	sdkmath "cosmossdk.io/math"
	proto "github.com/cosmos/gogoproto/proto"
)

// AtomLock é o ATOM provisionado por um utilizador como QUALIFICADOR do bónus
// ATOM (§13.64/§13.119). Não é rendimento por si: habilita a camada ATOM sobre
// o LUG em staking do mesmo dono — as mesmas regras do Staking Reservoir
// (drip de % do saldo, rateado por amount × tranche).
//
// O ATOM fica em escrow no módulo e é devolvido no unlock — NUNCA é confiscado
// (§13.38). Fica inoperável até o IBC abrir: antes disso não existe denom ATOM.
type AtomLock struct {
	// Depositor is the address that locked the ATOM.
	Depositor string `protobuf:"bytes,1,opt,name=depositor,proto3" json:"depositor"`
	// Denom is the ATOM denom locked (IBC denom once the bridge opens).
	Denom string `protobuf:"bytes,2,opt,name=denom,proto3" json:"denom"`
	// Amount is the locked ATOM (base units).
	Amount sdkmath.Int `protobuf:"bytes,3,opt,name=amount,proto3,customtype=cosmossdk.io/math.Int" json:"amount"`
	// StartHeight is the height at which the lock started.
	StartHeight int64 `protobuf:"varint,4,opt,name=start_height,json=startHeight,proto3" json:"start_height"`
	// MinAtLock is the minimum required in force when the lock was created.
	MinAtLock sdkmath.Int `protobuf:"bytes,5,opt,name=min_at_lock,json=minAtLock,proto3,customtype=cosmossdk.io/math.Int" json:"min_at_lock"`
}

func (m *AtomLock) Reset()         { *m = AtomLock{} }
func (m *AtomLock) String() string { return proto.CompactTextString(m) }
func (*AtomLock) ProtoMessage()    {}
func (*AtomLock) Descriptor() ([]byte, []int) {
	return nil, nil
}

func init() {
	proto.RegisterType((*AtomLock)(nil), "luggom.luggom.v1.AtomLock")
}

// AtomMinLockBaseUnits é o mínimo de qualificação: 100 ATOM fixo (§13.116),
// sem +20/ano. Em unidades base (1 ATOM = 10^6).
var atomMinLockBaseUnits = sdkmath.NewInt(100_000_000)

// MinAtomLock devolve o mínimo de ATOM (unidades base) para qualificar.
func MinAtomLock() sdkmath.Int { return atomMinLockBaseUnits }

// AtomLockQualifies indica se uma quantidade de ATOM satisfaz o mínimo vigente.
func AtomLockQualifies(amount sdkmath.Int) bool {
	return !amount.IsNil() && amount.GTE(atomMinLockBaseUnits)
}
