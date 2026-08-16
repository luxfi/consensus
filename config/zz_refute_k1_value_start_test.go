// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"errors"
	"testing"
)

// Probe: a sybil-protected chain cannot START with K==1 on ANY network at ANY
// live count. The node's chain manager runs exactly this predicate before it
// wires the engine, so if it refuses everywhere, the synthesized 1-of-1 cert is
// unreachable on every staked chain — including a sovereign L1 whose networkID
// is neither 1 nor 2.
func TestRefute_K1_neverStartsAValueChain(t *testing.T) {
	p := SingleValidatorParams()
	for _, networkID := range []uint32{1, 2, 3, 1337, 96369, 200200, 36963, 494949, 1872} {
		for _, liveN := range []int{0, 1, 2, 4, 5, 11, 21} {
			err := p.ValidateForLiveValueNetwork(networkID, liveN)
			if !errors.Is(err, ErrKTooLowForValue) {
				t.Fatalf("K=1 admitted on networkID=%d liveN=%d: err=%v", networkID, liveN, err)
			}
		}
	}
}

// Probe: an operator override cannot sneak K==1 past the same predicate either —
// the manager applies overrides BEFORE validating, so this is the real call order.
func TestRefute_K1_overrideStillRefused(t *testing.T) {
	p := MainnetParams()
	p.K, p.AlphaPreference, p.AlphaConfidence = 1, 1, 1
	if err := p.ValidateForLiveValueNetwork(1, 21); err == nil {
		t.Fatal("K=1 override admitted on mainnet")
	}
}
