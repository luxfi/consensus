// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// zz_red_replay_rebind_test.go — RED regression guard (adversarial). Kept: it fails the moment the binding/contiguity fix is reverted.
//
// certVouchesFor binds the cert to the block through Position.BlockID. That field is
// NOT in CanonicalVoteMessage (cert_codec.go:34 says so on the wire, cert.go:245 in the
// message builder), so it is free for anyone holding a cert to rewrite without touching
// a signature. This probe rewrites it and serves an unrelated block.
package chain

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

func TestRED_ReplayAcceptsAnyBlockViaRebindingAnHonestCert(t *testing.T) {
	const N = uint64(1_000_000)
	const k = 5

	vs := newTestValidatorSet(5)
	base := newCatchupVM()
	vm := &advancingVM{catchupVM: base}
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	tip := newTestBlock(N, ids.Empty, "applied@N")
	base.register(tip)
	if err := vm.SetPreference(context.Background(), tip.id); err != nil {
		t.Fatalf("seed applied head: %v", err)
	}
	gap := buildGap(base, tip, k)

	// Boot-seed shape: the ledger knows only the top of the gap.
	top := gap[k-1]
	if _, err := rt.Transitive.consensus.FinalizeBranch(top.id, top.height, top.parentID); err != nil {
		t.Fatalf("seed ledger: %v", err)
	}
	if _, _, known := rt.Transitive.consensus.FinalizedAt(gap[0].height); known {
		t.Fatalf("precondition: ledger must not know height %d", gap[0].height)
	}

	// The attacker's block. Never proposed, never voted on, never finalized. It only
	// has to be at a height in the gap.
	evil := newTestBlock(gap[0].height, ids.GenerateTestID(), "EVIL@N+1")
	// A real peer supplies bytes; the VM parses them. It is NOT in our block store, so
	// held==false and the normal Verify runs.
	base.mu.Lock()
	base.byBytes[string(evil.bytes)] = evil
	base.mu.Unlock()

	// An HONEST cert for gap[0] — obtainable from any peer via CertForBlock. Decode it
	// and rewrite ONLY the two unsigned transport ids, promoting the original outer id
	// into the explicit canonical slot so canonicalVoteMessageFor rebuilds byte-identical
	// input (cert.go:268 — the Empty-canonical degrade is what makes this free).
	honest := catchupCertFor(t, vs, chainID, gap[0], []int{0, 1, 2, 3}, 3)
	qc, err := UnmarshalQuorumCert(honest)
	if err != nil {
		t.Fatalf("decode honest cert: %v", err)
	}
	qc.Position.CanonicalID = gap[0].id       // was Empty (degraded to BlockID)
	qc.Position.ParentCanonicalID = gap[0].parentID
	qc.Position.BlockID = evil.id             // <- unsigned
	qc.Position.ParentID = evil.parentID      // <- unsigned
	forged, err := qc.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal forged cert: %v", err)
	}

	// Sanity: the forged cert still verifies as a real 4-of-5 quorum cert.
	if verr := rt.Transitive.verifyCert(qc, 0); verr != nil {
		t.Fatalf("rebinding broke the signatures (attack does not apply): %v", verr)
	}

	err = rt.AcceptCatchupBlock(context.Background(), evil.bytes, forged)
	if got := evil.AcceptCalled(); got != 0 {
		t.Fatalf("EXPLOITED: an uncertified attacker block was applied to the VM "+
			"(Accept called %d times, AcceptCatchupBlock err=%v)", got, err)
	}
	t.Logf("refused: %v", err)
}
