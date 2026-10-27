package govtally

import (
	"context"
	"time"

	govkeeper "github.com/cosmos/cosmos-sdk/x/gov/keeper"

	"luggom/x/luggom/types"
)

// NewGovPeriods adapta o x/gov para types.GovKeeper.
//
// Vive neste pacote por duas razões: já é a ponte entre o gov e o luggom (o tally
// ponderado), e `govkeeper.Keeper.Params` é um `collections.Item` — campo, não
// método — pelo que `*govkeeper.Keeper` não satisfaz a interface sozinho.
//
// O atraso dos payees do notariado deriva daqui em tempo de execução: uma
// remoção só começa a votar quando atinge o min_deposit e o x/gov executa a
// proposta no EndBlock em que a votação termina, logo o pior caso é
// max_deposit_period + voting_period (+1 dia para alguém notar).
func NewGovPeriods(k *govkeeper.Keeper) types.GovKeeper { return govPeriods{k} }

type govPeriods struct{ k *govkeeper.Keeper }

func (g govPeriods) GovPeriods(ctx context.Context) (time.Duration, time.Duration, error) {
	if g.k == nil {
		return 0, 0, nil
	}
	p, err := g.k.Params.Get(ctx)
	if err != nil {
		return 0, 0, err
	}
	return durOrZero(p.MaxDepositPeriod), durOrZero(p.VotingPeriod), nil
}

func durOrZero(d *time.Duration) time.Duration {
	if d == nil {
		return 0
	}
	return *d
}
