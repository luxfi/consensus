// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// derived_authority_test.go — a certificate states its quorum; it does not choose it.
//
// The floor a certificate is counted against is a function of the SET it is
// weighed against and the RUNG it attests. The Threshold field carries that
// number so a reader knows what was required, and VerifyWeighted checks the
// field against what the set actually derives — so the field is a claim, never a
// value the verifier adopts.
//
// Without the clause the field is load-bearing on the count-only road, because
// Verify's last clause counts distinct valid accepts against the certificate's
// OWN Threshold. A certificate declaring 1 clears it on one signature.
package chain

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// certDeclaring assembles a certificate over votes and stamps the threshold a set
// of n signers DERIVES for the rung — the only declaration VerifyWeighted admits.
//
// Assembly runs at the vote count rather than at the floor on purpose: an
// under-quorum certificate is one no honest assembler can build and one every
// verifier must still refuse, so a test that wants to state one has to build it
// the way an adversary would. The ordering and dedup clauses still run.
func certDeclaring(t *testing.T, pos VotePosition, tier Finality, n int, votes []SignedVote) *QuorumCert {
	t.Helper()
	c, err := AssembleQuorumCert(pos, tier, uint32(len(votes)), votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	c.Threshold = uint32(SignerFloor(tier, n))
	return c
}

// TestSignerFloorIsTheOneDefinition holds the two rungs' floors to the arithmetic
// the rest of the standard states them in, and holds a non-accept rung to zero —
// which every caller reads as a refusal.
func TestSignerFloorIsTheOneDefinition(t *testing.T) {
	for n := 1; n <= 64; n++ {
		if got, want := SignerFloor(Nova, n), NovaSignerFloor(n); got != want {
			t.Fatalf("n=%d: nova floor %d, want %d", n, got, want)
		}
		if got, want := SignerFloor(Quasar, n), config.TwoThirdsCount(n); got != want {
			t.Fatalf("n=%d: quasar floor %d, want %d", n, got, want)
		}
	}
	for _, rung := range []Finality{Photon, Wave, Horizon} {
		if got := SignerFloor(rung, 41); got != 0 {
			t.Fatalf("%s is not an accept rung and must derive no floor, got %d", rung, got)
		}
	}
	// A set size below one must not produce a floor no quorum could ever reach.
	// Both rungs are total there and answer one — the predicate refuses an
	// unresolved set before consulting this.
	for _, n := range []int{0, -1, -41} {
		for _, rung := range []Finality{Nova, Quasar} {
			if got := SignerFloor(rung, n); got != 1 {
				t.Fatalf("%s over n=%d derives %d, want 1 — an unresolved set has no bar to raise",
					rung, n, got)
			}
			if got := Quorum(rung, n); got != 1 {
				t.Fatalf("%s quorum over n=%d is %d, want 1 — the count-only road has the same "+
					"answer for a set it cannot see", rung, n, got)
			}
		}
	}
}

// TestDerivedThresholdRefusesAnUnderClaim is finding-3, stated as the attack it
// is. A certificate carrying a real export quorum but DECLARING a quorum of one
// is refused — and refused on the derived clause, before the rung's own floors,
// because the number it declared was never the set's to give.
//
// The under-claim is what makes the count-only road exploitable: Verify counts
// against the declaration, so declaring 1 turns "α distinct validators agreed"
// into "one did".
func TestDerivedThresholdRefusesAnUnderClaim(t *testing.T) {
	vs := newTestValidatorSet(4)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	set := &stakeMap{w: map[ids.NodeID]uint64{
		vs.nodeID(0): 1, vs.nodeID(1): 1, vs.nodeID(2): 1, vs.nodeID(3): 1}, total: 4}

	votes := make([]SignedVote, 0, 4)
	for i := 0; i < 4; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}

	// The honest certificate: four of four, declaring the floor the set derives.
	honest := certDeclaring(t, pos, Quasar, 4, votes)
	if honest.Threshold != uint32(config.TwoThirdsCount(4)) {
		t.Fatalf("the honest cert declares %d, the set derives %d", honest.Threshold, config.TwoThirdsCount(4))
	}
	if err := honest.VerifyWeighted(vs, set, 1); err != nil {
		t.Fatalf("a unanimous export cert declaring the derived floor must verify: %v", err)
	}

	// The same votes, under-claiming. Every signature still verifies and the stake
	// is still unanimous — the ONLY thing wrong is the number the cert names.
	under := certDeclaring(t, pos, Quasar, 4, votes)
	under.Threshold = 1
	err := under.VerifyWeighted(vs, set, 1)
	if !errors.Is(err, ErrQCThresholdNotDerived) {
		t.Fatalf("a cert declaring a quorum of one must be refused on the derived clause, got %v", err)
	}

	// And an OVER-claim, for the same reason in the other direction: a certificate
	// asserting a quorum this set does not require is still naming a number that is
	// not the set's. Tolerating it would let a cert redefine the rung upward exactly
	// as tolerating the under-claim lets it redefine the rung down.
	over := certDeclaring(t, pos, Quasar, 4, votes)
	over.Threshold = 4
	if err := over.VerifyWeighted(vs, set, 1); !errors.Is(err, ErrQCThresholdNotDerived) {
		t.Fatalf("a cert declaring a quorum the set does not require must be refused, got %v", err)
	}
}

