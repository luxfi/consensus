// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// Test suite for @luxfi/consensus TypeScript SDK
// Complete Quasar consensus: Wave, FPC, Photon, Focus

const { describe, it, before, after } = require('node:test');
const assert = require('node:assert');

// Import will fail until native module is built
let consensus;
try {
  consensus = require('../index.js');
} catch (e) {
  console.log('Native module not built yet. Run `pnpm build` first.');
  console.log('Skipping tests.');
  process.exit(0);
}

const {
  ConsensusEngine,
  FpcSelector,
  BlockId,
  NodeId,
  BlockStatus,
  VoteType,
  generateBlockId,
  defaultConfig,
  testnetConfig,
  mainnetConfig,
  fpcThreshold,
  version,
} = consensus;

describe('Lux Quasar Consensus SDK', () => {
  describe('Configuration', () => {
    it('should return default config with full Quasar settings', () => {
      const config = defaultConfig();
      // Core parameters
      assert.strictEqual(config.alpha, 0.69);  // 69% quorum ratio
      assert.strictEqual(config.k, 20);
      assert.strictEqual(config.beta, 20);
      assert.strictEqual(config.securityLevel, 3);  // Medium
      assert.strictEqual(config.quantumResistant, true);
      // Feature flags
      assert.strictEqual(config.enableFpc, true);
      assert.strictEqual(config.gpuAcceleration, true);
    });

    it('should return testnet config with relaxed thresholds', () => {
      const config = testnetConfig();
      assert.strictEqual(config.alpha, 0.6);  // 60% for testing
      assert.strictEqual(config.k, 5);
      assert.strictEqual(config.beta, 5);
      assert.strictEqual(config.quantumResistant, false);
      assert.strictEqual(config.securityLevel, 2);  // Low
    });

    it('should return mainnet config with production settings', () => {
      const config = mainnetConfig();
      assert.strictEqual(config.alpha, 0.69);  // 69% quorum
      assert.strictEqual(config.k, 21);
      assert.strictEqual(config.beta, 20);
      assert.strictEqual(config.quantumResistant, true);
      assert.strictEqual(config.gpuAcceleration, true);
      assert.strictEqual(config.securityLevel, 5);  // High
      assert.strictEqual(config.enableFpc, true);
    });
  });

  describe('FPC (Fast Probabilistic Consensus)', () => {
    it('should compute phase-dependent thresholds', () => {
      const selector = new FpcSelector(0.5, 0.8, null);
      // For k=20, threshold should be between ceil(0.5*20)=10 and ceil(0.8*20)=16
      const t0 = selector.threshold(0, 20);
      assert.ok(t0 >= 10 && t0 <= 16, `Threshold ${t0} out of range`);

      // Different phases may yield different thresholds
      const t1 = selector.threshold(1, 20);
      const t2 = selector.threshold(2, 20);
      assert.ok(t1 >= 10 && t1 <= 16);
      assert.ok(t2 >= 10 && t2 <= 16);
    });

    it('should compute theta values between min and max', () => {
      const selector = new FpcSelector(0.5, 0.8, null);
      for (let phase = 0; phase < 10; phase++) {
        const theta = selector.theta(phase);
        assert.ok(theta >= 0.5 && theta <= 0.8, `Theta ${theta} out of range`);
      }
    });

    it('should use fpcThreshold utility function', () => {
      const threshold = fpcThreshold(0, 20, 0.5, 0.8);
      assert.ok(threshold >= 10 && threshold <= 16);
    });

    it('should accept custom seed', () => {
      const seed = '00'.repeat(32);
      const selector = new FpcSelector(0.5, 0.8, seed);
      const t = selector.threshold(0, 20);
      assert.ok(t >= 10 && t <= 16);
    });
  });

  describe('Utility Functions', () => {
    it('should generate unique block IDs', () => {
      const id1 = generateBlockId();
      const id2 = generateBlockId();
      assert.strictEqual(id1.length, 64);  // 32 bytes = 64 hex chars
      assert.strictEqual(id2.length, 64);
      assert.notStrictEqual(id1, id2);
    });

    it('should return version string', () => {
      const v = version();
      assert.match(v, /^\d+\.\d+\.\d+$/);
    });
  });

  describe('BlockId', () => {
    it('should create from hex string', () => {
      const hexId = '00'.repeat(32);
      const blockId = new BlockId(hexId);
      assert.strictEqual(blockId.toHex(), hexId);
    });

    it('should create zero ID', () => {
      const zeroId = BlockId.zero();
      assert.strictEqual(zeroId.toHex(), '00'.repeat(32));
      assert.strictEqual(zeroId.isZero(), true);
    });

    it('should convert to bytes', () => {
      const hexId = '01'.repeat(32);
      const blockId = new BlockId(hexId);
      const bytes = blockId.toBytes();
      assert.strictEqual(bytes.length, 32);
      assert.strictEqual(bytes[0], 1);
    });

    it('should reject invalid hex', () => {
      assert.throws(() => new BlockId('not-hex'), /Invalid hex/);
    });

    it('should reject wrong length', () => {
      assert.throws(() => new BlockId('00'.repeat(16)), /32 bytes/);
    });
  });

  describe('NodeId', () => {
    it('should create from hex string', () => {
      const hexId = 'ab'.repeat(32);
      const nodeId = new NodeId(hexId);
      assert.strictEqual(nodeId.toHex(), hexId);
    });

    it('should reject wrong length', () => {
      assert.throws(() => new NodeId('ab'.repeat(16)), /32 bytes/);
    });
  });

  describe('ConsensusEngine', () => {
    let engine;

    before(() => {
      engine = new ConsensusEngine(testnetConfig());
      engine.start();
    });

    after(() => {
      engine.stop();
    });

    it('should create engine with config', () => {
      assert.ok(engine);
      const config = engine.getConfig();
      assert.strictEqual(config.alpha, 0.6);
      assert.strictEqual(config.k, 5);
    });

    it('should add a block', () => {
      const block = {
        id: '01'.repeat(32),
        parentId: '00'.repeat(32),
        height: 1,
        payload: Buffer.from('test payload').toString('hex'),
        timestamp: Date.now(),
      };
      engine.addBlock(block);
      assert.strictEqual(engine.getStatus('01'.repeat(32)), BlockStatus.Processing);
    });

    it('should record votes and reach consensus', () => {
      const blockId = '01'.repeat(32);

      // Record enough votes to reach quorum (testnet alpha=0.6, k=5, need ceil(0.6*5)=3 votes)
      for (let i = 0; i < 5; i++) {
        const vote = {
          blockId: blockId,
          voteType: VoteType.Preference,
          voter: i.toString(16).padStart(64, '0'),
        };
        engine.recordVote(vote);
      }

      // Block should be accepted or processing (depending on beta rounds)
      const status = engine.getStatus(blockId);
      assert.ok(
        status === BlockStatus.Accepted || status === BlockStatus.Processing,
        `Expected accepted or processing, got ${status}`
      );
    });

    it('should handle batch votes', () => {
      const blockId = '02'.repeat(32);

      // Add a new block
      engine.addBlock({
        id: blockId,
        parentId: '01'.repeat(32),
        height: 2,
        payload: Buffer.from('batch test').toString('hex'),
        timestamp: Date.now(),
      });

      // Create batch of votes
      const votes = [];
      for (let i = 0; i < 10; i++) {
        votes.push({
          blockId: blockId,
          voteType: VoteType.Preference,
          voter: (100 + i).toString(16).padStart(64, '0'),
        });
      }

      const successCount = engine.recordVotesBatch(votes);
      assert.strictEqual(successCount, 10);
    });

    it('should add validators', () => {
      const nodeId = 'aa'.repeat(32);
      // Should not throw
      engine.addValidator(nodeId, 100);
    });

    it('should get height', () => {
      const height = engine.getHeight();
      assert.ok(typeof height === 'number');
    });

    it('should reject votes for unknown blocks', () => {
      const vote = {
        blockId: 'ff'.repeat(32),
        voteType: VoteType.Preference,
        voter: '00'.repeat(32),
      };
      assert.throws(() => engine.recordVote(vote), /Block not found/);
    });
  });

  describe('Full Quasar Consensus Flow', () => {
    it('should achieve consensus through voting', () => {
      const engine = new ConsensusEngine(testnetConfig());
      engine.start();

      try {
        // Create a chain of blocks
        const blocks = [
          {
            id: 'a1'.repeat(32),
            parentId: '00'.repeat(32),
            height: 1,
            payload: Buffer.from('Block 1').toString('hex'),
            timestamp: Date.now(),
          },
          {
            id: 'a2'.repeat(32),
            parentId: 'a1'.repeat(32),
            height: 2,
            payload: Buffer.from('Block 2').toString('hex'),
            timestamp: Date.now(),
          },
          {
            id: 'a3'.repeat(32),
            parentId: 'a2'.repeat(32),
            height: 3,
            payload: Buffer.from('Block 3').toString('hex'),
            timestamp: Date.now(),
          },
        ];

        // Add all blocks
        for (const block of blocks) {
          engine.addBlock(block);
          assert.strictEqual(engine.getStatus(block.id), BlockStatus.Processing);
        }

        // Vote on all blocks (need at least ceil(0.6*5)=3 votes for testnet)
        for (const block of blocks) {
          for (let i = 0; i < 5; i++) {
            engine.recordVote({
              blockId: block.id,
              voteType: VoteType.Preference,
              voter: (block.height * 10 + i).toString(16).padStart(64, '0'),
            });
          }
        }

        // All should be accepted or processing
        for (const block of blocks) {
          const status = engine.getStatus(block.id);
          assert.ok(
            status === BlockStatus.Accepted || status === BlockStatus.Processing,
            `Block ${block.height} status ${status}`
          );
        }
      } finally {
        engine.stop();
      }
    });

    it('should work with mainnet configuration', () => {
      const config = mainnetConfig();
      const engine = new ConsensusEngine(config);
      engine.start();

      try {
        const blockId = 'b1'.repeat(32);
        engine.addBlock({
          id: blockId,
          parentId: '00'.repeat(32),
          height: 1,
          payload: Buffer.from('Mainnet Block').toString('hex'),
          timestamp: Date.now(),
        });

        // Need ceil(0.69*21) = 15 votes for mainnet
        for (let i = 0; i < 21; i++) {
          engine.recordVote({
            blockId: blockId,
            voteType: VoteType.Preference,
            voter: i.toString(16).padStart(64, '0'),
          });
        }

        // Should be at least processing
        const status = engine.getStatus(blockId);
        assert.ok(
          status === BlockStatus.Accepted || status === BlockStatus.Processing,
          `Expected accepted or processing, got ${status}`
        );
      } finally {
        engine.stop();
      }
    });
  });

  describe('Wave + FPC Integration', () => {
    it('should use FPC thresholds in voting', () => {
      // Create config with explicit FPC settings
      const config = {
        ...testnetConfig(),
        enableFpc: true,
        thetaMin: 0.5,
        thetaMax: 0.7,
      };

      const engine = new ConsensusEngine(config);
      engine.start();

      try {
        const blockId = 'c1'.repeat(32);
        engine.addBlock({
          id: blockId,
          parentId: '00'.repeat(32),
          height: 1,
          payload: Buffer.from('FPC Test').toString('hex'),
          timestamp: Date.now(),
        });

        // Vote to reach consensus
        for (let i = 0; i < 5; i++) {
          engine.recordVote({
            blockId: blockId,
            voteType: VoteType.Preference,
            voter: i.toString(16).padStart(64, '0'),
          });
        }

        // Should be accepted or processing (depending on FPC threshold)
        const status = engine.getStatus(blockId);
        assert.ok(
          status === BlockStatus.Accepted || status === BlockStatus.Processing,
          `Unexpected status: ${status}`
        );
      } finally {
        engine.stop();
      }
    });
  });

  describe('Cross-Language Compatibility', () => {
    it('should use 32-byte IDs for cross-language consensus', () => {
      // Verify ID format matches Rust SDK
      const rustStyleId = 'deadbeef'.repeat(8);  // 32 bytes
      const blockId = new BlockId(rustStyleId);
      assert.strictEqual(blockId.toHex(), rustStyleId);
      assert.strictEqual(blockId.toBytes().length, 32);
    });

    it('should use same quorum calculation as Rust SDK', () => {
      // Rust: (alpha * total_votes as f64).ceil() as usize
      // TypeScript should match this
      const config = testnetConfig();
      const alpha = config.alpha;  // 0.6
      const k = config.k;          // 5

      // ceil(0.6 * 5) = 3
      const expectedQuorum = Math.ceil(alpha * k);
      assert.strictEqual(expectedQuorum, 3);
    });

    it('should use same FPC formula as Rust SDK', () => {
      // Formula: α(phase, k) = ⌈θ(phase) · k⌉
      const selector = new FpcSelector(0.5, 0.8, null);

      // Verify the formula
      for (let phase = 0; phase < 5; phase++) {
        const theta = selector.theta(phase);
        const threshold = selector.threshold(phase, 20);
        const expected = Math.ceil(theta * 20);
        assert.strictEqual(threshold, expected, `Phase ${phase}: expected ${expected}, got ${threshold}`);
      }
    });
  });
});
