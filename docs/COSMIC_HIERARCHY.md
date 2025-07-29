# Lux Consensus: Cosmic Scale Hierarchy

## Overview

The Lux Consensus system is organized using a cosmic metaphor that reflects the scale and scope of each component. This pedagogical approach makes the architecture intuitive and memorable, with each level representing increasing scale and complexity.

## Scale Hierarchy (Smallest to Largest)

### 1. **Photon** - Quantum Sampling (Subatomic Scale)
- **Location**: `/photon/`
- **Purpose**: The fundamental quantum of consensus - individual vote sampling
- **Metaphor**: Like photons in physics, these are the smallest, fastest units of consensus information
- **Key Operations**:
  - Quantum sampling of network participants
  - Initial preference recording
  - Vote propagation initiation

### 2. **Wave** - Propagation & Thresholding (Wave Scale)
- **Location**: `/wave/`
- **Purpose**: Propagates consensus preferences through the network
- **Metaphor**: Like waves in physics, consensus spreads through the network
- **Key Operations**:
  - Network-wide vote propagation
  - Threshold detection
  - Preference amplification

### 3. **Focus** - Confidence Aggregation (Optical Scale)
- **Location**: `/focus/`
- **Purpose**: Focuses distributed votes into concentrated confidence
- **Metaphor**: Like optical focusing, concentrates scattered signals into clarity
- **Key Operations**:
  - Confidence score calculation
  - Multi-round aggregation
  - Consensus convergence detection

### 4. **Beam** - Linear Chain Finalization (Directed Energy Scale)
- **Location**: `/beam/`
- **Purpose**: Finalizes linear blockchain consensus
- **Metaphor**: Like a laser beam, provides directed, coherent finalization
- **Key Operations**:
  - Block finalization
  - Chain tip management
  - Linear ordering guarantee

### 5. **Flare** - DAG Vertex Rapid Ordering (Solar Flare Scale)
- **Location**: `/flare/`
- **Purpose**: Rapidly orders vertices in DAG structures
- **Metaphor**: Like solar flares, provides burst ordering of parallel vertices
- **Key Operations**:
  - Vertex conflict resolution
  - Rapid partial ordering
  - Parallel transaction handling

### 6. **Nova** - DAG Consensus Finalization (Stellar Explosion Scale)
- **Location**: `/nova/`
- **Purpose**: Finalizes entire DAG structures with explosive finality
- **Metaphor**: Like supernovas, provides powerful, final consensus across DAGs
- **Key Operations**:
  - DAG-wide finalization
  - Conflict set resolution
  - Multi-vertex atomic commits

## Engine Scale (Stellar to Galactic)

### 7. **Pulsar** - Linear Chain Engine (Stellar Scale)
- **Location**: `/engine/pulsar/`
- **Purpose**: Manages single-chain consensus operations
- **Metaphor**: Like pulsars, provides regular, rhythmic consensus pulses
- **Uses**: Photon → Wave → Focus → Beam

### 8. **Nebula** - DAG Chain Engine (Stellar Cluster Scale)
- **Location**: `/engine/nebula/`
- **Purpose**: Manages DAG-based consensus operations
- **Metaphor**: Like nebulae, handles complex, cloud-like structures
- **Uses**: Photon → Wave → Focus → Flare → Nova

### 9. **Quasar** - Unified Quantum-Secure Engine (Galactic Core Scale)
- **Location**: `/engine/quasar/`
- **Purpose**: Unifies all consensus mechanisms with quantum security
- **Metaphor**: Like quasars, the most powerful consensus engine
- **Features**:
  - Combines Pulsar and Nebula capabilities
  - Post-quantum cryptography via Ringtail
  - Dual-certificate finality

## Runtime Scale (Orbital to Universal)

### 10. **Orbit** - Linear Chain Runtime (Planetary Scale)
- **Location**: `/runtimes/orbit/`
- **Purpose**: Runtime environment for single blockchain
- **Metaphor**: Like planetary orbits, stable and predictable
- **Manages**: Single Pulsar engine instance

### 11. **Galaxy** - DAG Runtime (Galactic Scale)
- **Location**: `/runtimes/galaxy/`
- **Purpose**: Runtime environment for DAG structures
- **Metaphor**: Like galaxies, contains many parallel chains
- **Manages**: Single Nebula engine instance

### 12. **Gravity** - Universal Consensus Coordination (Universal Scale)
- **Location**: `/runtimes/gravity/`
- **Purpose**: Coordinates multiple consensus systems
- **Metaphor**: Like gravity, binds all consensus systems together
- **Features**:
  - Cross-runtime transaction atomicity
  - Multi-chain coordination
  - Universal state management
  - Consensus bridges between systems

## Practical Example Flow

### Linear Chain Transaction (using Orbit runtime):
```
1. Transaction submitted to Orbit runtime
2. Photon: Samples network validators
3. Wave: Propagates vote across network
4. Focus: Aggregates confidence scores
5. Beam: Finalizes block in chain
6. Orbit: Updates chain state
```

### DAG Transaction (using Galaxy runtime):
```
1. Vertex submitted to Galaxy runtime
2. Photon: Samples network validators
3. Wave: Propagates vertex preferences
4. Focus: Aggregates vertex confidence
5. Flare: Orders vertex among conflicts
6. Nova: Finalizes DAG structure
7. Galaxy: Updates DAG state
```

### Cross-Runtime Transaction (using Gravity runtime):
```
1. Cross-chain tx submitted to Gravity
2. Gravity: Validates source and destination
3. Quasar: Provides unified consensus
4. Bridge: Atomically moves assets
5. Both runtimes: Update states
6. Gravity: Confirms universal state
```

## Design Principles

1. **Scale Hierarchy**: Each level represents increasing scale and complexity
2. **Composability**: Smaller components combine to create larger systems
3. **Clarity**: Cosmic metaphors make architecture intuitive
4. **Modularity**: Each component has clear boundaries and responsibilities
5. **Pedagogical**: Easy to understand and teach to newcomers

## Implementation Notes

- All packages are at the root level for easy imports
- Engines combine multiple consensus stages
- Runtimes manage engines and provide APIs
- The cosmic scale provides natural organization
- Post-quantum security is built-in via Ringtail integration

This architecture enables developers to understand the system at any scale, from individual votes (photons) to universal consensus coordination (gravity).