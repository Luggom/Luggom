package types

import (
	sdkmath "cosmossdk.io/math"
	proto "github.com/cosmos/gogoproto/proto"
)

// PreSalePosition é uma compra do pre-sale com vesting LINEAR (Bloco D, §13.132 §4).
//
// O LUG fica em `luggom_presale` e é libertado para o comprador à medida que o
// tempo passa:
//
//	vestido(t) = TotalLug × (t − StartTime) ÷ (EndTime − StartTime)
//
// O estado do módulo é hand-written (como OtcVault/SavingsLock): struct Go com
// tags protobuf que implementa proto.Message, sem passar pelo .proto.
type PreSalePosition struct {
	Buyer       string      `protobuf:"bytes,1,opt,name=buyer,proto3" json:"buyer,omitempty"`
	TotalLug    sdkmath.Int `protobuf:"bytes,2,opt,name=total_lug,json=totalLug,proto3,customtype=cosmossdk.io/math.Int" json:"total_lug"`
	ReleasedLug sdkmath.Int `protobuf:"bytes,3,opt,name=released_lug,json=releasedLug,proto3,customtype=cosmossdk.io/math.Int" json:"released_lug"`
	StartTime   int64       `protobuf:"varint,4,opt,name=start_time,json=startTime,proto3" json:"start_time,omitempty"`
	EndTime     int64       `protobuf:"varint,5,opt,name=end_time,json=endTime,proto3" json:"end_time,omitempty"`
}

func (m *PreSalePosition) Reset()         { *m = PreSalePosition{} }
func (m *PreSalePosition) String() string { return proto.CompactTextString(m) }
func (*PreSalePosition) ProtoMessage()    {}
func (*PreSalePosition) Descriptor() ([]byte, []int) {
	return nil, nil
}

func init() {
	proto.RegisterType((*PreSalePosition)(nil), "luggom.luggom.v1.PreSalePosition")
}

// VestedAt devolve o LUG já vestido à hora `now` (Unix). Linear: 0 antes do
// início, TotalLug no fim, proporcional no meio. Nunca abaixo de zero nem acima
// do total.
func (p PreSalePosition) VestedAt(now int64) sdkmath.Int {
	if p.TotalLug.IsNil() || !p.TotalLug.IsPositive() {
		return sdkmath.ZeroInt()
	}
	if now <= p.StartTime {
		return sdkmath.ZeroInt()
	}
	if now >= p.EndTime {
		return p.TotalLug
	}
	span := p.EndTime - p.StartTime
	if span <= 0 {
		return p.TotalLug
	}
	return p.TotalLug.MulRaw(now - p.StartTime).QuoRaw(span)
}