// TestMintingNeedsTheSet is MED-2 and the finding after it, in the form the fix
// takes: the minting door names no quorum of its own and mints nothing without a
// set to derive one from.
//
// BuildVerifiedQuorumCert is the exported producer of the finality authority token,
// and it USED to count the caller's votes against the caller's own alpha whenever no
// stake source was wired. One vote and an alpha of one satisfied every clause on that
// branch with the same self-declared 1 — for the export rung until that was refused
// outright, and for the accept rung after, on a chain of five, while a certificate
// carrying exactly those bytes was refused on arrival. AcceptWithCert trusts the
// token, which is the whole point of the token.
//
// A floor is a property of the SET. There is no set here, so there is no floor and
// nothing to mint — at BOTH rungs, which is the rule stated once instead of a list
// of tier names kept in step with another list.
func TestMintingNeedsTheSet(t *testing.T) {
	vs := newTestValidatorSet(4)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	votes := make([]SignedVote, 0, 4)
	for i := 0; i < 4; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	one := votes[:1]

	for _, rung := range []Finality{Nova, Quasar, Horizon} {
		v, err := BuildVerifiedQuorumCert(vs, nil, rung, 1, pos, one)
		if !errors.Is(err, ErrCertNeedsStake) {
			t.Fatalf("%s minted over one signer with no set, err=%v", rung, err)
		}
		if !errors.Is(err, ErrNoVerifiedQC) {
			t.Fatalf("%s: the refusal must still present as the liveness answer, got %v", rung, err)
		}
		if !v.IsZero() {
			t.Fatalf("%s: a refused build must return the zero token", rung)
		}
	}

	// With a set in hand the floor is the set's, and the caller has no say in it:
	// there is no alpha to pass. Four of four at unit stake carries both rungs.
	set := &stakeMap{w: map[ids.NodeID]uint64{
		vs.nodeID(0): 1, vs.nodeID(1): 1, vs.nodeID(2): 1, vs.nodeID(3): 1}, total: 4}
	for _, rung := range []Finality{Nova, Quasar} {
		tok, err := BuildVerifiedQuorumCert(vs, set, rung, 1, pos, votes)
		if err != nil {
			t.Fatalf("%s over a real set must mint: %v", rung, err)
		}
		if tok.IsZero() || tok.Cert() == nil {
			t.Fatalf("%s: a build that returned no error must carry a witness", rung)
		}
		if got, want := tok.Cert().Threshold, uint32(SignerFloor(rung, 4)); got != want {
			t.Fatalf("%s: the minted cert declares %d, the set derives %d", rung, got, want)
		}
	}

	// A rung that is not an accept tier derives no floor at all, so there is no
	// quorum for a certificate to state and none is stated.
	if _, err := BuildVerifiedQuorumCert(vs, set, Photon, 1, pos, votes); !errors.Is(err, ErrCertFloorUnstatable) {
		t.Fatalf("a non-accept rung derives no floor and must mint nothing, got %v", err)
	}
}

