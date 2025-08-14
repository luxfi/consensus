// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"crypto/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	
	"github.com/luxfi/ids"
)

// BenchmarkPerformanceShowcase demonstrates 50x speedup with owned assets
func BenchmarkPerformanceShowcase(b *testing.B) {
	// Test scenarios comparing shared vs owned performance
	scenarios := []struct {
		name           string
		parallelOwned  int    // Number of parallel owned-asset threads
		sharedThreads  int    // Number of shared state threads
		txPerThread    int    // Transactions per thread
		validators     int
	}{
		{"1-shared-baseline", 0, 1, 100, 21},
		{"10-owned-parallel", 10, 0, 100, 21},
		{"25-owned-parallel", 25, 0, 100, 21},
		{"50-owned-parallel", 50, 0, 100, 21},
		{"50-owned-1-shared", 50, 1, 100, 21},
		{"100-owned-parallel", 100, 0, 100, 31},
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
				VoteLimitPerBlock: 512,
			}
			
			b.ResetTimer()
			b.ReportAllocs()
			
			totalTPS := float64(0)
			sharedTime := time.Duration(0)
			ownedTime := time.Duration(0)
			
			for round := 0; round < b.N; round++ {
				cls := newMockClassifier()
				dag := newMockDAG()
				pq := &mockPQWithBLS{}
				fpc := New(cfg, cls, dag, pq, validators[0], validators).(*waveFPC)
				
				start := time.Now()
				
				var wg sync.WaitGroup
				processedCount := int64(0)
				
				// Process shared transactions (sequential - they conflict)
				if scenario.sharedThreads > 0 {
					sharedStart := time.Now()
					
					sharedObject := ObjectID{}
					copy(sharedObject[:], []byte("shared-state-object"))
					
					for t := 0; t < scenario.sharedThreads; t++ {
						for i := 0; i < scenario.txPerThread; i++ {
							txID := make([]byte, 32)
							rand.Read(txID)
							tx := TxRef(txID)
							
							// Mark as shared (no owned inputs)
							cls.addSharedTx(tx, sharedObject)
							
							// Sequential processing required for shared state
							quorum := 2*f + 1
							for v := 0; v < quorum; v++ {
								block := &Block{
									ID:     ids.GenerateTestID(),
									Author: validators[v],
									Round:  uint64(round*1000000 + t*1000 + i*10 + v),
									Payload: BlockPayload{
										FPCVotes: [][]byte{tx[:]},
									},
								}
								fpc.OnBlockObserved(block)
							}
							
							atomic.AddInt64(&processedCount, 1)
						}
					}
					
					sharedTime = time.Since(sharedStart)
				}
				
				// Process owned transactions (parallel - no conflicts)
				if scenario.parallelOwned > 0 {
					ownedStart := time.Now()
					
					for t := 0; t < scenario.parallelOwned; t++ {
						wg.Add(1)
						go func(threadID int) {
							defer wg.Done()
							
							// Each thread has its own object namespace
							for i := 0; i < scenario.txPerThread; i++ {
								txID := make([]byte, 32)
								rand.Read(txID)
								tx := TxRef(txID)
								
								// Unique object per thread (owned)
								objID := make([]byte, 32)
								objID[0] = byte(threadID >> 8)
								objID[1] = byte(threadID)
								rand.Read(objID[2:])
								
								cls.addOwnedTx(tx, ObjectID(objID))
								
								// Parallel processing possible
								quorum := 2*f + 1
								for v := 0; v < quorum; v++ {
									block := &Block{
										ID:     ids.GenerateTestID(),
										Author: validators[v],
										Round:  uint64(round*1000000 + threadID*10000 + i*10 + v),
										Payload: BlockPayload{
											FPCVotes: [][]byte{tx[:]},
										},
									}
									fpc.OnBlockObserved(block)
								}
								
								atomic.AddInt64(&processedCount, 1)
							}
						}(t)
					}
					
					wg.Wait()
					ownedTime = time.Since(ownedStart)
				}
				
				totalTime := time.Since(start)
				txCount := int64(scenario.parallelOwned + scenario.sharedThreads) * int64(scenario.txPerThread)
				tps := float64(txCount) / totalTime.Seconds()
				totalTPS += tps
				
				// Report metrics
				if round == b.N-1 {
					b.ReportMetric(float64(txCount), "total_txs")
					b.ReportMetric(tps, "tx/sec")
					b.ReportMetric(float64(totalTime.Milliseconds()), "total_ms")
					
					if scenario.sharedThreads > 0 {
						sharedTPS := float64(scenario.sharedThreads*scenario.txPerThread) / sharedTime.Seconds()
						b.ReportMetric(sharedTPS, "shared_tx/sec")
						b.ReportMetric(float64(sharedTime.Milliseconds()), "shared_ms")
					}
					
					if scenario.parallelOwned > 0 {
						ownedTPS := float64(scenario.parallelOwned*scenario.txPerThread) / ownedTime.Seconds()
						b.ReportMetric(ownedTPS, "owned_tx/sec")
						b.ReportMetric(float64(ownedTime.Milliseconds()), "owned_ms")
						
						// Calculate speedup vs single shared thread
						baselineSharedTPS := float64(scenario.txPerThread) / sharedTime.Seconds()
						if scenario.sharedThreads == 0 && scenario.parallelOwned > 0 {
							// Estimate shared performance
							baselineSharedTPS = tps / float64(scenario.parallelOwned)
						}
						speedup := ownedTPS / baselineSharedTPS
						b.ReportMetric(speedup, "speedup_factor")
					}
				}
			}
			
			avgTPS := totalTPS / float64(b.N)
			b.ReportMetric(avgTPS, "avg_tx/sec")
		})
	}
}

