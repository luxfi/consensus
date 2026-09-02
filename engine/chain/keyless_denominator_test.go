// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// keylessSet is a StakeSource whose members are split into those that hold a key
// and those that do not. It is the shape a P-chain set has when it still carries
// stakers registered before a signing key was required: they hold weight, they
// are counted as members, and no signature of theirs will ever verify.
//
// The three projections are read over the SIGNERS, which is the rule this file
// exists to pin. Weight answers 0 for a keyless member for the same reason it
// answers 0 for a stranger: neither can put stake behind a vote.
type keylessSet struct {
	weight  map[ids.NodeID]uint64
	keyless map[ids.NodeID]bool
}

func (s *keylessSet) Weight(id ids.NodeID, _ uint64) uint64 {
	if s.keyless[id] {
		return 0
	}
	return s.weight[id]
}

func (s *keylessSet) SignerStake(_ uint64) uint64 {
	var total uint64
	for id, w := range s.weight {
		if !s.keyless[id] {
			total += w
		}
	}
	return total
}

func (s *keylessSet) SignerCount(_ uint64) int {
	n := 0
	for id := range s.weight {
		if !s.keyless[id] {
			n++
		}
	}
	return n
}

// TestKeylessStakeIsNotInTheDenominator is R5.
//
// Three members hold a hundred each and a key; a fourth holds two hundred and no
// key. Two fifths of what the chain carries therefore belongs to a member that
// can never cast a vote — past the third at which a denominator read over the
// whole membership puts the export rung permanently out of reach.
//
// Every member that CAN sign does. That is the whole of the signing set, and it
// is the strongest cert the set is capable of producing: if this one is refused,
// no cert is ever accepted at this epoch and export finality is stranded for good.
func TestKeylessStakeIsNotInTheDenominator(t *testing.T) {
	vs := newTestValidatorSet(4)
	set := &keylessSet{
		weight:  map[ids.NodeID]uint64{},
		keyless: map[ids.NodeID]bool{},
	}
	for i, w := range []uint64{100, 100, 100, 200} {
		set.weight[vs.nodeID(i)] = w
	}
	set.keyless[vs.nodeID(3)] = true

	const epoch = uint64(7)

	// What the set can actually produce, and what it would be measured against
	// if the member that cannot sign were counted.
	if got, want := set.SignerStake(epoch), uint64(300); got != want {
		t.Fatalf("signer stake = %d, want %d", got, want)
	}
	if got, want := set.SignerCount(epoch), 3; got != want {
		t.Fatalf("signer count = %d, want %d", got, want)
	}
	const carried = uint64(500)
	if strandedFloor := config.TwoThirdsStakeFloor(carried); set.SignerStake(epoch) > strandedFloor {
		t.Fatalf("the fixture does not reproduce R5: %d clears floor(2*%d/3)=%d, so the "+
			"membership-roll denominator would not have stranded it",
			set.SignerStake(epoch), carried, strandedFloor)
	}

	// The cert: every signer in the set, over one position.
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 9, Round: 1}
	votes := make([]SignedVote, 0, 3)
	for i := 0; i < 3; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: []byte{0x01}})
	}
	cert, err := AssembleQuorumCert(pos, Quasar, uint32(len(votes)), votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}

	if err := cert.VerifyWeighted(alwaysValid{}, set, epoch); err != nil {
		t.Fatalf("export refused with the whole signing set agreeing: %v\n"+
			"R5: the floor was read over stake that cannot vote", err)
	}

	// And the rung is still a rung: two of the three signers is short of two
	// thirds of the stake that CAN sign, so the fix did not flatten the floor
	// into an accept-anything.
	short, err := AssembleQuorumCert(pos, Quasar, 2, votes[:2])
	if err != nil {
		t.Fatalf("assemble short: %v", err)
	}
	if err := short.VerifyWeighted(alwaysValid{}, set, epoch); err == nil {
		t.Fatal("two hundred of three hundred cleared the export floor; the rung is gone")
	}
}

// alwaysValid resolves every signature as correct: this file is about the
// weighted half, and the keyless member never appears as a voter.
type alwaysValid struct{}

func (alwaysValid) VerifyVote(ids.NodeID, []byte, []byte, uint64) bool { return true }
