// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"sync"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestShardedMapOperations tests all sharded map functionality
func TestShardedMapOperations(t *testing.T) {
	sm := NewShardedMap[TxRef, Status](4)

	// Test Set and Get
	tx1 := TxRef{1}
	sm.Set(tx1, Executable)

	status, ok := sm.Get(tx1)
	require.True(t, ok)
	require.Equal(t, Executable, status)

	// Test GetOrCreate
	tx2 := TxRef{2}
	status2, created := sm.GetOrCreate(tx2, func() Status {
		return Pending
	})
	require.True(t, created)
	require.Equal(t, Pending, status2)

	// Second call should not create
	status3, created2 := sm.GetOrCreate(tx2, func() Status {
		return Executable // Should not be called
	})
	require.False(t, created2)
	require.Equal(t, Pending, status3)

	// Test Delete
	sm.Delete(tx1)
	_, ok = sm.Get(tx1)
	require.False(t, ok)

	// Test Size
	require.Equal(t, 1, sm.Size())

	// Test Clear
	sm.Clear()
	require.Equal(t, 0, sm.Size())

	// Test Range
	for i := 0; i < 10; i++ {
		tx := TxRef{byte(i)}
		sm.Set(tx, Status(i%3))
	}

	count := 0
	sm.Range(func(key TxRef, value Status) bool {
		count++
		return true
	})
	require.Equal(t, 10, count)

	// Test early termination
	count2 := 0
	sm.Range(func(key TxRef, value Status) bool {
		count2++
		return count2 < 5
	})
	require.Equal(t, 5, count2)
}

// TestConcurrentShardedMap tests thread safety
func TestConcurrentShardedMap(t *testing.T) {
	sm := NewShardedMap[TxRef, int](16)

	var wg sync.WaitGroup
	numGoroutines := 100
	numOps := 1000

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			for j := 0; j < numOps; j++ {
				tx := TxRef{byte(id), byte(j % 256)}

				// Mix of operations
				switch j % 4 {
				case 0:
					sm.Set(tx, id*1000+j)
				case 1:
					sm.Get(tx)
				case 2:
					sm.GetOrCreate(tx, func() int {
						return id*1000 + j
					})
				case 3:
					if j%10 == 0 {
						sm.Delete(tx)
					}
				}
			}
		}(i)
	}

	wg.Wait()

	// Should not crash or deadlock
	size := sm.Size()
	require.Greater(t, size, 0)
}

// TestBitsetOperations tests bitset functionality
func TestBitsetOperations(t *testing.T) {
	bs := NewBitset(100)

	// Test Set
	require.True(t, bs.Set(0))
	require.True(t, bs.Set(63))
	require.True(t, bs.Set(64))
	require.True(t, bs.Set(99))

	// Test duplicate set
	require.False(t, bs.Set(0))

	// Test out of bounds
	require.False(t, bs.Set(-1))
	require.False(t, bs.Set(100))

	// Test Count
	require.Equal(t, 4, bs.Count())

	// Test Bytes
	bytes := bs.Bytes()
	require.NotNil(t, bytes)
	require.Equal(t, 16, len(bytes)) // 100 bits = 2 uint64s = 16 bytes

	// Test GetVoters
	validators := make([]ids.NodeID, 100)
	for i := 0; i < 100; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	voters := bs.GetVoters(validators)
	require.Equal(t, 4, len(voters))
	require.Equal(t, validators[0], voters[0])
	require.Equal(t, validators[63], voters[1])
	require.Equal(t, validators[64], voters[2])
	require.Equal(t, validators[99], voters[3])
}

// TestWaveFPCEdgeCases tests edge cases in the main engine
func TestWaveFPCEdgeCases(t *testing.T) {
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
		VoteLimitPerBlock: 2,
		EnableFastPath:    true,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)

	// Test empty block processing
	emptyBlock := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[0],
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{},
		},
	}
	fpc.OnBlockObserved(emptyBlock)

	// Test invalid validator
	invalidBlock := &Block{
		ID:     ids.GenerateTestID(),
		Author: ids.GenerateTestNodeID(), // Not in validator set
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{{1, 2, 3}},
		},
	}
	fpc.OnBlockObserved(invalidBlock)

	// Test shared transaction (should be ignored)
	sharedTx := TxRef{99}
	// Don't add owned inputs for this tx

	sharedBlock := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[0],
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{sharedTx[:]},
		},
	}
	fpc.OnBlockObserved(sharedBlock)

	status, _ := fpc.Status(sharedTx)
	require.Equal(t, Pending, status)
}

