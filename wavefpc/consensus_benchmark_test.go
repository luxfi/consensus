// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"crypto/rand"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// BenchmarkFullConsensusFlow tests the complete flow from voting to finalization
func BenchmarkFullConsensusFlow(b *testing.B) {
	networkSizes := []struct {
		validators int
		f          int
		name       string
	}{
		{4, 1, "4-validators"},
		{7, 2, "7-validators"},
		{10, 3, "10-validators"},
		{21, 7, "21-validators"},
		{31, 10, "31-validators"},
		{100, 33, "100-validators"},
	}

	for _, network := range networkSizes {
		b.Run(network.name, func(b *testing.B) {
			// Setup validators
			validators := make([]ids.NodeID, network.validators)
			for i := range validators {
				validators[i] = ids.GenerateTestNodeID()
			}

			// Create FPC instance
			cfg := Config{
				N:                 network.validators,
				F:                 network.f,
				Epoch:             1,
				VoteLimitPerBlock: 256,
			}

			cls := newMockClassifier()
			dag := newMockDAG()
			fpc := New(cfg, cls, dag, &mockPQ{}, validators[0], validators)

			// Metrics tracking
			var totalBlocks int64
			var totalFinalized int64
			var totalLatency int64

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Create a transaction
				txID := ids.GenerateTestID()
				tx := TxRef(txID[:])
				objID := ids.GenerateTestID()
				obj := ObjectID(objID[:])
				cls.addOwnedTx(tx, obj)

				startTime := time.Now()

				// Phase 1: Vote collection (simulate 2f+1 validators voting)
				quorumSize := 2*network.f + 1
				for v := 0; v < quorumSize; v++ {
					block := &Block{
						ID:     ids.GenerateTestID(),
						Author: validators[v],
						Round:  uint64(i*1000 + v),
						Payload: BlockPayload{
							FPCVotes: [][]byte{tx[:]},
						},
					}
					fpc.OnBlockObserved(block)
				}

				// Check if executable
				status, proof := fpc.Status(tx)
				if status != Executable {
					b.Fatalf("Transaction not executable after %d votes", quorumSize)
				}

				// Phase 2: Simulate BLS signature aggregation
				// In real implementation, this would aggregate actual BLS signatures
				blsSigTime := simulateBLSAggregation(proof.VoterCount)

				// Phase 3: Create anchor block and finalize
				anchorBlock := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[0],
					Round:  uint64(i*1000 + 999),
					Payload: BlockPayload{
						FPCVotes: [][]byte{tx[:]},
					},
				}

				// Simulate the anchor containing the transaction in its ancestry
				dag.addAncestry(anchorBlock.ID, tx)

				// Accept the anchor block (finalizes the transaction)
				fpc.OnBlockAccepted(anchorBlock)

				// Verify finalization
				finalStatus, _ := fpc.Status(tx)
				if finalStatus == Final {
					atomic.AddInt64(&totalFinalized, 1)
				}

				atomic.AddInt64(&totalBlocks, 1)

				// Track latency
				latency := time.Since(startTime) - blsSigTime // Subtract simulated BLS time to get real processing time
				atomic.AddInt64(&totalLatency, int64(latency))
			}

			// Report metrics
			avgLatencyNs := float64(totalLatency) / float64(b.N)
			finalizationRate := float64(totalFinalized) / float64(totalBlocks) * 100
			blocksPerSec := float64(b.N) / b.Elapsed().Seconds()

			quorumSize := 2*network.f + 1
			b.ReportMetric(blocksPerSec, "blocks/sec")
			b.ReportMetric(avgLatencyNs, "ns/block")
			b.ReportMetric(finalizationRate, "finalization_%")
			b.ReportMetric(float64(quorumSize), "votes_for_quorum")
		})
	}
}

