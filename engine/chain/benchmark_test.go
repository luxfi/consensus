// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/luxfi/consensus/engine/chain/chaintest"
	"github.com/luxfi/ids"
)

// BenchmarkEngine encapsulates the chain engine with additional tracking for benchmarks
type BenchmarkEngine struct {
	*Transitive
	blocks         map[ids.ID]*chaintest.TestBlock
	blocksByHeight map[uint64][]*chaintest.TestBlock
	votes          map[ids.ID]map[ids.NodeID]bool
	finalized      map[ids.ID]bool
	preferred      ids.ID
	mu             sync.RWMutex
}

// NewBenchmarkEngine creates a new benchmark engine
func NewBenchmarkEngine() *BenchmarkEngine {
	return &BenchmarkEngine{
		Transitive:     New(),
		blocks:         make(map[ids.ID]*chaintest.TestBlock),
		blocksByHeight: make(map[uint64][]*chaintest.TestBlock),
		votes:          make(map[ids.ID]map[ids.NodeID]bool),
		finalized:      make(map[ids.ID]bool),
		preferred:      chaintest.Genesis.ID(),
	}
}

// AddBlock adds a block to the engine
func (e *BenchmarkEngine) AddBlock(block *chaintest.TestBlock) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.blocks[block.ID()] = block
	e.blocksByHeight[block.Height()] = append(e.blocksByHeight[block.Height()], block)

	// Initialize vote tracking for this block
	if e.votes[block.ID()] == nil {
		e.votes[block.ID()] = make(map[ids.NodeID]bool)
	}

	return nil
}

// AddVote adds a vote for a block
func (e *BenchmarkEngine) AddVote(blockID ids.ID, voterID ids.NodeID) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.votes[blockID] == nil {
		e.votes[blockID] = make(map[ids.NodeID]bool)
	}
	e.votes[blockID][voterID] = true

	// Check finalization (simplified: 2/3 of 100 validators)
	if len(e.votes[blockID]) >= 67 {
		e.finalized[blockID] = true
	}

	return nil
}

// ProcessReorg processes a chain reorganization
func (e *BenchmarkEngine) ProcessReorg(newChainTip *chaintest.TestBlock) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find common ancestor
	currentHeight := newChainTip.Height()
	for currentHeight > 0 {
		if blocks, exists := e.blocksByHeight[currentHeight]; exists {
			for _, b := range blocks {
				if e.finalized[b.ID()] {
					// Found finalized ancestor, reorg from here
					e.preferred = newChainTip.ID()
					return nil
				}
			}
		}
		currentHeight--
	}

	e.preferred = newChainTip.ID()
	return nil
}

// CheckFinalization checks if a block is finalized
func (e *BenchmarkEngine) CheckFinalization(blockID ids.ID) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return e.finalized[blockID]
}

// Reset resets the engine state for benchmarking
func (e *BenchmarkEngine) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.blocks = make(map[ids.ID]*chaintest.TestBlock)
	e.blocksByHeight = make(map[uint64][]*chaintest.TestBlock)
	e.votes = make(map[ids.ID]map[ids.NodeID]bool)
	e.finalized = make(map[ids.ID]bool)
	e.preferred = chaintest.Genesis.ID()
	e.bootstrapped = false
}

// Benchmark single block addition
func BenchmarkAddSingleBlock(b *testing.B) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	parent := chaintest.Genesis

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		block := chaintest.BuildChild(parent)
		_ = engine.AddBlock(block)
		parent = block
	}
}

// Benchmark batch block additions
func BenchmarkAddBlockBatch100(b *testing.B) {
	benchmarkAddBlockBatch(b, 100)
}

func BenchmarkAddBlockBatch1000(b *testing.B) {
	benchmarkAddBlockBatch(b, 1000)
}

func BenchmarkAddBlockBatch10000(b *testing.B) {
	benchmarkAddBlockBatch(b, 10000)
}

func benchmarkAddBlockBatch(b *testing.B, batchSize int) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	// Pre-generate blocks
	blocks := make([]*chaintest.TestBlock, batchSize)
	parent := chaintest.Genesis
	for i := 0; i < batchSize; i++ {
		blocks[i] = chaintest.BuildChild(parent)
		parent = blocks[i]
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine.Reset()
		_ = engine.Start(ctx, 1)

		for _, block := range blocks {
			_ = engine.AddBlock(block)
		}
	}
}

// Benchmark single vote processing
func BenchmarkProcessSingleVote(b *testing.B) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	block := chaintest.BuildChild(chaintest.Genesis)
	_ = engine.AddBlock(block)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		voterID := ids.GenerateTestNodeID()
		_ = engine.AddVote(block.ID(), voterID)
	}
}

