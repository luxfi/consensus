// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// flatSet is a StakeSource over n seats of equal weight, every one of them keyed.
// It is the plainest possible set: no spectators, no lopsided weights, nothing for
// a case to turn on except how many parties there are.
type flatSet struct {
	vs *testValidatorSet
	n  int
	w  uint64
}

func (s flatSet) Weight(id ids.NodeID, _ uint64) uint64 {
	for i := 0; i < s.n; i++ {
		if s.vs.nodeID(i) == id {
			return s.w
		}
	}
	return 0
}

func (s flatSet) SignerStake(uint64) uint64  { return uint64(s.n) * s.w }
func (s flatSet) SignerCount(uint64) int     { return s.n }
func (s flatSet) CarriedStake(uint64) uint64 { return uint64(s.n) * s.w }

// unanimous assembles a Quasar cert signed by every seat of the set.
func (s flatSet) unanimous(t *testing.T) *QuorumCert {
	t.Helper()
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 9, Round: 1}
	votes := make([]SignedVote, 0, s.n)
	for i := 0; i < s.n; i++ {
		votes = append(votes, SignedVote{NodeID: s.vs.nodeID(i), Accept: true, Signature: []byte{0x01}})
	}
	cert, err := AssembleQuorumCert(pos, Quasar, uint32(len(votes)), votes)
	if err != nil {
		t.Fatalf("assemble over %d seats: %v", s.n, err)
	}
	// The certificate declares the floor this set derives for the export rung — the
	// only threshold VerifyWeighted admits — so a row that refuses is refused on the
	// committee floor and not on a number the certificate chose for itself.
	cert.Threshold = uint32(SignerFloor(Quasar, s.n))
	return cert
}

// TestExportNeedsAByzantineCommittee is the floor the export rung has on the SET.
//
// A supermajority is a claim about a fault budget: f = ⌊(n−1)/3⌋ validators may be
// arbitrarily malicious and the remainder still agree on one history. Below four
// signers that budget is ZERO. One, two or three parties can produce a unanimous
// certificate carrying a hundred per cent of the signer stake, and it tolerates
// nothing — a single compromised key is not one fault absorbed by a margin, it is
// a forged export certificate that every verifier on every chain accepts.
//
// Neither of the rung's two quorum floors catches it, and that is the point. Both
// are read over n, so both shrink with it: at n=1, ⌊2·1/3⌋+1 is 1 and one signature
// is a supermajority of one, over a stake floor of ⌊2·w/3⌋ that the same signature
// clears outright. The quorum predicates are satisfiable at every set size; whether
// the answer MEANS anything is a separate question, and this is where it is asked.
//
// The floor is minBFTCommittee, the same constant committee SELECTION floors at:
// one definition of "the smallest set a Byzantine argument can be made about",
// enforced once at the sampler and once at the certificate.
func TestExportNeedsAByzantineCommittee(t *testing.T) {
	vs := newTestValidatorSet(minBFTCommittee)

	for n := 1; n < minBFTCommittee; n++ {
		set := flatSet{vs: vs, n: n, w: 100}
		cert := set.unanimous(t)

		// Both quorum floors are MET. If either were short, the refusal below
		// would prove nothing about the committee clause.
		signer := set.SignerStake(0)
		if voted := uint64(n) * 100; voted <= config.TwoThirdsStakeFloor(signer) {
			t.Fatalf("n=%d: unanimity holds %d of %d, which does not clear the stake "+
				"floor %d — this case does not reach the committee clause",
				n, voted, signer, config.TwoThirdsStakeFloor(signer))
		}
		if floor := config.TwoThirdsCount(n); cert.VoterCount() < floor {
			t.Fatalf("n=%d: unanimity is %d signers, below the count floor %d — this case "+
				"does not reach the committee clause", n, cert.VoterCount(), floor)
		}

		err := cert.VerifyWeighted(alwaysValid{}, set, 0)
		if err == nil {
			t.Errorf("n=%d: %d signers holding every unit of stake minted an EXPORT "+
				"certificate. f=⌊(n−1)/3⌋ is %d there, so the certificate tolerates no "+
				"Byzantine fault and one compromised key forges it",
				n, n, (n-1)/3)
			continue
		}
		if !errors.Is(err, ErrQCBelowThreshold) {
			t.Errorf("n=%d: refused with %v, want ErrQCBelowThreshold — a stake refusal "+
				"here would mean the committee clause never ran", n, err)
		}
	}

	// And at the floor the same shape carries: this is a floor on the set, not a
	// ban on small chains certifying anything.
	set := flatSet{vs: vs, n: minBFTCommittee, w: 100}
	if err := set.unanimous(t).VerifyWeighted(alwaysValid{}, set, 0); err != nil {
		t.Fatalf("n=%d is the minimum Byzantine committee and its unanimous certificate "+
			"was refused: %v", minBFTCommittee, err)
	}
}

