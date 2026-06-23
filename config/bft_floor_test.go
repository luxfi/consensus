// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// bft_floor_test.go — round-2 CRITICAL-2 tests: Valid() rejects an α quorum too
// small for K to be Byzantine-safe, and value/PoS chains forbid K=3 (f=0).
package config

import (
	"errors"
	"testing"
)

// TestValid_RejectsSubBFTAlpha proves the overlap-bound 2α−K ≥ ⌊(K-1)/3⌋+1 is
// enforced: K=3/α=2 is at the f=0 boundary (admitted by Valid, gated by the
// value check), while genuinely sub-quorum configs (3-of-5 with f=1, 2-of-4) are
// rejected outright.
func TestValid_RejectsSubBFTAlpha(t *testing.T) {
	cases := []struct {
		name    string
		k, a    int
		wantErr error
	}{
		// 3-of-5, f=1: overlap 2·3−5=1 < f+1=2 → UNSAFE (two quorums share only 1,
		// which could be the single Byzantine node). The bound correctly rejects.
		{"3-of-5 (sub-quorum, f=1)", 5, 3, ErrAlphaBelowBFTQuorum},
		// 2-of-4, f=1: overlap 0 < 2 → UNSAFE.
		{"2-of-4 (sub-quorum)", 4, 2, ErrAlphaBelowBFTQuorum},
		// K=3/α=2, f=0: overlap 1 ≥ 1 → admitted by Valid (the value check forbids
		// it for real value separately).
		{"3-of-3-style K=3/α=2 (f=0 boundary)", 3, 2, nil},
		// 3-of-4, f=1: overlap 2 ≥ 2 → the minimal BFT quorum, admitted.
		{"3-of-4 (minimal BFT)", 4, 3, nil},
		// 8-of-11 (testnet shape), f=3: overlap 5 ≥ 4 → admitted.
		{"8-of-11", 11, 8, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Parameters{K: tc.k, Alpha: 0.69, Beta: 2, AlphaPreference: tc.a, AlphaConfidence: tc.a}
			err := p.Valid()
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestProductionPresetsPassBFT proves every PRODUCTION preset satisfies the new
// BFT floor — the bound flags only genuinely unsafe configs, never the shipped
// ones. (Default/Mainnet/Testnet are the BFT multi-node params; Single/SoloGPU
// are K=1; Local/XChain/Burst are K=3 dev params.)
func TestProductionPresetsPassBFT(t *testing.T) {
	presets := map[string]Parameters{
		"Default":         DefaultParams(),
		"Mainnet":         MainnetParams(),
		"Testnet":         TestnetParams(),
		"Local":           LocalParams(),
		"Burst":           BurstParams(),
		"SoloGPU":         SoloGPUParams(),
		"XChain":          XChainParams(),
		"SingleValidator": SingleValidatorParams(),
	}
	for name, p := range presets {
		if err := p.Valid(); err != nil {
			t.Errorf("%s preset must be Valid(), got %v", name, err)
		}
	}
}

// TestValidateForValueNetwork_ForbidsK3 is the CRITICAL-2 value gate: a value /
// PoS network REQUIRES Byzantine tolerance (f≥1 ⟹ K≥4). K=3 (f=0) and K=1 are
// rejected for value even though plain Valid() admits them; K≥4 is admitted
// (subject to the mainnet/testnet K floors).
func TestValidateForValueNetwork_ForbidsK3(t *testing.T) {
	devnet := uint32(1337)

	// K=3/α=2 (LocalParams) — fine for dev, FORBIDDEN for value (f=0).
	if err := LocalParams().Valid(); err != nil {
		t.Fatalf("LocalParams must pass plain Valid(): %v", err)
	}
	if err := LocalParams().ValidateForValueNetwork(devnet); !errors.Is(err, ErrKTooLowForValue) {
		t.Fatalf("K=3 must be forbidden for a value network, got %v", err)
	}

	// K=1 single-validator — also not a value-across-parties regime.
	if err := SingleValidatorParams().ValidateForValueNetwork(devnet); !errors.Is(err, ErrKTooLowForValue) {
		t.Fatalf("K=1 must be forbidden for a value network, got %v", err)
	}

	// K=4 (minimal BFT) — admitted on a devnet value chain.
	k4 := Parameters{K: 4, Alpha: 0.69, Beta: 2, AlphaPreference: 3, AlphaConfidence: 3}
	if err := k4.ValidateForValueNetwork(devnet); err != nil {
		t.Fatalf("K=4 (f=1) must be admitted for a value network, got %v", err)
	}

	// DefaultParams (K=20) — admitted; this is what the node selects for multi-node.
	if err := DefaultParams().ValidateForValueNetwork(devnet); err != nil {
		t.Fatalf("DefaultParams must be admitted for a value network, got %v", err)
	}

	// Mainnet floor still applies on top of the value floor.
	if err := DefaultParams().ValidateForValueNetwork(1); err != nil {
		// Default K=20 ≥ 11 → ok on mainnet.
		t.Fatalf("DefaultParams must satisfy the mainnet floor: %v", err)
	}
	lowMain := Parameters{K: 4, Alpha: 0.69, Beta: 2, AlphaPreference: 3, AlphaConfidence: 3}
	if err := lowMain.ValidateForValueNetwork(1); !errors.Is(err, ErrKTooLowForMainnet) {
		t.Fatalf("K=4 must fail the mainnet K>=11 floor, got %v", err)
	}
}

// TestByzantineFaultTolerance pins the f = ⌊(K-1)/3⌋ table the bounds depend on.
func TestByzantineFaultTolerance(t *testing.T) {
	want := map[int]int{1: 0, 2: 0, 3: 0, 4: 1, 7: 2, 10: 3, 11: 3, 20: 6, 21: 6}
	for k, f := range want {
		if got := (Parameters{K: k}).ByzantineFaultTolerance(); got != f {
			t.Errorf("f(K=%d)=%d, want %d", k, got, f)
		}
	}
}