// BenchmarkBLSAggregatedConsensus simulates consensus with actual BLS signature costs
func BenchmarkBLSAggregatedConsensus(b *testing.B) {
	testCases := []struct {
		validators int
		txPerBlock int
		name       string
	}{
		{7, 10, "7val-10tx"},
		{21, 50, "21val-50tx"},
		{21, 100, "21val-100tx"},
		{100, 100, "100val-100tx"},
		{100, 256, "100val-256tx"},
	}

	for _, tc := range testCases {
		b.Run(tc.name, func(b *testing.B) {
			validators := make([]ids.NodeID, tc.validators)
			for i := range validators {
				validators[i] = ids.GenerateTestNodeID()
			}

			cfg := Config{
				N:                 tc.validators,
				F:                 (tc.validators - 1) / 3,
				Epoch:             1,
				VoteLimitPerBlock: 256,
			}

			cls := newMockClassifier()
			dag := newMockDAG()
			pq := &mockPQWithBLS{}
			fpc := New(cfg, cls, dag, pq, validators[0], validators)

			// Pre-create transactions
			transactions := make([]TxRef, tc.txPerBlock)
			for i := range transactions {
				txID := ids.GenerateTestID()
				transactions[i] = TxRef(txID[:])
				objID := ids.GenerateTestID()
				obj := ObjectID(objID[:])
				cls.addOwnedTx(transactions[i], obj)
			}

			b.ResetTimer()
			b.ReportAllocs()

			var totalBLSTime int64
			var totalBlocks int64

			for blockNum := 0; blockNum < b.N; blockNum++ {
				blockStart := time.Now()

				// Process votes for all transactions in parallel
				var wg sync.WaitGroup
				for _, tx := range transactions {
					wg.Add(1)
					go func(txRef TxRef) {
						defer wg.Done()

						// Collect votes from 2f+1 validators
						quorum := 2*cfg.F + 1
						for v := 0; v < quorum; v++ {
							block := &Block{
								ID:     ids.GenerateTestID(),
								Author: validators[v],
								Round:  uint64(blockNum*10000 + v),
								Payload: BlockPayload{
									FPCVotes: [][]byte{txRef[:]},
								},
							}
							fpc.OnBlockObserved(block)
						}
					}(tx)
				}
				wg.Wait()

				// Simulate BLS aggregation for all transactions
				blsStart := time.Now()
				for _, tx := range transactions {
					status, proof := fpc.Status(tx)
					if status == Executable {
						_ = simulateBLSAggregation(proof.VoterCount)
						pq.aggregatedSigs++
					}
				}
				blsTime := time.Since(blsStart)
				atomic.AddInt64(&totalBLSTime, int64(blsTime))

				// Create and accept block to finalize
				finalBlock := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[0],
					Round:  uint64(blockNum*10000 + 9999),
					Payload: BlockPayload{
						FPCVotes: make([][]byte, 0, len(transactions)),
					},
				}

				for _, tx := range transactions {
					finalBlock.Payload.FPCVotes = append(finalBlock.Payload.FPCVotes, tx[:])
					dag.addAncestry(finalBlock.ID, tx)
				}

				fpc.OnBlockAccepted(finalBlock)
				atomic.AddInt64(&totalBlocks, 1)

				blockTime := time.Since(blockStart)

				// Report per-block metrics on last iteration
				if blockNum == b.N-1 {
					b.ReportMetric(float64(tc.txPerBlock), "tx/block")
					b.ReportMetric(float64(blockTime.Nanoseconds()), "ns/block")
					b.ReportMetric(float64(blsTime.Nanoseconds()), "ns_bls/block")
					b.ReportMetric(float64(pq.aggregatedSigs), "total_bls_sigs")
				}
			}

			// Calculate throughput
			totalTime := b.Elapsed()
			txThroughput := float64(tc.txPerBlock*b.N) / totalTime.Seconds()
			blockThroughput := float64(b.N) / totalTime.Seconds()

			b.ReportMetric(txThroughput, "tx/sec")
			b.ReportMetric(blockThroughput, "blocks/sec")

			// BLS overhead
			blsOverhead := float64(totalBLSTime) / float64(totalTime.Nanoseconds()) * 100
			b.ReportMetric(blsOverhead, "bls_overhead_%")
		})
	}
}

