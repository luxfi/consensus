package pq

import (
	"context"
	"reflect"
	"testing"
	"time"
	"unsafe"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestConsensusEngine(t *testing.T) {
	params := config.LocalParams()
	engine := NewConsensus(params)

	require.NotNil(t, engine)
	require.NotNil(t, engine.quasar)
	require.NotNil(t, engine.finality)
}

func TestConsensusInitialize(t *testing.T) {
	ctx := context.Background()
	engine := NewConsensus(config.LocalParams())

	blsKey := make([]byte, 48)
	pqKey := make([]byte, 256)

	err := engine.Initialize(ctx, blsKey, pqKey)
	require.NoError(t, err)
}

func TestConsensusProcessBlock(t *testing.T) {
	ctx := context.Background()
	engine := NewConsensus(config.LocalParams())

	blockID := ids.GenerateTestID()
	// votes map is block -> vote count, not node -> vote
	// We need one block with enough votes
	votes := map[string]int{
		blockID.String(): 5, // 5 votes for this block
	}

	// Process block should succeed with sufficient votes
	err := engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err)
}

func TestConsensusFinalization(t *testing.T) {
	ctx := context.Background()
	engine := NewConsensus(config.LocalParams())

	// Initialize engine
	err := engine.Initialize(ctx, make([]byte, 48), make([]byte, 256))
	require.NoError(t, err)

	// Process enough blocks to trigger finalization
	blockID := ids.GenerateTestID()
	// votes map is block -> vote count
	votes := map[string]int{
		blockID.String(): 5, // 5 votes for this block (sufficient for local params)
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err)

	// Check if block is finalized (with sufficient votes)
	finalized := engine.IsFinalized(blockID)
	require.True(t, finalized)
}

func TestConsensusFinalityChannel(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	ch := engine.FinalityChannel()
	require.NotNil(t, ch)

	// Test that we can receive from the channel
	select {
	case <-ch:
		// Got an event
	case <-time.After(10 * time.Millisecond):
		// Timeout is ok, channel exists
	}
}

func TestConsensusHeight(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	height := engine.Height()
	require.Equal(t, uint64(0), height)

	// Process a block to increase height
	engine.height = 100

	height = engine.Height()
	require.Equal(t, uint64(100), height)
}

func TestConsensusMetrics(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	metrics := engine.Metrics()
	require.NotNil(t, metrics)
	require.Contains(t, metrics, "height")
	require.Contains(t, metrics, "round")
	require.Contains(t, metrics, "finalized")
}

func TestNewFunction(t *testing.T) {
	engine := New()
	require.NotNil(t, engine)
	require.IsType(t, &PostQuantum{}, engine)
}

func TestConsensusSetFinalizedCallback(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	engine.SetFinalizedCallback(func(event FinalityEvent) {
		// Callback is set
	})

	// Callback is set, we can't easily test it fires without complex setup
	require.NotNil(t, engine.quasar)
}

func BenchmarkConsensusProcessBlock(b *testing.B) {
	ctx := context.Background()
	engine := NewConsensus(config.LocalParams())

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		"node1": 1,
		"node2": 1,
		"node3": 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.ProcessBlock(ctx, blockID, votes)
	}
}

func BenchmarkConsensusIsFinalized(b *testing.B) {
	engine := NewConsensus(config.LocalParams())
	blockID := ids.GenerateTestID()
	engine.finalized[blockID] = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.IsFinalized(blockID)
	}
}

func TestProcessBlockContextCancellation(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	// Initialize engine
	err := engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))
	require.NoError(t, err)

	// Fill up the finality channel
	for i := 0; i < 100; i++ {
		engine.finality <- FinalityEvent{}
	}

	// Process a block - should handle full channel gracefully
	blockID := ids.GenerateTestID()
	votes := map[string]int{
		blockID.String(): 5,
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err)
}

