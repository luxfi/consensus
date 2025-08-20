package config

import (
	"testing"
)

func TestWaveConfig(t *testing.T) {
	cfg := DefaultWave()

	// Check that configuration is set correctly
	if !cfg.Enable {
		t.Error("Enable should be true")
	}

	if cfg.VoteLimitPerBlock != 256 {
		t.Errorf("VoteLimitPerBlock mismatch: got %d, want 256", cfg.VoteLimitPerBlock)
	}

	if !cfg.ExecuteOwned {
		t.Error("ExecuteOwned should be true")
	}

	if !cfg.ExecuteMixedOnFinal {
		t.Error("ExecuteMixedOnFinal should be true")
	}

	if !cfg.EpochFence {
		t.Error("EpochFence should be true")
	}

	if string(cfg.VotePrefix) != "WAVE/V1" {
		t.Errorf("VotePrefix mismatch: got %s, want WAVE/V1", cfg.VotePrefix)
	}
}

func TestWaveConfigCustom(t *testing.T) {
	cfg := WaveConfig{
		Enable:              false,
		VoteLimitPerBlock:   512,
		ExecuteOwned:        false,
		ExecuteMixedOnFinal: false,
		EpochFence:          false,
		VotePrefix:          []byte("CUSTOM"),
	}

	if cfg.Enable {
		t.Error("Enable should be false")
	}

	if cfg.VoteLimitPerBlock != 512 {
		t.Errorf("VoteLimitPerBlock should be 512, got %d", cfg.VoteLimitPerBlock)
	}

	if string(cfg.VotePrefix) != "CUSTOM" {
		t.Errorf("VotePrefix should be CUSTOM, got %s", cfg.VotePrefix)
	}
}

func BenchmarkWaveConfig(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = DefaultWave()
	}
}
