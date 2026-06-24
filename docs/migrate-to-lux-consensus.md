# Migrating to Lux Consensus

Lux consensus (Quasar) is **leaderless, permissionless, post-quantum-final**,
and ZAP-native — block/vote gossip rides the
[zap-proto transport](https://github.com/zap-proto/go), no gRPC, no separate
coordination cluster. The core is [`replog`](../replog): a replicated,
totally-ordered command log — `Commit(payload)` proposes, and once consensus
finalizes, your `apply(payload)` runs once, in order, on every replica.

Pick your starting point:

- **[Raft → Lux](raft-to-lux.md)** — `etcd/raft`, `hashicorp/raft`,
  `seaweedfs/raft`. A drop-in: `server.Do` → `log.Commit`, FSM → `apply`,
  `Leader()` → the deterministic pinned writer.
- **[Thrift → Lux](thrift-to-lux.md)** — Apache Thrift RPC → ZAP
  transport+schema; a Thrift coordination metastore → `replog`.
- **[ZooKeeper → Lux](zookeeper-to-lux.md)** — (also etcd-as-coordinator,
  Consul) config/locks/leader-election/membership/watches folded into the
  service.

The one idea that recurs: you rarely need Raft or a coordination ensemble — you
need an ordered replicated log (`replog`) plus, for non-commutative ops, a
**deterministic pinned writer** instead of an elected leader.
