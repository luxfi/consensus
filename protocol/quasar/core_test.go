package quasar

import (
	"context"
	"testing"
	"time"
)

func TestQuasar(t *testing.T) {
	// Create quantum aggregator with threshold of 1 validator
	qa, err := NewQuasar(1)
	if err != nil {
		t.Fatalf("Failed to create quantum aggregator: %v", err)
	}

	// Add a test validator
	err = qa.hybridConsensus.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	// Start the aggregator
	ctx := context.Background()
	err = qa.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start aggregator: %v", err)
	}

	// Submit blocks from each chain
	pBlock := &ChainBlock{
		ChainID:   [32]byte{1},
		ChainName: "P-Chain",
		ID:   [32]byte{0x01, 0x02, 0x03},
		Height:    100,
		Timestamp: time.Now(),
		Data:      []byte("P-Chain block data"),
	}
	qa.SubmitPChainBlock(pBlock)

	xBlock := &ChainBlock{
		ChainID:   [32]byte{2},
		ChainName: "X-Chain",
		ID:   [32]byte{0x04, 0x05, 0x06},
		Height:    200,
		Timestamp: time.Now(),
		Data:      []byte("X-Chain block data"),
	}
	qa.SubmitXChainBlock(xBlock)

	cBlock := &ChainBlock{
		ChainID:   [32]byte{3},
		ChainName: "C-Chain",
		ID:   [32]byte{0x07, 0x08, 0x09},
		Height:    300,
		Timestamp: time.Now(),
		Data:      []byte("C-Chain block data"),
	}
	qa.SubmitCChainBlock(cBlock)

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Check metrics
	processed, proofs := qa.GetMetrics()
	if processed != 3 {
		t.Errorf("Expected 3 processed blocks, got %d", processed)
	}

	// Check quantum height
	height := qa.GetQuantumHeight()
	if height != 3 {
		t.Errorf("Expected quantum height 3, got %d", height)
	}

	// Verify quantum finality for a block
	blockHash := qa.computeQuantumHash(pBlock)
	if !qa.VerifyQuantumFinality(blockHash) {
		t.Error("Failed to verify quantum finality for P-Chain block")
	}

	t.Logf("Quantum consensus test passed: processed=%d, proofs=%d, height=%d",
		processed, proofs, height)
}

func TestQuantumFinalityWithRingtail(t *testing.T) {
	// Test that Ringtail and BLS signatures work in parallel
	qa, err := NewQuasar(1)
	if err != nil {
		t.Fatalf("Failed to create quantum aggregator: %v", err)
	}

	// Add validator with both BLS and Ringtail keys
	err = qa.hybridConsensus.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	// Create a test message
	message := []byte("Test quantum finality message")

	// Sign with hybrid (BLS + Ringtail)
	sig, err := qa.hybridConsensus.SignMessage("validator1", message)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	// Verify both signatures are present
	if len(sig.BLS) == 0 {
		t.Error("BLS signature missing")
	}
	if len(sig.Ringtail) == 0 {
		t.Error("Ringtail (ML-DSA) signature missing")
	}

	// Verify hybrid signature
	if !qa.hybridConsensus.VerifyHybridSignature(message, sig) {
		t.Error("Failed to verify hybrid signature")
	}

	t.Log("Ringtail + BLS parallel consensus test passed")
}

func TestQuantumEpochFinalization(t *testing.T) {
	qa, err := NewQuasar(1)
	if err != nil {
		t.Fatalf("Failed to create quantum aggregator: %v", err)
	}

	// Add validator
	err = qa.hybridConsensus.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	// Submit multiple blocks
	for i := 0; i < 5; i++ {
		block := &ChainBlock{
			ChainID:   [32]byte{byte(i)},
			ChainName: "P-Chain",
			ID:   [32]byte{byte(i * 10)},
			Height:    uint64(100 + i),
			Timestamp: time.Now(),
			Data:      []byte("Block data"),
		}
		qa.processBlock(block)
	}

	// Finalize epoch
	qa.finalizeQuantumEpoch()

	// Check metrics
	_, proofs := qa.GetMetrics()
	if proofs != 1 {
		t.Errorf("Expected 1 quantum proof after epoch finalization, got %d", proofs)
	}

	t.Logf("Quantum epoch finalization test passed with %d blocks", 5)
}