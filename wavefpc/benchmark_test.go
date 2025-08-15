// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// BenchmarkThroughput measures vote processing throughput
func BenchmarkThroughput(b *testing.B) {
	sizes := []int{10, 100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size%d", size), func(b *testing.B) {
			n := 100
			f := 33
			validators := make([]ids.NodeID, n)
			for i := 0; i < n; i++ {
				validators[i] = ids.GenerateTestNodeID()
			}

			cfg := Config{
				N:                 n,
				F:                 f,
				Epoch:             1,
				VoteLimitPerBlock: 256,
			}

			cls := newMockClassifier()
			dag := newMockDAG()
			fpc := New(cfg, cls, dag, nil, validators[0], validators)

			// Create transactions
			txs := make([]TxRef, size)
			for i := 0; i < size; i++ {
				txID := ids.GenerateTestID()
				tx := TxRef(txID[:])
				objID := ids.GenerateTestID()
				obj := ObjectID(objID[:])
				cls.addOwnedTx(tx, obj)
				txs[i] = tx
			}

			b.ResetTimer()
			b.ReportAllocs()

			start := time.Now()
			processed := int64(0)

			for i := 0; i < b.N; i++ {
				tx := txs[i%size]
				block := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[i%n],
					Round:  uint64(i),
					Payload: BlockPayload{
						FPCVotes: [][]byte{tx[:]},
					},
				}
				fpc.OnBlockObserved(block)
				atomic.AddInt64(&processed, 1)
			}

			elapsed := time.Since(start)
			throughput := float64(processed) / elapsed.Seconds()
			b.ReportMetric(throughput, "votes/sec")
		})
	}
}

// BenchmarkLatency measures end-to-end latency from vote to decision
func BenchmarkLatency(b *testing.B) {
	n := 100
	f := 33
	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 n,
		F:                 f,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	dag := newMockDAG()
	fpc := New(cfg, cls, dag, nil, validators[0], validators)

	b.ResetTimer()
	b.ReportAllocs()

	latencies := make([]time.Duration, 0, b.N)

	for i := 0; i < b.N; i++ {
		// Create new transaction
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		objID := ids.GenerateTestID()
		obj := ObjectID(objID[:])
		cls.addOwnedTx(tx, obj)

		start := time.Now()

		// Vote until quorum (67 votes needed)
		for v := 0; v < 67; v++ {
			block := &Block{
				ID:     ids.GenerateTestID(),
				Author: validators[v],
				Round:  uint64(i*100 + v),
				Payload: BlockPayload{
					FPCVotes: [][]byte{tx[:]},
				},
			}
			fpc.OnBlockObserved(block)
		}

		// Check status
		status, _ := fpc.Status(tx)
		if status == Executable {
			latency := time.Since(start)
			latencies = append(latencies, latency)
		}
	}

	// Calculate average latency
	if len(latencies) > 0 {
		var total time.Duration
		for _, l := range latencies {
			total += l
		}
		avgLatency := total / time.Duration(len(latencies))
		b.ReportMetric(float64(avgLatency.Nanoseconds()), "ns/decision")
	}
}

// BenchmarkParallelVoting simulates parallel voting from multiple validators
func BenchmarkParallelVoting(b *testing.B) {
	n := 100
	f := 33
	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 n,
		F:                 f,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	dag := newMockDAG()
	fpc := New(cfg, cls, dag, nil, validators[0], validators)

	// Create many transactions
	numTxs := 10000
	txs := make([]TxRef, numTxs)
	for i := 0; i < numTxs; i++ {
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		objID := ids.GenerateTestID()
		obj := ObjectID(objID[:])
		cls.addOwnedTx(tx, obj)
		txs[i] = tx
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Use goroutines to simulate parallel validators
	numGoroutines := 10
	votesPerGoroutine := b.N / numGoroutines

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	start := time.Now()
	totalVotes := int64(0)

	for g := 0; g < numGoroutines; g++ {
		go func(validatorIdx int) {
			defer wg.Done()

			for i := 0; i < votesPerGoroutine; i++ {
				tx := txs[i%numTxs]
				block := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[validatorIdx%n],
					Round:  uint64(i),
					Payload: BlockPayload{
						FPCVotes: [][]byte{tx[:]},
					},
				}
				fpc.OnBlockObserved(block)
				atomic.AddInt64(&totalVotes, 1)
			}
		}(g)
	}

	wg.Wait()
	elapsed := time.Since(start)

	throughput := float64(totalVotes) / elapsed.Seconds()
	b.ReportMetric(throughput, "parallel-votes/sec")
}

