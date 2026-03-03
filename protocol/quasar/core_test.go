package quasar

import (
	"context"
	"testing"
	"time"
)

func TestQuasar(t *testing.T) {
	// Create quantum aggregator with threshold of 2 validators
	// Need 3 validators for threshold 2 (t < n)
	qa, err := NewQuasar(2)
	if err != nil {
		t.Fatalf("Failed to create quantum aggregator: %v", err)
	}

	// Initialize validators (need more than threshold)
	err = qa.InitializeValidators([]string{"validator1", "validator2", "validator3"})
	if err != nil {
		t.Fatalf("Failed to initialize validators: %v", err)
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
		ID:        [32]byte{0x01, 0x02, 0x03},
		Height:    100,
		Timestamp: time.Now(),
		Data:      []byte("P-Chain block data"),
	}
	qa.SubmitPChainBlock(pBlock)

	xBlock := &ChainBlock{
		ChainID:   [32]byte{2},
		ChainName: "X-Chain",
		ID:        [32]byte{0x04, 0x05, 0x06},
		Height:    200,
		Timestamp: time.Now(),
		Data:      []byte("X-Chain block data"),
	}
	qa.SubmitXChainBlock(xBlock)

	cBlock := &ChainBlock{
		ChainID:   [32]byte{3},
		ChainName: "C-Chain",
		ID:        [32]byte{0x07, 0x08, 0x09},
		Height:    300,
		Timestamp: time.Now(),
		Data:      []byte("C-Chain block data"),
	}
	qa.SubmitCChainBlock(cBlock)

	// Wait for blocks to enter pending state (self-vote from validator1)
	time.Sleep(100 * time.Millisecond)

	// Supply second vote from validator2 to meet threshold=2
	for _, block := range []*ChainBlock{pBlock, xBlock, cBlock} {
		blockHash := qa.computeQuantumHash(block)
		sig, err := qa.SignMessage("validator2", []byte(blockHash))
		if err != nil {
			t.Fatalf("Failed to sign block from validator2: %v", err)
		}
		if !qa.ReceiveVote(blockHash, "validator2", sig) {
			t.Fatalf("Failed to receive vote for block %s", block.ChainName)
		}
	}

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
	qa, err := NewQuasar(2)
	if err != nil {
		t.Fatalf("Failed to create quantum aggregator: %v", err)
	}

	// Add validator with both BLS and Ringtail keys
	err = qa.InitializeValidators([]string{"validator1", "validator2", "validator3"})
	if err != nil {
		t.Fatalf("Failed to initialize validators: %v", err)
	}

	// Create a test message
	message := []byte("Test quantum finality message")

	// Sign with hybrid (BLS + Ringtail)
	sig, err := qa.SignMessage("validator1", message)
	if err != nil {
		t.Fatalf("Failed to sign message: %v", err)
	}

	// Verify BLS signature is present
	// Note: Ringtail requires dual threshold mode with HybridConfig
	if len(sig.BLS) == 0 {
		t.Error("BLS signature missing")
	}
	if len(sig.Ringtail) > 0 {
		t.Log("Ringtail threshold signature present")
	}

	// Verify hybrid signature
	if !qa.VerifyQuasarSig(message, sig) {
		t.Error("Failed to verify hybrid signature")
	}

	t.Log("Ringtail + BLS parallel consensus test passed")
}

