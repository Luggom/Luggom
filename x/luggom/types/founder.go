package types

// VESTING DO FUNDADOR (§13.171).
//
// Cliff PURO de 24 meses: nada sai para o founder_address antes do cliff; aos 24
// meses liberta-se TODO o acumulado e, daí em diante, os 5% entram livres.
//
// A duração é uma CONSTANTE COMPILADA, não um param — fica fora do alcance da
// governança, como a cesta e o lote de 100k (§13.134 §3).
const (
	// founderVestingCliffSeconds é o cliff: 24 meses ≈ 720 dias (30 dias/mês).
	founderVestingCliffSeconds = int64(24 * 30 * 24 * 60 * 60)
)

// FounderVestingCliffSeconds devolve o cliff do Fundador, em segundos.
func FounderVestingCliffSeconds() int64 { return founderVestingCliffSeconds }