// BenchmarkConflictResolution measures performance under high conflict
func BenchmarkConflictResolution(b *testing.B) {
	conflictRatios := []float64{0.1, 0.3, 0.5, 0.7, 0.9}

	for _, ratio := range conflictRatios {
		b.Run(fmt.Sprintf("Conflict%.0f", ratio*100), func(b *testing.B) {
			n := 100
			f := 33
			validators := make([]ids.NodeID, n)
			for i := 0; i < n; i++ {
				validators[i] = ids.GenerateTestNodeID()
			}

			cfg := Config{
				N:                 n,
				F:                 f,
				Epoch:             1,
				VoteLimitPerBlock: 256,
			}

			cls := newMockClassifier()
			dag := newMockDAG()
			fpc := New(cfg, cls, dag, nil, validators[0], validators)

			// Create conflicting transaction sets
			numSets := 100
			sharedObjects := make([]ObjectID, numSets)
			txSets := make([][]TxRef, numSets)

			for s := 0; s < numSets; s++ {
				objID := ids.GenerateTestID()
				sharedObjects[s] = ObjectID(objID[:])

				// Create conflicting transactions for this object
				numConflicting := int(float64(10) * ratio)
				if numConflicting < 2 {
					numConflicting = 2
				}

				txSets[s] = make([]TxRef, numConflicting)
				for c := 0; c < numConflicting; c++ {
					txID := ids.GenerateTestID()
					tx := TxRef(txID[:])
					cls.addOwnedTx(tx, sharedObjects[s])
					txSets[s][c] = tx

					// Set conflicts
					for prev := 0; prev < c; prev++ {
						cls.setConflict(tx, txSets[s][prev])
					}
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				setIdx := i % numSets
				txIdx := i % len(txSets[setIdx])
				tx := txSets[setIdx][txIdx]

				block := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[i%n],
					Round:  uint64(i),
					Payload: BlockPayload{
						FPCVotes: [][]byte{tx[:]},
					},
				}
				fpc.OnBlockObserved(block)
			}
		})
	}
}

// BenchmarkMemoryUsage measures memory consumption under load
func BenchmarkMemoryUsage(b *testing.B) {
	txCounts := []int{1000, 10000, 100000}

	for _, count := range txCounts {
		b.Run(fmt.Sprintf("Txs%d", count), func(b *testing.B) {
			n := 100
			f := 33
			validators := make([]ids.NodeID, n)
			for i := 0; i < n; i++ {
				validators[i] = ids.GenerateTestNodeID()
			}

			cfg := Config{
				N:                 n,
				F:                 f,
				Epoch:             1,
				VoteLimitPerBlock: 256,
			}

			cls := newMockClassifier()
			dag := newMockDAG()
			fpc := New(cfg, cls, dag, nil, validators[0], validators)

			// Create transactions
			txs := make([]TxRef, count)
			for i := 0; i < count; i++ {
				txID := ids.GenerateTestID()
				tx := TxRef(txID[:])
				objID := ids.GenerateTestID()
				obj := ObjectID(objID[:])
				cls.addOwnedTx(tx, obj)
				txs[i] = tx
			}

			b.ResetTimer()
			b.ReportAllocs()

			// Process votes for all transactions
			for i := 0; i < b.N; i++ {
				tx := txs[i%count]
				block := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[i%n],
					Round:  uint64(i),
					Payload: BlockPayload{
						FPCVotes: [][]byte{tx[:]},
					},
				}
				fpc.OnBlockObserved(block)
			}

			// Report allocations per transaction
			if b.N > 0 {
				allocsPerTx := float64(b.N) / float64(count)
				b.ReportMetric(allocsPerTx, "allocs/tx")
			}
		})
	}
}

