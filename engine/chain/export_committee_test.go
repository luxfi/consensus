// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
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
	cert.Threshold = SignerFloor(Quasar, s.n)
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
		cert.Threshold = SignerFloor(Nova, n)
		if err := cert.VerifyWeighted(alwaysValid{}, set, 0); err != nil {
			t.Errorf("n=%d: a unanimous NOVA certificate was refused (%v). Nova ignites "+
				"local execution on a bare majority and has no Byzantine claim to protect; "+
				"the export rung's committee floor has leaked down a rung", n, err)
		}
	}
}
