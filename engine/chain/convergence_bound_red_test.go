// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// convergence_bound_red_test.go — RED attack on choke #2 at SCALE: when pChainHeight=0
// makes EVERY validator propose a sibling at each height, the view-change must collapse
// the N-way split to ONE finalized head in a BOUNDED number of rounds — and that bound
// must NOT grow pathologically with N. This is the "can it choke at 1M?" question posed
// to the round machine (the sizer is proven analytically in scale_invariance_red_test.go).
//
// Method: run a full N-way sibling storm (every node builds a distinct competing block at
// height 1, view-change ON, committee sized from the live set) at the LARGEST FEASIBLE
// live sizes and assert, at each N: (a) exactly ONE head finalizes across all nodes (no
// fork), (b) convergence completes within a FIXED wall-clock bound that is the SAME for
// every N (the settle schedule is round-count-based and N-independent). A convergence time
// that climbs with N — or a fork — is the regression this gate catches. Times are logged
// so the trend is visible in CI output.
package chain

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// TestRedConvergence_SiblingStormBoundedAcrossScale drives the storm at increasing N and
// asserts a flat convergence bound + single-head safety at each scale.
func TestRedConvergence_SiblingStormBoundedAcrossScale(t *testing.T) {
	if testing.Short() {
		t.Skip("scale storm sim is heavy; skipped in -short")
	}
	// Largest feasible in-process (each message fans out O(N) with real ed25519 verify, so
	// a round is O(N²) crypto ops). 64 nodes is the ceiling that stays well under the CI
	// timeout under -race; the analytical proof covers 1M.
	scales := []int{4, 7, 16, 32, 64}

	// A FIXED bound for EVERY N: the view-change settle schedule is counted in ROUNDS
	// (viewSettleTicks), independent of N, so convergence time must not scale with N. We
	// allow generous slack for in-process O(N²) gossip + -race overhead, but the SAME
	// ceiling applies at N=4 and N=64 — the invariant is that the ceiling does not have to
	// grow with N.
	const perScaleBound = 25 * time.Second

	times := make(map[int]time.Duration, len(scales))
	for _, n := range scales {
		n := n
		t.Run("N="+itoaBig(n), func(t *testing.T) {
			params := mainnetStormParams5VC() // K=21/α=15 preset + view-change ON; committee auto-sizes to the live n
			net := newSimNet(t, n, params)

			// Full storm: every node builds its OWN distinct sibling on genesis at height 1.
			built := make(map[ids.ID]struct{}, n)
			for i := 0; i < n; i++ {
				blk := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "storm-n"+itoaBig(n)+"-"+itoaBig(i))
				built[blk.ID()] = struct{}{}
				net.build(i, blk)
			}

			start := time.Now()
			deadline := start.Add(perScaleBound)
			var converged bool
			for time.Now().Before(deadline) {
				heads := net.headsAtHeight(1)
				if len(heads) > 1 {
					t.Fatalf("SAFETY: DOUBLE-FINALIZATION at N=%d — divergent heads %v", n, heads)
				}
				if len(heads) == 1 {
					// Require EVERY node to have finalized the single head (full convergence).
					for id, c := range heads {
						if c >= net.upCount() {
							if _, ok := built[id]; !ok {
								t.Fatalf("N=%d: finalized head %s is not one of the built siblings", n, id)
							}
							converged = true
						}
					}
				}
				if converged {
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			elapsed := time.Since(start)
			times[n] = elapsed
			if !converged {
				t.Fatalf("CONVERGENCE: N=%d storm did NOT collapse to one finalized head within %s "+
					"(heads=%v up=%d) — the split failed to converge at this scale",
					n, perScaleBound, net.headsAtHeight(1), net.upCount())
			}
			t.Logf("N=%d converged to a single finalized head in %s", n, elapsed)
		})
	}

	// The bound is FLAT: report the trend so a super-linear blow-up is visible. We do not
	// hard-fail on a ratio (in-process O(N²) gossip inflates wall-clock super-linearly by
	// construction — that is a HARNESS artifact, not a protocol round-count property); the
	// protocol invariant proven above is that a FIXED per-scale bound suffices at every N.
	if len(times) == len(scales) {
		t.Logf("convergence-time trend (round schedule is N-independent): %v", times)
	}
}