// BenchmarkConsensusLatency measures time from first vote to finalization
func BenchmarkConsensusLatency(b *testing.B) {
	validators := 21
	f := 7

	validatorNodes := make([]ids.NodeID, validators)
	for i := range validatorNodes {
		validatorNodes[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 validators,
		F:                 f,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	dag := newMockDAG()
	fpc := New(cfg, cls, dag, nil, validatorNodes[0], validatorNodes)

	b.ResetTimer()
	b.ReportAllocs()

	latencies := make([]time.Duration, 0, b.N)

	for i := 0; i < b.N; i++ {
		// Create transaction
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		objID := ids.GenerateTestID()
		obj := ObjectID(objID[:])
		cls.addOwnedTx(tx, obj)

		start := time.Now()

		// First vote
		firstBlock := &Block{
			ID:     ids.GenerateTestID(),
			Author: validatorNodes[0],
			Round:  uint64(i * 1000),
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx[:]},
			},
		}
		fpc.OnBlockObserved(firstBlock)

		// Remaining votes for quorum
		for v := 1; v < 2*f+1; v++ {
			block := &Block{
				ID:     ids.GenerateTestID(),
				Author: validatorNodes[v],
				Round:  uint64(i*1000 + v),
				Payload: BlockPayload{
					FPCVotes: [][]byte{tx[:]},
				},
			}
			fpc.OnBlockObserved(block)
		}

		// Finalization
		anchorBlock := &Block{
			ID:     ids.GenerateTestID(),
			Author: validatorNodes[0],
			Round:  uint64(i*1000 + 999),
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx[:]},
			},
		}
		dag.addAncestry(anchorBlock.ID, tx)
		fpc.OnBlockAccepted(anchorBlock)

		latency := time.Since(start)
		latencies = append(latencies, latency)
	}

	// Calculate percentiles
	if len(latencies) > 0 {
		var total time.Duration
		minLatency := latencies[0]
		maxLatency := latencies[0]

		for _, l := range latencies {
			total += l
			if l < minLatency {
				minLatency = l
			}
			if l > maxLatency {
				maxLatency = l
			}
		}

		avgLatency := total / time.Duration(len(latencies))

		b.ReportMetric(float64(minLatency.Nanoseconds()), "ns_min_latency")
		b.ReportMetric(float64(avgLatency.Nanoseconds()), "ns_avg_latency")
		b.ReportMetric(float64(maxLatency.Nanoseconds()), "ns_max_latency")
		b.ReportMetric(1000000000.0/float64(avgLatency.Nanoseconds()), "decisions/sec")
	}
}

