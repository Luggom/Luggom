package types

import (
	sdkmath "cosmossdk.io/math"
	proto "github.com/cosmos/gogoproto/proto"
)

// EscrowTrade is a P2P trade held in custody by the module (FASE 6B, §13.76/§13.24).
// The seller deposits LUG on-chain; the buyer delivers the external asset (BTC/XMR/USDT...)
// off-chain; the escrow releases the LUG when the seller confirms receipt.
// Timeout (default 60 min) → both sides reversed (never confiscates).
type EscrowTrade struct {
	// Id is the auto-increment trade id.
	Id uint64 `protobuf:"varint,1,opt,name=id,json=id,proto3" json:"id"`
	// Seller is the address that created the offer and deposited LUG.
	Seller string `protobuf:"bytes,2,opt,name=seller,json=seller,proto3" json:"seller"`
	// Buyer is the address that accepted (may be empty while OPEN).
	Buyer string `protobuf:"bytes,3,opt,name=buyer,json=buyer,proto3" json:"buyer"`
	// AmountLug is the LUG locked by the seller (uluggom).
	AmountLug sdkmath.Int `protobuf:"bytes,4,opt,name=amount_lug,json=amountLug,proto3,customtype=cosmossdk.io/math.Int" json:"amount_lug"`
	// Asset is the external asset requested (btc/xmr/usdt/usdc/atom — denom on-chain).
	Asset string `protobuf:"bytes,5,opt,name=asset,json=asset,proto3" json:"asset"`
	// AmountAsset is the amount of the external asset requested.
	AmountAsset sdkmath.Int `protobuf:"bytes,6,opt,name=amount_asset,json=amountAsset,proto3,customtype=cosmossdk.io/math.Int" json:"amount_asset"`
	// Status is the current lifecycle state.
	Status EscrowStatus `protobuf:"varint,7,opt,name=status,json=status,proto3,enum=luggom.luggom.v1.EscrowStatus" json:"status"`
	// CreatedAt is the Unix seconds when the trade was created.
	CreatedAt int64 `protobuf:"varint,8,opt,name=created_at,json=createdAt,proto3" json:"created_at"`
	// ExpiresAt is the Unix seconds when the trade times out (created + timeout).
	ExpiresAt int64 `protobuf:"varint,9,opt,name=expires_at,json=expiresAt,proto3" json:"expires_at"`
}

// DefaultEscrowTimeoutMinutes is the default trade timeout (60 min, §13.24).
const DefaultEscrowTimeoutMinutes = uint32(60)

func (m *EscrowTrade) Reset()         { *m = EscrowTrade{} }
func (m *EscrowTrade) String() string { return proto.CompactTextString(m) }
func (*EscrowTrade) ProtoMessage()    {}
func (*EscrowTrade) Descriptor() ([]byte, []int) {
	return nil, nil
}

func init() {
	proto.RegisterType((*EscrowTrade)(nil), "luggom.luggom.v1.EscrowTrade")
}
