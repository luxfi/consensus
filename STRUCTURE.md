# Consensus Package Structure

This document describes the Go-idiomatic structure of the consensus package.

## Directory Layout

```
consensus/
├── core/                      # interfaces + thin helpers
│   ├── interfaces/            # acceptor, context, decidable, status, state
│   └── utils/                 # bag, errors, factory helpers
│
├── protocol/                  # consensus protocols (singular)
│   ├── prism/                 # shared polling/quorum (splitter, refract, cut)
│   │   ├── splitter.go        
│   │   ├── refract.go         
│   │   ├── cut.go             
│   │   └── *.go               # polling logic
│   │
│   ├── photon/                # unary (1-choice)
│   │   ├── photon.go          
│   │   └── photon_test.go     
│   │
│   ├── pulse/                 # binary (2-choice)
│   │   ├── pulse.go           
│   │   └── pulse_test.go      
│   │
│   ├── wave/                  # n-ary (N-choice)
│   │   ├── wave.go            
│   │   └── wave_test.go       
│   │
│   ├── nova/                  # linear-chain
│   │   ├── consensus.go       
│   │   ├── block.go           
│   │   ├── bootstrap/         
│   │   └── *_test.go          
│   │
│   ├── nebula/                # DAG
│   │   ├── nebula.go          
│   │   ├── bootstrap/         
│   │   └── nebula_test.go     
│   │
│   └── finality/              # PQ finality (formerly quasar)
│       ├── engine.go          
│       └── ringtail.go        
│
├── runtime/                   # runtime implementations (singular)
│   ├── chain/                 # PQ-secured linear chain
│   ├── galaxy/                # DAG runtime
│   ├── gravity/               # universal coordinator
│   └── orbit/                 # simple linear chain
│
├── networking/                # network layer (singular)
│   ├── grpc/                  
│   ├── handler/               
│   ├── router/                
│   └── sender/                
│
├── validator/                 # validator management (singular)
│   ├── logger.go              
│   └── validator_test.go      
│
├── uptime/                    # uptime tracking (singular)
│   ├── manager.go             
│   └── manager_test.go        
│
├── utils/                     # shared utilities
│   ├── validator/             # validator utilities
│   ├── sampler/               
│   ├── set/                   
│   └── ...                    
│
├── config/                    # configuration
├── benchmark/                 # benchmarking
├── cmd/                       # command-line tools
└── [tests live with code]     # *_test.go files alongside *.go files
```

## Key Changes from Previous Structure

1. **No top-level test/ directory** - All `*_test.go` files now live alongside their corresponding `*.go` files
2. **core/ split into interfaces/ and utils/** - Pure interface definitions separated from utilities
3. **Singular package names** - `protocols` → `protocol`, `validators` → `validator`, etc.
4. **quasar renamed to finality** - Better reflects its purpose as PQ finality wrapper
5. **prism moved to protocol/prism** - Centralized polling/quorum logic

## Package Conventions

- Each package contains its own tests (`foo.go` + `foo_test.go`)
- Interfaces are in `core/interfaces/`
- Shared protocol logic is in `protocol/prism/`
- Each protocol stays lean and focused
- Utilities are properly organized under `utils/`

## Import Paths

```go
import (
    "github.com/luxfi/consensus/core/interfaces"
    "github.com/luxfi/consensus/core/utils"
    "github.com/luxfi/consensus/protocol/prism"
    "github.com/luxfi/consensus/protocol/finality"
    "github.com/luxfi/consensus/validator"
    "github.com/luxfi/consensus/utils/validator"
)
```

This structure follows Go idioms and makes the codebase more discoverable and maintainable.