func TestProcessBlockContextDone(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	// Initialize engine
	err := engine.Initialize(context.Background(), []byte("bls-key"), []byte("pq-key"))
	require.NoError(t, err)

	// Fill up the finality channel
	for i := 0; i < 100; i++ {
		engine.finality <- FinalityEvent{}
	}

	// Create cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Process a block with cancelled context
	blockID := ids.GenerateTestID()
	votes := map[string]int{
		blockID.String(): 5,
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	// Should return context error since channel is full and context is cancelled
	require.ErrorIs(t, err, context.Canceled)
}

func TestProcessBlockInsufficientVotesZeroTotal(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	blockID := ids.GenerateTestID()
	votes := map[string]int{} // Empty votes

	err := engine.ProcessBlock(ctx, blockID, votes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient votes")
}

func TestProcessBlockWithNilCertGen(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	// Don't initialize - certGen will be nil
	blockID := ids.GenerateTestID()
	votes := map[string]int{
		blockID.String(): 5,
	}

	err := engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err) // Should use fallback test keys
	require.True(t, engine.IsFinalized(blockID))
}

func TestProcessBlockAlreadyFinalized(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	blockID := ids.GenerateTestID()

	// Pre-mark as finalized
	engine.finalized[blockID] = true

	votes := map[string]int{
		blockID.String(): 5,
	}

	err := engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err) // Should return early without error
}

func TestProcessBlockEmptyCertificates(t *testing.T) {
	// This test verifies the certificate validation path
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	// Initialize with proper keys
	err := engine.Initialize(ctx, []byte("valid-bls-key"), []byte("valid-pq-key"))
	require.NoError(t, err)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		blockID.String(): 5,
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err)

	// Verify finality event was emitted
	select {
	case event := <-engine.FinalityChannel():
		require.Equal(t, blockID, event.BlockID)
		require.NotEmpty(t, event.PQProof)
		require.NotEmpty(t, event.BLSProof)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Expected finality event")
	}
}

func TestFinalityEventFields(t *testing.T) {
	event := FinalityEvent{
		Height:    100,
		BlockID:   ids.GenerateTestID(),
		Timestamp: time.Now(),
		PQProof:   []byte("pq-proof"),
		BLSProof:  []byte("bls-proof"),
	}

	require.Equal(t, uint64(100), event.Height)
	require.NotEmpty(t, event.BlockID)
	require.False(t, event.Timestamp.IsZero())
	require.Equal(t, []byte("pq-proof"), event.PQProof)
	require.Equal(t, []byte("bls-proof"), event.BLSProof)
}

func TestMetricsContent(t *testing.T) {
	params := config.LocalParams()
	engine := NewConsensus(params)

	// Set some state
	engine.height = 50
	engine.round = 25
	engine.finalized[ids.GenerateTestID()] = true
	engine.finalized[ids.GenerateTestID()] = true

	metrics := engine.Metrics()

	require.Equal(t, uint64(50), metrics["height"])
	require.Equal(t, uint64(25), metrics["round"])
	require.Equal(t, 2, metrics["finalized"])
	require.Equal(t, params.K, metrics["k"])
	require.Equal(t, params.Alpha, metrics["alpha"])
	require.Equal(t, params.Beta, metrics["beta"])
	require.Equal(t, params.BlockTime.String(), metrics["block_time"])
}

func TestIsFinalizedNotFound(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	blockID := ids.GenerateTestID()
	require.False(t, engine.IsFinalized(blockID))
}

func TestMultipleConcurrentProcessBlock(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	_ = engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))

	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(idx int) {
			blockID := ids.GenerateTestID()
			votes := map[string]int{
				blockID.String(): 5,
			}
			_ = engine.ProcessBlock(ctx, blockID, votes)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	require.Equal(t, uint64(10), engine.Height())
}

func TestProcessBlockBLSGenerationFailure(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	// Initialize with empty BLS key to trigger BLS generation failure
	err := engine.Initialize(ctx, nil, []byte("pq-key"))
	require.NoError(t, err)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		blockID.String(): 5,
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "BLS")
}

