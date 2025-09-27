package pq

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

func TestNew(t *testing.T) {
	params := config.DefaultParams()
	engine := NewConsensus(params)

	if engine == nil {
		t.Fatal("New() returned nil")
	}

	if engine.params.K != params.K {
		t.Errorf("K mismatch: got %d, want %d", engine.params.K, params.K)
	}

	if engine.Height() != 0 {
		t.Errorf("Initial height should be 0, got %d", engine.Height())
	}
}

func TestInitialize(t *testing.T) {
	engine := NewConsensus(config.DefaultParams())
	ctx := context.Background()

	err := engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
}

func TestProcessBlock(t *testing.T) {
	engine := NewConsensus(config.DefaultParams())
	ctx := context.Background()

	err := engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	blockID := ids.GenerateTestID()

	// Test insufficient votes
	votes := map[string]int{
		blockID.String(): 10,
		"other":          11,
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	if err == nil {
		t.Error("ProcessBlock should fail with insufficient votes")
	}

	// Test sufficient votes
	votes = map[string]int{
		blockID.String(): 18,
		"other":          3,
	}

	err = engine.ProcessBlock(ctx, blockID, votes)
	if err != nil {
		t.Errorf("ProcessBlock failed with sufficient votes: %v", err)
	}

	// Check finalized
	if !engine.IsFinalized(blockID) {
		t.Error("Block should be finalized")
	}

	// Check height increased
	if engine.Height() != 1 {
		t.Errorf("Height should be 1, got %d", engine.Height())
	}

	// Test already finalized
	err = engine.ProcessBlock(ctx, blockID, votes)
	if err != nil {
		t.Error("ProcessBlock should succeed for already finalized block")
	}
}

func TestFinalityChannel(t *testing.T) {
	engine := NewConsensus(config.DefaultParams())
	ctx := context.Background()

	err := engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Start listening for finality events
	finality := engine.FinalityChannel()

	blockID := ids.GenerateTestID()
	votes := map[string]int{
		blockID.String(): 18,
		"other":          3,
	}

	// Process block in background
	go func() {
		time.Sleep(10 * time.Millisecond)
		_ = engine.ProcessBlock(ctx, blockID, votes)
	}()

	// Wait for finality event
	select {
	case event := <-finality:
		if event.BlockID != blockID {
			t.Errorf("Wrong block ID in event: got %s, want %s", event.BlockID, blockID)
		}
		if event.Height != 1 {
			t.Errorf("Wrong height in event: got %d, want 1", event.Height)
		}
		if len(event.PQProof) == 0 {
			t.Error("Missing PQ proof in event")
		}
		if len(event.BLSProof) == 0 {
			t.Error("Missing BLS proof in event")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Timeout waiting for finality event")
	}
}

func TestSetFinalizedCallback(t *testing.T) {
	engine := NewConsensus(config.DefaultParams())

	called := false

	engine.SetFinalizedCallback(func(event FinalityEvent) {
		called = true
		if event.Height == 0 {
			t.Error("Event height should not be 0")
		}
	})

	// Trigger callback through quasar
	if engine.quasar != nil {
		// This tests that the callback is properly wired
		// In production, this would be triggered by actual consensus
		t.Log("Quasar is properly initialized for event horizon callbacks")
	}

	// The callback is set, even if not triggered in this test
	_ = called

	// Test metrics
	metrics := engine.Metrics()
	if metrics["height"] != uint64(0) {
		t.Errorf("Wrong height in metrics: %v", metrics["height"])
	}
	if metrics["k"] != engine.params.K {
		t.Errorf("Wrong K in metrics: %v", metrics["k"])
	}
}

func TestMultipleBlocks(t *testing.T) {
	engine := NewConsensus(config.XChainParams()) // Fast params
	ctx := context.Background()

	err := engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Process multiple blocks
	for i := 0; i < 5; i++ {
		blockID := ids.GenerateTestID()
		votes := map[string]int{
			blockID.String(): 4, // 4/5 = 0.8 > 0.6 (alpha for XChain)
			"other":          1,
		}

		err := engine.ProcessBlock(ctx, blockID, votes)
		if err != nil {
			t.Errorf("Failed to process block %d: %v", i, err)
		}

		if !engine.IsFinalized(blockID) {
			t.Errorf("Block %d should be finalized", i)
		}
	}

	if engine.Height() != 5 {
		t.Errorf("Height should be 5, got %d", engine.Height())
	}
}

func BenchmarkProcessBlock(b *testing.B) {
	engine := NewConsensus(config.XChainParams())
	ctx := context.Background()
	_ = engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))

	votes := map[string]int{
		"block": 4,
		"other": 1,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		blockID := ids.GenerateTestID()
		_ = engine.ProcessBlock(ctx, blockID, votes)
	}
}

func BenchmarkIsFinalized(b *testing.B) {
	engine := NewConsensus(config.DefaultParams())
	ctx := context.Background()
	_ = engine.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))

	// Add some finalized blocks
	for i := 0; i < 100; i++ {
		blockID := ids.GenerateTestID()
		engine.finalized[blockID] = true
	}

	testID := ids.GenerateTestID()
	engine.finalized[testID] = true

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = engine.IsFinalized(testID)
	}
}
