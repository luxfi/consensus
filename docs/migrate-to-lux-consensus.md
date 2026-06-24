# Migrating to Lux Consensus

From Raft (etcd/hashicorp/seaweedfs), Apache Thrift coordination, ZooKeeper, or
a hand-rolled leader — onto the Quasar family: **leaderless, permissionless,
post-quantum-final, ZAP-native** (block/vote gossip over the
[zap-proto transport](https://github.com/zap-proto/go), no gRPC).

## The one idea

Most services don't need Raft — they need a **replicated, totally-ordered log
of commands**, applied identically on every replica. That is
[`replog`](../replog):

```go
log := replog.New(chain, apply)        // chain := consensus.NewChain(consensus.DefaultConfig())
go log.Run(ctx, 20*time.Millisecond)   // applies finalized commands to your state machine
log.Commit(ctx, payload)               // propose + block until finalized & applied
```

`apply(payload []byte) error` is your entire state machine. Consensus orders
and finalizes; you apply. No leader-election FSM, no snapshot machinery — those
are the engine's job.

## From Raft

A drop-in. The master in our reference cutover replaced `seaweedfs/raft` +
`hashicorp/raft` with one `replog`-backed type:

| Raft | Lux consensus |
|---|---|
| `server.Do(cmd)` | `log.Commit(ctx, marshal(cmd))` |
| FSM `Apply(log)` | the `apply` func you pass `replog.New` |
| `Leader()` / forward-to-leader | **gone** — see below |
| `AddPeer` / `RemovePeer` | advisory (permissionless; no reconfig round) |
| snapshot / `LoadSnapshot` | no-op (finality replaces snapshots) |
| election timeout / heartbeat | no-op (no election) |

### Leaderless ≠ no writer

The log is leaderless: any replica proposes, consensus totally-orders, and
**commutative** commands (a monotonic counter, an LWW map) reconcile by order.
But some ops are **not** commutative — allocating IDs, growing/assigning
resources, GC. Running those on N nodes at once double-allocates.

Don't bring back an elected leader for them. Pin the writer **deterministically**
to the lowest member address: every node computes the same writer from the
(consensus-agreed) membership, with **no election and no forwarding**, and when
it leaves the next-lowest takes over.

```go
func (cs *ConsensusServer) Writer() string {       // was Leader()
    w := cs.self
    for _, m := range cs.members { if m < w { w = m } }
    return w
}
// gate non-commutative ops on:  cs.Writer() == cs.self
```

It is an app-level role *orthogonal* to the leaderless log — the decomplected
answer to "Raft gave me a leader for free."

## From Thrift

Thrift is an RPC + serialization framework — it is not consensus. If you used
Thrift for service calls, replace it with the ZAP transport + wire schemas (see
the [protobuf migration guide](https://github.com/zap-proto/go/blob/main/docs/migrate-from-protobuf.md);
the same applies to Thrift IDL). If you used a Thrift-based metastore for
*coordination* (e.g. Hive), that coordination becomes `replog` commands.

## From ZooKeeper / etcd-as-coordinator

ZK gives you five things; here is the Lux mapping:

| ZooKeeper | Lux consensus |
|---|---|
| replicated config / znodes | `replog` commands → your state machine |
| leader election | the deterministic **pinned writer** (no election) |
| locks | a `replog` command that records the holder; lease via a height/time bound |
| membership / ephemeral nodes | permissionless participation + advisory peer list |
| watches | drive them off the `apply` callback (apply = the event) |

You delete the separate ZK ensemble: coordination lives *in* the service, on
the same consensus that orders its data.

## Why bother

- **Leaderless**: no election stalls, no leader as a single point of failure,
  no client-side forward-to-leader hop — lower tail latency.
- **Permissionless**: no membership-reconfiguration round to admit/evict a node.
- **Post-quantum-final**: Quasar BLS + Corona + ML-DSA threshold signing.
- **ZAP-native**: votes gossip over the zap-proto transport — no gRPC, no
  separate coordination cluster to run.

## Checklist

- [ ] state machine expressed as `apply(payload) error`
- [ ] writes go through `log.Commit`; reads off applied in-memory state
- [ ] non-commutative ops gated on the deterministic pinned writer (not a leader)
- [ ] membership advisory; no reconfiguration rounds
- [ ] old Raft/ZK/Thrift-coordination dependency removed from the build
