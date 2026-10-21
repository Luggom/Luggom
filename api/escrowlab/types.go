// Package escrowlab implementa a Etapa G2 (Pool Escrow — **laboratório**):
// livro de ofertas P2P em memória na API, para UX/testnet antes do módulo on-chain.
// Não substitui escrow real (depósitos 100/100, 60 min) — ver docs/ETAPA_G2_POOL_ESCROW.md.
package escrowlab

import "time"

// MaxOfferTTLMinutes alinha ao produto: janela máxima de escrow (~60 min).
const MaxOfferTTLMinutes = 60

// FakeDenoms são denoms de teste aceites no lab (chain pode ainda não os ter — só simulação).
var FakeDenoms = []string{"uluggom", "utestatom", "utestbtc", "utestusdt"}

// Offer representa uma oferta P2P no livro de laboratório.
type Offer struct {
	ID            string `json:"id"`
	MakerAddress  string `json:"maker_address"`
	TakerAddress  string `json:"taker_address,omitempty"`
	GiveDenom     string `json:"give_denom"`
	GiveAmount    string `json:"give_amount"`
	WantDenom     string `json:"want_denom"`
	WantAmount    string `json:"want_amount"`
	Status        string `json:"status"` // open | completed | cancelled | expired
	CreatedAt     string `json:"created_at"`
	ExpiresAt     string `json:"expires_at"`
	CompletedAt   string `json:"completed_at,omitempty"`
	DisclaimerLab string `json:"disclaimer_lab"`
}

// ChainSnapshot expõe capacidades G2 em GET /api/v1/chain/info.
type ChainSnapshot struct {
	Enabled            bool     `json:"enabled"`
	MaxOfferTTLMinutes int      `json:"max_offer_ttl_minutes"`
	FakeDenoms         []string `json:"fake_denoms"`
	Model              string   `json:"model"`
	Reference          string   `json:"reference"`
}

// StatusResponse para GET /api/v1/escrow/lab/status.
type StatusResponse struct {
	ChainSnapshot
	OpenOffers int    `json:"open_offers"`
	Disclaimer string `json:"disclaimer"`
}

// NewChainSnapshot metadados quando o lab está ligado.
func NewChainSnapshot() ChainSnapshot {
	return ChainSnapshot{
		Enabled:            true,
		MaxOfferTTLMinutes: MaxOfferTTLMinutes,
		FakeDenoms:         FakeDenoms,
		Model:              "p2p_offer_book_memory",
		Reference:          "docs/ETAPA_G2_POOL_ESCROW.md — G2 lab; mainnet: governança + módulo escrow",
	}
}

// CreateOfferRequest corpo JSON para criar oferta.
type CreateOfferRequest struct {
	MakerAddress string `json:"maker_address"`
	GiveDenom    string `json:"give_denom"`
	GiveAmount   string `json:"give_amount"`
	WantDenom    string `json:"want_denom"`
	WantAmount   string `json:"want_amount"`
}

// AcceptOfferRequest aceita e completa no lab (swap atómico simulado).
type AcceptOfferRequest struct {
	OfferID       string `json:"offer_id"`
	TakerAddress  string `json:"taker_address"`
}

// CancelOfferRequest cancelamento pelo maker.
type CancelOfferRequest struct {
	OfferID      string `json:"offer_id"`
	MakerAddress string `json:"maker_address"`
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// Disclaimer texto curto para respostas HTTP / APK.
func Disclaimer() string {
	return "LAB G2: livro de ofertas em memória na API; sem depósito on-chain. Ver docs/ETAPA_G2_POOL_ESCROW.md."
}
