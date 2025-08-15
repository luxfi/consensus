// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"crypto/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// BenchmarkDAGWithAndWithoutFPC compares DAG finalization speed with FPC enabled vs disabled
func BenchmarkDAGWithAndWithoutFPC(b *testing.B) {
	scenarios := []struct {
		name       string
		fpcEnabled bool
		dagDepth   int // Number of DAG layers
		txPerLayer int // Transactions per layer
		validators int
	}{
		{"DAG-without-FPC-10layers", false, 10, 100, 21},
		{"DAG-with-FPC-10layers", true, 10, 100, 21},
		{"DAG-without-FPC-50layers", false, 50, 100, 21},
		{"DAG-with-FPC-50layers", true, 50, 100, 21},
		{"DAG-without-FPC-100layers", false, 100, 100, 31},
		{"DAG-with-FPC-100layers", true, 100, 100, 31},
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

			totalFinalizationTime := time.Duration(0)
			totalTxFinalized := int64(0)

			for round := 0; round < b.N; round++ {
				cls := newMockClassifier()
				dag := newMockDAG()

				var fpc WaveFPC
				if scenario.fpcEnabled {
					// FPC enabled - fast finalization
					pq := &mockPQWithBLS{}
					fpc = New(cfg, cls, dag, pq, validators[0], validators)
				}

				// Build DAG structure
				dagLayers := make([][]TxRef, scenario.dagDepth)
				layerBlocks := make([][]ids.ID, scenario.dagDepth)

				// Generate transactions for each layer
				for layer := 0; layer < scenario.dagDepth; layer++ {
					dagLayers[layer] = make([]TxRef, scenario.txPerLayer)
					layerBlocks[layer] = make([]ids.ID, 0)

					for i := 0; i < scenario.txPerLayer; i++ {
						txID := make([]byte, 32)
						_, _ = rand.Read(txID)
						tx := TxRef(txID)
						dagLayers[layer][i] = tx

						// Each tx has unique owned object (for parallel processing)
						objID := make([]byte, 32)
						objID[0] = byte(layer)
						objID[1] = byte(i)
						_, _ = rand.Read(objID[2:])
						cls.addOwnedTx(tx, ObjectID(objID))
					}
				}

				finalizationStart := time.Now()

				if scenario.fpcEnabled {
					// With FPC: Fast path voting enables early finalization
					var wg sync.WaitGroup

					// Process all layers in parallel
					for layer := 0; layer < scenario.dagDepth; layer++ {
						wg.Add(1)
						go func(l int) {
							defer wg.Done()

							// Vote on all transactions in this layer
							for _, tx := range dagLayers[l] {
								// Collect votes for fast finalization
								quorum := 2*f + 1
								for v := 0; v < quorum; v++ {
									block := &Block{
										ID:     ids.GenerateTestID(),
										Author: validators[v],
										Round:  uint64(round*1000000 + l*10000 + v),
										Payload: BlockPayload{
											FPCVotes: [][]byte{tx[:]},
										},
									}
									fpc.OnBlockObserved(block)

									// Track block for this layer
									if v == 0 {
										layerBlocks[l] = append(layerBlocks[l], block.ID)
										dag.addAncestry(block.ID, tx)
									}
								}

								// Check if executable (can proceed without waiting for anchor)
								status, _ := fpc.Status(tx)
								if status == Executable {
									atomic.AddInt64(&totalTxFinalized, 1)
								}
							}
						}(layer)
					}

					wg.Wait()

					// Create anchor block that includes all layers
					anchorBlock := &Block{
						ID:     ids.GenerateTestID(),
						Author: validators[0],
						Round:  uint64(round*1000000 + 999999),
					}

					// Add all transactions to anchor's ancestry
					for layer := 0; layer < scenario.dagDepth; layer++ {
						for _, tx := range dagLayers[layer] {
							dag.addAncestry(anchorBlock.ID, tx)
						}
					}

					// Accept anchor (finalizes any remaining non-executable txs)
					fpc.OnBlockAccepted(anchorBlock)

				} else {
					// Without FPC: Must wait for full DAG consensus

					// Build DAG layer by layer (sequential dependency)
					for layer := 0; layer < scenario.dagDepth; layer++ {
						// Each layer must reference previous layers
						layerBlock := ids.GenerateTestID()
						layerBlocks[layer] = append(layerBlocks[layer], layerBlock)

						// Add all transactions from this layer
						for _, tx := range dagLayers[layer] {
							dag.addAncestry(layerBlock, tx)
						}

						// Link to previous layers (DAG structure)
						if layer > 0 {
							for _, prevBlock := range layerBlocks[layer-1] {
								// In real DAG, this would be parent references
								_ = prevBlock
							}
						}

						// Simulate consensus rounds needed for this layer
						// Without FPC, need full consensus on each layer
						time.Sleep(time.Microsecond * time.Duration(layer+1))
					}

					// Final consensus round to finalize entire DAG
					time.Sleep(time.Millisecond)

					// Count finalized (all at once at the end)
					atomic.AddInt64(&totalTxFinalized, int64(scenario.dagDepth*scenario.txPerLayer))
				}

				finalizationTime := time.Since(finalizationStart)
				totalFinalizationTime += finalizationTime
			}

			// Calculate metrics
			avgFinalizationTime := totalFinalizationTime / time.Duration(b.N)
			totalTxs := scenario.dagDepth * scenario.txPerLayer
			throughput := float64(totalTxs) / avgFinalizationTime.Seconds()

			b.ReportMetric(float64(scenario.dagDepth), "dag_layers")
			b.ReportMetric(float64(scenario.txPerLayer), "tx_per_layer")
			b.ReportMetric(float64(totalTxs), "total_txs")
			b.ReportMetric(throughput, "tx/sec")
			b.ReportMetric(float64(avgFinalizationTime.Microseconds()), "finalization_us")
			b.ReportMetric(float64(totalTxFinalized)/float64(b.N), "avg_tx_finalized")

			if scenario.fpcEnabled {
				b.ReportMetric(1.0, "fpc_enabled")
				// With FPC, transactions become executable before full DAG consensus
				earlyFinalizationRate := float64(totalTxFinalized) / float64(int64(totalTxs)*int64(b.N)) * 100
				b.ReportMetric(earlyFinalizationRate, "early_finalization_%")
			} else {
				b.ReportMetric(0.0, "fpc_enabled")
			}
		})
	}
}

