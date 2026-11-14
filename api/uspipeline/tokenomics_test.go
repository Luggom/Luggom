package uspipeline

import "testing"

// Os números esperados aqui são os do keeper (x/luggom/keeper/otc_vault.go).
// Se o keeper mudar, este teste tem de mudar com ele — é o único travão contra
// as duas cópias divergirem em silêncio.
func TestBuildTokenomicsSnapshot_SingleSplit(t *testing.T) {
	cases := []struct {
		name    string
		pool    bool
		preSale bool
		atom    bool
	}{
		{"pre-pool pre-sale", false, true, false},
		{"pre-pool pre-sale atom", false, true, true},
		{"pre-pool sem pre-sale", false, false, false},
		{"pre-pool atom", false, false, true},
		{"pool", true, false, false},
		{"pool atom", true, false, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := BuildTokenomicsSnapshot(tc.pool, tc.preSale, tc.atom)

			if s.MinerBps != 3000 || s.BusinessCoreBps != 500 || s.OtcVaultBps != 4000 {
				t.Fatalf("miner/lbc/otc: %+v", s)
			}
			if s.PosMintFromMiningBps != 750 || s.AtomBonusBps != 750 {
				t.Fatalf("staking/atom: %+v", s)
			}
			if s.FounderBps != 500 || s.ValidatorFixedBps != 300 {
				t.Fatalf("founder/validator: %+v", s)
			}
			if s.GovReserveBps != 150 || s.NotaryReserveBps != 50 {
				t.Fatalf("gov/notary: %+v", s)
			}
			if s.PreSaleBps != 0 || s.PoolTreasuryMintBps != 0 || s.LlcTreasuryBps != 0 {
				t.Fatalf("legacy/pool/presale devem ser 0: %+v", s)
			}
			if s.AtomBonusCapBps != 700 {
				t.Fatalf("atom cap: %+v", s)
			}
			if s.PoolActive != tc.pool || s.PreSaleActive != tc.preSale || s.AtomBonusActive != tc.atom {
				t.Fatalf("flags: %+v", s)
			}

			sum := s.MinerBps + s.BusinessCoreBps + s.OtcVaultBps + s.PosMintFromMiningBps +
				s.AtomBonusBps + s.FounderBps + s.ValidatorFixedBps + s.GovReserveBps +
				s.NotaryReserveBps + s.PreSaleBps + s.PoolTreasuryMintBps + s.LlcTreasuryBps
			if sum != 10_000 {
				t.Fatalf("%s: split sums to %d bps, expected 10000", tc.name, sum)
			}
		})
	}
}
