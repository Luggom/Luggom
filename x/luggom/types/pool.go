package types

import (
	sdkmath "cosmossdk.io/math"
	proto "github.com/cosmos/gogoproto/proto"
)

// PoolPair holds the reserves of a LUG↔asset pair (FASE 6A, §13.76).
// The AMM uses the constant-product formula x·y=k (Uniswap v2 style).
type PoolPair struct {
	// Asset is the pool asset denom (btc/atom/usdt/usdc/xmr — §13.24).
	Asset string `protobuf:"bytes,1,opt,name=asset,json=asset,proto3" json:"asset"`
	// LugReserve is the LUG side of the pair (uluggom).
	LugReserve sdkmath.Int `protobuf:"bytes,2,opt,name=lug_reserve,json=lugReserve,proto3,customtype=cosmossdk.io/math.Int" json:"lug_reserve"`
	// AssetReserve is the asset side of the pair (base units).
	AssetReserve sdkmath.Int `protobuf:"bytes,3,opt,name=asset_reserve,json=assetReserve,proto3,customtype=cosmossdk.io/math.Int" json:"asset_reserve"`
	// TotalLpShares is the total LP shares minted for this pair.
	TotalLpShares sdkmath.Int `protobuf:"bytes,4,opt,name=total_lp_shares,json=totalLpShares,proto3,customtype=cosmossdk.io/math.Int" json:"total_lp_shares"`
}

// NewPoolPair creates an empty pair for an asset.
func NewPoolPair(asset string) PoolPair {
	return PoolPair{
		Asset:         asset,
		LugReserve:    sdkmath.ZeroInt(),
		AssetReserve:  sdkmath.ZeroInt(),
		TotalLpShares: sdkmath.ZeroInt(),
	}
}

// LpPosition is a liquidity provider's position in a pair.
type LpPosition struct {
	// Depositor is the LP address.
	Depositor string `protobuf:"bytes,1,opt,name=depositor,json=depositor,proto3" json:"depositor"`
	// Asset is the pair asset denom.
	Asset string `protobuf:"bytes,2,opt,name=asset,json=asset,proto3" json:"asset"`
	// Shares is the LP shares held (fraction of TotalLpShares).
	Shares sdkmath.Int `protobuf:"bytes,3,opt,name=shares,json=shares,proto3,customtype=cosmossdk.io/math.Int" json:"shares"`
}

func (m *PoolPair) Reset()         { *m = PoolPair{} }
func (m *PoolPair) String() string { return proto.CompactTextString(m) }
func (*PoolPair) ProtoMessage()    {}
func (*PoolPair) Descriptor() ([]byte, []int) {
	return nil, nil
}

func (m *LpPosition) Reset()         { *m = LpPosition{} }
func (m *LpPosition) String() string { return proto.CompactTextString(m) }
func (*LpPosition) ProtoMessage()    {}
func (*LpPosition) Descriptor() ([]byte, []int) {
	return nil, nil
}

func init() {
	proto.RegisterType((*PoolPair)(nil), "luggom.luggom.v1.PoolPair")
	proto.RegisterType((*LpPosition)(nil), "luggom.luggom.v1.LpPosition")
}

// LpSharesMinted computes the LP shares to mint for a deposit of (lugAmt, assetAmt)
// into a pair with reserves (pairLug, pairAsset, totalShares).
// First deposit: sqrt(lugAmt × assetAmt). Subsequent: proportional to the smaller ratio.
func LpSharesMinted(pairLug, pairAsset, totalShares, lugAmt, assetAmt sdkmath.Int) sdkmath.Int {
	if lugAmt.IsNil() || assetAmt.IsNil() || !lugAmt.IsPositive() || !assetAmt.IsPositive() {
		return sdkmath.ZeroInt()
	}
	if totalShares.IsNil() || totalShares.IsZero() || pairLug.IsNil() || pairLug.IsZero() || pairAsset.IsNil() || pairAsset.IsZero() {
		// First deposit: sqrt(x·y). Approximate with integer sqrt of the product.
		prod := lugAmt.Mul(assetAmt)
		return integerSqrt(prod)
	}
	// Proportional to the smaller ratio (keeps the pair balanced).
	ratioLug := lugAmt.Mul(totalShares).Quo(pairLug)
	ratioAsset := assetAmt.Mul(totalShares).Quo(pairAsset)
	if ratioLug.LT(ratioAsset) {
		return ratioLug
	}
	return ratioAsset
}

// integerSqrt returns the integer square root of n (floor).
func integerSqrt(n sdkmath.Int) sdkmath.Int {
	if n.IsNil() || n.IsNegative() {
		return sdkmath.ZeroInt()
	}
	if n.IsZero() {
		return sdkmath.ZeroInt()
	}
	x := n
	y := n.Add(sdkmath.OneInt()).QuoRaw(2)
	for y.LT(x) {
		x = y
		y = n.Quo(y).Add(y).QuoRaw(2)
	}
	return x
}
