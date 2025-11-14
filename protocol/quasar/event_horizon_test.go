package quasar

import (
	"context"
	"testing"
	"time"
)

func TestQuasar(t *testing.T) {
	// Create event horizon
	eh, err := New(1)
	if err != nil {
		t.Fatal(err)
	}

	// Add validator
	if err := eh.hybrid.AddValidator("validator1", 100); err != nil {
		t.Fatal(err)
	}

	// Start
	ctx := context.Background()
	eh.Start(ctx)

	// Submit blocks from all chains
	blocks := []*Block{
		{Chain: "P-Chain", ID: [32]byte{1}, Height: 100, Data: []byte("p-data")},
		{Chain: "X-Chain", ID: [32]byte{2}, Height: 200, Data: []byte("x-data")},
		{Chain: "C-Chain", ID: [32]byte{3}, Height: 300, Data: []byte("c-data")},
	}

	for _, b := range blocks {
		b.Timestamp = time.Now()
		eh.Submit(b)
	}

	time.Sleep(100 * time.Millisecond)

	// Check metrics
	height, processed, _ := eh.Stats()
	if processed != 3 {
		t.Errorf("want 3 blocks, got %d", processed)
	}
	if height != 3 {
		t.Errorf("want height 3, got %d", height)
	}

	// Verify finality
	hash := eh.hash(blocks[0])
	if !eh.Verify(hash) {
		t.Error("block not finalized")
	}

	t.Logf("✓ All chains processed: height=%d blocks=%d", height, processed)
}

func TestAutoRegister(t *testing.T) {
	eh, err := New(1)
	if err != nil {
		t.Fatal(err)
	}

	if err := eh.hybrid.AddValidator("validator1", 100); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	eh.Start(ctx)

	// Submit from unregistered chain - should auto-register
	bridge := &Block{
		Chain:     "Bridge",
		ID:        [32]byte{0xFF},
		Height:    1,
		Timestamp: time.Now(),
		Data:      []byte("bridge-tx"),
	}

	eh.Submit(bridge)
	time.Sleep(100 * time.Millisecond)

	// Check chain registered
	chains := eh.Chains()
	found := false
	for _, c := range chains {
		if c == "Bridge" {
			found = true
		}
	}

	if !found {
		t.Error("Bridge not auto-registered")
	}

	t.Logf("✓ Bridge auto-registered: %v", chains)
}

func TestQuantumSignatures(t *testing.T) {
	eh, err := New(1)
	if err != nil {
		t.Fatal(err)
	}

	if err := eh.hybrid.AddValidator("validator1", 100); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	eh.Start(ctx)

	// Submit block
	block := &Block{
		Chain:     "C-Chain",
		ID:        [32]byte{0xAA},
		Height:    1,
		Timestamp: time.Now(),
		Data:      []byte("test"),
	}

	eh.Submit(block)
	time.Sleep(100 * time.Millisecond)

	// Get finalized block
	hash := eh.hash(block)
	eh.mu.RLock()
	fb := eh.finality[hash]
	eh.mu.RUnlock()

	if fb == nil {
		t.Fatal("block not finalized")
	}

	// Check signatures
	if len(fb.Signatures) == 0 {
		t.Error("no validator signatures")
	}

	for _, sig := range fb.Signatures {
		if len(sig.BLS) == 0 {
			t.Error("missing BLS signature")
		}
		if len(sig.Ringtail) == 0 {
			t.Error("missing Ringtail signature")
		}
	}

	t.Log("✓ Hybrid signatures (BLS + Ringtail) verified")
}

func TestMultiChain(t *testing.T) {
	eh, err := New(1)
	if err != nil {
		t.Fatal(err)
	}

	if err := eh.hybrid.AddValidator("validator1", 100); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	eh.Start(ctx)

	// Submit from many chains
	chains := []string{"Bridge", "Oracle", "Gaming", "DeFi", "ZK"}
	for i, name := range chains {
		block := &Block{
			Chain:     name,
			ID:        [32]byte{byte(i)},
			Height:    uint64(i + 1),
			Timestamp: time.Now(),
			Data:      []byte(name),
		}
		eh.Submit(block)
	}

	time.Sleep(500 * time.Millisecond)

	// All chains should be registered
	registered := eh.Chains()
	if len(registered) < 8 { // 3 primary + 5 external
		t.Errorf("want ≥8 chains, got %d", len(registered))
	}

	t.Logf("✓ %d chains in event horizon: %v", len(registered), registered)
}
