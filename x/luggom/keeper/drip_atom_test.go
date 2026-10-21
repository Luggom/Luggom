package keeper_test

import (
	"testing"
	"time"

	sdkmath "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/stretchr/testify/require"

	"luggom/x/luggom/types"
)

// lugInt converte LUG (inteiro) em uluggom.
func lugInt(n int64) sdkmath.Int {
	return sdkmath.NewInt(n).MulRaw(1_000_000_000_000_000_000)
}

// monthlyDrip espelha o cálculo do keeper: 1/12 de 10% do saldo.
func monthlyDrip(balance sdkmath.Int) sdkmath.Int {
	return sdkmath.LegacyMustNewDecFromStr("0.10").MulInt(balance).QuoInt64(12).TruncateInt()
}

// monthlyCap é o teto mensal de um lock: 25% a.a. ÷ 12 do principal.
func monthlyCap(principal sdkmath.Int) sdkmath.Int {
	return sdkmath.LegacyMustNewDecFromStr("0.25").MulInt(principal).QuoInt64(12).TruncateInt()
}

// fundReservoirs configura os saldos dos dois reservatórios no mock do bank.
func fundReservoirs(f *mineFixture, staking, atom sdkmath.Int) {
	if f.bank.spendableByAddr == nil {
		f.bank.spendableByAddr = map[string]sdk.Coins{}
	}
	if staking.IsPositive() {
		addr := authtypes.NewModuleAddress(types.StakingReservoirModuleName)
		f.bank.spendableByAddr[addr.String()] = sdk.NewCoins(sdk.NewCoin("uluggom", staking))
	}
	if atom.IsPositive() {
		addr := authtypes.NewModuleAddress(types.AtomReservoirModuleName)
		f.bank.spendableByAddr[addr.String()] = sdk.NewCoins(sdk.NewCoin("uluggom", atom))
	}
}

// markAndDrip corre um drip para marcar a época e outro no mês seguinte para
// distribuir (a primeira passagem só marca a época).
func markAndDrip(t *testing.T, f *mineFixture) {
	t.Helper()
	jan := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	feb := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	require.NoError(t, f.keeper.RunMonthlyDrip(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(jan)))
	require.NoError(t, f.keeper.RunMonthlyDrip(sdk.UnwrapSDKContext(f.ctx).WithBlockTime(feb)))
}

func lockLug(t *testing.T, f *mineFixture, depositor string, amount sdkmath.Int, days uint32) uint64 {
	t.Helper()
	resp, err := f.keeper.LockSavings(f.ctx, &types.MsgLockSavings{
		Depositor:   depositor,
		Amount:      amount,
		TrancheDays: days,
	})
	require.NoError(t, err)
	return resp.LockId
}

// TestAtomDrip_SleepsBeforeIbc: sem IBC (AtomBonusActive=false) só o Staking paga;
// o Atom Reservoir não é tocado.
func TestAtomDrip_SleepsBeforeIbc(t *testing.T) {
	f := initMineFixture(t)
	depositor, _ := makeAddr(t, f.addressCodec, 0x71)

	id := lockLug(t, f, depositor, lugInt(1000), 365)
	stakingBal, atomBal := lugInt(1000), lugInt(500)
	fundReservoirs(f, stakingBal, atomBal)

	// Dá ATOM ao depositor — sem efeito enquanto o IBC estiver fechado.
	_, err := f.keeper.LockAtom(f.ctx, depositor, "atom", types.MinAtomLock())
	require.NoError(t, err)

	markAndDrip(t, f)

	lock, err := f.keeper.GetLock(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, monthlyDrip(stakingBal).String(), lock.YieldAccumulated.String())
}

// TestAtomDrip_PaysQualifiedAfterIbc: com IBC aberto, o lock qualificado recebe a
// camada ATOM (mesma regra do Staking).
func TestAtomDrip_PaysQualifiedAfterIbc(t *testing.T) {
	f := initMineFixture(t)
	depositor, _ := makeAddr(t, f.addressCodec, 0x72)

	id := lockLug(t, f, depositor, lugInt(1000), 365)
	stakingBal, atomBal := lugInt(1000), lugInt(500)
	fundReservoirs(f, stakingBal, atomBal)

	params := types.DefaultParams()
	params.PoolActive = true
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))

	_, err := f.keeper.LockAtom(f.ctx, depositor, "atom", types.MinAtomLock())
	require.NoError(t, err)

	markAndDrip(t, f)

	lock, err := f.keeper.GetLock(f.ctx, id)
	require.NoError(t, err)

	// Único lock: recebe a totalidade dos dois drips, limitado ao cap combinado.
	want := monthlyDrip(stakingBal).Add(monthlyDrip(atomBal))
	if capMonthly := monthlyCap(lugInt(1000)); want.GT(capMonthly) {
		want = capMonthly
	}
	require.Equal(t, want.String(), lock.YieldAccumulated.String())
	require.True(t, lock.YieldAccumulated.GT(monthlyDrip(stakingBal)), "a camada ATOM tem de acrescentar")
}

