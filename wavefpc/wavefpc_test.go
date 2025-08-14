// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"testing"
	
	"github.com/stretchr/testify/require"
	"github.com/luxfi/ids"
)

// mockClassifier for testing
type mockClassifier struct {
	ownedInputs map[TxRef][]ObjectID
	conflicts   map[TxRef]map[TxRef]bool
}

func newMockClassifier() *mockClassifier {
	return &mockClassifier{
		ownedInputs: make(map[TxRef][]ObjectID),
		conflicts:   make(map[TxRef]map[TxRef]bool),
	}
}

func (m *mockClassifier) OwnedInputs(tx TxRef) []ObjectID {
	return m.ownedInputs[tx]
}

func (m *mockClassifier) Conflicts(a, b TxRef) bool {
	if conflicts, ok := m.conflicts[a]; ok {
		return conflicts[b]
	}
	return false
}

func (m *mockClassifier) addOwnedTx(tx TxRef, objects ...ObjectID) {
	m.ownedInputs[tx] = objects
}

func (m *mockClassifier) setConflict(a, b TxRef) {
	if m.conflicts[a] == nil {
		m.conflicts[a] = make(map[TxRef]bool)
	}
	if m.conflicts[b] == nil {
		m.conflicts[b] = make(map[TxRef]bool)
	}
	m.conflicts[a][b] = true
	m.conflicts[b][a] = true
}

// mockDAG for testing
type mockDAG struct {
	ancestry map[ids.ID]map[TxRef]bool
}

func newMockDAG() *mockDAG {
	return &mockDAG{
		ancestry: make(map[ids.ID]map[TxRef]bool),
	}
}

func (m *mockDAG) InAncestry(blockID ids.ID, needleTx TxRef) bool {
	if txs, ok := m.ancestry[blockID]; ok {
		return txs[needleTx]
	}
	return false
}

func (m *mockDAG) GetBlockByAuthorRound(author ids.NodeID, round uint64) (ids.ID, bool) {
	return ids.Empty, false
}

func (m *mockDAG) addAncestry(blockID ids.ID, txs ...TxRef) {
	if m.ancestry[blockID] == nil {
		m.ancestry[blockID] = make(map[TxRef]bool)
	}
	for _, tx := range txs {
		m.ancestry[blockID][tx] = true
	}
}

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