func TestQuantumEpochFinalization(t *testing.T) {
	qa, err := NewQuasar(2)
	if err != nil {
		t.Fatalf("Failed to create quantum aggregator: %v", err)
	}

	// Add validator
	err = qa.InitializeValidators([]string{"validator1", "validator2", "validator3"})
	if err != nil {
		t.Fatalf("Failed to initialize validators: %v", err)
	}

	// Submit multiple blocks and supply second vote to meet threshold
	for i := 0; i < 5; i++ {
		block := &ChainBlock{
			ChainID:   [32]byte{byte(i)},
			ChainName: "P-Chain",
			ID:        [32]byte{byte(i * 10)},
			Height:    uint64(100 + i),
			Timestamp: time.Now(),
			Data:      []byte("Block data"),
		}
		qa.processBlock(block)

		// Second vote to meet threshold=2
		blockHash := qa.computeQuantumHash(block)
		sig, err := qa.SignMessage("validator2", []byte(blockHash))
		if err != nil {
			t.Fatalf("Failed to sign block from validator2: %v", err)
		}
		qa.ReceiveVote(blockHash, "validator2", sig)
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

func TestContextCancellation(t *testing.T) {
	t.Run("SignMessageWithContext_cancelled", func(t *testing.T) {
		hybrid, err := NewSigner(1)
		if err != nil {
			t.Fatalf("Failed to create hybrid: %v", err)
		}

		err = hybrid.AddValidator("validator1", 100)
		if err != nil {
			t.Fatalf("Failed to add validator: %v", err)
		}

		// Create cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Attempt to sign with cancelled context
		_, err = hybrid.SignMessageWithContext(ctx, "validator1", []byte("test"))
		if err == nil {
			t.Error("Expected error from cancelled context, got nil")
		}
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	})

	t.Run("VerifyQuasarSigWithContext_cancelled", func(t *testing.T) {
		hybrid, err := NewSigner(1)
		if err != nil {
			t.Fatalf("Failed to create hybrid: %v", err)
		}

		err = hybrid.AddValidator("validator1", 100)
		if err != nil {
			t.Fatalf("Failed to add validator: %v", err)
		}

		// Sign with valid context
		msg := []byte("test message")
		sig, err := hybrid.SignMessage("validator1", msg)
		if err != nil {
			t.Fatalf("Failed to sign: %v", err)
		}

		// Verify with cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result := hybrid.VerifyQuasarSigWithContext(ctx, msg, sig)
		if result {
			t.Error("Expected false from cancelled context verification")
		}
	})

	t.Run("AggregateSignaturesWithContext_cancelled", func(t *testing.T) {
		hybrid, err := NewSigner(1)
		if err != nil {
			t.Fatalf("Failed to create hybrid: %v", err)
		}

		err = hybrid.AddValidator("validator1", 100)
		if err != nil {
			t.Fatalf("Failed to add validator: %v", err)
		}

		// Create signature
		msg := []byte("test message")
		sig, err := hybrid.SignMessage("validator1", msg)
		if err != nil {
			t.Fatalf("Failed to sign: %v", err)
		}

		// Aggregate with cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err = hybrid.AggregateSignaturesWithContext(ctx, msg, []*QuasarSig{sig})
		if err == nil {
			t.Error("Expected error from cancelled context, got nil")
		}
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", err)
		}
	})

	t.Run("VerifyQuantumFinalityWithContext_cancelled", func(t *testing.T) {
		qa, err := NewQuasar(2)
		if err != nil {
			t.Fatalf("Failed to create quasar: %v", err)
		}

		err = qa.InitializeValidators([]string{"validator1", "validator2", "validator3"})
		if err != nil {
			t.Fatalf("Failed to initialize validators: %v", err)
		}

		// Process a block to create finality
		block := &ChainBlock{
			ChainID:   [32]byte{1},
			ChainName: "P-Chain",
			ID:        [32]byte{0x01},
			Height:    100,
			Timestamp: time.Now(),
			Data:      []byte("test"),
		}
		qa.processBlock(block)

		// Supply second vote to finalize (threshold=2)
		blockHash := qa.computeQuantumHash(block)
		sig, err := qa.SignMessage("validator2", []byte(blockHash))
		if err != nil {
			t.Fatalf("Failed to sign: %v", err)
		}
		qa.ReceiveVote(blockHash, "validator2", sig)

		// Verify with cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result := qa.VerifyQuantumFinalityWithContext(ctx, blockHash)
		if result {
			t.Error("Expected false from cancelled context verification")
		}
	})

	t.Run("WithContext_valid_context_works", func(t *testing.T) {
		hybrid, err := NewSigner(1)
		if err != nil {
			t.Fatalf("Failed to create hybrid: %v", err)
		}

		err = hybrid.AddValidator("validator1", 100)
		if err != nil {
			t.Fatalf("Failed to add validator: %v", err)
		}

		// Use valid context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		msg := []byte("test message")

		// Sign should succeed
		sig, err := hybrid.SignMessageWithContext(ctx, "validator1", msg)
		if err != nil {
			t.Fatalf("SignMessageWithContext failed: %v", err)
		}

		// Verify should succeed
		if !hybrid.VerifyQuasarSigWithContext(ctx, msg, sig) {
			t.Error("VerifyQuasarSigWithContext returned false for valid signature")
		}

		// Aggregate should succeed
		aggSig, err := hybrid.AggregateSignaturesWithContext(ctx, msg, []*QuasarSig{sig})
		if err != nil {
			t.Fatalf("AggregateSignaturesWithContext failed: %v", err)
		}

		// Verify aggregated should succeed
		if !hybrid.VerifyAggregatedSignatureWithContext(ctx, msg, aggSig) {
			t.Error("VerifyAggregatedSignatureWithContext returned false for valid signature")
		}
	})
}

// TestSingleNodeCannotFinalizeWithThreshold verifies that a single validator's
// signature is insufficient to finalize a block when threshold > 1.
// Regression test for the quorum bypass bug where processBlock() finalized
// with only a local signature.
func TestSingleNodeCannotFinalizeWithThreshold(t *testing.T) {
	qa, err := NewQuasar(2)
	if err != nil {
		t.Fatalf("Failed to create quasar: %v", err)
	}

	err = qa.InitializeValidators([]string{"validator1", "validator2", "validator3"})
	if err != nil {
		t.Fatalf("Failed to initialize validators: %v", err)
	}

	block := &ChainBlock{
		ChainID:   [32]byte{1},
		ChainName: "P-Chain",
		ID:        [32]byte{0xAA},
		Height:    999,
		Timestamp: time.Now(),
		Data:      []byte("should not finalize with 1 sig"),
	}

	// processBlock self-signs with validator1 only -- threshold=2 not met
	qa.processBlock(block)

	blockHash := qa.computeQuantumHash(block)

	// Block must NOT be finalized
	if qa.VerifyQuantumFinality(blockHash) {
		t.Fatal("SECURITY: block finalized with only 1 signature when threshold=2")
	}

	// Block should be pending
	if qa.GetPendingCount() != 1 {
		t.Fatalf("Expected 1 pending block, got %d", qa.GetPendingCount())
	}

	// Height must not have advanced
	if qa.GetQuantumHeight() != 0 {
		t.Fatalf("Expected height 0 (no finalization), got %d", qa.GetQuantumHeight())
	}
}

// TestThresholdFinalizationRequiresQuorum verifies that blocks finalize
// only after the required number of distinct validator signatures arrive.
func TestThresholdFinalizationRequiresQuorum(t *testing.T) {
	qa, err := NewQuasar(3)
	if err != nil {
		t.Fatalf("Failed to create quasar: %v", err)
	}

	err = qa.InitializeValidators([]string{"validator1", "validator2", "validator3", "validator4"})
	if err != nil {
		t.Fatalf("Failed to initialize validators: %v", err)
	}

	block := &ChainBlock{
		ChainID:   [32]byte{1},
		ChainName: "P-Chain",
		ID:        [32]byte{0xBB},
		Height:    100,
		Timestamp: time.Now(),
	}

	// Self-vote: 1 of 3 needed
	qa.processBlock(block)
	blockHash := qa.computeQuantumHash(block)

	if qa.VerifyQuantumFinality(blockHash) {
		t.Fatal("Finalized with 1/3 signatures")
	}

	// Second vote: 2 of 3 needed
	sig2, err := qa.SignMessage("validator2", []byte(blockHash))
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	qa.ReceiveVote(blockHash, "validator2", sig2)

	if qa.VerifyQuantumFinality(blockHash) {
		t.Fatal("Finalized with 2/3 signatures")
	}
	if qa.GetPendingCount() != 1 {
		t.Fatal("Block should still be pending")
	}

	// Third vote: threshold met
	sig3, err := qa.SignMessage("validator3", []byte(blockHash))
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	qa.ReceiveVote(blockHash, "validator3", sig3)

	if !qa.VerifyQuantumFinality(blockHash) {
		t.Fatal("Block should be finalized with 3/3 signatures")
	}
	if qa.GetPendingCount() != 0 {
		t.Fatal("No blocks should be pending after finalization")
	}
	if qa.GetQuantumHeight() != 1 {
		t.Fatalf("Expected height 1, got %d", qa.GetQuantumHeight())
	}
}

// TestSingleNodeModeImmediateFinalization verifies that threshold=1 (single-node)
// still produces immediate finalization from the self-vote.
func TestSingleNodeModeImmediateFinalization(t *testing.T) {
	qa, err := NewQuasar(1)
	if err != nil {
		t.Fatalf("Failed to create quasar: %v", err)
	}

	_, err = qa.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	block := &ChainBlock{
		ChainID:   [32]byte{1},
		ChainName: "P-Chain",
		ID:        [32]byte{0xCC},
		Height:    1,
		Timestamp: time.Now(),
	}

	qa.processBlock(block)
	blockHash := qa.computeQuantumHash(block)

	// Threshold=1: self-vote should finalize immediately
	if !qa.VerifyQuantumFinality(blockHash) {
		t.Fatal("Block should be finalized with threshold=1")
	}
	if qa.GetPendingCount() != 0 {
		t.Fatal("No blocks should be pending")
	}
	if qa.GetQuantumHeight() != 1 {
		t.Fatalf("Expected height 1, got %d", qa.GetQuantumHeight())
	}
}

// TestDuplicateVoteDoesNotDoubleCount verifies that the same validator
// voting twice does not count as two distinct votes.
func TestDuplicateVoteDoesNotDoubleCount(t *testing.T) {
	qa, err := NewQuasar(2)
	if err != nil {
		t.Fatalf("Failed to create quasar: %v", err)
	}

	err = qa.InitializeValidators([]string{"validator1", "validator2", "validator3"})
	if err != nil {
		t.Fatalf("Failed to initialize validators: %v", err)
	}

	block := &ChainBlock{
		ChainID:   [32]byte{1},
		ChainName: "P-Chain",
		ID:        [32]byte{0xDD},
		Height:    1,
		Timestamp: time.Now(),
	}

	qa.processBlock(block)
	blockHash := qa.computeQuantumHash(block)

	// Send validator1's vote again (duplicate)
	sig, err := qa.SignMessage("validator1", []byte(blockHash))
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}
	qa.ReceiveVote(blockHash, "validator1", sig)

	// Should still be pending -- 1 unique validator, threshold=2
	if qa.VerifyQuantumFinality(blockHash) {
		t.Fatal("SECURITY: duplicate vote from same validator bypassed threshold")
	}
	if qa.GetPendingCount() != 1 {
		t.Fatal("Block should still be pending")
	}
}