// BenchmarkScalability tests performance with varying network sizes
func BenchmarkScalability(b *testing.B) {
	networkSizes := []struct {
		n int
		f int
	}{
		{10, 3},
		{50, 16},
		{100, 33},
		{200, 66},
		{500, 166},
	}

	for _, size := range networkSizes {
		b.Run(fmt.Sprintf("N%d", size.n), func(b *testing.B) {
			validators := make([]ids.NodeID, size.n)
			for i := 0; i < size.n; i++ {
				validators[i] = ids.GenerateTestNodeID()
			}

			cfg := Config{
				N:                 size.n,
				F:                 size.f,
				Epoch:             1,
				VoteLimitPerBlock: 256,
			}

			cls := newMockClassifier()
			dag := newMockDAG()
			fpc := New(cfg, cls, dag, nil, validators[0], validators)

			// Create test transaction
			txID := ids.GenerateTestID()
			tx := TxRef(txID[:])
			objID := ids.GenerateTestID()
			obj := ObjectID(objID[:])
			cls.addOwnedTx(tx, obj)

			b.ResetTimer()
			b.ReportAllocs()

			quorum := 2*size.f + 1
			votesProcessed := 0

			for i := 0; i < b.N; i++ {
				// Vote from different validators
				validatorIdx := i % size.n
				block := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[validatorIdx],
					Round:  uint64(i),
					Payload: BlockPayload{
						FPCVotes: [][]byte{tx[:]},
					},
				}
				fpc.OnBlockObserved(block)
				votesProcessed++

				// Check if quorum reached
				if votesProcessed == quorum {
					status, _ := fpc.Status(tx)
					if status == Executable {
						b.ReportMetric(float64(votesProcessed), "votes-to-quorum")
					}
					// Reset for next round
					votesProcessed = 0
				}
			}
		})
	}
}

// BenchmarkEpochTransition measures performance during epoch changes
func BenchmarkEpochTransition(b *testing.B) {
	n := 100
	f := 33
	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 n,
		F:                 f,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	dag := newMockDAG()
	fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)

	// Create transactions
	numTxs := 1000
	txs := make([]TxRef, numTxs)
	for i := 0; i < numTxs; i++ {
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		objID := ids.GenerateTestID()
		obj := ObjectID(objID[:])
		cls.addOwnedTx(tx, obj)
		txs[i] = tx
	}

	b.ResetTimer()
	b.ReportAllocs()

	epochSize := 1000
	epochCount := 0

	for i := 0; i < b.N; i++ {
		// Simulate epoch transition
		if i%epochSize == 0 && i > 0 {
			epochCount++

			// Start epoch close
			fpc.OnEpochCloseStart()

			// Simulate epoch bit from validators
			for v := 0; v < 67; v++ { // 2f+1
				block := &Block{
					ID:     ids.GenerateTestID(),
					Author: validators[v],
					Round:  uint64(i + v),
					Payload: BlockPayload{
						EpochBit: true,
					},
				}
				fpc.OnBlockAccepted(block)
			}

			// Complete epoch close
			fpc.OnEpochClosed()
			fpc.epochPaused.Store(false) // Resume for test
		}

		// Normal voting
		tx := txs[i%numTxs]
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i%n],
			Round:  uint64(i),
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx[:]},
			},
		}
		fpc.OnBlockObserved(block)
	}

	if epochCount > 0 {
		b.ReportMetric(float64(epochCount), "epochs")
	}
}

// BenchmarkWorstCase tests performance under worst-case conditions
func BenchmarkWorstCase(b *testing.B) {
	n := 100
	f := 33
	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := Config{
		N:                 n,
		F:                 f,
		Epoch:             1,
		VoteLimitPerBlock: 256,
	}

	cls := newMockClassifier()
	dag := newMockDAG()
	fpc := New(cfg, cls, dag, nil, validators[0], validators)

	// Create maximum conflicts - all transactions conflict with each other
	sharedObjID := ids.GenerateTestID()
	sharedObj := ObjectID(sharedObjID[:])

	numTxs := 100
	conflictingTxs := make([]TxRef, numTxs)

	for i := 0; i < numTxs; i++ {
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		cls.addOwnedTx(tx, sharedObj)
		conflictingTxs[i] = tx

		// Set conflicts with all previous
		for j := 0; j < i; j++ {
			cls.setConflict(tx, conflictingTxs[j])
		}
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Process with maximum vote batches
	maxVotes := 256
	votes := make([][]byte, maxVotes)

	for i := 0; i < b.N; i++ {
		// Fill vote batch with conflicting transactions
		for v := 0; v < maxVotes; v++ {
			tx := conflictingTxs[(i+v)%numTxs]
			votes[v] = tx[:]
		}

		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i%n],
			Round:  uint64(i),
			Payload: BlockPayload{
				FPCVotes: votes,
			},
		}
		fpc.OnBlockObserved(block)
	}

	b.ReportMetric(float64(maxVotes), "votes/block")
}