// TestAtomDrip_OnlyQualifiedReceives: o não-qualificado recebe só a base; o
// qualificado recebe a base + ATOM.
func TestAtomDrip_OnlyQualifiedReceives(t *testing.T) {
	f := initMineFixture(t)
	plain, _ := makeAddr(t, f.addressCodec, 0x73)
	holder, _ := makeAddr(t, f.addressCodec, 0x74)

	idPlain := lockLug(t, f, plain, lugInt(1000), 365)
	idHolder := lockLug(t, f, holder, lugInt(1000), 365)

	stakingBal, atomBal := lugInt(1000), lugInt(500)
	fundReservoirs(f, stakingBal, atomBal)

	params := types.DefaultParams()
	params.PoolActive = true
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	_, err := f.keeper.LockAtom(f.ctx, holder, "atom", types.MinAtomLock())
	require.NoError(t, err)

	markAndDrip(t, f)

	plainLock, err := f.keeper.GetLock(f.ctx, idPlain)
	require.NoError(t, err)
	holderLock, err := f.keeper.GetLock(f.ctx, idHolder)
	require.NoError(t, err)

	// Dois locks de igual peso: a base divide-se; o ATOM vai só ao qualificado.
	baseShare := monthlyDrip(stakingBal).QuoRaw(2)
	require.Equal(t, baseShare.String(), plainLock.YieldAccumulated.String())
	require.True(t, holderLock.YieldAccumulated.GT(plainLock.YieldAccumulated),
		"o qualificado tem de ganhar mais (base + ATOM)")
}

// TestAtomDrip_CombinedCapApplies: base + ATOM nunca excedem 25% a.a. do principal.
func TestAtomDrip_CombinedCapApplies(t *testing.T) {
	f := initMineFixture(t)
	depositor, _ := makeAddr(t, f.addressCodec, 0x75)

	principal := lugInt(1000)
	id := lockLug(t, f, depositor, principal, 365)

	// Reservatórios gordos: a base sozinha excede o cap.
	fundReservoirs(f, lugInt(1_000_000), lugInt(1_000_000))

	params := types.DefaultParams()
	params.PoolActive = true
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	_, err := f.keeper.LockAtom(f.ctx, depositor, "atom", types.MinAtomLock())
	require.NoError(t, err)

	markAndDrip(t, f)

	lock, err := f.keeper.GetLock(f.ctx, id)
	require.NoError(t, err)
	require.Equal(t, monthlyCap(principal).String(), lock.YieldAccumulated.String())
}

// TestEarlyUnlock_ReturnsYieldToReservoirs: sair antes do vencimento devolve o
// yield já creditado ao reservatório de origem (nunca fica órfão no escrow).
func TestEarlyUnlock_ReturnsYieldToReservoirs(t *testing.T) {
	f := initMineFixture(t)
	depositor, _ := makeAddr(t, f.addressCodec, 0x77)

	id := lockLug(t, f, depositor, lugInt(1000), 365)
	stakingBal, atomBal := lugInt(1000), lugInt(500)

	params := types.DefaultParams()
	params.PoolActive = true
	require.NoError(t, f.keeper.Params.Set(f.ctx, params))
	_, err := f.keeper.LockAtom(f.ctx, depositor, "atom", types.MinAtomLock())
	require.NoError(t, err)

	fundReservoirs(f, stakingBal, atomBal)
	markAndDrip(t, f)

	lock, err := f.keeper.GetLock(f.ctx, id)
	require.NoError(t, err)
	require.True(t, lock.StakingYieldAccumulated.IsPositive())
	require.True(t, lock.AtomYieldAccumulated.IsPositive())

	// Antes do unlock: quanto os reservatórios já receberam de volta = 0.
	stakingAddr := authtypes.NewModuleAddress(types.StakingReservoirModuleName)
	atomAddr := authtypes.NewModuleAddress(types.AtomReservoirModuleName)
	require.True(t, sentToAmount(f.bank, stakingAddr).IsZero())
	require.True(t, sentToAmount(f.bank, atomAddr).IsZero())

	_, err = f.keeper.UnlockSavings(f.ctx, &types.MsgUnlockSavings{Depositor: depositor, LockId: id})
	require.NoError(t, err)

	// Após o unlock: o yield creditado voltou a cada reservatório.
	require.Equal(t, lock.StakingYieldAccumulated.String(), sentToAmount(f.bank, stakingAddr).String())
	require.Equal(t, lock.AtomYieldAccumulated.String(), sentToAmount(f.bank, atomAddr).String())

	// O lock deixou de existir.
	_, err = f.keeper.GetLock(f.ctx, id)
	require.Error(t, err)
}

// TestAtomLock_QualificationAndUnlock: o ATOM trava, qualifica e é devolvido no unlock.
func TestAtomLock_QualificationAndUnlock(t *testing.T) {
	f := initMineFixture(t)
	depositor, _ := makeAddr(t, f.addressCodec, 0x76)

	// Abaixo do mínimo: não qualifica.
	_, err := f.keeper.LockAtom(f.ctx, depositor, "atom", types.MinAtomLock().SubRaw(1))
	require.NoError(t, err)
	qualified, err := f.keeper.IsAtomQualified(f.ctx, depositor)
	require.NoError(t, err)
	require.False(t, qualified, "abaixo de 100 ATOM não qualifica")

	// Completa até ao mínimo: qualifica.
	_, err = f.keeper.LockAtom(f.ctx, depositor, "atom", sdkmath.NewInt(1))
	require.NoError(t, err)
	qualified, err = f.keeper.IsAtomQualified(f.ctx, depositor)
	require.NoError(t, err)
	require.True(t, qualified)

	// Unlock devolve tudo e remove o qualificador.
	released, err := f.keeper.UnlockAtom(f.ctx, depositor)
	require.NoError(t, err)
	require.Equal(t, types.MinAtomLock().String(), released.String())
	qualified, err = f.keeper.IsAtomQualified(f.ctx, depositor)
	require.NoError(t, err)
	require.False(t, qualified)
}
