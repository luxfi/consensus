// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"crypto/rand"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// BenchmarkParallelOwnedAssetFinalization demonstrates 1M+ TPS through parallel FPC
// This simulates the X-Chain scenario where owned assets can be finalized independently
func BenchmarkParallelOwnedAssetFinalization(b *testing.B) {
	scenarios := []struct {
		name          string
		validators    int
		parallelUsers int
		txPerUser     int
		blockTime     time.Duration
	}{
		{"100users-10tx", 21, 100, 10, 100 * time.Millisecond},
		{"1000users-10tx", 21, 1000, 10, 100 * time.Millisecond},
		{"10000users-10tx", 31, 10000, 10, 200 * time.Millisecond},
		{"100000users-1tx", 100, 100000, 1, 500 * time.Millisecond},
		{"1M-txs-batch", 100, 100000, 10, 1 * time.Second},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			// Setup validators
			validators := make([]ids.NodeID, scenario.validators)
			for i := range validators {
				validators[i] = ids.GenerateTestNodeID()
			}

			f := (scenario.validators - 1) / 3
			cfg := Config{
				N:                 scenario.validators,
				F:                 f,
				Epoch:             1,
				VoteLimitPerBlock: 1024, // Higher limit for batch processing
			}

			b.ResetTimer()
			b.ReportAllocs()

			totalTPS := float64(0)
			totalLatency := int64(0)
			rounds := 0

			for round := 0; round < b.N; round++ {
				roundStart := time.Now()

				// Create parallel FPC instances for sharding
				numShards := runtime.NumCPU()
				shards := make([]*waveFPC, numShards)
				for i := 0; i < numShards; i++ {
					cls := newMockClassifier()
					dag := newMockDAG()
					pq := &mockPQWithBLS{}
					shards[i] = New(cfg, cls, dag, pq, validators[0], validators).(*waveFPC)
				}

				// Generate owned-asset transactions (each user owns unique assets)
				type userTx struct {
					user  int
					tx    TxRef
					obj   ObjectID
					shard int
				}

				allTxs := make([]userTx, 0, scenario.parallelUsers*scenario.txPerUser)
				for user := 0; user < scenario.parallelUsers; user++ {
					for tx := 0; tx < scenario.txPerUser; tx++ {
						txID := make([]byte, 32)
						_, _ = rand.Read(txID)

						// Each user has their own object namespace
						objID := make([]byte, 32)
						// First 8 bytes are user ID for uniqueness
						objID[0] = byte(user >> 24)
						objID[1] = byte(user >> 16)
						objID[2] = byte(user >> 8)
						objID[3] = byte(user)
						_, _ = rand.Read(objID[8:])

						utx := userTx{
							user:  user,
							tx:    TxRef(txID),
							obj:   ObjectID(objID),
							shard: user % numShards,
						}
						allTxs = append(allTxs, utx)

						// Register with classifier
						shards[utx.shard].cls.(*mockClassifier).addOwnedTx(utx.tx, utx.obj)
					}
				}

				// Phase 1: Parallel voting (simulating validators voting on different owned objects)
				voteStart := time.Now()

				var wg sync.WaitGroup
				processedCount := int64(0)

				// Process votes in parallel across shards
				for shardIdx := 0; shardIdx < numShards; shardIdx++ {
					wg.Add(1)
					go func(shard int, fpc *waveFPC) {
						defer wg.Done()

						// Each shard processes its subset of transactions
						for _, utx := range allTxs {
							if utx.shard != shard {
								continue
							}

							// Collect votes from 2f+1 validators
							quorum := 2*f + 1
							for v := 0; v < quorum; v++ {
								block := &Block{
									ID:     ids.GenerateTestID(),
									Author: validators[v],
									Round:  uint64(round*1000000 + utx.user*1000 + v),
									Payload: BlockPayload{
										FPCVotes: [][]byte{utx.tx[:]},
									},
								}
								fpc.OnBlockObserved(block)
							}

							atomic.AddInt64(&processedCount, 1)
						}
					}(shardIdx, shards[shardIdx])
				}

				wg.Wait()
				voteTime := time.Since(voteStart)

				// Phase 2: Parallel BLS aggregation (simulating cryptographic work)
				blsStart := time.Now()

				aggregatedCount := int64(0)
				for shardIdx := 0; shardIdx < numShards; shardIdx++ {
					wg.Add(1)
					go func(shard int, fpc *waveFPC) {
						defer wg.Done()

						for _, utx := range allTxs {
							if utx.shard != shard {
								continue
							}

							status, proof := fpc.Status(utx.tx)
							if status == Executable {
								// Simulate BLS aggregation work
								_ = simulateBLSAggregation(proof.VoterCount)
								atomic.AddInt64(&aggregatedCount, 1)
							}
						}
					}(shardIdx, shards[shardIdx])
				}

				wg.Wait()
				blsTime := time.Since(blsStart)

				// Phase 3: Parallel finalization (owned assets can finalize independently)
				finalizeStart := time.Now()

				finalizedCount := int64(0)
				for shardIdx := 0; shardIdx < numShards; shardIdx++ {
					wg.Add(1)
					go func(shard int, fpc *waveFPC) {
						defer wg.Done()

						// Create per-shard anchor blocks
						anchorBlock := &Block{
							ID:     ids.GenerateTestID(),
							Author: validators[0],
							Round:  uint64(round*1000000 + 999999),
						}

						// Add all shard transactions to ancestry
						for _, utx := range allTxs {
							if utx.shard != shard {
								continue
							}
							fpc.dag.(*mockDAG).addAncestry(anchorBlock.ID, utx.tx)
						}

						// Accept the anchor (finalizes all transactions in parallel)
						fpc.OnBlockAccepted(anchorBlock)

						// Count finalized
						for _, utx := range allTxs {
							if utx.shard != shard {
								continue
							}
							status, _ := fpc.Status(utx.tx)
							if status == Final {
								atomic.AddInt64(&finalizedCount, 1)
							}
						}
					}(shardIdx, shards[shardIdx])
				}

				wg.Wait()
				finalizeTime := time.Since(finalizeStart)

				// Calculate metrics
				totalTime := time.Since(roundStart)
				txCount := scenario.parallelUsers * scenario.txPerUser
				tps := float64(txCount) / totalTime.Seconds()

				totalTPS += tps
				totalLatency += int64(totalTime)
				rounds++

				// Report detailed metrics for this round
				if round == b.N-1 {
					b.ReportMetric(float64(txCount), "total_txs")
					b.ReportMetric(float64(processedCount), "votes_processed")
					b.ReportMetric(float64(aggregatedCount), "bls_aggregated")
					b.ReportMetric(float64(finalizedCount), "txs_finalized")
					b.ReportMetric(tps, "tx/sec")
					b.ReportMetric(float64(voteTime.Milliseconds()), "vote_ms")
					b.ReportMetric(float64(blsTime.Milliseconds()), "bls_ms")
					b.ReportMetric(float64(finalizeTime.Milliseconds()), "finalize_ms")
					b.ReportMetric(float64(totalTime.Milliseconds()), "total_ms")
					b.ReportMetric(float64(numShards), "parallel_shards")

					// Efficiency metrics
					parallelSpeedup := float64(txCount) / (totalTime.Seconds() * float64(numShards))
					b.ReportMetric(parallelSpeedup, "parallel_efficiency")

					b.Logf("Achieved %.0f TPS with %d parallel users, %d tx each",
						tps, scenario.parallelUsers, scenario.txPerUser)
				}
			}

			// Report average metrics
			if rounds > 0 {
				avgTPS := totalTPS / float64(rounds)
				avgLatency := time.Duration(totalLatency / int64(rounds))
				b.ReportMetric(avgTPS, "avg_tx/sec")
				b.ReportMetric(float64(avgLatency.Milliseconds()), "avg_latency_ms")
			}
		})
	}
}

