# lux-consensus

Pure-Rust SDK for the Lux **Quasar** consensus stack — Wave threshold voting,
FPC (Fast Probabilistic Consensus), Photon validator sampling, Focus
confidence accumulation, and post-quantum hybrid finality via BLS12-381 +
Pulsar (Module-LWE threshold, FIPS 204 byte-equal).

## Features

| Flag      | What it pulls in                                                |
|-----------|-----------------------------------------------------------------|
| *default* | CPU-only, no extra deps beyond `blst` and `hex`                 |
| `simd`    | `blake3` (SIMD hashing) + `rayon` (data-parallel vote tally)    |
| `mlx`     | Apple Silicon GPU acceleration via `mlx-sys` (implies `simd`)   |
| `cuda`    | NVIDIA GPU acceleration (implies `simd`) — work in progress     |
| `gpu`     | `mlx` + `cuda` — auto-select per platform                       |

## Usage

```rust
use lux_consensus::*;

let config = QuasarConfig::mainnet();
let mut engine = QuasarEngine::new(config);
engine.start().unwrap();

let block = Block::new(
    ID::from([1u8; 32]),
    ID::from([0u8; 32]),
    1,
    b"Hello, Lux!".to_vec(),
);
engine.add(block.clone()).unwrap();

for i in 0..20 {
    let vote = Vote::new(block.id.clone(), VoteType::Preference, NodeID::from([i; 32]));
    engine.record_vote(vote).unwrap();
}

assert!(engine.is_accepted(&block.id));
engine.stop().unwrap();
```

## Architecture

- **Wave** — adaptive-threshold voting driven by FPC outputs
- **FPC** — PRF-derived per-round thresholds for fast probabilistic consensus
- **Photon** — luminance-tracked validator sampling
- **Focus** — β consecutive rounds of confidence before commit
- **Quasar** — post-quantum hybrid signature aggregation

## License

Lux Ecosystem License 1.2 — see the `LICENSE` file.