// Benchmark batch vote processing
func BenchmarkProcessVoteBatch100(b *testing.B) {
	benchmarkProcessVoteBatch(b, 100)
}

func BenchmarkProcessVoteBatch1000(b *testing.B) {
	benchmarkProcessVoteBatch(b, 1000)
}

func benchmarkProcessVoteBatch(b *testing.B, voteCount int) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	// Create multiple blocks
	blocks := make([]*chaintest.TestBlock, 10)
	parent := chaintest.Genesis
	for i := 0; i < 10; i++ {
		blocks[i] = chaintest.BuildChild(parent)
		_ = engine.AddBlock(blocks[i])
		parent = blocks[i]
	}

	// Pre-generate voters
	voters := make([]ids.NodeID, voteCount)
	for i := 0; i < voteCount; i++ {
		voters[i] = ids.GenerateTestNodeID()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Reset votes
		engine.votes = make(map[ids.ID]map[ids.NodeID]bool)
		engine.finalized = make(map[ids.ID]bool)

		// Process all votes
		for _, voter := range voters {
			for _, block := range blocks {
				_ = engine.AddVote(block.ID(), voter)
			}
		}
	}
}

// Benchmark chain reorganization
func BenchmarkChainReorgShallow(b *testing.B) {
	benchmarkChainReorg(b, 10, 5) // 10 blocks, reorg at height 5
}

func BenchmarkChainReorgDeep(b *testing.B) {
	benchmarkChainReorg(b, 100, 50) // 100 blocks, reorg at height 50
}

func BenchmarkChainReorgVeryDeep(b *testing.B) {
	benchmarkChainReorg(b, 1000, 500) // 1000 blocks, reorg at height 500
}

func benchmarkChainReorg(b *testing.B, chainLength int, reorgDepth int) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	// Build main chain
	mainChain := make([]*chaintest.TestBlock, chainLength)
	parent := chaintest.Genesis
	for i := 0; i < chainLength; i++ {
		mainChain[i] = chaintest.BuildChild(parent)
		_ = engine.AddBlock(mainChain[i])

		// Mark some blocks as finalized
		if i < reorgDepth {
			engine.finalized[mainChain[i].ID()] = true
		}
		parent = mainChain[i]
	}

	// Build alternative chain from reorg point
	altParent := mainChain[reorgDepth-1]
	altChain := make([]*chaintest.TestBlock, chainLength-reorgDepth)
	for i := 0; i < len(altChain); i++ {
		altChain[i] = chaintest.BuildChild(altParent)
		altParent = altChain[i]
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Add alternative chain blocks
		for _, block := range altChain {
			_ = engine.AddBlock(block)
		}

		// Process reorganization
		_ = engine.ProcessReorg(altChain[len(altChain)-1])
	}
}

// Benchmark finalization checking
func BenchmarkFinalizationCheck(b *testing.B) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	// Create blocks with varying finalization status
	blocks := make([]*chaintest.TestBlock, 1000)
	parent := chaintest.Genesis
	for i := 0; i < 1000; i++ {
		blocks[i] = chaintest.BuildChild(parent)
		_ = engine.AddBlock(blocks[i])

		// Finalize every 3rd block
		if i%3 == 0 {
			engine.finalized[blocks[i].ID()] = true
		}
		parent = blocks[i]
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Check finalization status of random blocks
		blockIdx := i % len(blocks)
		_ = engine.CheckFinalization(blocks[blockIdx].ID())
	}
}

// Benchmark concurrent block operations
func BenchmarkConcurrentBlockOperations(b *testing.B) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	// Number of concurrent goroutines
	concurrency := 10
	blocksPerRoutine := 100

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		var wg sync.WaitGroup
		wg.Add(concurrency)

		for j := 0; j < concurrency; j++ {
			go func(routineID int) {
				defer wg.Done()

				parent := chaintest.Genesis
				for k := 0; k < blocksPerRoutine; k++ {
					block := chaintest.BuildChild(parent)
					_ = engine.AddBlock(block)

					// Add some votes
					for v := 0; v < 5; v++ {
						voterID := ids.GenerateTestNodeID()
						_ = engine.AddVote(block.ID(), voterID)
					}

					parent = block
				}
			}(j)
		}

		wg.Wait()
		engine.Reset()
		_ = engine.Start(ctx, 1)
	}
}

