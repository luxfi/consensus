// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build skip
// +build skip

package engine

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// BenchmarkEngineCreation benchmarks consensus engine creation
func BenchmarkEngineCreation(b *testing.B) {
	config := DefaultConfig()
	config.K = 20
	config.AlphaPreference = 15
	config.AlphaConfidence = 15
	config.Beta = 20
	config.ConcurrentPolls = 1
	config.OptimalProcessing = 1
	config.MaxOutstandingItems = 1024
	config.MaxItemProcessingTime = 2 * time.Second

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		engine := NewConsensus(config)
		_ = engine
	}
}

// BenchmarkSingleBlockAdd benchmarks adding single blocks
func BenchmarkSingleBlockAdd(b *testing.B) {
	config := DefaultConfig()
	config.MaxOutstandingItems = 100000
	engine := NewConsensus(config)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockID := makeID(byte(i>>8), byte(i&0xFF))
		block := &TestBlock{
			id:       blockID,
			parentID: GenesisID,
			height:   uint64(i),
		}
		err := engine.Add(block)
		require.NoError(b, err)
	}
}

// BenchmarkBatchBlockOperations benchmarks batch block operations
func BenchmarkBatchBlockOperations(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Batch_%d", size), func(b *testing.B) {
			b.ResetTimer()

			for n := 0; n < b.N; n++ {
				config := DefaultConfig()
				config.MaxOutstandingItems = 100000
				engine := NewConsensus(config)

				for i := 0; i < size; i++ {
					blockID := makeID(byte(i>>16), byte(i>>8), byte(i&0xFF))
					block := &TestBlock{
						id:       blockID,
						parentID: GenesisID,
						height:   uint64(i),
					}
					err := engine.Add(block)
					require.NoError(b, err)
				}
			}
		})
	}
}

// BenchmarkSingleVoteProcessing benchmarks processing single votes
func BenchmarkSingleVoteProcessing(b *testing.B) {
	config := DefaultConfig()
	engine := NewConsensus(config)

	// Add blocks to vote on
	for i := 0; i < 100; i++ {
		blockID := makeID(byte(i))
		block := &TestBlock{
			id:       blockID,
			parentID: GenesisID,
			height:   uint64(i),
		}
		err := engine.Add(block)
		require.NoError(b, err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		voterID := makeID(byte(i>>8), byte(i&0xFF))
		blockID := makeID(byte(i % 100))

		vote := Vote{
			VoterID:      voterID,
			BlockID:      blockID,
			IsPreference: i%2 == 0,
		}

		err := engine.ProcessVote(vote)
		require.NoError(b, err)
	}
}

// BenchmarkBatchVoteProcessing benchmarks batch vote processing
func BenchmarkBatchVoteProcessing(b *testing.B) {
	sizes := []int{1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Batch_%d", size), func(b *testing.B) {
			config := DefaultConfig()
			engine := NewConsensus(config)

			// Add blocks to vote on
			for i := 0; i < 100; i++ {
				blockID := makeID(byte(i))
				block := &TestBlock{
					id:       blockID,
					parentID: GenesisID,
					height:   uint64(i),
				}
				err := engine.Add(block)
				require.NoError(b, err)
			}

			b.ResetTimer()
			for n := 0; n < b.N; n++ {
				for i := 0; i < size; i++ {
					voterID := makeID(byte(i>>16), byte(i>>8), byte(i&0xFF))
					blockID := makeID(byte(i % 100))

					vote := Vote{
						VoterID:      voterID,
						BlockID:      blockID,
						IsPreference: i%2 == 0,
					}

					err := engine.ProcessVote(vote)
					require.NoError(b, err)
				}
			}
		})
	}
}

// BenchmarkQueryOperations benchmarks various query operations
func BenchmarkQueryOperations(b *testing.B) {
	config := DefaultConfig()
	engine := NewConsensus(config)

	// Add blocks
	blockIDs := make([]ID, 1000)
	for i := 0; i < 1000; i++ {
		blockID := makeID(byte(i>>8), byte(i&0xFF))
		blockIDs[i] = blockID
		block := &TestBlock{
			id:       blockID,
			parentID: GenesisID,
			height:   uint64(i),
		}
		err := engine.Add(block)
		require.NoError(b, err)
	}

	b.Run("IsAccepted", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			blockID := blockIDs[i%1000]
			_ = engine.IsAccepted(blockID)
		}
	})

	b.Run("GetPreference", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = engine.GetPreference()
		}
	})

	b.Run("GetStats", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = engine.GetStats()
		}
	})
}

