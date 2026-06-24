# ZooKeeper → Lux Consensus

(Also covers `etcd`-as-coordinator and Consul.) ZooKeeper is a separate
coordination ensemble you run *alongside* your service. Lux consensus folds
that coordination **into** the service — the same consensus that orders your
data does your locks, config, and leader role, with no second cluster.

## ZK gives you five things — here's each one

### 1. Replicated config / znodes → `replog` commands

A znode write is a totally-ordered mutation applied on every replica — exactly
[`replog`](../replog):

```go
log := replog.New(chain, apply)        // apply(payload) error == your config FSM
go log.Run(ctx, 20*time.Millisecond)
log.Commit(ctx, marshal(set{path, val}))
```

Reads come off the applied in-memory tree; no `getData` round-trip.

### 2. Leader election → the deterministic pinned writer

ZK leader election (the lock-on-a-znode dance) is replaced by computing the
writer from the consensus-agreed membership — **no election, no herd, no
session timeout**:

```go
func writer(self string, members []string) string {
	w := self
	for _, m := range members { if m < w { w = m } }
	return w
}
// the leader-only work runs where  writer(self, members) == self
```

When the writer leaves the membership, the next-lowest takes over
deterministically. (See [raft-to-lux](raft-to-lux.md#leaderless--no-writer) for
why this beats an elected leader.)

### 3. Locks → a logged holder + lease

A lock is a `replog` command that records the holder, bounded by a height or
time lease so a dead holder's lock expires:

```go
log.Commit(ctx, marshal(acquire{key, holder: self, leaseUntil: now + ttl}))
// holds if apply set holder==self and the lease is unexpired
```

No ephemeral-node + watch machinery — the lock is just an ordered command.

### 4. Membership / ephemeral nodes → permissionless + advisory

Lux consensus is **permissionless**: participation is not gated by a fixed
roster, so there is no reconfiguration round to admit/evict a node. Keep an
advisory member list for observability; liveness is the transport's job.

### 5. Watches → the `apply` callback

A watch fires on a change; in Lux, **apply *is* the change**. Drive your
reactions from inside `apply` (or a channel it feeds) — every replica sees every
committed mutation, in order, locally.

## Mapping at a glance

| ZooKeeper | Lux consensus |
|---|---|
| znode write / config | `replog` command → your FSM |
| leader election | deterministic pinned writer (no election) |
| lock (ephemeral + watch) | logged holder + height/time lease |
| ephemeral membership | permissionless participation + advisory list |
| watch / `exists`+callback | the `apply` callback (apply = the event) |

## Why fold it in

You delete the ZK ensemble entirely: no separate process to deploy, monitor,
and keep quorate; no client session timeouts; no split between "the coordination
store" and "the data." Coordination lives in the service, post-quantum-final,
over the [zap-proto transport](https://github.com/zap-proto/go) — no gRPC.

## Checklist

- [ ] config/znode state → `replog` commands; reads off applied state
- [ ] leader election → deterministic pinned writer
- [ ] locks → logged holder + lease (no ephemeral-node/watch dance)
- [ ] watches → `apply`-driven reactions
- [ ] ZooKeeper ensemble + client removed