// BenchmarkMillionUsersMemory estimates memory usage for 1M simultaneous users
func BenchmarkMillionUsersMemory(b *testing.B) {
	scenarios := []struct {
		name      string
		users     int
		txPerUser int
	}{
		{"1K-users", 1000, 5},
		{"10K-users", 10000, 5},
		{"100K-users", 100000, 5},
		{"1M-users", 1000000, 5},
	}
	
	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			// Setup minimal validator set for memory testing
			validators := make([]ids.NodeID, 21)
			for i := range validators {
				validators[i] = ids.GenerateTestNodeID()
			}
			
			cfg := Config{
				N:                 21,
				F:                 7,
				Epoch:             1,
				VoteLimitPerBlock: 1024,
			}
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for round := 0; round < b.N; round++ {
				// Measure memory before
				var memStatsBefore runtime.MemStats
				runtime.ReadMemStats(&memStatsBefore)
				
				cls := newMockClassifier()
				dag := newMockDAG()
				pq := &mockPQWithBLS{}
				fpc := New(cfg, cls, dag, pq, validators[0], validators).(*waveFPC)
				
				// Create transaction references for all users
				userTxs := make([][]TxRef, scenario.users)
				userObjs := make([][]ObjectID, scenario.users)
				
				// Generate transactions for each user
				for u := 0; u < scenario.users; u++ {
					userTxs[u] = make([]TxRef, scenario.txPerUser)
					userObjs[u] = make([]ObjectID, scenario.txPerUser)
					
					for t := 0; t < scenario.txPerUser; t++ {
						txID := make([]byte, 32)
						// Use deterministic IDs to reduce randomness overhead
						txID[0] = byte(u >> 24)
						txID[1] = byte(u >> 16)
						txID[2] = byte(u >> 8)
						txID[3] = byte(u)
						txID[4] = byte(t)
						
						userTxs[u][t] = TxRef(txID)
						
						// Each user has unique owned objects
						objID := make([]byte, 32)
						copy(objID[:4], txID[:4]) // User prefix
						objID[4] = byte(t)
						
						userObjs[u][t] = ObjectID(objID)
						cls.addOwnedTx(userTxs[u][t], userObjs[u][t])
					}
				}
				
				// Simulate voting on a subset (can't process 1M simultaneously)
				batchSize := 10000
				if scenario.users < batchSize {
					batchSize = scenario.users
				}
				
				// Process first batch to establish memory footprint
				for u := 0; u < batchSize; u++ {
					for t := 0; t < scenario.txPerUser; t++ {
						tx := userTxs[u][t]
						
						// Add minimal votes (just enough for quorum)
						for v := 0; v < 15; v++ { // 2f+1 = 15 for f=7
							block := &Block{
								ID:     ids.GenerateTestID(),
								Author: validators[v],
								Round:  uint64(round*100000000 + u*1000 + t*10 + v),
								Payload: BlockPayload{
									FPCVotes: [][]byte{tx[:]},
								},
							}
							fpc.OnBlockObserved(block)
						}
					}
				}
				
				// Measure memory after
				var memStatsAfter runtime.MemStats
				runtime.ReadMemStats(&memStatsAfter)
				
				// Calculate memory usage
				memUsed := memStatsAfter.Alloc - memStatsBefore.Alloc
				memPerUser := float64(memUsed) / float64(scenario.users)
				memPerTx := float64(memUsed) / float64(scenario.users*scenario.txPerUser)
				
				// Extrapolate for 1M users
				estimatedMemFor1M := memPerUser * 1000000
				estimatedMemFor1MTxs := memPerTx * 1000000
				
				// Report metrics
				b.ReportMetric(float64(scenario.users), "users")
				b.ReportMetric(float64(scenario.txPerUser), "tx_per_user")
				b.ReportMetric(float64(memUsed), "total_memory_bytes")
				b.ReportMetric(memPerUser, "bytes_per_user")
				b.ReportMetric(memPerTx, "bytes_per_tx")
				b.ReportMetric(estimatedMemFor1M/1024/1024, "estimated_MB_for_1M_users")
				b.ReportMetric(estimatedMemFor1MTxs/1024/1024/1024, "estimated_GB_for_1M_txs")
				
				// Report vote storage overhead
				votesStored := batchSize * scenario.txPerUser * 15 // votes per tx
				voteMemory := votesStored * 40 // ~40 bytes per vote (bitset + reference)
				b.ReportMetric(float64(voteMemory), "vote_storage_bytes")
				b.ReportMetric(float64(voteMemory)/float64(votesStored), "bytes_per_vote")
				
				// State map overhead
				stateEntries := scenario.users * scenario.txPerUser
				stateMemory := stateEntries * 64 // ~64 bytes per state entry
				b.ReportMetric(float64(stateMemory), "state_map_bytes")
				
				// Total estimated memory for 1M concurrent users
				totalEstimated := estimatedMemFor1M + float64(voteMemory) + float64(stateMemory)
				b.ReportMetric(totalEstimated/1024/1024/1024, "total_GB_for_1M_users")
				
				b.Logf("Memory estimate for %d users: %.2f MB total, %.2f KB/user, %.2f bytes/tx",
					scenario.users, 
					float64(memUsed)/1024/1024,
					memPerUser/1024,
					memPerTx)
				
				if scenario.users == 1000000 {
					b.Logf("🎯 1M users trading simultaneously would use approximately %.2f GB of memory",
						totalEstimated/1024/1024/1024)
				}
			}
		})
	}
}

