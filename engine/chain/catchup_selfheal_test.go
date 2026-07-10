// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// catchup_selfheal_test.go — the behind-node SELF-HEAL fix: a verified cert for a block whose
// INTERMEDIATE ancestor is missing must trigger the ancestor fetch, not just log REFUSED.
//
// The live mainnet wedge (1086xxx): a slipped validator (192 behind) received verified α-of-K
// certs for blocks it tracked but whose ancestors it lacked; the finalize fold returned
// ErrAncestorNotTracked, HandleIncomingCert logged "REFUSED ... (behind; fetch and retry)" and
// returned WITHOUT ever calling requestCatchup — so the node never self-healed, stayed effectively
// n−1 at zero margin, and any flap dropped the fleet below α and stalled finality. The only fetch
// trigger in HandleIncomingCert was gated behind "cert's OWN block untracked", never the
// missing-intermediate-ancestor case.
//
// The fix: the fold surfaces the specific untracked ancestor as a typed *AncestorNotTracked, and
// HandleIncomingCert's error arm errors.As it and issues exactly one requestCatchup for that id.

package chain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// mapAncestry is a tiny read-only Ancestry over an explicit parent/height/canonical map, so the
// pure fold can be driven with an arbitrary (partial) tree.
type mapAncestry map[ids.ID]struct {
	parent          ids.ID
	height          uint64
	canonical       ids.ID
	parentCanonical ids.ID
}

func (m mapAncestry) Parent(id ids.ID) (ids.ID, uint64, ids.ID, ids.ID, bool) {
	e, ok := m[id]
	return e.parent, e.height, e.canonical, e.parentCanonical, ok
}

func (m mapAncestry) Children(id ids.ID) []ids.ID {
	var out []ids.ID
	for cid, e := range m {
		if e.parent == id {
			out = append(out, cid)
		}
	}
	return out
}

func (m mapAncestry) WrapperByCanonical(canonical ids.ID, height uint64) (ids.ID, bool) {
	for id, e := range m {
		if e.height == height && e.canonical == canonical {
			return id, true
		}
	}
	return ids.Empty, false
}

// TestFinalize_MissingAncestor_TypedError pins the fold half of the fix: an untracked ancestor on
// the certified path yields a *AncestorNotTracked carrying the SPECIFIC missing id, while
// errors.Is(err, ErrAncestorNotTracked) still holds (so existing classifiers are unchanged).
func TestFinalize_MissingAncestor_TypedError(t *testing.T) {
	tip := ids.GenerateTestID()
	led := seedLedger(tip, tip, 5) // certified through height 5

	missingParent := ids.GenerateTestID() // the height-7 ancestor we do NOT track
	certBlock := ids.GenerateTestID()      // certified block at height 8
	cert := Cert{Block: certBlock, Parent: missingParent, Height: 8, Canonical: certBlock}

	_, _, err := Finalize(led, cert, mapAncestry{ /* nothing tracked → parent walk misses */ })
	if err == nil {
		t.Fatal("a cert whose ancestor is untracked must NOT finalize (fail-closed defer)")
	}
	if !errors.Is(err, ErrAncestorNotTracked) {
		t.Fatalf("errors.Is(err, ErrAncestorNotTracked) must still hold; got %v", err)
	}
	var ant *AncestorNotTracked
	if !errors.As(err, &ant) {
		t.Fatalf("error must be the typed *AncestorNotTracked (so the fetch can target Missing); got %T", err)
	}
	if ant.Missing != missingParent {
		t.Fatalf("Missing = %s, want the untracked ancestor %s", ant.Missing, missingParent)
	}
	if ant.Target != certBlock {
		t.Fatalf("Target = %s, want the certified block %s", ant.Target, certBlock)
	}
}

// TestSelfHeal_CertMissingIntermediateAncestor_TriggersFetch is the receive-side half: a behind
// node that TRACKS the certified block but is missing an intermediate ancestor must fire exactly
// one ancestor fetch for the missing block when the verified cert arrives — the self-heal that was
// absent on the live path.
func TestSelfHeal_CertMissingIntermediateAncestor_TriggersFetch(t *testing.T) {
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	cu := &recordingCatchup{}
	rec := &recordingGossiper{}

	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, rec, vs.signerFor(0)),
		WithCatchup(cu),
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	rt := &Runtime{Transitive: e, config: NetworkConfig{
		ChainID: chainID, NetworkID: ids.GenerateTestID(), Logger: log.Noop(),
		Gossiper: &certQuorumGossiper{rec: rec}, Catchup: cu,
	}}

	// This node is CERTIFIED through height 5 (tip5) — so a cert at height 8 makes the fold WALK
	// the ancestry (not first-finalize) and discover the gap.
	tip5 := ids.GenerateTestID()
	e.consensus.mu.Lock()
	e.consensus.ledger = seedLedger(tip5, tip5, 5)
	e.consensus.mu.Unlock()

	// TRACK a certified block at height 8 whose parent (height 7) we have NEVER tracked — the
	// missing-intermediate-ancestor state. trackVerifiedBlock inserts it into consensus + pending
	// WITHOUT going through the gossip parent-fetch, so only the CERT path can trigger the fetch.
	missingParent := ids.GenerateTestID()
	blk8 := &verifyOnceBlock{
		id: ids.GenerateTestID(), parentID: missingParent, height: 8,
		timestamp: time.Now(), bytes: []byte("certified-8-missing-ancestor"),
	}
	trackVerifiedBlock(rt, blk8, 0)

	// A VALID α-of-K cert (all 5 sign) for the tracked height-8 block.
	certBytes := buildCertAtRound(t, vs, chainID, blk8.id, missingParent, 8, 0, 5)

	if got := rt.HandleIncomingCert(certBytes); got {
		t.Fatal("a cert with a missing ancestor must NOT finalize (behind; deferred)")
	}

	// THE FIX: exactly one ancestor fetch, targeting the MISSING intermediate ancestor.
	if got := cu.count(); got != 1 {
		t.Fatalf("a verified cert with a missing intermediate ancestor must trigger ONE ancestor fetch, got %d "+
			"(pre-fix: 0 — the node logged REFUSED and never self-healed)", got)
	}
	if m, _ := cu.lastMissing(); m != missingParent {
		t.Fatalf("catch-up must target the MISSING ANCESTOR %s, got %s", missingParent, m)
	}
}
