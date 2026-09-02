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

// CarriedStake is every member's weight, keyed or not — what the chain CARRIES. No floor
// is read against it; it is here so the gap the rest of this file is about is a number an
// operator can see rather than one only the verifier knows.
func (s *keylessSet) CarriedStake(_ uint64) uint64 {
	var total uint64
	for _, w := range s.weight {
		total += w
	}
	return total
}

// TestKeylessStakeIsNotInTheDenominator is R5, the STAKE half.
//
// Six members hold a hundred each and a key; a seventh holds three hundred and no
// key. A third of what the chain carries therefore belongs to a member that can
// never cast a vote — the point at which a denominator read over the whole
// membership puts the export rung permanently out of reach.
//
// The COUNT floor is deliberately the same number either way here — ⌊2·6/3⌋+1 and
// ⌊2·7/3⌋+1 are both 5 — so the stake denominator is the only thing that decides
// this case, and a fix that moved only the count would fail it.
//
// Every member that CAN sign does. That is the whole of the signing set, and it
// is the strongest cert the set is capable of producing: if this one is refused,
// no cert is ever accepted at this epoch and export finality is stranded for good.
func TestKeylessStakeIsNotInTheDenominator(t *testing.T) {
	vs := newTestValidatorSet(7)
	set := &keylessSet{
		weight:  map[ids.NodeID]uint64{},
		keyless: map[ids.NodeID]bool{},
	}
	for i, w := range []uint64{100, 100, 100, 100, 100, 100, 300} {
		set.weight[vs.nodeID(i)] = w
	}
	set.keyless[vs.nodeID(6)] = true

	const epoch = uint64(7)

	// What the set can actually produce, and what it would be measured against
	// if the member that cannot sign were counted.
	if got, want := set.SignerStake(epoch), uint64(600); got != want {
		t.Fatalf("signer stake = %d, want %d", got, want)
	}
	if got, want := set.SignerCount(epoch), 6; got != want {
		t.Fatalf("signer count = %d, want %d", got, want)
	}
	carried := set.CarriedStake(epoch)
	if carried != 900 {
		t.Fatalf("carried stake = %d, want 900", carried)
	}
	if strandedFloor := config.TwoThirdsStakeFloor(carried); set.SignerStake(epoch) > strandedFloor {
		t.Fatalf("the fixture does not reproduce R5: %d clears floor(2*%d/3)=%d, so the "+
			"membership-roll denominator would not have stranded it",
			set.SignerStake(epoch), carried, strandedFloor)
	}
	// And the count is NOT what strands it, so this case isolates the stake half.
	if config.TwoThirdsCount(6) != config.TwoThirdsCount(7) {
		t.Fatalf("the count floors differ over six signers and seven members (%d vs %d); "+
			"this case is supposed to turn on stake alone",
			config.TwoThirdsCount(6), config.TwoThirdsCount(7))
	}

	// The cert: every signer in the set, over one position.
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 9, Round: 1}
	votes := make([]SignedVote, 0, 6)
	for i := 0; i < 6; i++ {
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

	// And the rung is still a rung: four of the six signers is short of two thirds
	// of the stake that CAN sign, so the fix did not flatten the floor into an
	// accept-anything.
	short, err := AssembleQuorumCert(pos, Quasar, 4, votes[:4])
	if err != nil {
		t.Fatalf("assemble short: %v", err)
	}
	if err := short.VerifyWeighted(alwaysValid{}, set, epoch); err == nil {
		t.Fatal("four hundred of six hundred cleared the export floor; the rung is gone")
	}
}