// TestGossipRoadDerivesItsFloorToo is the same PRINCIPLE at the other door, in
// the form that door can carry.
//
// A certificate that ARRIVES is promoted to the authority token by verifyCert,
// and on a chain with no stake source that gate fell to the count-only Verify —
// which counts votes against the number the CERTIFICATE declares. So a gossiped
// Quasar cert declaring a quorum of one, carrying one signature, was promoted and
// finalized.
//
// The two doors answer differently and the difference is the threat, not an
// oversight:
//
//   - MINTING a token from raw votes derives the floor from the set, so with no set
//     there is no floor and nothing to mint. Refused outright, at both rungs.
//   - ADMITTING one that arrived checks N real signatures from N distinct in-set
//     validators. What was missing was only that its declared quorum was its own.
//     So the floor is derived here too — from the node's live committee, which is
//     this road's authoritative view of its set — and the equal-stake deployment
//     keeps the export rung it has always had. Not "such a chain cannot export",
//     but "it may not export on a quorum the certificate wrote for itself".
func TestGossipRoadDerivesItsFloorToo(t *testing.T) {
	// A real stake-less engine. Its committee is K, and the
	// export quorum over five seats is four — so a certificate declaring one is a
	// certificate naming its own quorum, and a certificate declaring four is the
	// one this node would have built.
	vs := newTestValidatorSet(5)
	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	if e.stakeSource != nil {
		t.Fatal("this engine is supposed to have no stake model; the case tests the other road")
	}

	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: ids.GenerateTestID()}
	votes := make([]SignedVote, 0, 4)
	for i := 0; i < 4; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}

	honest, err := AssembleQuorumCert(pos, Quasar, uint32(Quorum(Quasar, 5)), votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := e.verifyCert(honest, 1); err != nil {
		t.Fatalf("a certificate declaring the quorum this committee derives must pass: %v", err)
	}

	forged := *honest
	forged.Threshold = 1
	if err := e.verifyCert(&forged, 1); !errors.Is(err, ErrQCThresholdNotDerived) {
		t.Fatalf("a gossiped export cert naming its own quorum of one was admitted: %v", err)
	}

	// One signature under that declaration is the whole attack: before the clause,
	// Verify counted one accept against a declared one and answered nil.
	lone, err := AssembleQuorumCert(pos, Quasar, 1, votes[:1])
	if err != nil {
		t.Fatalf("assemble lone: %v", err)
	}
	if lone.Verify(vs, 1) != nil {
		t.Fatal("the lone certificate must clear the structural predicate, or the case proves nothing")
	}
	if err := e.verifyCert(lone, 1); !errors.Is(err, ErrQCThresholdNotDerived) {
		t.Fatalf("one signature under a self-named quorum of one was admitted: %v", err)
	}
}

