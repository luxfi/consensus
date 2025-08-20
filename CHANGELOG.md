# Changelog

All notable changes to the Lux Consensus project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [2.0.0] - 2024-12-20

### Changed - BREAKING
- **Major Refactoring**: Complete replacement of `Sampler/Sample` pattern with `Emitter/Emit`
  - All packages now use the photon-based light emission metaphor
  - Renamed `prism.Sampler` to `photon.Emitter` throughout codebase
  - Changed method `Sample()` to `Emit()` for K-of-N selection
  - Moved K-of-N logic from `prism/` to new `photon/` package
  - `prism/` now purely handles DAG geometry (cuts, frontiers, refraction)

### Added
- **Photon Package**: New K-of-N committee selection with light theme
  - `photon.Emitter` interface for weighted peer selection
  - Luminance tracking system using "lux" units (10-1000 range)
  - Performance-based node weighting
  - Brighter nodes (higher lux) have increased selection probability
  
- **QZMQ Transport**: Post-quantum secure messaging layer
  - Hybrid classical-quantum key exchange
  - AES-256-GCM encryption with automatic fallback
  - Automatic key rotation based on usage thresholds
  - 1-RTT handshake for ultra-low latency

- **Performance Improvements**:
  - Support for 1ms block times on 100Gbps networks (X-Chain)
  - Sub-5ms finality for local networks
  - Zero-allocation luminance updates (72ns per operation)
  - Optimized wave consensus (3.38μs per vote round)

### Fixed
- All golangci-lint errors resolved
- Improved error handling with proper checking
- Fixed integer overflow issues in message serialization
- Corrected ineffectual assignments in tests

### Security
- Post-quantum certificates using ML-KEM-768/1024 and ML-DSA-44/65
- Dual certificate system (BLS + post-quantum) for quantum resistance
- Ringtail engine integration for quantum-resistant signatures

### Testing
- Maintained 96%+ test coverage across critical packages
- Added comprehensive benchmarks for all consensus components
- Full CI/CD pipeline with multi-platform support

### Documentation
- Updated README with architecture overview and performance metrics
- Created LLM.md for AI assistant guidance
- Added quick start guide and usage examples
- Documented luminance tracking and photon emission patterns

## [1.0.0] - 2024-11-01

### Added
- Initial implementation of Lux Quasar consensus engine
- Wave consensus mechanism with threshold voting
- DAG structure support with Flare certificates
- Post-quantum security foundations
- Multi-chain support (Q-Chain, C-Chain, X-Chain)
- Leaderless, fully decentralized architecture

---

*For more details on the photon/emitter refactoring, see [LLM.md](./LLM.md)*