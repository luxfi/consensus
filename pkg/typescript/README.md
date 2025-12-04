# @luxfi/consensus

TypeScript/Node.js bindings for the Lux Consensus SDK.

Native performance through NAPI-RS with full TypeScript type safety.

## Features

- **Native Performance**: Rust-based consensus engine compiled to native Node.js addon
- **Type Safety**: Full TypeScript definitions for all APIs
- **69% Byzantine Tolerance**: Industry-leading fault tolerance (2% above standard 67%)
- **Sub-second Finality**: ~500ms block time with 2-round finality
- **Quantum-Ready**: Optional ML-DSA (FIPS 204) post-quantum signatures
- **GPU Acceleration**: Optional MLX (Apple Silicon) and CUDA support

## Installation

```bash
npm install @luxfi/consensus
# or
pnpm add @luxfi/consensus
# or
yarn add @luxfi/consensus
```

## Quick Start

```typescript
import {
  ConsensusEngine,
  testnetConfig,
  VoteType,
  generateBlockId,
} from '@luxfi/consensus';

// Create engine with testnet configuration
const engine = new ConsensusEngine(testnetConfig());
engine.start();

// Create and add a block
const blockId = generateBlockId();
engine.addBlock({
  id: blockId,
  parentId: '00'.repeat(32), // Genesis parent
  height: 1,
  payload: Buffer.from('Hello, Lux!').toString('hex'),
  timestamp: Date.now(),
});

// Record votes (need alpha votes for acceptance)
const config = testnetConfig();
for (let i = 0; i < config.alpha; i++) {
  engine.recordVote({
    blockId: blockId,
    voteType: VoteType.Preference,
    voter: i.toString(16).padStart(64, '0'),
  });
}

// Check if accepted
console.log('Block accepted:', engine.isAccepted(blockId)); // true

engine.stop();
```

## Configuration

### Default Configuration

```typescript
import { defaultConfig } from '@luxfi/consensus';

const config = defaultConfig();
// {
//   alpha: 20,           // 69% quorum threshold
//   k: 20,               // Sample size
//   maxOutstanding: 10,  // Max concurrent polls
//   maxPollDelayMs: 1000,
//   networkTimeoutMs: 5000,
//   maxMessageSize: 2097152, // 2MB
//   securityLevel: 5,    // NIST Level 5
//   quantumResistant: true,
//   gpuAcceleration: true,
// }
```

### Testnet Configuration

Lower thresholds for development and testing:

```typescript
import { testnetConfig } from '@luxfi/consensus';

const config = testnetConfig();
// alpha: 5, k: 5, quantumResistant: false, gpuAcceleration: false
```

### Mainnet Configuration

Production settings with full security:

```typescript
import { mainnetConfig } from '@luxfi/consensus';

const config = mainnetConfig();
// alpha: 20, k: 21, quantumResistant: true, gpuAcceleration: true
```

## API Reference

### ConsensusEngine

The main consensus engine class.

```typescript
class ConsensusEngine {
  constructor(config?: ConsensusConfig);

  start(): void;
  stop(): void;

  addBlock(block: Block): void;
  recordVote(vote: Vote): void;
  recordVotesBatch(votes: Vote[]): number;

  isAccepted(blockId: string): boolean;
  getStatus(blockId: string): BlockStatus;
}
```

### Types

```typescript
interface Block {
  id: string; // 32-byte hex
  parentId: string; // 32-byte hex
  height: number;
  payload: string; // hex-encoded
  timestamp: number; // Unix ms
}

interface Vote {
  blockId: string;
  voteType: VoteType;
  voter: string; // 32-byte hex
  signature?: string;
}

enum BlockStatus {
  Unknown,
  Processing,
  Rejected,
  Accepted,
}

enum VoteType {
  Preference,
  Commit,
  Cancel,
}
```

## Platform Support

| Platform         | Architecture | Status |
| ---------------- | ------------ | ------ |
| macOS            | arm64        | ✅     |
| macOS            | x64          | ✅     |
| Linux            | arm64 (glibc)| ✅     |
| Linux            | x64 (glibc)  | ✅     |
| Linux            | arm64 (musl) | ✅     |
| Linux            | x64 (musl)   | ✅     |
| Windows          | x64          | ✅     |

## Building from Source

```bash
# Install dependencies
pnpm install

# Build release
pnpm build

# Build debug
pnpm build:debug

# Run tests
pnpm test
```

## Performance

Benchmarks on Apple M2 Pro:

| Operation      | Throughput      |
| -------------- | --------------- |
| Add block      | 1.2M blocks/sec |
| Record vote    | 850K votes/sec  |
| Batch votes    | 1.5M votes/sec  |

## Related Packages

- [`lux-consensus`](../rust) - Rust SDK
- [`lux-consensus-py`](../python) - Python SDK
- [`libluxconsensus`](../c) - C SDK

## License

MIT License - See [LICENSE](../../LICENSE) for details.

## Contributing

Contributions are welcome! Please see our [Contributing Guide](../../CONTRIBUTING.md).