// TestTheTwoRoadsDeriveTheNumberTheyDeclare is the regression the K<6 fixtures
// could not catch.
//
// There are TWO derived counts, because there are two predicates. SignerFloor is
// the distinct-signer GUARD the weighted road carries beside a stake clause;
// Quorum is the count that IS the predicate where no stake clause stands beside
// it. They agree at Quasar and diverge at Nova past five signers, because
// NovaSignerFloor SATURATES at three — correct as a guard on a road whose majority
// is carried in stake, and wrong as a majority on a road that has only the count.
//
// Reading the wrong one on the count-only road makes a node refuse its OWN
// certificates: the assembler declares NovaQuorum(n) there, which is 4 at six
// signers where NovaSignerFloor is 3. Every committee the fixtures use is 4 or 5,
// which is exactly where the two agree, so nothing in the suite would have said so.
func TestTheTwoRoadsDeriveTheNumberTheyDeclare(t *testing.T) {
	diverged := false
	for n := 1; n <= 64; n++ {
		// What the count-only assembler declares (engine.go, the nil-stake arm).
		if got, want := Quorum(Nova, n), NovaQuorum(n); got != want {
			t.Fatalf("n=%d: the count-only road derives %d for nova, its assembler declares %d",
				n, got, want)
		}
		// What the weighted assembler declares (engine.go, the stake arm).
		if got, want := SignerFloor(Nova, n), NovaSignerFloor(n); got != want {
			t.Fatalf("n=%d: the weighted road derives %d for nova, its assembler declares %d",
				n, got, want)
		}
		// The export rung is one number on both roads, and attestation.go declares it.
		if Quorum(Quasar, n) != SignerFloor(Quasar, n) {
			t.Fatalf("n=%d: the export supermajority reads as two different counts (%d, %d)",
				n, Quorum(Quasar, n), SignerFloor(Quasar, n))
		}
		if Quorum(Nova, n) != SignerFloor(Nova, n) {
			diverged = true
		}
	}
	if !diverged {
		t.Fatal("the two nova counts never diverged over 64 sizes — if that is now true they " +
			"are one quantity and should be one function; if it is not, this test stopped looking")
	}
	// State the divergence outright at the size the fixtures never reach.
	if Quorum(Nova, 6) != 4 || SignerFloor(Nova, 6) != 3 {
		t.Fatalf("at six signers the count-only majority is %d and the weighted guard is %d, "+
			"want 4 and 3", Quorum(Nova, 6), SignerFloor(Nova, 6))
	}
	for _, rung := range []Finality{Photon, Wave, Horizon} {
		if Quorum(rung, 41) != 0 {
			t.Fatalf("%s is not an accept rung and must derive no quorum", rung)
		}
	}
}

// TestDerivedThresholdSurvivesAnUnresolvedSet holds the one place the clause steps
// aside. A source reporting no signers derives no floor, so there is nothing to
// compare a declaration against — and the rung's own clause refuses the cert under
// the name it has always been refused by. Stepping aside is not a pass.
func TestDerivedThresholdSurvivesAnUnresolvedSet(t *testing.T) {
	vs := newTestValidatorSet(4)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	votes := []SignedVote{{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)}}

	cert := certDeclaring(t, pos, Quasar, 0, votes)
	cert.Threshold = 1 // SignerFloor over an unresolved set is 1; say so outright.
	err := cert.VerifyWeighted(vs, &stakeMap{w: map[ids.NodeID]uint64{}, total: 0}, 1)
	if err == nil {
		t.Fatal("an unresolved set must refuse, not pass")
	}
	if errors.Is(err, ErrQCThresholdNotDerived) {
		t.Fatalf("an unresolved set derives no floor and must not be named by the derived clause: %v", err)
	}
}

// arrivalFleet is one node holding a set of `signers` weighted validators, under a
// committee of K seats. The two numbers are deliberately allowed to differ: K sizes
// the sample a ROUND asks, and the set is what a certificate's floor is a property
// of. Every place that has confused the two has produced a second floor.
type arrivalFleet struct {
	vs      *testValidatorSet
	stake   *stakeMap
	chainID ids.ID
	rt      *Runtime
	vm      *catchupVM
}

