package types

import (
	sdkmath "cosmossdk.io/math"
	proto "github.com/cosmos/gogoproto/proto"
)

// UsdOracleSnapshot stores the last committed FX basket used for MsgMineUSD (Etapa C2).
type UsdOracleSnapshot struct {
	ChfUsd      sdkmath.LegacyDec `protobuf:"bytes,1,opt,name=chf_usd,json=chfUsd,proto3,customtype=cosmossdk.io/math.LegacyDec" json:"chf_usd"`
	EurUsd      sdkmath.LegacyDec `protobuf:"bytes,2,opt,name=eur_usd,json=eurUsd,proto3,customtype=cosmossdk.io/math.LegacyDec" json:"eur_usd"`
	GbpUsd      sdkmath.LegacyDec `protobuf:"bytes,3,opt,name=gbp_usd,json=gbpUsd,proto3,customtype=cosmossdk.io/math.LegacyDec" json:"gbp_usd"`
	BlockHeight int64             `protobuf:"varint,4,opt,name=block_height,json=blockHeight,proto3" json:"block_height"`
	// UpdatedAt is the Unix seconds (UTC) when the snapshot was committed.
	// Usado para o staleness por tempo (§13.56: >36h sem update → fallback/pausa).
	UpdatedAt int64 `protobuf:"varint,5,opt,name=updated_at,json=updatedAt,proto3" json:"updated_at"`
	// BrlUsd is BRL per 1 USD (e.g. 5.0). Guardado por DIA para a conversão de um
	// pagamento em BRL usar a taxa da data do pagamento, não a da mineração.
	BrlUsd sdkmath.LegacyDec `protobuf:"bytes,6,opt,name=brl_usd,json=brlUsd,proto3,customtype=cosmossdk.io/math.LegacyDec" json:"brl_usd"`
	// CadUsd is CAD per 1 USD.
	CadUsd sdkmath.LegacyDec `protobuf:"bytes,7,opt,name=cad_usd,json=cadUsd,proto3,customtype=cosmossdk.io/math.LegacyDec" json:"cad_usd"`
}

func (m *UsdOracleSnapshot) Reset()         { *m = UsdOracleSnapshot{} }
func (m *UsdOracleSnapshot) String() string { return proto.CompactTextString(m) }
func (*UsdOracleSnapshot) ProtoMessage()    {}
func (*UsdOracleSnapshot) Descriptor() ([]byte, []int) {
	return nil, nil
}

func init() {
	proto.RegisterType((*UsdOracleSnapshot)(nil), "luggom.luggom.v1.UsdOracleSnapshot")
}