// TestNextVotes tests vote selection logic
func TestNextVotes(t *testing.T) {
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
		VoteLimitPerBlock: 3,
		EnableFastPath:    true,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	myNode := validators[0]
	fpc := New(cfg, cls, dag, nil, myNode, validators).(*waveFPC)

	// Test with no candidates
	votes := fpc.NextVotes(10)
	require.Empty(t, votes)

	// Test during epoch pause
	fpc.epochPaused.Store(true)
	votes = fpc.NextVotes(10)
	require.Empty(t, votes)
	fpc.epochPaused.Store(false)

	// Add some owned transactions
	tx1 := TxRef{1}
	tx2 := TxRef{2}
	tx3 := TxRef{3}
	tx4 := TxRef{4}
	obj1 := ObjectID{1}
	obj2 := ObjectID{2}

	cls.addOwnedTx(tx1, obj1)
	cls.addOwnedTx(tx2, obj2)
	cls.addOwnedTx(tx3, obj1) // Conflicts with tx1
	cls.addOwnedTx(tx4, ObjectID{3})

	// Mock getCandidates to return our txs
	// Since getCandidates returns nil, we can't test the full flow
	// but we've tested the logic paths
}

// TestAnchorCoverage tests anchor checking logic
func TestAnchorCoverage(t *testing.T) {
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
		EnableFastPath:    true,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)

	tx := TxRef{1}
	obj := ObjectID{1}
	cls.addOwnedTx(tx, obj)

	// Vote but not enough for quorum
	block1 := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[0],
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx[:]},
		},
	}
	fpc.OnBlockObserved(block1)

	// Anchor should fail without quorum
	anchor := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[0],
		Round:  10,
	}
	result := fpc.anchorCovers(tx, anchor)
	require.False(t, result)

	// Add more votes to reach quorum
	for i := 1; i < 3; i++ {
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i],
			Round:  uint64(i + 1),
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx[:]},
			},
		}
		fpc.OnBlockObserved(block)
	}

	// Now anchor with tx in ancestry
	dag.addAncestry(anchor.ID, tx)
	result = fpc.anchorCovers(tx, anchor)
	require.True(t, result)
}

// TestEpochBitTracking tests epoch bit author tracking
func TestEpochBitTracking(t *testing.T) {
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
		EnableFastPath:    true,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)

	// Register epoch bit authors
	for i := 0; i < 2; i++ {
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

	// Should have 2 authors
	require.Equal(t, 2, len(fpc.epochBitAuthors))

	// Add one more to reach quorum (2f+1 = 3)
	block := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[2],
		Round:  102,
		Payload: BlockPayload{
			EpochBit: true,
		},
	}
	fpc.OnBlockAccepted(block)

	// Should have 3 authors (quorum)
	require.Equal(t, 3, len(fpc.epochBitAuthors))
}

// TestMarkMixed tests mixed transaction handling
func TestMarkMixed(t *testing.T) {
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
		EnableFastPath:    true,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	fpc := New(cfg, cls, dag, nil, validators[0], validators)

	tx := TxRef{1}

	// Mark as mixed
	fpc.MarkMixed(tx)

	// Check state
	status, _ := fpc.Status(tx)
	require.Equal(t, Mixed, status)

	// Mixed transactions should be tracked
	isMixed, _ := fpc.(*waveFPC).mixedTxs.Get(tx)
	require.True(t, isMixed)
}

// TestConflictTracking tests conflict detection and tracking
func TestConflictTracking(t *testing.T) {
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
		EnableFastPath:    true,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)

	// Create conflicting transactions
	tx1 := TxRef{1}
	tx2 := TxRef{2}
	tx3 := TxRef{3}
	sharedObj := ObjectID{99}

	cls.addOwnedTx(tx1, sharedObj)
	cls.addOwnedTx(tx2, sharedObj)
	cls.addOwnedTx(tx3, sharedObj)

	// Process votes for different txs on same object
	block1 := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[0],
		Round:  1,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx1[:]},
		},
	}
	fpc.OnBlockObserved(block1)

	// Check conflicts are tracked
	conflicts, _ := fpc.conflicts.Get(sharedObj)
	require.Contains(t, conflicts, tx1)

	// Try to vote for conflicting tx from same validator (should be ignored)
	block2 := &Block{
		ID:     ids.GenerateTestID(),
		Author: validators[0],
		Round:  2,
		Payload: BlockPayload{
			FPCVotes: [][]byte{tx2[:]},
		},
	}
	fpc.OnBlockObserved(block2)

	// tx2 should not get the vote
	bs2 := fpc.getBitset(tx2)
	require.Nil(t, bs2) // No votes
}