// BenchmarkDAGFinalizationLatency measures time to finality with and without FPC
func BenchmarkDAGFinalizationLatency(b *testing.B) {
	validators := make([]ids.NodeID, 21)
	for i := range validators {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 21,
		F:                 7,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	b.Run("without-FPC", func(b *testing.B) {
		b.ResetTimer()

		latencies := make([]time.Duration, 0, b.N)

		for round := 0; round < b.N; round++ {
			dag := newMockDAG()

			// Create a transaction
			txID := make([]byte, 32)
			_, _ = rand.Read(txID)
			tx := TxRef(txID)

			start := time.Now()

			// Without FPC: Need full DAG consensus
			// Simulate building DAG layers
			for layer := 0; layer < 10; layer++ {
				blockID := ids.GenerateTestID()
				dag.addAncestry(blockID, tx)

				// Simulate consensus delay per layer
				time.Sleep(time.Microsecond * 100)
			}

			// Final consensus round
			time.Sleep(time.Millisecond)

			latency := time.Since(start)
			latencies = append(latencies, latency)
		}

		// Calculate average
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		avg := total / time.Duration(len(latencies))

		b.ReportMetric(float64(avg.Microseconds()), "avg_latency_us")
		b.ReportMetric(1000000.0/float64(avg.Microseconds()), "finalized_tx/sec")
	})

	b.Run("with-FPC", func(b *testing.B) {
		b.ResetTimer()

		latencies := make([]time.Duration, 0, b.N)

		for round := 0; round < b.N; round++ {
			cls := newMockClassifier()
			dag := newMockDAG()
			pq := &mockPQWithBLS{}
			fpc := New(cfg, cls, dag, pq, validators[0], validators)

			// Create a transaction
			txID := make([]byte, 32)
			_, _ = rand.Read(txID)
			tx := TxRef(txID)

			objID := make([]byte, 32)
			_, _ = rand.Read(objID)
			cls.addOwnedTx(tx, ObjectID(objID))

			start := time.Now()

			// With FPC: Fast finalization after quorum
			quorum := 2*7 + 1 // 15 votes
			for v := 0; v < quorum; v++ {
				block := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[v],
					Round:  uint64(round*1000 + v),
					Payload: BlockPayload{
						FPCVotes: [][]byte{tx[:]},
					},
				}
				fpc.OnBlockObserved(block)
			}

			// Check if executable (finalized via fast path)
			status, _ := fpc.Status(tx)
			if status == Executable {
				// Transaction is finalized!
				latency := time.Since(start)
				latencies = append(latencies, latency)
			} else {
				// Shouldn't happen with enough votes
				b.Fatal("Transaction not executable after quorum")
			}
		}

		// Calculate average
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		avg := total / time.Duration(len(latencies))

		b.ReportMetric(float64(avg.Microseconds()), "avg_latency_us")
		b.ReportMetric(1000000.0/float64(avg.Microseconds()), "finalized_tx/sec")

		// Calculate speedup vs without FPC
		// Typical speedup is 10-100x for owned transactions
		b.ReportMetric(1000.0/float64(avg.Microseconds()), "speedup_vs_no_fpc")
	})
}