func TestProcessBlockPQGenerationFailure(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	// Initialize with valid BLS key but empty PQ key to trigger PQ generation failure
	err := engine.Initialize(ctx, []byte("bls-key"), nil)
	require.NoError(t, err)

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		blockID.String(): 5,
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "PQ")
}

func TestProcessBlockVotingEdgeCases(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	_ = engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))

	// Test with exactly at threshold
	blockID := ids.GenerateTestID()
	// LocalParams has Alpha=0.69, so need 69% votes
	votes := map[string]int{
		blockID.String(): 69,
		"other":          31, // 69/100 = 0.69 = Alpha exactly
	}

	err := engine.ProcessBlock(ctx, blockID, votes)
	require.NoError(t, err)
	require.True(t, engine.IsFinalized(blockID))
}

func TestProcessBlockVotingBelowThreshold(t *testing.T) {
	engine := NewConsensus(config.LocalParams())
	ctx := context.Background()

	_ = engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))

	blockID := ids.GenerateTestID()
	// LocalParams has Alpha=0.69, so 68% should fail
	votes := map[string]int{
		blockID.String(): 68,
		"other":          32, // 68/100 = 0.68 < 0.69 Alpha
	}

	err := engine.ProcessBlock(ctx, blockID, votes)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insufficient votes")
	require.False(t, engine.IsFinalized(blockID))
}

func TestSetFinalizedCallbackWithNilCert(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	callbackCalled := false
	var receivedEvent FinalityEvent

	engine.SetFinalizedCallback(func(event FinalityEvent) {
		callbackCalled = true
		receivedEvent = event
	})

	// The callback is set on quasar, verify quasar is properly configured
	require.NotNil(t, engine.quasar)

	// We can't directly trigger the internal callback from here,
	// but we verify the callback wrapping logic is set up
	_ = callbackCalled
	_ = receivedEvent
}

// TestBlockToFinalityEvent tests the blockToFinalityEvent helper function
func TestBlockToFinalityEvent(t *testing.T) {
	testTime := time.Now()
	validID := ids.GenerateTestID()
	validIDStr := validID.String()

	testCases := []struct {
		name    string
		block   *quasar.Block
		wantBLS []byte
		wantPQ  []byte
	}{
		{
			name: "block with certificate",
			block: &quasar.Block{
				Hash:      validIDStr,
				Height:    100,
				Timestamp: testTime,
				Cert: &quasar.BlockCert{
					BLS: []byte("bls-proof-data"),
					PQ:  []byte("pq-proof-data"),
				},
			},
			wantBLS: []byte("bls-proof-data"),
			wantPQ:  []byte("pq-proof-data"),
		},
		{
			name: "block without certificate",
			block: &quasar.Block{
				Hash:      validIDStr,
				Height:    50,
				Timestamp: testTime,
				Cert:      nil,
			},
			wantBLS: nil,
			wantPQ:  nil,
		},
		{
			name: "block with empty proofs in cert",
			block: &quasar.Block{
				Hash:      validIDStr,
				Height:    75,
				Timestamp: testTime,
				Cert: &quasar.BlockCert{
					BLS: []byte{},
					PQ:  []byte{},
				},
			},
			wantBLS: []byte{},
			wantPQ:  []byte{},
		},
		{
			name: "block with invalid hash",
			block: &quasar.Block{
				Hash:      "invalid-hash",
				Height:    25,
				Timestamp: testTime,
				Cert:      nil,
			},
			wantBLS: nil,
			wantPQ:  nil,
		},
		{
			name: "block with empty hash",
			block: &quasar.Block{
				Hash:      "",
				Height:    10,
				Timestamp: testTime,
				Cert:      nil,
			},
			wantBLS: nil,
			wantPQ:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := blockToFinalityEvent(tc.block)

			require.Equal(t, tc.block.Height, event.Height)
			require.Equal(t, tc.block.Timestamp, event.Timestamp)
			require.Equal(t, tc.wantBLS, event.BLSProof)
			require.Equal(t, tc.wantPQ, event.PQProof)
		})
	}
}