func newArrivalFleet(t *testing.T, k, signers int) *arrivalFleet {
	t.Helper()
	vs := newTestValidatorSet(k)
	weights := make([]uint64, signers)
	for i := range weights {
		weights[i] = 1
	}
	f := &arrivalFleet{
		vs:      vs,
		stake:   newStakeMap(vs, weights...),
		chainID: ids.GenerateTestID(),
		vm:      newCatchupVM(),
	}
	f.rt = NewRuntime(NetworkConfig{
		ChainID:      f.chainID,
		NetworkID:    ids.GenerateTestID(),
		NodeID:       vs.nodeID(0),
		Logger:       log.Noop(),
		Params:       ptrParams(matrixParams(k, quorumq(k))),
		VoteVerifier: vs,
		VoteSigner:   vs.signerFor(0),
		StakeSource:  f.stake,
		Gossiper:     &recordingGossiper{},
		VM:           f.vm,
	})
	if err := f.rt.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = f.rt.Stop(context.Background()) })
	return f
}

// novaCert is the certificate an honest assembler on this chain produces: `voters`
// signatures under the floor the SET derives, which is the number VerifyWeighted
// demands and the only one it admits.
func (f *arrivalFleet) novaCert(t *testing.T, blk *verifyOnceBlock, voters int) (*QuorumCert, []byte) {
	t.Helper()
	pos := VotePosition{ChainID: f.chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
	votes := make([]SignedVote, 0, voters)
	for i := 0; i < voters; i++ {
		votes = append(votes, SignedVote{NodeID: f.vs.nodeID(i), Accept: true, Signature: f.vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, Nova, uint32(SignerFloor(Nova, f.stake.SignerCount(0))), votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return cert, b
}

// TestTheDoorAdmitsWhatTheGateAdmits is the invariant a pre-filter has to keep and
// could not: a certificate the finality predicate ACCEPTS must reach it.
//
// Both arrival roads used to read a floor of their own before verifyCert ran — the
// sample committee's majority for Nova, Alpha() for Quasar. That is a THIRD spelling
// of a quantity with one definition, and it is a different quantity: the gate derives
// the accept rung's floor from the SIGNING SET, where NovaSignerFloor SATURATES at
// three, while the door read the committee's majority, which grows with K. At six
// seats the gate accepts a certificate declaring three and the door demanded four; at
// eleven it demanded six. And where the set is SMALLER than the committee the door
// asked the set for more signatures than the set's own floor: three signers derive
// two, and a door reading a committee of four asked for three.
//
// So an honest certificate — the exact bytes this fleet's own assembler produces —
// never reached the predicate that would have admitted it. The rows below are the
// sizes where the two numbers part; a door in front of the gate has to pass all of
// them, and the arithmetic assertion is what makes each row non-vacuous.
func TestTheDoorAdmitsWhatTheGateAdmits(t *testing.T) {
	for _, row := range []struct{ k, signers, voters int }{
		// A set smaller than its committee: the floor is the SET's two, and the
		// committee's majority is three.
		{k: 4, signers: 3, voters: 2},
		// Set == committee, past the point NovaSignerFloor saturates. The stake
		// majority still needs more voters than the floor names, which is exactly
		// why an honest certificate carries more signatures than it declares.
		{k: 6, signers: 6, voters: 4},
		{k: 9, signers: 9, voters: 5},
		{k: 11, signers: 11, voters: 6},
	} {
		t.Run(fmt.Sprintf("k=%d/signers=%d", row.k, row.signers), func(t *testing.T) {
			f := newArrivalFleet(t, row.k, row.signers)
			derived := SignerFloor(Nova, row.signers)
			if NovaQuorum(row.k) <= derived {
				t.Fatalf("vacuous: the committee majority %d must exceed the set's floor %d, "+
					"or this row proves nothing about a door reading the committee",
					NovaQuorum(row.k), derived)
			}

			// THE GATE, on its own: the predicate accepts this certificate.
			blk := newTestBlock(1, ids.Empty, "arrival")
			trackVerifiedBlock(f.rt, blk, 0)
			cert, wire := f.novaCert(t, blk, row.voters)
			if err := f.rt.Transitive.verifyCert(cert, 0); err != nil {
				t.Fatalf("the finality predicate refused an honest certificate: %v", err)
			}
			if int(cert.Threshold) != derived {
				t.Fatalf("the cert declares %d, the set derives %d", cert.Threshold, derived)
			}

			// THE GOSSIP ROAD: it reaches the gate, and finalizes.
			if !f.rt.HandleIncomingCert(wire) {
				t.Fatalf("the gossip road dropped a certificate the predicate accepts — "+
					"a door reading the committee's majority %d refuses the set's floor %d",
					NovaQuorum(row.k), derived)
			}
			if got := blk.AcceptCalled(); got != 1 {
				t.Fatalf("VM.Accept=%d want 1", got)
			}

			// THE CATCH-UP ROAD: the mirror of it, and the same answer. A separate
			// runtime so the height gate does not see the block already finalized.
			g := newArrivalFleet(t, row.k, row.signers)
			far := newTestBlock(41, ids.GenerateTestID(), "arrival-frontier")
			g.vm.register(far)
			_, farWire := g.novaCert(t, far, row.voters)
			if err := g.rt.VerifyCatchupCertificate(context.Background(), far.bytes, farWire); err != nil {
				t.Fatalf("the catch-up road refused a certificate the predicate accepts: %v", err)
			}
		})
	}
}

// TestTheArrivalRoadsRefuseAQuorumTheSetDoesNotDerive is the other half, and the
// reason dropping the pre-filters costs nothing: what the door used to catch, the
// gate catches — and catches harder, because the gate demands EQUALITY with the
// derived floor where the door asked only for "at least".
func TestTheArrivalRoadsRefuseAQuorumTheSetDoesNotDerive(t *testing.T) {
	f := newArrivalFleet(t, 9, 9)
	blk := newTestBlock(1, ids.Empty, "self-named")
	trackVerifiedBlock(f.rt, blk, 0)

	// One signature under a self-named quorum of one: what a door reading any
	// "at least" floor also refused, stated the way an attacker builds it.
	pos := VotePosition{ChainID: f.chainID, Height: 1, Round: 0, BlockID: blk.id, ParentID: ids.Empty}
	lone, err := AssembleQuorumCert(pos, Nova, 1,
		[]SignedVote{{NodeID: f.vs.nodeID(0), Accept: true, Signature: f.vs.sign(0, pos)}})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if lone.Verify(f.vs, 0) != nil {
		t.Fatal("the lone certificate must clear the structural predicate, or the case proves nothing")
	}
	if err := f.rt.Transitive.verifyCert(lone, 0); !errors.Is(err, ErrQCThresholdNotDerived) {
		t.Fatalf("a self-named quorum of one must be refused on the derived clause, got %v", err)
	}
	wire, err := lone.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if f.rt.HandleIncomingCert(wire) || blk.AcceptCalled() != 0 {
		t.Fatal("SAFETY BREAK: one signature under a self-named quorum finalized a block")
	}

	// And an OVER-claim, which no "at least" door ever caught: a certificate
	// declaring more than its set requires is naming a number that is not the
	// set's, in the other direction.
	honest, honestWire := f.novaCert(t, blk, 5)
	over := *honest
	over.Threshold = uint32(NovaQuorum(9))
	if int(over.Threshold) == SignerFloor(Nova, 9) {
		t.Fatal("vacuous: the over-claim must differ from the derived floor")
	}
	if err := f.rt.Transitive.verifyCert(&over, 0); !errors.Is(err, ErrQCThresholdNotDerived) {
		t.Fatalf("an over-claim must be refused on the derived clause, got %v", err)
	}

	// The honest certificate, last, so the two refusals above are attributable to
	// the number each names and not to the fixture.
	if !f.rt.HandleIncomingCert(honestWire) || blk.AcceptCalled() != 1 {
		t.Fatal("control broke: the honest certificate must finalize")
	}
}
