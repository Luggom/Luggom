package main

import (
	"encoding/json"
	"net/http"

	"luggom/api/escrowlab"
)

// escrowLabNotFound (A-3, hard test): com a lab desligada, responde como uma
// ROTA INEXISTENTE (404 padrão) — não confirma que a superfície existe.
func (s *Server) escrowLabNotFound(w http.ResponseWriter, r *http.Request) {
	http.NotFound(w, r)
}

func (s *Server) handleEscrowLabStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.cfg.EscrowLab {
		s.escrowLabNotFound(w, r)
		return
	}
	snap := escrowlab.NewChainSnapshot()
	writeJSON(w, http.StatusOK, escrowlab.StatusResponse{
		ChainSnapshot: snap,
		OpenOffers:    s.escrow.OpenCount(),
		Disclaimer:    escrowlab.Disclaimer(),
	})
}

func (s *Server) handleEscrowLabOffersList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.cfg.EscrowLab {
		s.escrowLabNotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"offers": s.escrow.ListOpen(),
	})
}

func (s *Server) handleEscrowLabOffersCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.cfg.EscrowLab {
		s.escrowLabNotFound(w, r)
		return
	}
	var body escrowlab.CreateOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	o, err := s.escrow.Create(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleEscrowLabOffersAccept(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.cfg.EscrowLab {
		s.escrowLabNotFound(w, r)
		return
	}
	var body escrowlab.AcceptOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	o, err := s.escrow.Accept(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) handleEscrowLabOffersCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.cfg.EscrowLab {
		s.escrowLabNotFound(w, r)
		return
	}
	var body escrowlab.CancelOfferRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	o, err := s.escrow.Cancel(body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, o)
}