// TestFromStringBlockID tests ids.FromString behavior used in SetFinalizedCallback
func TestFromStringBlockID(t *testing.T) {
	// Generate a valid ID and get its string representation
	validID := ids.GenerateTestID()
	validIDStr := validID.String()

	testCases := []struct {
		name      string
		hash      string
		shouldErr bool
	}{
		{
			name:      "valid base58 ID",
			hash:      validIDStr,
			shouldErr: false,
		},
		{
			name:      "empty string",
			hash:      "",
			shouldErr: true,
		},
		{
			name:      "invalid base58",
			hash:      "!!!invalid!!!",
			shouldErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			blockID, err := ids.FromString(tc.hash)
			if tc.shouldErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotEqual(t, ids.Empty, blockID)
			}
		})
	}
}

// TestSetFinalizedCallbackInvocation tests that the callback is properly invoked
// by using reflection to access the private finalizedCb field in quasar.BLS
func TestSetFinalizedCallbackInvocation(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	callbackCalled := false
	var receivedEvent FinalityEvent

	engine.SetFinalizedCallback(func(event FinalityEvent) {
		callbackCalled = true
		receivedEvent = event
	})

	// Use reflection to access the private finalizedCb field
	blsValue := reflect.ValueOf(engine.quasar).Elem()
	finalizedCbField := blsValue.FieldByName("finalizedCb")

	// Make the field accessible using unsafe
	finalizedCbField = reflect.NewAt(finalizedCbField.Type(), unsafe.Pointer(finalizedCbField.UnsafeAddr())).Elem()

	// Get the callback function
	cb := finalizedCbField.Interface().(func(*quasar.Block))

	// Generate a valid ID for the test
	validID := ids.GenerateTestID()

	// Invoke the callback with a test block
	testBlock := &quasar.Block{
		Hash:      validID.String(),
		Height:    42,
		Timestamp: time.Now(),
		Cert: &quasar.BlockCert{
			BLS: []byte("test-bls"),
			PQ:  []byte("test-pq"),
		},
	}

	cb(testBlock)

	require.True(t, callbackCalled)
	require.Equal(t, uint64(42), receivedEvent.Height)
	require.Equal(t, []byte("test-bls"), receivedEvent.BLSProof)
	require.Equal(t, []byte("test-pq"), receivedEvent.PQProof)
}

// TestSetFinalizedCallbackInvocationNilCert tests callback with nil certificate
func TestSetFinalizedCallbackInvocationNilCert(t *testing.T) {
	engine := NewConsensus(config.LocalParams())

	callbackCalled := false
	var receivedEvent FinalityEvent

	engine.SetFinalizedCallback(func(event FinalityEvent) {
		callbackCalled = true
		receivedEvent = event
	})

	// Use reflection to access the private finalizedCb field
	blsValue := reflect.ValueOf(engine.quasar).Elem()
	finalizedCbField := blsValue.FieldByName("finalizedCb")

	// Make the field accessible using unsafe
	finalizedCbField = reflect.NewAt(finalizedCbField.Type(), unsafe.Pointer(finalizedCbField.UnsafeAddr())).Elem()

	// Get the callback function
	cb := finalizedCbField.Interface().(func(*quasar.Block))

	// Generate a valid ID for the test
	validID := ids.GenerateTestID()

	// Invoke the callback with a block that has no cert
	testBlock := &quasar.Block{
		Hash:      validID.String(),
		Height:    100,
		Timestamp: time.Now(),
		Cert:      nil,
	}

	cb(testBlock)

	require.True(t, callbackCalled)
	require.Equal(t, uint64(100), receivedEvent.Height)
	require.Nil(t, receivedEvent.BLSProof)
	require.Nil(t, receivedEvent.PQProof)
}
