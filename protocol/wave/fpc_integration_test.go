package wave

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/core/types"
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/ids"
)

// TestWaveWithFPCEnabled demonstrates FPC usage with dynamic thresholds
func TestWaveWithFPCEnabled(t *testing.T) {
	// Create config with FPC enabled
	cfg := Config{
		K:         100,
		Alpha:     0.6, // Ignored when FPC enabled
		Beta:      10,
		RoundTO:   100 * time.Millisecond,
		EnableFPC: true,
		ThetaMin:  0.5,
		ThetaMax:  0.8,
	}

	cut := &MockCut{k: cfg.K}
	tx := &MockTransport{}

	wave := New[ids.ID](cfg, cut, tx)

	// Verify FPC is enabled
	if wave.fpcSelector == nil {
		t.Fatal("FPC selector should be initialized when EnableFPC=true")
	}

	// Run multiple rounds and verify phase-dependent thresholds
	itemID := ids.GenerateTestID()
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		wave.Tick(ctx, itemID)
	}

	// Phase should have incremented
	if wave.phase != 5 {
		t.Errorf("Expected phase=5, got %d", wave.phase)
	}

	// Verify FPC selector range
	min, max := wave.fpcSelector.Range()
	if min != 0.5 || max != 0.8 {
		t.Errorf("Expected theta range [0.5, 0.8], got [%f, %f]", min, max)
	}
}

// TestWaveWithFPCDisabled verifies backward compatibility without FPC
func TestWaveWithFPCDisabled(t *testing.T) {
	// Create config with FPC disabled (default behavior)
	cfg := Config{
		K:         100,
		Alpha:     0.6,
		Beta:      10,
		RoundTO:   100 * time.Millisecond,
		EnableFPC: false, // Explicitly disabled
	}

	cut := &MockCut{k: cfg.K}
	tx := &MockTransport{}

	wave := New[ids.ID](cfg, cut, tx)

	// Verify FPC is NOT enabled
	if wave.fpcSelector != nil {
		t.Fatal("FPC selector should be nil when EnableFPC=false")
	}

	// Should use fixed alpha threshold
	itemID := ids.GenerateTestID()
	ctx := context.Background()

	wave.Tick(ctx, itemID)

	// Phase still increments but FPC not used
	if wave.phase != 1 {
		t.Errorf("Expected phase=1, got %d", wave.phase)
	}
}

// TestFPCThresholdVariation demonstrates dynamic threshold changes per phase
func TestFPCThresholdVariation(t *testing.T) {
	cfg := Config{
		K:         100,
		Beta:      10,
		RoundTO:   100 * time.Millisecond,
		EnableFPC: true,
		ThetaMin:  0.5,
		ThetaMax:  0.8,
	}

	cut := &MockCut{k: cfg.K}
	tx := &MockTransport{}

	wave := New[ids.ID](cfg, cut, tx)

	// Collect thresholds for different phases
	thresholds := make(map[int]bool)

	for phase := uint64(1); phase <= 100; phase++ {
		threshold := wave.fpcSelector.SelectThreshold(phase, cfg.K)

		// Threshold should be in valid range
		if threshold < int(0.5*float64(cfg.K)) || threshold > int(0.8*float64(cfg.K))+1 {
			t.Errorf("Threshold %d for phase %d outside expected range", threshold, phase)
		}

		thresholds[threshold] = true
	}

	// Should have multiple different thresholds across phases
	if len(thresholds) < 5 {
		t.Errorf("Expected variety in thresholds, got only %d unique values", len(thresholds))
	}

	t.Logf("FPC generated %d different threshold values across 100 phases", len(thresholds))
}