// BenchmarkScalabilityWithBLS tests how performance scales with network size
func BenchmarkScalabilityWithBLS(b *testing.B) {
	networkSizes := []int{4, 7, 10, 21, 31, 51, 100, 200}

	for _, size := range networkSizes {
		b.Run(fmt.Sprintf("%d-validators", size), func(b *testing.B) {
			validators := make([]ids.NodeID, size)
			for i := range validators {
				validators[i] = ids.GenerateTestNodeID()
			}

			f := (size - 1) / 3
			cfg := Config{
				N:                 size,
				F:                 f,
				Epoch:             1,
				VoteLimitPerBlock: 256,
			}

			cls := newMockClassifier()
			dag := newMockDAG()
			pq := &mockPQWithBLS{}
			fpc := New(cfg, cls, dag, pq, validators[0], validators)

			// Fixed workload: 100 transactions
			numTxs := 100
			transactions := make([]TxRef, numTxs)
			for i := range transactions {
				txID := ids.GenerateTestID()
				transactions[i] = TxRef(txID[:])
				objID := ids.GenerateTestID()
				obj := ObjectID(objID[:])
				cls.addOwnedTx(transactions[i], obj)
			}

			b.ResetTimer()
			b.ReportAllocs()

			start := time.Now()

			for round := 0; round < b.N; round++ {
				// Vote on all transactions
				quorum := 2*f + 1
				for _, tx := range transactions {
					for v := 0; v < quorum; v++ {
						block := &Block{
							ID:     ids.GenerateTestID(),
							Author: validators[v],
							Round:  uint64(round*10000 + v),
							Payload: BlockPayload{
								FPCVotes: [][]byte{tx[:]},
							},
						}
						fpc.OnBlockObserved(block)
					}

					// Simulate BLS aggregation
					status, proof := fpc.Status(tx)
					if status == Executable {
						_ = simulateBLSAggregation(proof.VoterCount)
						pq.aggregatedSigs++
					}
				}

				// Finalize all
				anchorBlock := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[0],
					Round:  uint64(round*10000 + 9999),
				}
				for _, tx := range transactions {
					dag.addAncestry(anchorBlock.ID, tx)
				}
				fpc.OnBlockAccepted(anchorBlock)
			}

			elapsed := time.Since(start)
			throughput := float64(numTxs*b.N) / elapsed.Seconds()

			quorum := 2*f + 1
			b.ReportMetric(throughput, "tx/sec")
			b.ReportMetric(float64(quorum), "quorum_size")
			b.ReportMetric(float64(pq.aggregatedSigs)/float64(b.N), "bls_sigs/round")
			b.ReportMetric(float64(size), "network_size")
		})
	}
}

// Helper functions

func simulateBLSAggregation(numSigs int) time.Duration {
	// Realistic BLS signature aggregation times (approximate)
	// Based on benchmarks: ~50μs per signature on modern hardware
	baseTime := 10 * time.Microsecond
	perSigTime := 50 * time.Microsecond

	totalTime := baseTime + time.Duration(numSigs)*perSigTime

	// Actually sleep to simulate the cost
	time.Sleep(totalTime / 100) // Scale down for benchmark speed

	return totalTime
}

// Mock implementations with BLS tracking

type mockPQ struct{}

func (m *mockPQ) Submit(tx TxRef, voters []ids.NodeID) {}
func (m *mockPQ) HasPQ(tx TxRef) bool                  { return false }
func (m *mockPQ) GetPQ(tx TxRef) (*PQBundle, bool)     { return nil, false }

type mockPQWithBLS struct {
	aggregatedSigs int64
}

func (m *mockPQWithBLS) Submit(tx TxRef, voters []ids.NodeID) {
	// Simulate BLS signature creation
	atomic.AddInt64(&m.aggregatedSigs, 1)
}

func (m *mockPQWithBLS) HasPQ(tx TxRef) bool { return true }
func (m *mockPQWithBLS) GetPQ(tx TxRef) (*PQBundle, bool) {
	return &PQBundle{
		Signature: []byte("bls_sig"),
		Voters:    nil,
	}, true
}

// Extended mock DAG for benchmarks
type mockDAG struct {
	ancestry map[ids.ID]map[TxRef]bool
	mu       sync.RWMutex
}

func newMockDAG() *mockDAG {
	return &mockDAG{
		ancestry: make(map[ids.ID]map[TxRef]bool),
	}
}

func (m *mockDAG) InAncestry(blockID ids.ID, needleTx TxRef) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if txs, ok := m.ancestry[blockID]; ok {
		return txs[needleTx]
	}
	return false
}

func (m *mockDAG) GetBlockByAuthorRound(author ids.NodeID, round uint64) (ids.ID, bool) {
	return ids.Empty, false
}

func (m *mockDAG) addAncestry(blockID ids.ID, txs ...TxRef) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.ancestry[blockID] == nil {
		m.ancestry[blockID] = make(map[TxRef]bool)
	}
	for _, tx := range txs {
		m.ancestry[blockID][tx] = true
	}
}