// BenchmarkMillionTPS attempts to achieve 1M+ TPS through massive parallelization
func BenchmarkMillionTPS(b *testing.B) {
	// Optimal configuration for max TPS
	validators := 31 // Byzantine fault tolerance with f=10
	f := 10
	numShards := runtime.NumCPU() * 2 // Oversubscribe for better utilization
	txPerShard := 100000 / numShards  // Target 100K transactions total

	b.Logf("Running 1M TPS benchmark with %d shards, %d tx/shard", numShards, txPerShard)

	validatorNodes := make([]ids.NodeID, validators)
	for i := range validatorNodes {
		validatorNodes[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 validators,
		F:                 f,
		Epoch:             1,
		VoteLimitPerBlock: 2048, // High limit for batch processing
	}

	b.ResetTimer()
	b.ReportAllocs()

	for iter := 0; iter < b.N; iter++ {
		start := time.Now()

		// Create sharded FPC instances
		type shard struct {
			fpc    *waveFPC
			txs    []TxRef
			objs   []ObjectID
			votes  int64
			finals int64
		}

		shards := make([]*shard, numShards)

		// Initialize shards and generate transactions
		var initWg sync.WaitGroup
		for i := 0; i < numShards; i++ {
			initWg.Add(1)
			go func(idx int) {
				defer initWg.Done()

				cls := newMockClassifier()
				dag := newMockDAG()
				pq := &mockPQWithBLS{}
				fpc := New(cfg, cls, dag, pq, validatorNodes[0], validatorNodes).(*waveFPC)

				s := &shard{
					fpc:  fpc,
					txs:  make([]TxRef, txPerShard),
					objs: make([]ObjectID, txPerShard),
				}

				// Generate transactions for this shard
				for j := 0; j < txPerShard; j++ {
					txID := make([]byte, 32)
					_, _ = rand.Read(txID)
					s.txs[j] = TxRef(txID)

					objID := make([]byte, 32)
					// Shard prefix for uniqueness
					objID[0] = byte(idx)
					_, _ = rand.Read(objID[1:])
					s.objs[j] = ObjectID(objID)

					cls.addOwnedTx(s.txs[j], s.objs[j])
				}

				shards[idx] = s
			}(i)
		}
		initWg.Wait()

		// Process all transactions in parallel
		var processWg sync.WaitGroup

		// Vote collection phase
		voteStart := time.Now()
		for i, s := range shards {
			processWg.Add(1)
			go func(shardIdx int, sh *shard) {
				defer processWg.Done()

				quorum := 2*f + 1

				// Process all transactions in this shard
				for txIdx, tx := range sh.txs {
					// Simulate quorum voting
					for v := 0; v < quorum; v++ {
						block := &Block{
							ID:     ids.GenerateTestID(),
							Author: validatorNodes[v],
							Round:  uint64(iter*10000000 + shardIdx*100000 + txIdx*100 + v),
							Payload: BlockPayload{
								FPCVotes: [][]byte{tx[:]},
							},
						}
						sh.fpc.OnBlockObserved(block)
					}
					atomic.AddInt64(&sh.votes, int64(quorum))
				}
			}(i, s)
		}
		processWg.Wait()
		voteTime := time.Since(voteStart)

		// BLS aggregation phase (simulated)
		blsStart := time.Now()
		for _, s := range shards {
			processWg.Add(1)
			go func(sh *shard) {
				defer processWg.Done()

				for _, tx := range sh.txs {
					status, proof := sh.fpc.Status(tx)
					if status == Executable {
						// Simulate BLS work (scaled down for benchmark speed)
						time.Sleep(time.Microsecond)
						_ = proof.VoterCount
					}
				}
			}(s)
		}
		processWg.Wait()
		blsTime := time.Since(blsStart)

		// Finalization phase
		finalizeStart := time.Now()
		for i, s := range shards {
			processWg.Add(1)
			go func(shardIdx int, sh *shard) {
				defer processWg.Done()

				// Create anchor block for this shard
				anchor := &Block{
					ID:     ids.GenerateTestID(),
					Author: validatorNodes[0],
					Round:  uint64(iter*10000000 + shardIdx*100000 + 99999),
				}

				// Add all transactions to ancestry
				for _, tx := range sh.txs {
					sh.fpc.dag.(*mockDAG).addAncestry(anchor.ID, tx)
				}

				// Accept anchor (finalizes all transactions)
				sh.fpc.OnBlockAccepted(anchor)

				// Count finalized
				for _, tx := range sh.txs {
					status, _ := sh.fpc.Status(tx)
					if status == Final {
						atomic.AddInt64(&sh.finals, 1)
					}
				}
			}(i, s)
		}
		processWg.Wait()
		finalizeTime := time.Since(finalizeStart)

		// Calculate results
		totalTime := time.Since(start)
		totalTxs := numShards * txPerShard
		totalVotes := int64(0)
		totalFinals := int64(0)

		for _, s := range shards {
			totalVotes += s.votes
			totalFinals += s.finals
		}

		tps := float64(totalTxs) / totalTime.Seconds()

		// Report metrics
		b.ReportMetric(float64(totalTxs), "total_txs")
		b.ReportMetric(float64(totalVotes), "total_votes")
		b.ReportMetric(float64(totalFinals), "finalized_txs")
		b.ReportMetric(tps, "tx/sec")
		b.ReportMetric(float64(voteTime.Microseconds())/1000, "vote_ms")
		b.ReportMetric(float64(blsTime.Microseconds())/1000, "bls_ms")
		b.ReportMetric(float64(finalizeTime.Microseconds())/1000, "finalize_ms")
		b.ReportMetric(float64(totalTime.Milliseconds()), "total_ms")
		b.ReportMetric(float64(numShards), "shards")

		// Check if we achieved 1M+ TPS
		if tps >= 1000000 {
			b.Logf("🎉 ACHIEVED %.0f TPS (>1M!) with %d shards", tps, numShards)
		} else {
			b.Logf("Achieved %.0f TPS with %d shards", tps, numShards)
		}

		// Finalization rate
		finalizationRate := float64(totalFinals) / float64(totalTxs) * 100
		b.ReportMetric(finalizationRate, "finalization_rate_%")
	}
}

// BenchmarkOptimizationComparison compares fast path vs regular consensus
func BenchmarkOptimizationComparison(b *testing.B) {
	scenarios := []struct {
		name       string
		txType     string // "owned", "shared", "mixed"
		txCount    int
		validators int
	}{
		{"owned-100tx", "owned", 100, 21},
		{"shared-100tx", "shared", 100, 21},
		{"mixed-100tx", "mixed", 100, 21},
		{"owned-1000tx", "owned", 1000, 31},
		{"shared-1000tx", "shared", 1000, 31},
		{"mixed-1000tx", "mixed", 1000, 31},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			validators := make([]ids.NodeID, scenario.validators)
			for i := range validators {
				validators[i] = ids.GenerateTestNodeID()
			}

			f := (scenario.validators - 1) / 3
			cfg := Config{
				N:                 scenario.validators,
				F:                 f,
				Epoch:             1,
				VoteLimitPerBlock: 256,
			}

			b.ResetTimer()

			for round := 0; round < b.N; round++ {
				cls := newMockClassifier()
				dag := newMockDAG()
				pq := &mockPQWithBLS{}
				fpc := New(cfg, cls, dag, pq, validators[0], validators).(*waveFPC)

				// Generate transactions based on type
				transactions := make([]TxRef, scenario.txCount)
				objects := make([]ObjectID, scenario.txCount)

				for i := 0; i < scenario.txCount; i++ {
					txID := make([]byte, 32)
					_, _ = rand.Read(txID)
					transactions[i] = TxRef(txID)

					objID := make([]byte, 32)
					if scenario.txType == "owned" {
						// Unique object per transaction
						_, _ = rand.Read(objID)
						objects[i] = ObjectID(objID)
						cls.addOwnedTx(transactions[i], objects[i])
					} else if scenario.txType == "shared" {
						// Same object for all (simulating shared state)
						copy(objID[:], []byte("shared-object"))
						objects[i] = ObjectID(objID)
						cls.addSharedTx(transactions[i], objects[i])
					} else { // mixed
						if i%2 == 0 {
							_, _ = rand.Read(objID)
							cls.addOwnedTx(transactions[i], ObjectID(objID))
						} else {
							copy(objID[:], []byte("shared-object"))
							cls.addSharedTx(transactions[i], ObjectID(objID))
						}
						objects[i] = ObjectID(objID)
					}
				}

				start := time.Now()

				// Process based on type
				if scenario.txType == "owned" {
					// Fast path: parallel processing
					var wg sync.WaitGroup
					for _, tx := range transactions {
						wg.Add(1)
						go func(txRef TxRef) {
							defer wg.Done()

							// Vote collection
							quorum := 2*f + 1
							for v := 0; v < quorum; v++ {
								block := &Block{
									ID:     ids.GenerateTestID(),
									Author: validators[v],
									Round:  uint64(round*100000 + v),
									Payload: BlockPayload{
										FPCVotes: [][]byte{txRef[:]},
									},
								}
								fpc.OnBlockObserved(block)
							}
						}(tx)
					}
					wg.Wait()
				} else {
					// Regular path: sequential processing
					for _, tx := range transactions {
						quorum := 2*f + 1
						for v := 0; v < quorum; v++ {
							block := &Block{
								ID:     ids.GenerateTestID(),
								Author: validators[v],
								Round:  uint64(round*100000 + v),
								Payload: BlockPayload{
									FPCVotes: [][]byte{tx[:]},
								},
							}
							fpc.OnBlockObserved(block)
						}
					}
				}

				processingTime := time.Since(start)

				// Count executable transactions
				executableCount := 0
				for _, tx := range transactions {
					status, _ := fpc.Status(tx)
					if status == Executable {
						executableCount++
					}
				}

				// Report metrics
				tps := float64(scenario.txCount) / processingTime.Seconds()
				b.ReportMetric(tps, fmt.Sprintf("%s_tx/sec", scenario.txType))
				b.ReportMetric(float64(processingTime.Microseconds()), fmt.Sprintf("%s_us", scenario.txType))
				b.ReportMetric(float64(executableCount), fmt.Sprintf("%s_executable", scenario.txType))
			}
		})
	}
}

// Helper to create mock classifier with owned/shared distinction
func (m *mockClassifier) addSharedTx(tx TxRef, obj ObjectID) {
	// Shared transactions have no owned inputs
	m.ownedInputs[tx] = []ObjectID{}
	m.sharedInputs[tx] = []ObjectID{obj}
}

func (m *mockClassifier) SharedInputs(tx TxRef) []ObjectID {
	return m.sharedInputs[tx]
}