// TestFPCDeterminism verifies FPC produces same thresholds for same phases
func TestFPCDeterminism(t *testing.T) {
	cfg1 := Config{
		K:         100,
		Beta:      10,
		RoundTO:   100 * time.Millisecond,
		EnableFPC: true,
		ThetaMin:  0.5,
		ThetaMax:  0.8,
	}

	cfg2 := Config{
		K:         100,
		Beta:      10,
		RoundTO:   100 * time.Millisecond,
		EnableFPC: true,
		ThetaMin:  0.5,
		ThetaMax:  0.8,
	}

	cut := &MockCut{k: 100}
	tx := &MockTransport{}

	wave1 := New[ids.ID](cfg1, cut, tx)
	wave2 := New[ids.ID](cfg2, cut, tx)

	// Same phase should give same threshold
	for phase := uint64(1); phase <= 50; phase++ {
		t1 := wave1.fpcSelector.SelectThreshold(phase, 100)
		t2 := wave2.fpcSelector.SelectThreshold(phase, 100)

		if t1 != t2 {
			t.Errorf("Non-deterministic: phase %d gave thresholds %d and %d", phase, t1, t2)
		}
	}

	t.Log("FPC threshold selection is deterministic across instances")
}

// TestFPCvsFixedAlpha compares FPC vs fixed alpha behavior
func TestFPCvsFixedAlpha(t *testing.T) {
	// FPC-enabled config
	fpcCfg := Config{
		K:         100,
		Beta:      10,
		RoundTO:   100 * time.Millisecond,
		EnableFPC: true,
		ThetaMin:  0.5,
		ThetaMax:  0.8,
	}

	// Fixed alpha config
	fixedCfg := Config{
		K:         100,
		Alpha:     0.6,
		Beta:      10,
		RoundTO:   100 * time.Millisecond,
		EnableFPC: false,
	}

	cut := &MockCut{k: 100}
	tx := &MockTransport{}

	fpcWave := New[ids.ID](fpcCfg, cut, tx)
	fixedWave := New[ids.ID](fixedCfg, cut, tx)

	// FPC should vary thresholds
	fpcThresholds := make(map[int]bool)
	for i := 0; i < 20; i++ {
		threshold := fpcWave.fpcSelector.SelectThreshold(uint64(i), 100)
		fpcThresholds[threshold] = true
	}

	// Fixed alpha always uses same threshold (60)
	fixedThreshold := int(float64(100) * 0.6)

	t.Logf("FPC: %d unique thresholds across 20 rounds", len(fpcThresholds))
	t.Logf("Fixed Alpha: Always uses threshold=%d", fixedThreshold)

	if len(fpcThresholds) < 3 {
		t.Error("FPC should produce variety of thresholds")
	}

	// Verify fixed wave doesn't have FPC
	if fixedWave.fpcSelector != nil {
		t.Error("Fixed wave should not have FPC selector")
	}
}

// Mock implementations for testing
type MockCut struct {
	k int
}

func (m *MockCut) Sample(k int) []types.NodeID {
	nodes := make([]types.NodeID, k)
	for i := 0; i < k; i++ {
		nodes[i] = ids.GenerateTestNodeID()
	}
	return nodes
}

func (m *MockCut) Luminance() prism.Luminance {
	return prism.Luminance{
		ActivePeers: m.k,
		TotalPeers:  m.k,
		Lx:          float64(m.k),
	}
}

type MockTransport struct{}

func (t *MockTransport) RequestVotes(ctx context.Context, peers []types.NodeID, item ids.ID) <-chan Photon[ids.ID] {
	ch := make(chan Photon[ids.ID], len(peers))
	go func() {
		defer close(ch)
		for _, peer := range peers {
			select {
			case <-ctx.Done():
				return
			case ch <- Photon[ids.ID]{
				Item:      item,
				Prefer:    true,
				Sender:    peer,
				Timestamp: time.Now(),
			}:
			}
		}
	}()
	return ch
}

func (t *MockTransport) MakeLocalPhoton(item ids.ID, prefer bool) Photon[ids.ID] {
	return Photon[ids.ID]{
		Item:      item,
		Prefer:    prefer,
		Sender:    ids.GenerateTestNodeID(),
		Timestamp: time.Now(),
	}
}
