// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"sync"
	"testing"
	
	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
)

// mockClassifier for testing
type mockClassifier struct {
	ownedInputs  map[TxRef][]ObjectID
	sharedInputs map[TxRef][]ObjectID
	conflicts    map[TxRef]map[TxRef]bool
	mu           sync.RWMutex
}

func newMockClassifier() *mockClassifier {
	return &mockClassifier{
		ownedInputs:  make(map[TxRef][]ObjectID),
		sharedInputs: make(map[TxRef][]ObjectID),
		conflicts:    make(map[TxRef]map[TxRef]bool),
	}
}

func (m *mockClassifier) OwnedInputs(tx TxRef) []ObjectID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ownedInputs[tx]
}

func (m *mockClassifier) Conflicts(a, b TxRef) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if conflicts, ok := m.conflicts[a]; ok {
		return conflicts[b]
	}
	return false
}

func (m *mockClassifier) addOwnedTx(tx TxRef, objects ...ObjectID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ownedInputs[tx] = objects
}

func (m *mockClassifier) setConflict(a, b TxRef) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.conflicts[a] == nil {
		m.conflicts[a] = make(map[TxRef]bool)
	}
	if m.conflicts[b] == nil {
		m.conflicts[b] = make(map[TxRef]bool)
	}
	m.conflicts[a][b] = true
	m.conflicts[b][a] = true
}

// Note: mockDAG is defined in consensus_benchmark_test.go

func TestBasicFPCFlow(t *testing.T) {
	// Setup
	n := 4
	f := 1
	quorum := 2*f + 1 // Need 3 votes
	
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
	
	// Create a transaction with owned inputs
	tx1 := TxRef{1}
	obj1 := ObjectID{1}
	cls.addOwnedTx(tx1, obj1)
	
	// Test: Initial status should be Pending
	status, _ := fpc.Status(tx1)
	require.Equal(t, Pending, status)
	
	// Simulate votes from validators
	// Vote 1 from validator 0
	block1 := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[0],
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx1[:]},
		},
	}
	fpc.OnBlockObserved(block1)
	
	status, proof := fpc.Status(tx1)
	require.Equal(t, Pending, status)
	require.Equal(t, 1, proof.VoterCount)
	
	// Vote 2 from validator 1
	block2 := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[1],
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx1[:]},
		},
	}
	fpc.OnBlockObserved(block2)
	
	status, proof = fpc.Status(tx1)
	require.Equal(t, Pending, status)
	require.Equal(t, 2, proof.VoterCount)
	
	// Vote 3 from validator 2 - should reach quorum
	block3 := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[2],
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx1[:]},
		},
	}
	fpc.OnBlockObserved(block3)
	
	status, proof = fpc.Status(tx1)
	require.Equal(t, Executable, status, "Should be executable after %d votes", quorum)
	require.Equal(t, 3, proof.VoterCount)
	
	// Test anchoring: Accept a block that includes tx1 in ancestry
	dag.addAncestry(block3.ID, tx1)
	fpc.OnBlockAccepted(block3)
	
	status, _ = fpc.Status(tx1)
	require.Equal(t, Final, status, "Should be final after anchoring")
}

func TestConflictingTransactions(t *testing.T) {
	// Setup
	n := 4
	f := 1
	
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
	
	// Create two conflicting transactions (same object)
	tx1 := TxRef{1}
	tx2 := TxRef{2}
	obj1 := ObjectID{1}
	
	cls.addOwnedTx(tx1, obj1)
	cls.addOwnedTx(tx2, obj1)
	cls.setConflict(tx1, tx2)
	
	// Vote for tx1 from validators 0, 1, 2
	for i := 0; i < 3; i++ {
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i],
			Round:  uint64(i + 1),
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx1[:]},
			},
		}
		fpc.OnBlockObserved(block)
	}
	
	// tx1 should be executable
	status1, _ := fpc.Status(tx1)
	require.Equal(t, Executable, status1)
	
	// Now try to vote for conflicting tx2 from same validators
	// These votes should be ignored (equivocation)
	for i := 0; i < 3; i++ {
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i],
			Round:  uint64(i + 10),
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx2[:]},
			},
		}
		fpc.OnBlockObserved(block)
	}
	
	// tx2 should still be pending (votes were ignored)
	status2, proof2 := fpc.Status(tx2)
	require.Equal(t, Pending, status2)
	require.Equal(t, 0, proof2.VoterCount, "Conflicting votes should be ignored")
}

