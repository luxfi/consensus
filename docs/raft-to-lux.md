# Raft → Lux Consensus

For services on `etcd/raft`, `hashicorp/raft`, or `seaweedfs/raft`. Lux
consensus (Quasar) is **leaderless, permissionless, post-quantum-final**, and
ZAP-native — votes gossip over the [zap-proto transport](https://github.com/zap-proto/go),
no gRPC.

## The insight

You don't need Raft — you need a **replicated, totally-ordered command log**
applied identically on every replica. That is [`replog`](../replog):

```go
chain := consensus.NewChain(consensus.DefaultConfig())
log := replog.New(chain, apply)        // apply(payload []byte) error  == your FSM
go log.Run(ctx, 20*time.Millisecond)   // applies finalized commands, in order
log.Commit(ctx, marshal(cmd))          // propose + block until finalized & applied
```

## Drop-in mapping

| Raft | Lux consensus |
|---|---|
| `server.Do(cmd)` | `log.Commit(ctx, marshal(cmd))` |
| FSM `Apply(*raft.Log)` | the `apply` func you pass to `replog.New` |
| `Leader()` + forward-to-leader | **gone** — see *Leaderless ≠ no writer* |
| `AddPeer` / `RemovePeer` | advisory; permissionless, no reconfiguration round |
| `Snapshot` / `LoadSnapshot` / `Restore` | no-op — finality replaces snapshotting |
| `SetElectionTimeout` / heartbeat | no-op — no election |
| `IsLeader()` gate | `cs.Writer() == cs.self` (below) |

The whole replacement is one type. Our reference (a storage master) swapped
`seaweedfs/raft` **and** `hashicorp/raft` for a single `replog`-backed
`ConsensusServer` with these methods as thin shims.

## Leaderless ≠ no writer

The log is leaderless: any replica proposes, consensus totally-orders, and
**commutative** commands (a monotonic counter, an LWW map) reconcile by order.
But **non-commutative** ops — allocating IDs, growing/assigning resources, GC —
double-allocate if N nodes run them at once.

Do **not** re-introduce an elected leader. Pin the writer **deterministically**
to the lowest member address — every node computes the same one from the
consensus-agreed membership, no election, no forwarding; when it leaves, the
next-lowest takes over:

```go
func (cs *ConsensusServer) Writer() string {
	w := cs.self
	for _, m := range cs.members {
		if m < w { w = m }
	}
	return w
}
// gate non-commutative ops on:  cs.Writer() == cs.self
```

An app-level role *orthogonal* to the leaderless log — the decomplected answer
to "Raft handed me a leader for free."

## Why it's better

No election stalls. No leader as a single point of failure. No forward-to-leader
hop (lower tail latency). No reconfiguration round to add/evict a node. PQ-final
(Quasar BLS + Corona + ML-DSA). And consensus rides your existing ZAP transport
— no separate Raft RPC port, no gRPC.

## Checklist

- [ ] FSM expressed as `apply(payload) error`
- [ ] writes via `log.Commit`; reads off applied in-memory state
- [ ] non-commutative ops gated on the deterministic pinned writer
- [ ] `raft` dependency removed from `go.mod`