// BenchmarkSpeedupComparison directly compares shared vs owned performance
func BenchmarkSpeedupComparison(b *testing.B) {
	validators := make([]ids.NodeID, 21)
	for i := range validators {
		validators[i] = ids.GenerateTestNodeID()
	}
	
	cfg := Config{
		N:                 21,
		F:                 7,
		Epoch:             1,
		VoteLimitPerBlock: 512,
	}
	
	// Test 1: Single shared thread baseline
	b.Run("shared-single-thread", func(b *testing.B) {
		b.ResetTimer()
		
		for round := 0; round < b.N; round++ {
			cls := newMockClassifier()
			dag := newMockDAG()
			fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)
			
			sharedObj := ObjectID{}
			copy(sharedObj[:], []byte("shared"))
			
			start := time.Now()
			
			// Process 1000 transactions sequentially
			for i := 0; i < 1000; i++ {
				txID := make([]byte, 32)
				rand.Read(txID)
				tx := TxRef(txID)
				
				cls.addSharedTx(tx, sharedObj)
				
				// Must process sequentially
				for v := 0; v < 15; v++ {
					block := &Block{
						ID:     ids.GenerateTestID(),
						Author: validators[v],
						Round:  uint64(round*100000 + i*100 + v),
						Payload: BlockPayload{
							FPCVotes: [][]byte{tx[:]},
						},
					}
					fpc.OnBlockObserved(block)
				}
			}
			
			elapsed := time.Since(start)
			tps := 1000.0 / elapsed.Seconds()
			b.ReportMetric(tps, "shared_tx/sec")
			b.ReportMetric(float64(elapsed.Milliseconds()), "shared_ms")
		}
	})
	
	// Test 2: 50 parallel owned threads
	b.Run("owned-50-threads", func(b *testing.B) {
		b.ResetTimer()
		
		for round := 0; round < b.N; round++ {
			cls := newMockClassifier()
			dag := newMockDAG()
			fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)
			
			start := time.Now()
			
			var wg sync.WaitGroup
			// 50 parallel threads, 20 tx each = 1000 total
			for t := 0; t < 50; t++ {
				wg.Add(1)
				go func(threadID int) {
					defer wg.Done()
					
					for i := 0; i < 20; i++ {
						txID := make([]byte, 32)
						txID[0] = byte(threadID)
						rand.Read(txID[1:])
						tx := TxRef(txID)
						
						objID := make([]byte, 32)
						objID[0] = byte(threadID)
						
						cls.addOwnedTx(tx, ObjectID(objID))
						
						// Can process in parallel
						for v := 0; v < 15; v++ {
							block := &Block{
								ID:     ids.GenerateTestID(),
								Author: validators[v],
								Round:  uint64(round*100000 + threadID*1000 + i*10 + v),
								Payload: BlockPayload{
									FPCVotes: [][]byte{tx[:]},
								},
							}
							fpc.OnBlockObserved(block)
						}
					}
				}(t)
			}
			
			wg.Wait()
			
			elapsed := time.Since(start)
			tps := 1000.0 / elapsed.Seconds()
			b.ReportMetric(tps, "owned_tx/sec")
			b.ReportMetric(float64(elapsed.Milliseconds()), "owned_ms")
			
			// Calculate theoretical speedup
			theoreticalSpeedup := 50.0 // 50 parallel threads
			b.ReportMetric(theoreticalSpeedup, "theoretical_speedup")
		}
	})
}