// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_epoch_binding_test.go — MEDIUM fix: stake-weighted finality must be
// evaluated against the validator set/stake AT THE CERT-POSITION EPOCH, and a
// cert must be cryptographically pinned to that epoch. Two properties:
//
//	(1) EPOCH-PINNING (soundness): a cert assembled from votes cast under
//	    validator-set-root R verifies only against R. Re-presenting it as
//	    certifying under a different set-root R' (a cross-epoch laundering) fails
//	    clause-6 signature verification — every signature was over R.
//	(2) NO-FLIP: an already-correct cert at (height H, root R), whose voters held
//	    a ⅔-stake supermajority AT H, is NOT flipped by a later stake change.
//	    VerifyWeighted reads stake at Position.Height, so a change at a different
//	    height cannot retroactively invalidate (or validate) the cert.
//
// Together these turn "the ⅔-by-stake predicate is measured at the cert-position
// epoch" from a Gate-D ASSUMPTION into an ENFORCED invariant.
package chain

import (
	"testing"

	"github.com/luxfi/ids"
)

// epochStakeSource is a height-aware test StakeSource: it returns the weighted
// set for the epoch that contains a given height. It exists to PROVE the engine
// reads stake at the cert-position height (the node's single-epoch stub discards
// height; this test double does not, so it can witness which height is read).
type epochStakeSource struct {
	// byHeightBoundary maps an inclusive lower-height boundary to a (weights,
	// total) snapshot. The snapshot for height h is the one with the greatest
	// boundary <= h.
	boundaries []uint64
	weights    map[uint64]map[ids.NodeID]uint64
	totals     map[uint64]uint64
}

func (e *epochStakeSource) snapshotFor(height uint64) (map[ids.NodeID]uint64, uint64) {
	var pick uint64
	found := false
	for _, b := range e.boundaries {
		if b <= height && (!found || b > pick) {
			pick, found = b, true
		}
	}
	if !found {
		return nil, 0
	}
	return e.weights[pick], e.totals[pick]
}

func (e *epochStakeSource) Weight(nodeID ids.NodeID, height uint64) uint64 {
	w, _ := e.snapshotFor(height)
	return w[nodeID]
}

func (e *epochStakeSource) TotalStake(height uint64) uint64 {
	_, t := e.snapshotFor(height)
	return t
}

// TestEpochBinding_CrossEpochCertRejected pins property (1): a cert is bound to
// the validator-set-root it was signed under; mutating the cert's position root
// to a different epoch's root makes every signature fail verification.
func TestEpochBinding_CrossEpochCertRejected(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	blockID := ids.GenerateTestID()

	rootEpochA := ids.GenerateTestID() // set-root at the cert's true epoch
	rootEpochB := ids.GenerateTestID() // a DIFFERENT epoch's set-root

	// Votes are cast under epoch-A's set-root.
	posA := VotePosition{
		ChainID: chainID, Height: 10, Round: 0, BlockID: blockID, ParentID: ids.Empty,
		ValidatorSetRoot: rootEpochA,
	}
	certA, err := AssembleQuorumCert(posA, 3, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, posA)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, posA)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, posA)},
	})
	if err != nil {
		t.Fatalf("assemble epoch-A cert: %v", err)
	}

	// Sound: the cert verifies against its OWN (epoch-A) position.
	if err := certA.Verify(vs); err != nil {
		t.Fatalf("epoch-A cert must verify against its own set-root: %v", err)
	}

	// Cross-epoch laundering: re-stamp the SAME signed votes under epoch-B's
	// root. The signatures were over epoch-A's canonical message, so verifying
	// against epoch-B's position MUST fail clause-6 (signature invalid).
	certLaundered := &QuorumCert{
		Version:   certA.Version,
		Type:      certA.Type,
		Position:  VotePosition{ChainID: chainID, Height: 10, Round: 0, BlockID: blockID, ParentID: ids.Empty, ValidatorSetRoot: rootEpochB},
		Threshold: certA.Threshold,
		Votes:     certA.Votes, // same signatures, different claimed epoch
	}
	if err := certLaundered.Verify(vs); err == nil {
		t.Fatal("MEDIUM: a cert re-presented under a DIFFERENT validator-set-root must FAIL verification (cross-epoch laundering)")
	}

	// And the canonical messages for the two epochs differ (the binding is real,
	// not incidental) — a direct assertion that the root is in the signed bytes.
	if string(CanonicalVoteMessage(posA)) == string(CanonicalVoteMessage(certLaundered.Position)) {
		t.Fatal("MEDIUM: positions differing only in ValidatorSetRoot must produce DIFFERENT canonical messages")
	}
}