// TestReceiveVoteForUnknownBlock verifies ReceiveVote returns false
// for blocks not in the pending set.
func TestReceiveVoteForUnknownBlock(t *testing.T) {
	qa, err := NewQuasar(1)
	if err != nil {
		t.Fatalf("Failed to create quasar: %v", err)
	}

	_, err = qa.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	sig, err := qa.SignMessage("validator1", []byte("nonexistent-hash"))
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	if qa.ReceiveVote("nonexistent-hash", "validator1", sig) {
		t.Fatal("ReceiveVote should return false for unknown block")
	}
}

// TestReceiveVoteForAlreadyFinalizedBlock verifies that votes for
// already-finalized blocks are rejected.
func TestReceiveVoteForAlreadyFinalizedBlock(t *testing.T) {
	qa, err := NewQuasar(1)
	if err != nil {
		t.Fatalf("Failed to create quasar: %v", err)
	}

	_, err = qa.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	block := &ChainBlock{
		ChainID:   [32]byte{1},
		ChainName: "P-Chain",
		ID:        [32]byte{0xEE},
		Height:    1,
		Timestamp: time.Now(),
	}

	// Threshold=1: finalized immediately
	qa.processBlock(block)
	blockHash := qa.computeQuantumHash(block)

	// Try to vote again after finalization
	sig, err := qa.SignMessage("validator1", []byte(blockHash))
	if err != nil {
		t.Fatalf("Failed to sign: %v", err)
	}

	if qa.ReceiveVote(blockHash, "validator1", sig) {
		t.Fatal("ReceiveVote should return false for already-finalized block")
	}
}
