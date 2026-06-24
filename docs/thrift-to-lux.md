# Thrift → Lux

Apache Thrift is an **RPC + serialization** framework — it is not a consensus
system. So a Thrift migration has two halves, and which one you need depends on
what you used Thrift *for*.

## 1. Thrift as RPC (the common case)

Thrift's IDL + generated clients/servers + `TBinaryProtocol` map onto the ZAP
stack the same way protobuf+gRPC does — **schema → wire → transport**:

| Thrift | ZAP |
|---|---|
| `.thrift` IDL `struct` / `service` | `.zap` schema `struct` / `interface` |
| generated `TBinaryProtocol` codec | zero-copy `New<Msg>`/`Wrap<Msg>` — no encode step |
| `TBinaryProtocol.write/read` | the bytes **are** the message |
| `TSocket` / `TFramedTransport` | [`zap-proto/go/transport`](https://github.com/zap-proto/go) — TCP/Unix/QUIC, PQ |
| `service Foo { … }` + processor | `interface Foo` + `transport.Serve(ln, DispatchFoo)` |
| oneway methods | fire-and-forget `Send` |

Follow the [protobuf → ZAP guide](https://github.com/zap-proto/go/blob/main/docs/migrate-from-protobuf.md)
— it is identical for Thrift IDL: translate `.thrift` structs/services to
`.zap`, generate with `zapgen`, dial over `transport`. You gain zero-copy reads,
post-quantum transport security, and promise pipelining (chain dependent calls
with no round-trips — Thrift's request/response could not).

## 2. Thrift as a coordination dependency

If your Thrift use was a **metastore** that other services coordinated through
(e.g. a Hive Metastore behind `TBinaryProtocol`), that coordination — not the
RPC — is what moves to consensus. The shared state it served becomes
[`replog`](../replog) commands:

```go
log := replog.New(chain, applyMetadata)  // applyMetadata(payload) error == your FSM
go log.Run(ctx, 20*time.Millisecond)
log.Commit(ctx, marshal(change))         // every metadata mutation, totally-ordered
```

Reads come off the applied in-memory state; writes go through `Commit`. You
delete the standalone metastore process — the coordination lives *in* the
service, on the consensus that orders its data, with no separate cluster to run.

If the coordination needed single-writer semantics for non-commutative ops
(sequence allocation, compaction), gate them on the deterministic **pinned
writer** — see [raft-to-lux](raft-to-lux.md#leaderless--no-writer).

## Which half do I need?

- Thrift was your service RPC → **half 1** (schema+transport). No consensus.
- Thrift fronted shared, mutated, coordinated state → **half 2** (`replog`).
- Both → do both; they are independent.

## Checklist

- [ ] `.thrift` structs/services → `.zap`, generated, `.thrift` deleted
- [ ] clients dial `transport`; servers serve `Dispatch<Svc>`
- [ ] any coordinated metastore state → `replog` commands
- [ ] Thrift runtime + IDL compiler removed from the build