// TestEpochBinding_StakeReadAtCertHeight pins property (2): VerifyWeighted reads
// stake at the cert's position height, and a stake change at a LATER height does
// not flip an already-correct cert.
func TestEpochBinding_StakeReadAtCertHeight(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	blockID := ids.GenerateTestID()
	root := ids.GenerateTestID()

	// Epoch boundaries: at heights [0,99] node0 holds 97/100 (a ⅔+ supermajority
	// with {0,1,2}); at heights [100,∞) node0 has UNBONDED to 1 and the stake is
	// spread {1,1,1,97} so {0,1,2} hold only 3/100 (below ⅔).
	ess := &epochStakeSource{
		boundaries: []uint64{0, 100},
		weights: map[uint64]map[ids.NodeID]uint64{
			0:   {vs.nodeID(0): 97, vs.nodeID(1): 1, vs.nodeID(2): 1, vs.nodeID(3): 1},
			100: {vs.nodeID(0): 1, vs.nodeID(1): 1, vs.nodeID(2): 1, vs.nodeID(3): 97},
		},
		totals: map[uint64]uint64{0: 100, 100: 100},
	}

	// A cert finalized at height 10 (epoch [0,99]) by {0,1,2}: 99/100 stake → valid.
	pos10 := VotePosition{ChainID: chainID, Height: 10, Round: 0, BlockID: blockID, ParentID: ids.Empty, ValidatorSetRoot: root}
	cert10, err := AssembleQuorumCert(pos10, 3, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos10)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos10)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos10)},
	})
	if err != nil {
		t.Fatalf("assemble height-10 cert: %v", err)
	}

	// Correct at its OWN epoch: stake read at height 10 → 99/100 > ⅔ → accepted.
	if err := cert10.VerifyWeighted(vs, ess); err != nil {
		t.Fatalf("cert at height 10 must verify against height-10 stake (99/100): %v", err)
	}

	// NO-FLIP: the later stake change (node0 unbonds at height 100) does NOT
	// affect this cert — its Position.Height is 10, so VerifyWeighted still reads
	// the height-10 snapshot. Re-verify is stable.
	if err := cert10.VerifyWeighted(vs, ess); err != nil {
		t.Fatalf("a later-epoch stake change must NOT flip an already-correct height-10 cert: %v", err)
	}

	// Contrast — PROVE the source actually returns the worse snapshot at height
	// 100, so the no-flip above is meaningful (not a vacuous test). A cert built
	// at height 100 by {0,1,2} holds only 3/100 → MUST be rejected.
	pos100 := VotePosition{ChainID: chainID, Height: 100, Round: 0, BlockID: ids.GenerateTestID(), ParentID: blockID, ValidatorSetRoot: ids.GenerateTestID()}
	cert100, err := AssembleQuorumCert(pos100, 3, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos100)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos100)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos100)},
	})
	if err != nil {
		t.Fatalf("assemble height-100 cert: %v", err)
	}
	if err := cert100.VerifyWeighted(vs, ess); err == nil {
		t.Fatal("a cert at height 100 by {0,1,2} holds only 3/100 of stake and MUST be rejected (proves height-100 snapshot is read)")
	}
}

// TestEpochBinding_RoundTripPreservesRoot guards the wire codec: the new
// ValidatorSetRoot must survive Marshal→Unmarshal so a gossiped cert carries its
// epoch binding to followers.
func TestEpochBinding_RoundTripPreservesRoot(t *testing.T) {
	vs := newTestValidatorSet(3)
	root := ids.GenerateTestID()
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 7, Round: 2, BlockID: ids.GenerateTestID(), ParentID: ids.GenerateTestID(), ValidatorSetRoot: root}
	cert, err := AssembleQuorumCert(pos, 3, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := UnmarshalQuorumCert(b)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Position.ValidatorSetRoot != root {
		t.Fatalf("ValidatorSetRoot lost on the wire: got %s want %s", got.Position.ValidatorSetRoot, root)
	}
	if !cert.Equal(got) {
		t.Fatal("cert did not round-trip through the wire codec with the set-root field")
	}
	// The decoded cert still verifies (sigs bound to the same root).
	if err := got.Verify(vs); err != nil {
		t.Fatalf("decoded cert must still verify: %v", err)
	}
}