// TestExportCommitteeFloorLeavesNovaAlone — the clause belongs to the export rung
// and must not migrate down the ladder.
//
// Nova authorizes LOCAL EXECUTION, which the chain can still reorg away, and it is
// crash-fault-safe rather than Byzantine-safe by construction. A four-signer floor
// there would stop a small or a partitioned chain making any progress at all, in
// exchange for a guarantee the rung never offered. Its own floor is
// NovaSignerFloor, which saturates BELOW the committee size on purpose.
func TestExportCommitteeFloorLeavesNovaAlone(t *testing.T) {
	vs := newTestValidatorSet(minBFTCommittee)

	for n := 1; n < minBFTCommittee; n++ {
		set := flatSet{vs: vs, n: n, w: 100}
		pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 9, Round: 1}
		votes := make([]SignedVote, 0, n)
		for i := 0; i < n; i++ {
			votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: []byte{0x01}})
		}
		cert, err := AssembleQuorumCert(pos, Nova, uint32(len(votes)), votes)
		if err != nil {
			t.Fatalf("n=%d: assemble nova: %v", n, err)
		}
		cert.Threshold = uint32(SignerFloor(Nova, n))
		if err := cert.VerifyWeighted(alwaysValid{}, set, 0); err != nil {
			t.Errorf("n=%d: a unanimous NOVA certificate was refused (%v). Nova ignites "+
				"local execution on a bare majority and has no Byzantine claim to protect; "+
				"the export rung's committee floor has leaked down a rung", n, err)
		}
	}
}

// countOnlyEngine is a real stake-less engine whose committee is exactly k.
//
// With no stake source effectiveCommittee answers the CONFIGURED K verbatim — no
// clamp, no floor — so k IS the number verifyCert reads a declaration against, and
// setting it is how a case names the committee it is asking about.
func countOnlyEngine(t *testing.T, vs *testValidatorSet, k int) (*Transitive, ids.ID) {
	t.Helper()
	chainID := ids.GenerateTestID()
	e := NewWithConfig(
		Config{Params: config.Parameters{K: k, AlphaPreference: k, AlphaConfidence: k, Beta: 2}},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	if e.stakeSource != nil {
		t.Fatal("this engine is supposed to have no stake model; the case tests the count-only road")
	}
	if got, _ := e.effectiveCommittee(1); got != k {
		t.Fatalf("the committee is %d and the case asked for %d — it is not testing the size it says it is", got, k)
	}
	return e, chainID
}

// arrival is a certificate as it ARRIVES: unanimous over the committee's k seats,
// declaring the quorum that committee derives. Everything a certificate can be made
// to satisfy is satisfied, so whatever refuses it refuses it on the committee.
func arrival(t *testing.T, vs *testValidatorSet, chainID ids.ID, tier Finality, k int) *QuorumCert {
	t.Helper()
	pos := VotePosition{ChainID: chainID, Height: 9, Round: 1, BlockID: ids.GenerateTestID()}
	votes := make([]SignedVote, 0, k)
	for i := 0; i < k; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, tier, uint32(Quorum(tier, k)), votes)
	if err != nil {
		t.Fatalf("assemble %s over %d seats: %v", tier, k, err)
	}
	return cert
}