// BenchmarkConcurrentOperations benchmarks concurrent operations
func BenchmarkConcurrentOperations(b *testing.B) {
	threadCounts := []int{1, 2, 4, 8}

	for _, numThreads := range threadCounts {
		b.Run(fmt.Sprintf("Threads_%d", numThreads), func(b *testing.B) {
			b.ResetTimer()

			for n := 0; n < b.N; n++ {
				config := DefaultConfig()
				config.MaxOutstandingItems = 10000
				engine := NewConsensus(config)

				operationsPerThread := 1000
				var wg sync.WaitGroup
				wg.Add(numThreads)

				for threadID := 0; threadID < numThreads; threadID++ {
					go func(tid int) {
						defer wg.Done()

						for i := 0; i < operationsPerThread; i++ {
							blockID := makeID(byte(tid), byte(i>>8), byte(i&0xFF))
							block := &TestBlock{
								id:       blockID,
								parentID: GenesisID,
								height:   uint64(i),
							}
							err := engine.Add(block)
							if err != nil {
								b.Errorf("Failed to add block: %v", err)
							}
						}
					}(threadID)
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkMemoryUsage benchmarks memory usage with large datasets
func BenchmarkMemoryUsage(b *testing.B) {
	sizes := []int{1000, 10000, 100000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Blocks_%d", size), func(b *testing.B) {
			b.ResetTimer()

			for n := 0; n < b.N; n++ {
				config := DefaultConfig()
				config.MaxOutstandingItems = 200000
				engine := NewConsensus(config)

				for i := 0; i < size; i++ {
					blockID := makeID(byte(i>>24), byte(i>>16), byte(i>>8), byte(i&0xFF))
					data := fmt.Sprintf("Block data %d", i)
					block := &TestBlock{
						id:       blockID,
						parentID: GenesisID,
						height:   uint64(i),
						data:     []byte(data),
					}
					err := engine.Add(block)
					require.NoError(b, err)
				}
			}
		})
	}
}

// BenchmarkPerformance benchmarks overall performance metrics
func BenchmarkPerformance(b *testing.B) {
	b.Run("Add1000Blocks", func(b *testing.B) {
		b.ResetTimer()

		for n := 0; n < b.N; n++ {
			config := DefaultConfig()
			config.MaxOutstandingItems = 10000
			engine := NewConsensus(config)

			start := time.Now()
			for i := 0; i < 1000; i++ {
				blockID := makeID(byte(i>>8), byte(i&0xFF))
				block := &TestBlock{
					id:       blockID,
					parentID: GenesisID,
					height:   uint64(i),
				}
				err := engine.Add(block)
				require.NoError(b, err)
			}
			elapsed := time.Since(start)

			if elapsed > time.Second {
				b.Errorf("Adding 1000 blocks took %v, expected < 1s", elapsed)
			}
		}
	})

	b.Run("Process10000Votes", func(b *testing.B) {
		b.ResetTimer()

		for n := 0; n < b.N; n++ {
			config := DefaultConfig()
			engine := NewConsensus(config)

			// Add blocks to vote on
			for i := 0; i < 1000; i++ {
				blockID := makeID(byte(i>>8), byte(i&0xFF))
				block := &TestBlock{
					id:       blockID,
					parentID: GenesisID,
					height:   uint64(i),
				}
				err := engine.Add(block)
				require.NoError(b, err)
			}

			start := time.Now()
			for i := 0; i < 10000; i++ {
				voterID := makeID(byte(i>>8), byte(i&0xFF))
				blockIndex := i % 1000
				blockID := makeID(byte(blockIndex>>8), byte(blockIndex&0xFF))

				vote := Vote{
					VoterID:      voterID,
					BlockID:      blockID,
					IsPreference: i%2 == 0,
				}

				err := engine.ProcessVote(vote)
				require.NoError(b, err)
			}
			elapsed := time.Since(start)

			if elapsed > 2*time.Second {
				b.Errorf("Processing 10000 votes took %v, expected < 2s", elapsed)
			}
		}
	})
}

// Helper function to create IDs
func makeID(bytes ...byte) ID {
	id := ID{}
	for i, b := range bytes {
		if i < len(id) {
			id[i] = b
		}
	}
	return id
}

// TestBlock implements the Block interface for benchmarking
type TestBlock struct {
	id       ID
	parentID ID
	height   uint64
	data     []byte
}

func (b *TestBlock) ID() ID           { return b.id }
func (b *TestBlock) ParentID() ID     { return b.parentID }
func (b *TestBlock) Height() uint64   { return b.height }
func (b *TestBlock) Timestamp() int64 { return time.Now().Unix() }
func (b *TestBlock) Data() []byte     { return b.data }
func (b *TestBlock) Verify() error    { return nil }
