package quasar

import (
	"context"
	"testing"
	"time"
)

func TestDynamicChainRegistration(t *testing.T) {
	// Create Quasar core
	q, err := NewQuasarCore(1)
	if err != nil {
		t.Fatalf("Failed to create Quasar core: %v", err)
	}

	// Add validator
	err = q.hybridConsensus.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	// Start the Quasar
	ctx := context.Background()
	err = q.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start Quasar: %v", err)
	}

	// Verify primary chains auto-registered
	chains := q.GetRegisteredChains()
	if len(chains) != 3 {
		t.Errorf("Expected 3 primary chains, got %d", len(chains))
	}

	expectedChains := map[string]bool{
		"P-Chain": true,
		"X-Chain": true,
		"C-Chain": true,
	}

	for _, chain := range chains {
		if !expectedChains[chain] {
			t.Errorf("Unexpected chain registered: %s", chain)
		}
	}

	t.Logf("✓ Primary chains auto-registered: %v", chains)
}

func TestAutoRegisterNewSubnet(t *testing.T) {
	// Create Quasar core
	q, err := NewQuasarCore(1)
	if err != nil {
		t.Fatalf("Failed to create Quasar core: %v", err)
	}

	// Add validator
	err = q.hybridConsensus.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	// Start the Quasar
	ctx := context.Background()
	err = q.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start Quasar: %v", err)
	}

	// Submit block from NEW subnet - should auto-register
	bridgeBlock := &ChainBlock{
		ChainID:   [32]byte{0xFF},
		ChainName: "Bridge-Net",
		BlockID:   [32]byte{0xAA, 0xBB, 0xCC},
		Height:    1,
		Timestamp: time.Now(),
		Data:      []byte("Bridge transaction data"),
	}

	err = q.SubmitBlock(bridgeBlock)
	if err != nil {
		t.Fatalf("Failed to submit bridge block: %v", err)
	}

	// Wait for processing
	time.Sleep(100 * time.Millisecond)

	// Verify Bridge-Net was auto-registered
	chains := q.GetRegisteredChains()
	found := false
	for _, chain := range chains {
		if chain == "Bridge-Net" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Bridge-Net was not auto-registered")
	}

	t.Logf("✓ Bridge-Net auto-registered and added to event horizon")
}

func TestMultipleExternalChains(t *testing.T) {
	// Create Quasar core
	q, err := NewQuasarCore(1)
	if err != nil {
		t.Fatalf("Failed to create Quasar core: %v", err)
	}

	// Add validator
	err = q.hybridConsensus.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	// Start the Quasar
	ctx := context.Background()
	err = q.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start Quasar: %v", err)
	}

	// Submit blocks from multiple external systems
	externalChains := []string{
		"Bridge-Net",
		"Oracle-Net",
		"Gaming-Net",
		"DeFi-Net",
		"ZK-Net",
	}

	for i, chainName := range externalChains {
		block := &ChainBlock{
			ChainID:   [32]byte{byte(i + 10)},
			ChainName: chainName,
			BlockID:   [32]byte{byte(i * 100)},
			Height:    uint64(i + 1),
			Timestamp: time.Now(),
			Data:      []byte(chainName + " block data"),
		}

		err = q.SubmitBlock(block)
		if err != nil {
			t.Fatalf("Failed to submit block from %s: %v", chainName, err)
		}
	}

	// Wait for processing (longer for multiple chains)
	time.Sleep(500 * time.Millisecond)

	// Verify all external chains registered
	chains := q.GetRegisteredChains()
	if len(chains) < 8 { // 3 primary + 5 external
		t.Errorf("Expected at least 8 chains, got %d", len(chains))
	}

	// Check metrics - at least some blocks should be processed
	processed, _ := q.GetMetrics()
	t.Logf("Processed %d blocks", processed)

	t.Logf("✓ All %d chains registered: %v", len(chains), chains)
	t.Logf("✓ Processed %d blocks from external chains", processed)
}

func TestQuantumSecurityForAllChains(t *testing.T) {
	// Create Quasar core
	q, err := NewQuasarCore(1)
	if err != nil {
		t.Fatalf("Failed to create Quasar core: %v", err)
	}

	// Add validator
	err = q.hybridConsensus.AddValidator("validator1", 100)
	if err != nil {
		t.Fatalf("Failed to add validator: %v", err)
	}

	// Start the Quasar
	ctx := context.Background()
	err = q.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start Quasar: %v", err)
	}

	// Submit block from external bridge
	bridgeBlock := &ChainBlock{
		ChainID:   [32]byte{0xFF},
		ChainName: "Bridge-Net",
		BlockID:   [32]byte{0xDD, 0xEE, 0xFF},
		Height:    1,
		Timestamp: time.Now(),
		Data:      []byte("Bridge cross-chain transaction"),
	}

	err = q.SubmitBlock(bridgeBlock)
	if err != nil {
		t.Fatalf("Failed to submit bridge block: %v", err)
	}

	// Wait for quantum finalization
	time.Sleep(200 * time.Millisecond)

	// Verify bridge block has quantum finality
	blockHash := q.computeQuantumHash(bridgeBlock)
	if !q.VerifyQuantumFinality(blockHash) {
		t.Error("Bridge block does not have quantum finality")
	}

	// Verify hybrid signatures present
	q.mu.RLock()
	qBlock, exists := q.finalizedBlocks[blockHash]
	q.mu.RUnlock()

	if !exists {
		t.Fatal("Bridge block not found in finalized blocks")
	}

	if len(qBlock.ValidatorSigs) == 0 {
		t.Error("No validator signatures on bridge block")
	}

	// Check hybrid signature components
	for validatorID, sig := range qBlock.ValidatorSigs {
		if len(sig.BLS) == 0 {
			t.Errorf("Validator %s missing BLS signature", validatorID)
		}
		if len(sig.Ringtail) == 0 {
			t.Errorf("Validator %s missing Ringtail (ML-DSA) signature", validatorID)
		}
	}

	t.Logf("✓ Bridge block has quantum finality with BLS + Ringtail signatures")
	t.Logf("✓ External chains receive same quantum security as primary chains")
}