// TestKeylessCountIsNotInTheDenominator is R5, the COUNT half, isolated.
//
// Four members hold a hundred each and a key; two more hold ONE each and no key.
// The keyless weight is deliberately negligible, so the stake floor is cleared
// whichever denominator it is read against — 400 exceeds ⌊2·400/3⌋=266 and
// ⌊2·402/3⌋=268 alike. Only the COUNT can decide this case: ⌊2·4/3⌋+1 is 3 over
// the signers and ⌊2·6/3⌋+1 is 5 over the roll, and four signatures is every one
// the set can produce.
//
// It is the case a stake-only fix passes and a correct one must also pass. Two
// members with a rounding error's worth of stake between them would otherwise
// strand the export rung of a chain whose four real validators all agree.
func TestKeylessCountIsNotInTheDenominator(t *testing.T) {
	vs := newTestValidatorSet(6)
	set := &keylessSet{
		weight:  map[ids.NodeID]uint64{},
		keyless: map[ids.NodeID]bool{},
	}
	for i, w := range []uint64{100, 100, 100, 100, 1, 1} {
		set.weight[vs.nodeID(i)] = w
	}
	set.keyless[vs.nodeID(4)] = true
	set.keyless[vs.nodeID(5)] = true

	const epoch = uint64(7)

	if got, want := set.SignerCount(epoch), 4; got != want {
		t.Fatalf("signer count = %d, want %d", got, want)
	}
	// The stake half is satisfied under EITHER reading, so it cannot be what
	// decides — this is what makes the case about the count and nothing else.
	signer, carried := set.SignerStake(epoch), set.CarriedStake(epoch)
	if signer <= config.TwoThirdsStakeFloor(signer) || signer <= config.TwoThirdsStakeFloor(carried) {
		t.Fatalf("the fixture does not isolate the count: %d of %d carried does not clear "+
			"both stake floors (%d and %d)", signer, carried,
			config.TwoThirdsStakeFloor(signer), config.TwoThirdsStakeFloor(carried))
	}
	// And the count floors really do differ, or the case proves nothing.
	if config.TwoThirdsCount(6) <= 4 {
		t.Fatalf("⌊2*6/3⌋+1 = %d is within reach of four signers; the roll denominator "+
			"would not have stranded this set", config.TwoThirdsCount(6))
	}

	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 9, Round: 1}
	votes := make([]SignedVote, 0, 4)
	for i := 0; i < 4; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: []byte{0x01}})
	}
	cert, err := AssembleQuorumCert(pos, Quasar, uint32(len(votes)), votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := cert.VerifyWeighted(alwaysValid{}, set, epoch); err != nil {
		t.Fatalf("export refused with all four signers agreeing: %v\n"+
			"R5: the COUNT floor was read over seats that cannot sign", err)
	}

	// The rung is still a rung, and its edge is sharp on the signer denominator:
	// three signers sit exactly ON ⌊2*4/3⌋+1 and carry, two sit below it and do not.
	edge, err := AssembleQuorumCert(pos, Quasar, 3, votes[:3])
	if err != nil {
		t.Fatalf("assemble edge: %v", err)
	}
	if err := edge.VerifyWeighted(alwaysValid{}, set, epoch); err != nil {
		t.Fatalf("three signers sit on the export floor and were refused: %v", err)
	}
	short, err := AssembleQuorumCert(pos, Quasar, 2, votes[:2])
	if err != nil {
		t.Fatalf("assemble short: %v", err)
	}
	if err := short.VerifyWeighted(alwaysValid{}, set, epoch); err == nil {
		t.Fatal("two of four signers cleared the export rung; the floor is gone")
	}
}

// TestCarriedStakeIsVisibleToAnOperator — the signal the denominator change would
// otherwise have removed.
//
// Once the floors are read over the signers, a chain whose members lose their keys
// keeps certifying CORRECTLY: two thirds of a shrinking denominator is still two
// thirds of it, and ResponsiveStakePct — voted over signer — stays at one the whole
// way down. The number that falls is the one no floor reads, and it is only
// reportable because the seam carries it.
//
// Six signers hold a hundred each and a seventh member holds nine thousand and no
// key. Every signer votes: the responsive ratio is a perfect 1.0, and the signing
// set is six per cent of what the chain carries. An operator alarming on the first
// number sees nothing; on the second, everything.
func TestCarriedStakeIsVisibleToAnOperator(t *testing.T) {
	vs := newTestValidatorSet(7)
	set := &keylessSet{
		weight:  map[ids.NodeID]uint64{},
		keyless: map[ids.NodeID]bool{},
	}
	for i, w := range []uint64{100, 100, 100, 100, 100, 100, 9000} {
		set.weight[vs.nodeID(i)] = w
	}
	set.keyless[vs.nodeID(6)] = true

	const epoch = uint64(0)
	votes := make([]SignedVote, 0, 6)
	for i := 0; i < 6; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: []byte{0x01}})
	}

	e, _ := newQuorumEngineOpts(t, dyn5(), vs, 0, &recordingGossiper{}, WithStakeWeighting(set))
	e.recordResponsiveStake(set, votes, epoch)

	s := e.FinalityStatus()
	if s.ResponsiveStakePct != 1 {
		t.Fatalf("responsiveStakePct = %v, want 1: every signer voted, so the ratio a "+
			"quorum has to clear is perfect and stays perfect however far the set shrinks",
			s.ResponsiveStakePct)
	}
	if got, want := s.SignerStakePct, 600.0/9600.0; got != want {
		t.Fatalf("signerStakePct = %v, want %v — the fraction of what the chain carries "+
			"that can sign at all is the signal an operator alarms on", got, want)
	}
	if s.SignerStakePct >= 0.1 {
		t.Fatalf("the fixture does not reproduce the case: %v of the carried stake can "+
			"sign, which is not a set that has shrunk out from under its finality",
			s.SignerStakePct)
	}
}

// alwaysValid resolves every signature as correct: this file is about the
// weighted half, and the keyless member never appears as a voter.
type alwaysValid struct{}

func (alwaysValid) VerifyVote(ids.NodeID, []byte, []byte, uint64) bool { return true }
