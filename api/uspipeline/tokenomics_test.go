package uspipeline

import "testing"

// Os números esperados aqui são os do keeper (§13.132 + §13.154). Se o keeper
// mudar, este teste tem de mudar com ele — é o único travão contra as duas
// cópias divergirem em silêncio (foi o que aconteceu entre o §13.132 e hoje).
func TestBuildTokenomicsSnapshot_Phase1(t *testing.T) {
	s := BuildTokenomicsSnapshot(false, true, false)
	if !s.PrePoolBoostOperational || s.PoolActive || !s.PreSaleActive {
		t.Fatalf("phase1: %+v", s)
	}
	if s.MinerBps != 3000 || s.PreSaleBps != 3500 {
		t.Fatalf("miner/presale: %+v", s)
	}
	if s.PosMintFromMiningBps != 750 || s.AtomBonusBps != 750 {
		t.Fatalf("staking/atom: %+v", s)
	}
	if s.PoolTreasuryMintBps != 1000 {
		t.Fatalf("pool deve ser 10%% (dotação única) na Fase 1: %+v", s)
	}
	if s.OtcVaultBps != 0 {
		t.Fatalf("otc should be 0 in phase 1: %+v", s)
	}
	if s.GovReserveBps != 200 || s.FounderBps != 500 || s.ValidatorFixedBps != 300 {
		t.Fatalf("constants: %+v", s)
	}
	if s.NotaryReserveBps != 0 {
		t.Fatalf("notariado só existe na Fase 2: %+v", s)
	}
	if s.AtomBonusCapBps != 700 || s.LlcTreasuryBps != 0 {
		t.Fatalf("atom cap/llc: %+v", s)
	}
}

func TestBuildTokenomicsSnapshot_Transition(t *testing.T) {
	s := BuildTokenomicsSnapshot(false, false, false)
	if !s.PrePoolBoostOperational || s.PoolActive || s.PreSaleActive {
		t.Fatalf("transition: %+v", s)
	}
	// O pre-sale e a POOL encerram; as fatias vão para staking e ATOM (30% cada).
	if s.MinerBps != 3000 || s.PreSaleBps != 0 || s.OtcVaultBps != 0 {
		t.Fatalf("miner/presale/otc: %+v", s)
	}
	if s.PoolTreasuryMintBps != 0 {
		t.Fatalf("POOL encerra na Transição: %+v", s)
	}
	if s.PosMintFromMiningBps != 3000 || s.AtomBonusBps != 3000 {
		t.Fatalf("staking/atom: %+v", s)
	}
}

func TestBuildTokenomicsSnapshot_Phase2(t *testing.T) {
	s := BuildTokenomicsSnapshot(true, false, false)
	if s.PrePoolBoostOperational || !s.PoolActive || s.PreSaleActive {
		t.Fatalf("phase2: %+v", s)
	}
	if s.MinerBps != 3000 || s.PreSaleBps != 0 {
		t.Fatalf("miner/presale: %+v", s)
	}
	if s.OtcVaultBps != 4000 {
		t.Fatalf("otc deve ser 40%% na Fase 2 (§13.154): %+v", s)
	}
	if s.PosMintFromMiningBps != 1000 || s.AtomBonusBps != 1000 {
		t.Fatalf("staking/atom: %+v", s)
	}
	// A Gov Reserve mantém 2% na Fase 2, partida (§13.154).
	if s.GovReserveBps != 150 || s.NotaryReserveBps != 50 {
		t.Fatalf("gov 1,5%% + notariado 0,5%%: %+v", s)
	}
	if s.GovReserveBps+s.NotaryReserveBps != 200 {
		t.Fatalf("gov+notariado têm de somar 2%%: %+v", s)
	}
	if s.FounderBps != 500 || s.ValidatorFixedBps != 300 {
		t.Fatalf("constants: %+v", s)
	}
}

// TestTokenomicsSplitSumsTo100Percent garante que todas as combinações
// (Fase 1 / Transição / Fase 2, com ATOM on/off) somam exatamente 10000 bps.
// Previne drift entre esta cópia e os BPS do keeper (x/luggom/keeper/otc_vault.go).
func TestTokenomicsSplitSumsTo100Percent(t *testing.T) {
	cases := []struct {
		name          string
		poolActive    bool
		preSaleActive bool
		atomBonus     bool
	}{
		{"fase1 no-atom", false, true, false},
		{"fase1 atom", false, true, true},
		{"transicao no-atom", false, false, false},
		{"transicao atom", false, false, true},
		{"fase2 no-atom", true, false, false},
		{"fase2 atom", true, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := BuildTokenomicsSnapshot(tc.poolActive, tc.preSaleActive, tc.atomBonus)
			sum := s.MinerBps + s.PreSaleBps + s.FounderBps + s.PosMintFromMiningBps +
				s.PoolTreasuryMintBps + s.ValidatorFixedBps + s.GovReserveBps +
				s.NotaryReserveBps + s.AtomBonusBps + s.OtcVaultBps
			if sum != 10_000 {
				t.Fatalf("%s: split sums to %d bps, expected 10000", tc.name, sum)
			}
		})
	}
}