// TestExportCommitteeFloorHoldsOnTheCountOnlyRoad is the same floor as
// TestExportNeedsAByzantineCommittee, on the OTHER road.
//
// There are two roads to the export rung and only one of them had this clause. The
// weighted road reads the floor off the stake source (verifyQuasarSupermajority);
// the count-only road — a chain with no stake model — reads its committee from
// effectiveCommittee, and read NOTHING against it but the derived quorum. That
// clause cannot catch a small committee, because it shrinks with it:
// TwoThirdsCount(2) is 2, so two signatures over a two-member committee ARE the
// number that committee derives and the certificate passes as the one this node
// would have built itself.
//
// What passed there was export finality with a Byzantine budget of zero, and it did
// not stop at the node that admitted it: TryAccept re-gossips the bytes and
// applyBranchFinalization serves them as the catch-up proof, so one admission made
// the node a second source for a certificate carrying no fault budget.
//
// It had to arrive from a peer and it took every key in the committee, which is why
// this was a gap in Go's own uniformity rather than a live forgery. The road it
// joins is Go's WEIGHTED road, pinned by the test above — same sentinel, same
// TwoThirdsCount number. There is no third implementation in the comparison: Rust's
// Cert::verify is the structural predicate its weighted road runs first, not an
// accept rule, and C++'s verify_cert fails closed on an empty stake model, so
// neither has a stake-less accept road for this clause to exist on.
func TestExportCommitteeFloorHoldsOnTheCountOnlyRoad(t *testing.T) {
	vs := newTestValidatorSet(minBFTCommittee)

	for n := 1; n < minBFTCommittee; n++ {
		e, chainID := countOnlyEngine(t, vs, n)
		cert := arrival(t, vs, chainID, Quasar, n)

		// Both clauses this road already had are MET. If either were short, the
		// refusal below would prove nothing about the committee clause.
		if err := cert.Verify(vs, 1); err != nil {
			t.Fatalf("committee=%d: the certificate does not clear the structural predicate (%v), "+
				"so this case never reaches the committee clause", n, err)
		}
		if floor := Quorum(Quasar, n); int(cert.Threshold) != floor {
			t.Fatalf("committee=%d: the certificate declares %d where the committee derives %d, so "+
				"the derived clause refuses it and the committee clause never runs", n, cert.Threshold, floor)
		}

		err := e.verifyCert(cert, 1)
		if err == nil {
			t.Errorf("committee=%d: a gossiped EXPORT certificate signed by every seat was ADMITTED. "+
				"f=⌊(n−1)/3⌋ is %d there, so it tolerates no Byzantine fault and one compromised key "+
				"forges irreversible finality — which this node would then re-gossip and serve on "+
				"catch-up. Go's weighted road refuses it",
				n, (n-1)/3)
			continue
		}
		if !errors.Is(err, ErrQCBelowThreshold) {
			t.Errorf("committee=%d: refused with %v, want ErrQCBelowThreshold — the weighted road's "+
				"own sentinel, so the two Go roads refuse the same certificate by the same name", n, err)
		}
	}

	// And at the floor the same shape carries: a floor on the committee, not a
	// retirement of the road.
	e, chainID := countOnlyEngine(t, vs, minBFTCommittee)
	if err := e.verifyCert(arrival(t, vs, chainID, Quasar, minBFTCommittee), 1); err != nil {
		t.Fatalf("committee=%d is the minimum Byzantine committee and its unanimous certificate "+
			"was refused: %v", minBFTCommittee, err)
	}
}

// TestCountOnlyCommitteeFloorLeavesNovaAlone — the clause belongs to the export
// rung on this road too, and must not migrate down the ladder.
//
// Nova authorizes local execution the chain can still reorg away. A four-member
// floor there would stop a small or partitioned stake-less chain making any
// progress at all, in exchange for a guarantee the rung never offered.
func TestCountOnlyCommitteeFloorLeavesNovaAlone(t *testing.T) {
	vs := newTestValidatorSet(minBFTCommittee)

	for n := 1; n < minBFTCommittee; n++ {
		e, chainID := countOnlyEngine(t, vs, n)
		if err := e.verifyCert(arrival(t, vs, chainID, Nova, n), 1); err != nil {
			t.Errorf("committee=%d: a unanimous NOVA certificate was refused (%v). Nova ignites "+
				"local execution and has no Byzantine claim to protect; the export rung's "+
				"committee floor has leaked down a rung", n, err)
		}
	}
}
