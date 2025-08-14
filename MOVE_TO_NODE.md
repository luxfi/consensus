# Components to Move to luxfi/node

The following components contain transport/networking code and should be moved to luxfi/node:

## To Move:
- networking/* - All networking code
- proto/* - All protobuf definitions
- validators/gvalidators/* - gRPC validator services
- utils/transport/* - Transport utilities
- utils/networking/* - Networking utilities
- cmd/* that starts servers or does RPC
- Any remaining gRPC/ZMQ/HTTP dependencies

## Node-side Adapters to Create:
- node/adapters/storage/pebble - Implements consensus/engine/core.Storage
- node/adapters/storage/badger - Implements consensus/engine/core.Storage
- node/adapters/network/p2p - Implements consensus/engine/core.Backend
- node/adapters/clock - Implements consensus/engine/core.Clock
- node/adapters/metrics/prometheus - Implements consensus/engine/core.Metrics
- node/adapters/logger - Implements consensus/engine/core.Logger

## Wiring Example:
```go
// In luxfi/node
deps := engine.Deps{
    Log:     nodeLogger,
    Clock:   nodeClock,
    Store:   badgerAdapter,
    Back:    p2pBackend,
    Metrics: promAdapter,
}
params := engine.Params{
    K:               21,
    AlphaPreference: 15,
    AlphaConfidence: 18,
    Beta:            8,
    SlotMillis:      200,
    PQEnabled:       true,
}

chain := chainengine.New(params, deps)      // linear Nova
dag   := dagenengine.New(params, deps)      // Nebula
pq    := pqengine.WrapChain(chain, params, deps) // add Quasar on top
```