func TestEpochFence(t *testing.T) {
	// Setup
	n := 4
	f := 1
	
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
	
	myNode := validators[0]
	fpc := New(cfg, cls, dag, nil, myNode, validators).(*waveFPC)
	
	// Create a transaction
	tx1 := TxRef{1}
	obj1 := ObjectID{1}
	cls.addOwnedTx(tx1, obj1)
	
	// Start epoch close
	fpc.OnEpochCloseStart()
	
	// NextVotes should return empty during epoch pause
	votes := fpc.NextVotes(10)
	require.Empty(t, votes, "Should not vote during epoch pause")
	
	// Existing votes still count
	block := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[1],
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx1[:]},
		},
	}
	fpc.OnBlockObserved(block)
	
	status, proof := fpc.Status(tx1)
	require.Equal(t, Pending, status)
	require.Equal(t, 1, proof.VoterCount, "Votes still counted during epoch pause")
	
	// Simulate epoch close with EpochBit from 2f+1 validators
	for i := 0; i < 3; i++ {
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i],
			Round:  uint64(100 + i),
			Payload: BlockPayload{
				EpochBit: true,
			},
		}
		fpc.OnBlockAccepted(block)
	}
	
	// Complete epoch close
	fpc.OnEpochClosed()
	
	// Should be able to vote again
	fpc.epochPaused.Store(false) // Reset for test
	
	// Manually add tx1 as candidate (normally from mempool)
	// In real implementation, NextVotes would get from mempool
}

func TestMetrics(t *testing.T) {
	// Setup
	n := 4
	f := 1
	
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
	
	// Create and vote for a transaction
	tx1 := TxRef{1}
	obj1 := ObjectID{1}
	cls.addOwnedTx(tx1, obj1)
	
	// Get 3 votes (quorum)
	for i := 0; i < 3; i++ {
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i],
			Round:  1,
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx1[:]},
			},
		}
		fpc.OnBlockObserved(block)
	}
	
	// Check metrics
	metrics := fpc.GetMetrics()
	require.Equal(t, uint64(3), metrics.TotalVotes)
	require.Equal(t, uint64(1), metrics.ExecutableTxs)
	require.Equal(t, uint64(0), metrics.FinalTxs)
	
	// Anchor the transaction
	anchorBlock := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[0],
		Round:  10,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx1[:]},
		},
	}
	dag.addAncestry(anchorBlock.ID, tx1)
	fpc.OnBlockAccepted(anchorBlock)
	
	metrics = fpc.GetMetrics()
	require.Equal(t, uint64(1), metrics.FinalTxs)
}

// ============================================================================
// Benchmarks
// ============================================================================

func BenchmarkSingleOwnedTransaction(b *testing.B) {
	// Setup
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
	
	// Create owned transaction
	tx := TxRef{1}
	obj := ObjectID{1}
	cls.addOwnedTx(tx, obj)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
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
}

func BenchmarkConcurrentTransactions(b *testing.B) {
	// Setup large network
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
	
	// Create many non-conflicting transactions
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
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
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
			i++
		}
	})
}