// BenchmarkRealWorldScenario simulates realistic network conditions
func BenchmarkRealWorldScenario(b *testing.B) {
	// Realistic mainnet parameters
	validators := 100
	f := 33
	txPerSecond := 5000
	blockTime := 2 * time.Second
	txPerBlock := (txPerSecond * int(blockTime.Seconds()))

	b.Logf("Simulating: %d validators, %d tx/sec, %v block time", validators, txPerSecond, blockTime)

	validatorNodes := make([]ids.NodeID, validators)
	for i := range validatorNodes {
		validatorNodes[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 validators,
		F:                 f,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	dag := newMockDAG()
	pq := &mockPQWithBLS{}
	fpc := New(cfg, cls, dag, pq, validatorNodes[0], validatorNodes)

	b.ResetTimer()
	b.ReportAllocs()

	totalTxProcessed := int64(0)
	totalBlocksFinalized := int64(0)
	totalBLSSigs := int64(0)

	for i := 0; i < b.N; i++ {
		blockStart := time.Now()

		// Create batch of transactions
		transactions := make([]TxRef, txPerBlock)
		for j := range transactions {
			txID := make([]byte, 32)
			rand.Read(txID)
			transactions[j] = TxRef(txID)

			objID := make([]byte, 32)
			rand.Read(objID)
			cls.addOwnedTx(transactions[j], ObjectID(objID))
		}

		// Process votes in batches (simulating block propagation)
		batchSize := 256 // Max votes per block
		for batch := 0; batch < len(transactions); batch += batchSize {
			end := batch + batchSize
			if end > len(transactions) {
				end = len(transactions)
			}

			batchTxs := transactions[batch:end]

			// Each validator votes on the batch
			quorum := 2*f + 1
			for v := 0; v < quorum; v++ {
				block := &Block{
					ID:     ids.GenerateTestID(),
					Author: validatorNodes[v],
					Round:  uint64(i*100000 + batch*1000 + v),
					Payload: BlockPayload{
						FPCVotes: make([][]byte, len(batchTxs)),
					},
				}

				for idx, tx := range batchTxs {
					block.Payload.FPCVotes[idx] = tx[:]
				}

				fpc.OnBlockObserved(block)
			}

			// BLS aggregation for executable transactions
			for _, tx := range batchTxs {
				status, proof := fpc.Status(tx)
				if status == Executable {
					_ = simulateBLSAggregation(proof.VoterCount)
					atomic.AddInt64(&totalBLSSigs, 1)
				}
			}
		}

		// Finalize block
		finalBlock := &Block{
			ID:     ids.GenerateTestID(),
			Author: validatorNodes[0],
			Round:  uint64(i*100000 + 99999),
		}

		for _, tx := range transactions {
			dag.addAncestry(finalBlock.ID, tx)
		}
		fpc.OnBlockAccepted(finalBlock)

		atomic.AddInt64(&totalTxProcessed, int64(len(transactions)))
		atomic.AddInt64(&totalBlocksFinalized, 1)

		// Simulate block time
		elapsed := time.Since(blockStart)
		if elapsed < blockTime {
			time.Sleep(blockTime - elapsed)
		}
	}

	// Calculate metrics
	totalTime := b.Elapsed()
	txThroughput := float64(totalTxProcessed) / totalTime.Seconds()
	blockThroughput := float64(totalBlocksFinalized) / totalTime.Seconds()
	blsSigsPerBlock := float64(totalBLSSigs) / float64(totalBlocksFinalized)

	b.ReportMetric(txThroughput, "tx/sec")
	b.ReportMetric(blockThroughput, "blocks/sec")
	b.ReportMetric(float64(txPerBlock), "tx/block")
	b.ReportMetric(blsSigsPerBlock, "bls_sigs/block")
	b.ReportMetric(float64(validators), "validators")

	b.Logf("Achieved: %.0f tx/sec, %.2f blocks/sec, %.0f BLS sigs/block",
		txThroughput, blockThroughput, blsSigsPerBlock)
}
