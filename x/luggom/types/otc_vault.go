package types

import (
	sdkmath "cosmossdk.io/math"
	proto "github.com/cosmos/gogoproto/proto"
)

// OTC LOCKED vault state (§13.132). Ver a tabela das fases em
// x/luggom/keeper/otc_vault.go.
//
// O cofre acumula o LUG da Fase 2 (40% de cada mineração, sem tecto) e vende-o
// só por BTC, sempre ao preço da cesta, em lotes mínimos de 100.000 LUG.
//
// O BTC recebido é LASTRO PERMANENTE: nunca sai do cofre, nunca é queimado e
// nunca é trocado por LUG. Só pode ser migrado para outra ponte/denom do MESMO
// ativo com 87% de governança — a diretriz permanente é "nunca vendemos o nosso
// ouro". Como o BTC só entra via IBC, o cofre fica vazio até essa abertura.
type OtcVault struct {
	// AccumulatedLug é o LUG acumulado no cofre (uluggom) — o stock à venda.
	AccumulatedLug sdkmath.Int `protobuf:"bytes,1,opt,name=accumulated_lug,json=accumulatedLug,proto3,customtype=cosmossdk.io/math.Int" json:"accumulated_lug"`
	// RetainedAssets é o lastro permanente (BTC). Vazio até o IBC abrir.
	RetainedAssets []OtcRetainedAsset `protobuf:"bytes,2,rep,name=retained_assets,json=retainedAssets,proto3" json:"retained_assets,omitempty"`
}

// OtcRetainedAsset é uma posição de ativo retida no cofre (denom + quantidade).
type OtcRetainedAsset struct {
	Denom  string      `protobuf:"bytes,1,opt,name=denom,proto3" json:"denom"`
	Amount sdkmath.Int `protobuf:"bytes,2,opt,name=amount,proto3,customtype=cosmossdk.io/math.Int" json:"amount"`
}

func (m *OtcVault) Reset()         { *m = OtcVault{} }
func (m *OtcVault) String() string { return proto.CompactTextString(m) }
func (*OtcVault) ProtoMessage()    {}
func (*OtcVault) Descriptor() ([]byte, []int) {
	return nil, nil
}

func (m *OtcRetainedAsset) Reset()         { *m = OtcRetainedAsset{} }
func (m *OtcRetainedAsset) String() string { return proto.CompactTextString(m) }
func (*OtcRetainedAsset) ProtoMessage()    {}
func (*OtcRetainedAsset) Descriptor() ([]byte, []int) {
	return nil, nil
}

func init() {
	proto.RegisterType((*OtcVault)(nil), "luggom.luggom.v1.OtcVault")
	proto.RegisterType((*OtcRetainedAsset)(nil), "luggom.luggom.v1.OtcRetainedAsset")
}

// OtcPhase identifica o regime de split vigente (§13.132 + §13.159).
//
//	Fase 1 (pré-pool, pre-sale activa): supply total <10M LUG — minerador 30% +
//	  pre-sale 35% + POOL 10% + staking 7,5% + ATOM 7,5% + fundador 5% +
//	  validadores 3% + gov 2%.
//	Transição (pré-pool, pre-sale encerrada): ≥10M e pool ainda fechada — a
//	  pre-sale e a POOL encerram e o staking/ATOM sobem para 30% cada; ainda sem OTC.
//	Fase 2 (definitiva): pool + IBC abertos — minerador 30% + OTC LOCKED 40%
//	  contínuo + staking 10% + ATOM 10% + fundador 5% + validadores 3% +
//	  gov 2% (0,5% notariado + 1,5% tesouraria). Permanente e imutável.
type OtcPhase string

const (
	OtcPhase1          OtcPhase = "phase1"
	OtcPhaseTransition OtcPhase = "transition"
	OtcPhase2          OtcPhase = "phase2"
)

func (p OtcPhase) String() string { return string(p) }

// otcAllowedAssets são os únicos ativos aceites na venda do cofre (§13.117):
// o LUG do cofre troca só por BTC. O PAXG foi descartado — é um claim regulado
// (Paxos) com capacidade de freeze, o que contradiz a promessa anti-freeze.
var otcAllowedAssets = []string{"btc"}

// otcMinLotLug é o lote mínimo de venda do cofre: 100.000 LUG em uluggom
// (§13.132; era 10.000 no §13.117 e 100.000 no §13.116). Com os lotes de 100k o
// tecto é GROSSISTA: a descoberta de preço fica nos pools externos e o cofre faz
// o tecto. Constante compilada ⇒ fora do alcance da governança.
var otcMinLotLug = func() sdkmath.Int {
	v, ok := sdkmath.NewIntFromString("100000000000000000000000") // 100_000 × 10^18
	if !ok {
		panic("bad otcMinLotLug")
	}
	return v
}()

// OtcAllowedAssets devolve os denoms aceites na venda do cofre.
func OtcAllowedAssets() []string {
	return append([]string(nil), otcAllowedAssets...)
}

// IsOtcAllowedAsset indica se o denom pode ser usado para comprar o LUG do cofre.
func IsOtcAllowedAsset(denom string) bool {
	for _, a := range otcAllowedAssets {
		if a == denom {
			return true
		}
	}
	return false
}

// MinOtcLotLug devolve o lote mínimo de venda (100.000 LUG) em uluggom.
func MinOtcLotLug() sdkmath.Int { return otcMinLotLug }

// ValidateOtcSale valida as regras da venda do cofre: só BTC e lote mínimo de
// 100.000 LUG (§13.132). O preço é a cesta (§13.132: a cesta daquele dia e ano,
// calculada pelo oráculo — o cofre NUNCA vende abaixo dela), aplicado na
// liquidação.
func ValidateOtcSale(assetDenom string, lugAmount sdkmath.Int) error {
	if !IsOtcAllowedAsset(assetDenom) {
		return ErrOtcAssetNotAllowed.Wrapf("%q (só BTC)", assetDenom)
	}
	if lugAmount.IsNil() || !lugAmount.IsPositive() {
		return ErrOtcLotBelowMinimum.Wrap("amount must be positive")
	}
	if lugAmount.LT(otcMinLotLug) {
		return ErrOtcLotBelowMinimum.Wrapf("lot %s is below the 100k LUG minimum %s",
			lugAmount.String(), otcMinLotLug.String())
	}
	return nil
}