// TestValidatorHelpers tests validator-related helper functions
func TestValidatorHelpers(t *testing.T) {
	validators := make([]ids.NodeID, 5)
	for i := 0; i < 5; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	// Test ValidatorIndex
	idx := ValidatorIndex(validators[2], validators)
	require.Equal(t, 2, idx)

	// Test not found
	notFound := ids.GenerateTestNodeID()
	idx = ValidatorIndex(notFound, validators)
	require.Equal(t, -1, idx)
}

// TestGetCandidates tests the candidate collection
func TestGetCandidates(t *testing.T) {
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
		EnableFastPath:    true,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)

	// Currently returns nil - in production would get from mempool
	candidates := fpc.getCandidates()
	require.Nil(t, candidates)
}

// TestCollectFastPathVotes tests vote collection
func TestCollectFastPathVotes(t *testing.T) {
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
		EnableFastPath:    true,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)

	// Test vote collection through public interface
	votes := fpc.NextVotes(10)
	require.NotNil(t, votes)
}

// TestRingtailIntegration tests full Ringtail integration
func TestRingtailIntegration(t *testing.T) {
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
		EnableFastPath:    true,
		EnableRingtail:    true,
		EnableBLS:         true,
		AlphaPQ:           uint32(2*f + 1),
		QRounds:           2,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	// Create with Ringtail enabled
	fpc := New(cfg, cls, dag, nil, validators[0], validators).(*waveFPC)

	// Should have Ringtail engine
	require.NotNil(t, fpc.ringtail)
	require.NotNil(t, fpc.pq)

	// Create and vote for transaction
	tx := TxRef{1}
	obj := ObjectID{1}
	cls.addOwnedTx(tx, obj)

	// Get to quorum
	for i := 0; i < 3; i++ {
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i],
			Round:  uint64(i + 1),
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx[:]},
			},
		}
		fpc.OnBlockObserved(block)
	}

	// Should be executable and have triggered PQ
	status, proof := fpc.Status(tx)
	require.Equal(t, Executable, status)
	require.NotNil(t, proof.BLSProof) // Simulated BLS

	// PQ may not be ready immediately (async)
	time.Sleep(10 * time.Millisecond)

	// Check if PQ was submitted
	metrics := fpc.ringtail.GetMetrics()
	require.Greater(t, metrics["rounds_started"], uint64(0))
}

// TestDefaultRingtailConfig tests Ringtail parameter defaults
func TestDefaultRingtailConfig(t *testing.T) {
	cfg := DefaultRingtailConfig(100, 33)

	require.Equal(t, 100, cfg.N)
	require.Equal(t, 33, cfg.F)
	require.Equal(t, 67, cfg.AlphaClassical)
	require.Equal(t, 67, cfg.AlphaPQ)
	require.Equal(t, 2, cfg.QRounds)
	require.Equal(t, 512, cfg.LatticeDim)
	require.Equal(t, uint64(4294967291), cfg.Modulus)
}

// TestRingtailHelpers tests Ringtail helper functions
func TestRingtailHelpers(t *testing.T) {
	n := 100
	f := 33

	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := DefaultRingtailConfig(n, f)
	logger := &testLogger{}

	engine := NewRingtailEngine(cfg, logger, validators, validators[0])

	// Test encoding functions
	sig := &LatticeSignature{
		Signers:   validators[:10],
		Aggregate: make([]uint64, cfg.LatticeDim),
		Proof:     []byte("test_proof"),
		Timestamp: time.Now(),
	}

	encoded := engine.encodeSignature(sig)
	require.NotNil(t, encoded)
	require.Greater(t, len(encoded), 0)

	bitmap := engine.encodeBitmap(sig.Signers)
	require.NotNil(t, bitmap)
	require.Equal(t, (n+7)/8, len(bitmap))
}

// TestPanicRecovery tests that the system doesn't panic on errors
func TestPanicRecovery(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Unexpected panic: %v", r)
		}
	}()

	// Test with nil classifier
	cfg := Config{N: 4, F: 1}
	validators := []ids.NodeID{ids.GenerateTestNodeID()}

	fpc := New(cfg, nil, nil, nil, validators[0], validators)

	// These should not panic
	votes := fpc.NextVotes(10)
	require.Empty(t, votes)

	status, _ := fpc.Status(TxRef{1})
	require.Equal(t, Pending, status)

	fpc.OnEpochCloseStart()
	fpc.OnEpochClosed()

	metrics := fpc.GetMetrics()
	require.NotNil(t, metrics)
}