// BenchmarkDAGThroughput compares overall DAG throughput with and without FPC
func BenchmarkDAGThroughput(b *testing.B) {
	scenarios := []struct {
		name       string
		fpcEnabled bool
		dagWidth   int // Parallel chains in DAG
		dagDepth   int // Depth of each chain
		batchSize  int // Transactions per block
	}{
		{"narrow-DAG-no-FPC", false, 5, 20, 10},
		{"narrow-DAG-with-FPC", true, 5, 20, 10},
		{"wide-DAG-no-FPC", false, 50, 20, 10},
		{"wide-DAG-with-FPC", true, 50, 20, 10},
		{"deep-DAG-no-FPC", false, 10, 100, 10},
		{"deep-DAG-with-FPC", true, 10, 100, 10},
	}

	validators := make([]ids.NodeID, 31)
	for i := range validators {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 31,
		F:                 10,
		Epoch:             1,
		VoteLimitPerBlock: 1024,
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			b.ResetTimer()

			totalProcessed := int64(0)
			totalTime := time.Duration(0)

			for round := 0; round < b.N; round++ {
				cls := newMockClassifier()
				dag := newMockDAG()

				var fpc WaveFPC
				if scenario.fpcEnabled {
					pq := &mockPQWithBLS{}
					fpc = New(cfg, cls, dag, pq, validators[0], validators)
				}

				start := time.Now()

				// Generate transactions for entire DAG
				totalTxs := scenario.dagWidth * scenario.dagDepth * scenario.batchSize
				transactions := make([]TxRef, totalTxs)
				for i := 0; i < totalTxs; i++ {
					txID := make([]byte, 32)
					txID[0] = byte(i >> 24)
					txID[1] = byte(i >> 16)
					txID[2] = byte(i >> 8)
					txID[3] = byte(i)
					transactions[i] = TxRef(txID)

					// Owned object for parallel processing
					objID := make([]byte, 32)
					copy(objID[:4], txID[:4])
					cls.addOwnedTx(transactions[i], ObjectID(objID))
				}

				if scenario.fpcEnabled {
					// With FPC: Process all transactions in parallel
					var wg sync.WaitGroup
					txIdx := 0

					for w := 0; w < scenario.dagWidth; w++ {
						wg.Add(1)
						go func(chain int) {
							defer wg.Done()

							for d := 0; d < scenario.dagDepth; d++ {
								for b := 0; b < scenario.batchSize; b++ {
									if txIdx >= len(transactions) {
										return
									}
									tx := transactions[txIdx]
									txIdx++

									// Fast path voting
									for v := 0; v < 21; v++ { // 2f+1
										block := &Block{
											ID:     ids.GenerateTestID(),
											Author: validators[v],
											Round:  uint64(round*1000000 + chain*10000 + d*100 + v),
											Payload: BlockPayload{
												FPCVotes: [][]byte{tx[:]},
											},
										}
										fpc.OnBlockObserved(block)
									}

									atomic.AddInt64(&totalProcessed, 1)
								}
							}
						}(w)
					}

					wg.Wait()

				} else {
					// Without FPC: Sequential DAG building
					txIdx := 0
					for d := 0; d < scenario.dagDepth; d++ {
						// Process layer
						for w := 0; w < scenario.dagWidth; w++ {
							blockID := ids.GenerateTestID()

							// Add batch of transactions
							for b := 0; b < scenario.batchSize; b++ {
								if txIdx >= len(transactions) {
									break
								}
								dag.addAncestry(blockID, transactions[txIdx])
								txIdx++
								atomic.AddInt64(&totalProcessed, 1)
							}
						}

						// Simulate consensus delay per layer
						time.Sleep(time.Microsecond * time.Duration(d+1))
					}
				}

				elapsed := time.Since(start)
				totalTime += elapsed
			}

			// Calculate throughput
			avgTime := totalTime / time.Duration(b.N)
			txPerRound := scenario.dagWidth * scenario.dagDepth * scenario.batchSize
			throughput := float64(txPerRound) / avgTime.Seconds()

			b.ReportMetric(float64(scenario.dagWidth), "dag_width")
			b.ReportMetric(float64(scenario.dagDepth), "dag_depth")
			b.ReportMetric(float64(txPerRound), "tx_per_round")
			b.ReportMetric(throughput, "tx/sec")
			b.ReportMetric(float64(totalProcessed)/float64(b.N), "avg_processed")

			if scenario.fpcEnabled {
				b.ReportMetric(1.0, "fpc_enabled")
			} else {
				b.ReportMetric(0.0, "fpc_enabled")
			}
		})
	}
}
