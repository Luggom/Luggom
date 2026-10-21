package escrowlab

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"
)

const disclaimerLab = "LAB: ofertas só em memória na API; sem travas on-chain. Testnet/dev — não é escrow de produção."

// Store é o livro de ofertas em memória (thread-safe).
type Store struct {
	mu     sync.RWMutex
	offers map[string]*Offer
}

// NewStore cria um livro vazio.
func NewStore() *Store {
	return &Store{offers: make(map[string]*Offer)}
}

func randomID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func denomAllowed(d string) bool {
	d = strings.TrimSpace(strings.ToLower(d))
	for _, x := range FakeDenoms {
		if d == x {
			return true
		}
	}
	return false
}

// Create valida e regista uma oferta aberta.
func (s *Store) Create(req CreateOfferRequest) (*Offer, error) {
	maker := strings.TrimSpace(req.MakerAddress)
	if maker == "" {
		return nil, fmt.Errorf("maker_address obrigatório")
	}
	gd := strings.TrimSpace(strings.ToLower(req.GiveDenom))
	wd := strings.TrimSpace(strings.ToLower(req.WantDenom))
	if !denomAllowed(gd) || !denomAllowed(wd) {
		return nil, fmt.Errorf("denom inválido; use um de: %v", FakeDenoms)
	}
	if gd == wd {
		return nil, fmt.Errorf("give_denom e want_denom devem diferir")
	}
	ga := strings.TrimSpace(req.GiveAmount)
	wa := strings.TrimSpace(req.WantAmount)
	if ga == "" || wa == "" {
		return nil, fmt.Errorf("give_amount e want_amount obrigatórios")
	}

	now := time.Now().UTC()
	exp := now.Add(MaxOfferTTLMinutes * time.Minute)
	o := &Offer{
		ID:            randomID(),
		MakerAddress:  maker,
		GiveDenom:     gd,
		GiveAmount:    ga,
		WantDenom:     wd,
		WantAmount:    wa,
		Status:        "open",
		CreatedAt:     now.Format(time.RFC3339),
		ExpiresAt:     exp.Format(time.RFC3339),
		DisclaimerLab: disclaimerLab,
	}

	s.mu.Lock()
	s.offers[o.ID] = o
	s.mu.Unlock()
	return o, nil
}

// ListOpen devolve ofertas abertas não expiradas.
func (s *Store) ListOpen() []*Offer {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	out := make([]*Offer, 0)
	for _, o := range s.offers {
		if o.Status != "open" {
			continue
		}
		if exp, err := time.Parse(time.RFC3339, o.ExpiresAt); err == nil && !now.Before(exp) {
			o.Status = "expired"
			continue
		}
		out = append(out, o)
	}
	return out
}

// Accept completa uma oferta no lab (contraparte simulada).
func (s *Store) Accept(req AcceptOfferRequest) (*Offer, error) {
	id := strings.TrimSpace(req.OfferID)
	taker := strings.TrimSpace(req.TakerAddress)
	if id == "" || taker == "" {
		return nil, fmt.Errorf("offer_id e taker_address obrigatórios")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.offers[id]
	if !ok {
		return nil, fmt.Errorf("oferta não encontrada")
	}
	if o.Status != "open" {
		return nil, fmt.Errorf("oferta não está aberta (status=%s)", o.Status)
	}
	if exp, err := time.Parse(time.RFC3339, o.ExpiresAt); err == nil && time.Now().UTC().After(exp) {
		o.Status = "expired"
		return nil, fmt.Errorf("oferta expirada")
	}
	if strings.EqualFold(o.MakerAddress, taker) {
		return nil, fmt.Errorf("taker não pode ser o maker")
	}
	o.TakerAddress = taker
	o.Status = "completed"
	o.CompletedAt = nowRFC3339()
	return o, nil
}

// Cancel cancela pelo maker.
func (s *Store) Cancel(req CancelOfferRequest) (*Offer, error) {
	id := strings.TrimSpace(req.OfferID)
	maker := strings.TrimSpace(req.MakerAddress)
	if id == "" || maker == "" {
		return nil, fmt.Errorf("offer_id e maker_address obrigatórios")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	o, ok := s.offers[id]
	if !ok {
		return nil, fmt.Errorf("oferta não encontrada")
	}
	if !strings.EqualFold(o.MakerAddress, maker) {
		return nil, fmt.Errorf("só o maker pode cancelar")
	}
	if o.Status != "open" {
		return nil, fmt.Errorf("oferta não está aberta (status=%s)", o.Status)
	}
	o.Status = "cancelled"
	return o, nil
}

// OpenCount conta ofertas abertas não expiradas (marca expiradas sob lock).
func (s *Store) OpenCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	n := 0
	for _, o := range s.offers {
		if o.Status != "open" {
			continue
		}
		if exp, err := time.Parse(time.RFC3339, o.ExpiresAt); err == nil && !now.Before(exp) {
			o.Status = "expired"
			continue
		}
		n++
	}
	return n
}
