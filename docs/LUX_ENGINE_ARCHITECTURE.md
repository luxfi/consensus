# Lux Engine Architecture

## Core Components

### Consensus Stages (Photonic)
- **photon/** - Sampling stage
- **wave/** - Thresholding stage  
- **focus/** - Confidence stage

### Finalizers
- **beam/** - Linear chain finalizer (PQ-secured)
- **flare/** - DAG vertex ordering
- **nova/** - DAG consensus finalizer

### Engines
- **engine/chain/** - PQ-secured blockchain engine
- **engine/dag/** - PQ-secured DAG engine  
- **engine/pq/** - Post-quantum consensus engine

### Runtimes
- **runtimes/chain/** - PQ-secured chain runtime
- **runtimes/dag/** - PQ-secured DAG runtime
- **runtimes/consensus/** - PQ-secured consensus runtime

## Integration Points

### Node Repository Hooks
1. **Consensus Interface** - Pluggable consensus stages
2. **Engine Interface** - Chain/DAG/PQ engine selection
3. **Runtime Interface** - Execution environment
4. **Ringtail Integration** - Post-quantum cryptography

### Data Flow
```
Network → Photon → Wave → Focus → [Beam|Flare+Nova] → Engine → Runtime
                                           ↓
                                      Ringtail PQ
```

## Post-Quantum Security
- All consensus stages use Ringtail for quantum-resistant signatures
- Chain engine uses PQ-secured block hashing
- DAG engine uses PQ-secured vertex validation
- Consensus runtime provides PQ-secured state transitions