func BenchmarkConflictDetection(b *testing.B) {
	// Setup
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
	
	// Create conflicting transactions
	sharedObj := ObjectID{1}
	numConflicts := 100
	conflictingTxs := make([]TxRef, numConflicts)
	
	for i := 0; i < numConflicts; i++ {
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
	
	for i := 0; i < b.N; i++ {
		tx := conflictingTxs[i%numConflicts]
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
}

func BenchmarkVoteAggregation(b *testing.B) {
	// Setup
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
	
	// Single transaction getting many votes
	tx := TxRef{1}
	obj := ObjectID{1}
	cls.addOwnedTx(tx, obj)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	// Benchmark vote aggregation for reaching quorum
	for i := 0; i < b.N; i++ {
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
}

func BenchmarkBlockProcessing(b *testing.B) {
	// Setup
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
	numTxs := 100
	txs := make([][]byte, numTxs)
	for i := 0; i < numTxs; i++ {
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		objID := ids.GenerateTestID()
		obj := ObjectID(objID[:])
		cls.addOwnedTx(tx, obj)
		txs[i] = tx[:]
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	// Benchmark processing blocks with many votes
	for i := 0; i < b.N; i++ {
		// Create block with multiple votes
		votesInBlock := txs[:10] // 10 votes per block
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i%n],
			Round:  uint64(i),
			Payload: BlockPayload{
				FPCVotes: votesInBlock,
			},
		}
		fpc.OnBlockObserved(block)
	}
}

func BenchmarkStatusQuery(b *testing.B) {
	// Setup
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
	
	// Populate with many transactions
	numTxs := 1000
	txs := make([]TxRef, numTxs)
	for i := 0; i < numTxs; i++ {
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		objID := ids.GenerateTestID()
		obj := ObjectID(objID[:])
		cls.addOwnedTx(tx, obj)
		txs[i] = tx
		
		// Vote for each transaction
		for v := 0; v < 67; v++ { // Get to quorum
			block := &Block{
				ID:     ids.GenerateTestID(),
				Author: validators[v],
				Round:  uint64(i),
				Payload: BlockPayload{
					FPCVotes: [][]byte{tx[:]},
				},
			}
			fpc.OnBlockObserved(block)
		}
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	// Benchmark status queries
	for i := 0; i < b.N; i++ {
		tx := txs[i%numTxs]
		_, _ = fpc.Status(tx)
	}
}

func BenchmarkMetricsCollection(b *testing.B) {
	// Setup
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
	
	// Populate with activity
	for i := 0; i < 100; i++ {
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		objID := ids.GenerateTestID()
		obj := ObjectID(objID[:])
		cls.addOwnedTx(tx, obj)
		
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
	
	b.ResetTimer()
	b.ReportAllocs()
	
	for i := 0; i < b.N; i++ {
		_ = fpc.GetMetrics()
	}
}

func BenchmarkWorstCaseEquivocation(b *testing.B) {
	// Setup with many validators trying to equivocate
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
	
	// Create conflicting transactions
	tx1 := TxRef{1}
	tx2 := TxRef{2}
	sharedObj := ObjectID{1}
	
	cls.addOwnedTx(tx1, sharedObj)
	cls.addOwnedTx(tx2, sharedObj)
	cls.setConflict(tx1, tx2)
	
	b.ResetTimer()
	b.ReportAllocs()
	
	// Benchmark detecting and handling equivocation
	for i := 0; i < b.N; i++ {
		var tx TxRef
		if i%2 == 0 {
			tx = tx1
		} else {
			tx = tx2
		}
		
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
}

func BenchmarkLargeBlockVotes(b *testing.B) {
	// Setup
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
	maxVotes := 256
	votes := make([][]byte, maxVotes)
	for i := 0; i < maxVotes; i++ {
		txID := ids.GenerateTestID()
		tx := TxRef(txID[:])
		objID := ids.GenerateTestID()
		obj := ObjectID(objID[:])
		cls.addOwnedTx(tx, obj)
		votes[i] = tx[:]
	}
	
	b.ResetTimer()
	b.ReportAllocs()
	
	// Benchmark processing blocks with maximum votes
	for i := 0; i < b.N; i++ {
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
}