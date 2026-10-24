# Upgrades — halt height

> **Why this exists.** While the validator set is one host, an upgrade is "stop the
> four nodes and swap the binary". Once external validators join, that stops
> working: every node has to stop at **the same height** and come back with the
> **same binary**, or the ones left behind diverge and get jailed.

## How an upgrade is announced

The operator posts, in the Telegram group and in the release:

- **halt height** (not a time — the height is what consensus agrees on);
- the **sha256** of the new `luggomd`;
- what changed, and whether the state layout changed;
- the deadline (at least 24 h of notice).

Rule of thumb: 5 s per block, so 24 h is about **17,280 blocks**. The announced
height is `current height + (hours × 720)`.

> `x/upgrade` is wired in the app but **no handler is registered**. A governance
> `SoftwareUpgrade` will not do what you expect. Until that changes, the mechanism
> is `--halt-height`.

## What each validator does

### 1. Days before — verify, do not swap

Download the new binary next to the old one and check it:

```sh
sha256sum ./luggomd-new        # must match the announced hash
```

Then check the node and the distance to the halt:

```sh
./scripts/upgrade-preflight.sh \
  --bin ./luggomd-new \
  --home /opt/luggom/home \
  --node tcp://127.0.0.1:26657 \
  --sha256 <announced-hash> \
  --halt-height <announced-height>
```

It prints `PRONTO` or tells you what is missing (hash, sync, distance).

### 2. Before the height — arm the halt

The node needs `--halt-height` and, for the duration of the window,
`Restart=no`. **This second part matters:** `luggomd` exits with **status 1**
when it halts, so a unit with `Restart=on-failure` (the default in our package)
will restart it, it will halt again, and it will loop until the height passes.

```sh
sudo systemctl edit luggom-chain
```

```ini
[Service]
Restart=no
ExecStart=
ExecStart=/opt/luggom/build/luggomd start --home /opt/luggom/home --halt-height <HEIGHT>
```

```sh
sudo systemctl daemon-reload
sudo systemctl restart luggom-chain
```

### 3. At the height — the node stops itself

When the height is reached the node stops. In the log you will see:

```
ERR error in proxyAppConn.FinalizeBlock err="halt per configuration height N ..."
ERR CONSENSUS FAILURE!!! err="failed to apply block; error halt per configuration height N"
INF Stopping Node
```

**That is the halt, not a failure.** The process exits with status 1. The block
at the halt height is **not** applied — the node resumes from the one before.

### 4. Swap and resume

```sh
sudo install -m 0755 ./luggomd-new /opt/luggom/build/luggomd
```

Remove the drop-in (or at least the `--halt-height` and `Restart=no` lines —
**starting the new binary with the halt flag still set stops it again
immediately**):

```sh
sudo systemctl edit --full luggom-chain   # or: sudo rm /etc/systemd/system/luggom-chain.service.d/override.conf
sudo systemctl daemon-reload
sudo systemctl start luggom-chain
```

### 5. Verify

- `catching_up: false` and the height is above the halt height;
- `luggomd query staking validators` shows `BOND_STATUS_BONDED`;
- the node is signing blocks (no `missed_blocks` growth).

## If you miss the height

**Do not restart the old binary.** Download the new one and start with it.

A node that was stopped during the halt and comes back on the old binary will
fail on the first block it cannot validate, and will accumulate downtime — which
means *jail*. Unjailing is possible (`luggomd tx slashing unjail`), but the bond
stays locked for the unbonding period.

## Rollback

There is no state rollback after the halt: the chain moved on. Keep the old
binary only for the case where the **new** binary fails to start — and even then,
restoring it means restoring the node's data directory from a backup taken
before the halt.

## Checklist

- [ ] announced hash verified
- [ ] preflight printed `PRONTO`
- [ ] `--halt-height` and `Restart=no` in the drop-in
- [ ] node stopped at the height (status 1 is expected)
- [ ] binary swapped, halt flags removed
- [ ] `catching_up: false`, height above the halt, `BONDED`, signing

## On our own nodes (the operator)

The four nodes run from **one** unit that executes `scripts/run-local-testnet.sh`,
so the halt is armed with an environment variable rather than by editing the
start line:

```sh
sudo systemctl edit luggom-chain
```

```ini
[Service]
Environment=LUGGOM_HALT_HEIGHT=<HEIGHT>
Restart=no
```

```sh
sudo systemctl daemon-reload && sudo systemctl restart luggom-chain
ps -o args= -p "$(cat /opt/luggom/home/testnet-pids/fundador.pid)" | grep -o -- '--halt-height [0-9]*'
```

The last line must print the flag — if it does not, the variable did not reach
the runner and the nodes will **not** halt.

After the swap: set `LUGGOM_HALT_HEIGHT=0` (or remove the drop-in), remove
`Restart=no`, and start again.

**Smoke test, no upgrade needed:** arm a height far in the future (current +
50,000), restart, confirm the flag in `ps`, then set it back to `0` and restart.
Nothing halts, and the plumbing is proven.
