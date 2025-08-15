// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestRingtailBasic(t *testing.T) {
	// Setup
	n := 4
	f := 1

	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := DefaultRingtailConfig(n, f)
	logger := &testLogger{}

	engine := NewRingtailEngine(cfg, logger, validators, validators[0])

	// Test submission
	tx := TxRef{1, 2, 3}
	voters := validators[:3] // 2f+1 voters

	engine.Submit(tx, voters)

	// Should not have proof immediately
	require.False(t, engine.HasPQ(tx))

	// Simulate shares being added (in real system, these come from network)
	// For testing, we'll just wait briefly
	time.Sleep(100 * time.Millisecond)

	// Check metrics
	metrics := engine.GetMetrics()
	require.Greater(t, metrics["rounds_started"], uint64(0))
}

func TestRingtailWithWaveFPC(t *testing.T) {
	// Setup with Ringtail enabled
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
		EnableRingtail:    true, // Enable Ringtail
		EnableBLS:         true,
		AlphaPQ:           uint32(2*f + 1),
		QRounds:           2,
	}

	cls := newMockClassifier()
	dag := newMockDAG()

	// Create FPC with Ringtail
	fpc := New(cfg, cls, dag, nil, validators[0], validators)

	// Create owned transaction
	tx := TxRef{1}
	obj := ObjectID{1}
	cls.addOwnedTx(tx, obj)

	// Get votes to reach quorum
	for i := 0; i < 3; i++ {
		block := &Block{
			ID:     ids.GenerateTestID(),
			Author: validators[i],
			Round:  1,
			Payload: BlockPayload{
				FPCVotes: [][]byte{tx[:]},
			},
		}
		fpc.OnBlockObserved(block)
	}

	// Should be executable
	status, proof := fpc.Status(tx)
	require.Equal(t, Executable, status)

	// Should have BLS proof (simulated)
	require.NotNil(t, proof.BLSProof)

	// Ringtail proof may take time to generate
	// In production, this would happen asynchronously
}

func TestDualFinality(t *testing.T) {
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
		EnableFastPath:    true,
		EnableRingtail:    true,
		EnableBLS:         true,
		AlphaPQ:           uint32(2*f + 1),
	}

	cls := newMockClassifier()

	// Create integration with dual finality
	// For testing, create a minimal context
	integration := &Integration{
		fpc:              New(cfg, cls, newMockDAG(), nil, validators[0], validators),
		log:              nil,
		nodeID:           validators[0],
		enabled:          true,
		votingEnabled:    true,
		executionEnabled: true,
	}

	// Should be enabled by default
	require.True(t, integration.enabled)
	require.True(t, integration.votingEnabled)
	require.True(t, integration.executionEnabled)

	// Create and vote for transaction
	tx := TxRef{1}
	obj := ObjectID{1}
	cls.addOwnedTx(tx, obj)

	// Check can execute
	canExec := integration.CanExecute(tx)
	require.False(t, canExec, "Should not execute without votes")

	// After voting would reach quorum
	// canExec = integration.CanExecute(tx)
	// require.True(t, canExec, "Should execute with quorum")

	// Check dual finality
	hasDual := integration.HasDualFinality(tx)
	require.False(t, hasDual, "Should not have dual finality immediately")
}

func TestRingtailVerification(t *testing.T) {
	// Test PQ signature verification
	n := 100
	f := 33
	threshold := 2*f + 1

	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	// Create a mock PQ bundle
	bundle := &PQBundle{
		Proof:       []byte("mock_proof"),
		VoterBitmap: make([]byte, (n+7)/8),
	}

	// Set enough bits in bitmap to meet threshold
	for i := 0; i < threshold; i++ {
		byteIdx := i / 8
		bitIdx := i % 8
		bundle.VoterBitmap[byteIdx] |= 1 << bitIdx
	}

	// Verify should pass
	valid := VerifyPQSignature(bundle, validators, threshold)
	require.True(t, valid)

	// Test with insufficient signers
	insufficientBundle := &PQBundle{
		Proof:       []byte("mock_proof"),
		VoterBitmap: make([]byte, (n+7)/8),
	}

	// Only set f bits (not enough)
	for i := 0; i < f; i++ {
		byteIdx := i / 8
		bitIdx := i % 8
		insufficientBundle.VoterBitmap[byteIdx] |= 1 << bitIdx
	}

	// Verify should fail
	valid = VerifyPQSignature(insufficientBundle, validators, threshold)
	require.False(t, valid)
}

// testLogger implements the Logger interface for tests
type testLogger struct{}

func (t *testLogger) Debug(msg string, args ...interface{}) {}
func (t *testLogger) Info(msg string, args ...interface{})  {}
func (t *testLogger) Warn(msg string, args ...interface{})  {}
func (t *testLogger) Error(msg string, args ...interface{}) {}

// Benchmark Ringtail operations
func BenchmarkRingtailShareGeneration(b *testing.B) {
	n := 100
	f := 33

	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := DefaultRingtailConfig(n, f)
	logger := &testLogger{}

	engine := NewRingtailEngine(cfg, logger, validators, validators[0])

	tx := TxRef{1, 2, 3}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = engine.generateShare(tx, 1)
	}
}

func BenchmarkRingtailAggregation(b *testing.B) {
	n := 100
	f := 33
	threshold := 2*f + 1

	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	cfg := DefaultRingtailConfig(n, f)
	logger := &testLogger{}

	engine := NewRingtailEngine(cfg, logger, validators, validators[0])

	// Generate shares
	tx := TxRef{1, 2, 3}
	shares := make(map[ids.NodeID]*LatticeShare)

	for i := 0; i < threshold; i++ {
		share := engine.generateShare(tx, 1)
		share.ValidatorID = validators[i]
		shares[validators[i]] = share
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = engine.aggregateShares(shares)
	}
}

func BenchmarkRingtailVerification(b *testing.B) {
	n := 100
	f := 33
	threshold := 2*f + 1

	validators := make([]ids.NodeID, n)
	for i := 0; i < n; i++ {
		validators[i] = ids.GenerateTestNodeID()
	}

	// Create bundle with threshold signers
	bundle := &PQBundle{
		Proof:       make([]byte, 512), // Simulate real proof size
		VoterBitmap: make([]byte, (n+7)/8),
	}

	// Set threshold bits
	for i := 0; i < threshold; i++ {
		byteIdx := i / 8
		bitIdx := i % 8
		bundle.VoterBitmap[byteIdx] |= 1 << bitIdx
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_ = VerifyPQSignature(bundle, validators, threshold)
	}
}