// Benchmark mixed operations (realistic workload)
func BenchmarkMixedOperations(b *testing.B) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	// Pre-generate data
	blocks := make([]*chaintest.TestBlock, 100)
	parent := chaintest.Genesis
	for i := 0; i < 100; i++ {
		blocks[i] = chaintest.BuildChild(parent)
		parent = blocks[i]
	}

	voters := make([]ids.NodeID, 50)
	for i := 0; i < 50; i++ {
		voters[i] = ids.GenerateTestNodeID()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine.Reset()
		_ = engine.Start(ctx, 1)

		// Simulate realistic consensus workflow
		for j, block := range blocks {
			// Add block
			_ = engine.AddBlock(block)

			// Process votes (varying number)
			voteCount := 30 + (j % 40) // 30-70 votes
			for v := 0; v < voteCount && v < len(voters); v++ {
				_ = engine.AddVote(block.ID(), voters[v])
			}

			// Check finalization periodically
			if j%10 == 0 {
				_ = engine.CheckFinalization(block.ID())
			}

			// Occasional reorg (every 20 blocks)
			if j%20 == 0 && j > 0 {
				_ = engine.ProcessReorg(blocks[j])
			}
		}
	}
}

// Benchmark engine lifecycle (start/stop)
func BenchmarkEngineStartStop(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine := NewBenchmarkEngine()
		_ = engine.Start(ctx, uint32(i))

		// Add some blocks to make it realistic
		parent := chaintest.Genesis
		for j := 0; j < 10; j++ {
			block := chaintest.BuildChild(parent)
			_ = engine.AddBlock(block)
			parent = block
		}

		_ = engine.Stop(ctx)
	}
}

// Benchmark health checks
func BenchmarkHealthCheck(b *testing.B) {
	engine := NewBenchmarkEngine()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	// Add some state
	parent := chaintest.Genesis
	for i := 0; i < 100; i++ {
		block := chaintest.BuildChild(parent)
		_ = engine.AddBlock(block)
		parent = block
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = engine.HealthCheck(ctx)
	}
}

// Benchmark block verification (simulated cryptographic operations)
func BenchmarkBlockVerification(b *testing.B) {
	ctx := context.Background()

	// Pre-generate blocks with varying complexity
	blocks := make([]*chaintest.TestBlock, 1000)
	parent := chaintest.Genesis
	for i := 0; i < 1000; i++ {
		blocks[i] = chaintest.BuildChild(parent)
		parent = blocks[i]
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		block := blocks[i%len(blocks)]
		_ = block.Verify(ctx)
	}
}

// Benchmark block acceptance
func BenchmarkBlockAcceptance(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		block := chaintest.BuildChild(chaintest.Genesis)
		_ = block.Accept(ctx)
	}
}

// Benchmark block rejection
func BenchmarkBlockRejection(b *testing.B) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		block := chaintest.BuildChild(chaintest.Genesis)
		_ = block.Reject(ctx)
	}
}

// Benchmark memory usage with large chains
func BenchmarkMemoryUsageLargeChain(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine := NewBenchmarkEngine()
		ctx := context.Background()
		_ = engine.Start(ctx, 1)

		// Build a large chain
		parent := chaintest.Genesis
		for j := 0; j < 10000; j++ {
			block := chaintest.BuildChild(parent)
			_ = engine.AddBlock(block)

			// Add some votes to increase memory pressure
			if j%100 == 0 {
				for v := 0; v < 50; v++ {
					voterID := ids.GenerateTestNodeID()
					_ = engine.AddVote(block.ID(), voterID)
				}
			}

			parent = block
		}
	}
}

// Helper function to create a benchmark table
func BenchmarkBlockAdditionTable(b *testing.B) {
	sizes := []int{1, 10, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size-%d", size), func(b *testing.B) {
			benchmarkAddBlockBatch(b, size)
		})
	}
}

// Benchmark GetBlock operation
func BenchmarkGetBlock(b *testing.B) {
	engine := New()
	ctx := context.Background()
	_ = engine.Start(ctx, 1)

	// Pre-generate block IDs
	blockIDs := make([]ids.ID, 1000)
	nodeIDs := make([]ids.NodeID, 100)
	for i := 0; i < 1000; i++ {
		blockIDs[i] = ids.GenerateTestID()
	}
	for i := 0; i < 100; i++ {
		nodeIDs[i] = ids.GenerateTestNodeID()
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		blockID := blockIDs[i%len(blockIDs)]
		nodeID := nodeIDs[i%len(nodeIDs)]
		_ = engine.GetBlock(ctx, nodeID, uint32(i), blockID)
	}
}

// Benchmark bootstrapping process
func BenchmarkBootstrapping(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		engine := New()
		ctx := context.Background()

		// Start triggers bootstrapping
		_ = engine.Start(ctx, uint32(i))

		// Verify bootstrapped
		if !engine.IsBootstrapped() {
			b.Error("Engine should be bootstrapped")
		}

		_ = engine.Stop(ctx)
	}
}
