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
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
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
	c.Threshold = SignerFloor(tier, n)
	return c
}

// TestSignerFloorIsTheOneDefinition holds the two rungs' floors to the arithmetic
// the rest of the standard states them in, and holds a non-accept rung to zero —
// which every caller reads as a refusal.
func TestSignerFloorIsTheOneDefinition(t *testing.T) {
	for n := 1; n <= 64; n++ {
		if got, want := SignerFloor(Nova, n), uint32(NovaSignerFloor(n)); got != want {
			t.Fatalf("n=%d: nova floor %d, want %d", n, got, want)
		}
		if got, want := SignerFloor(Quasar, n), uint32(config.TwoThirdsCount(n)); got != want {
			t.Fatalf("n=%d: quasar floor %d, want %d", n, got, want)
		}
	}
	for _, rung := range []Finality{Photon, Wave, Horizon} {
		if got := SignerFloor(rung, 41); got != 0 {
			t.Fatalf("%s is not an accept rung and must derive no floor, got %d", rung, got)
		}
	}
	// The function is exported and its result is unsigned, so a set size below one
	// must not underflow into a floor no quorum could ever reach. Both rungs are
	// total there and answer one — the predicate refuses an unresolved set before
	// consulting this, and a wrapped floor would make that refusal look like a bar.
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

// TestCountOnlyRoadCannotMintExport is MED-2. BuildVerifiedQuorumCert is the sole
// producer of the finality authority token, and its count-only branch counts votes
// against the number the CALLER names. Handed an export rung, one vote and an
// alpha of one, every clause on that branch is satisfied by the same self-declared
// 1 — and AcceptWithCert trusts the token it returns, which is the whole point of
// the token.
//
// An export certificate's floors are read off a validator set. With no set there is
// nothing to read them from, so there is nothing to mint.
func TestCountOnlyRoadCannotMintExport(t *testing.T) {
	vs := newTestValidatorSet(4)
	pos := VotePosition{ChainID: ids.GenerateTestID(), Height: 1, BlockID: ids.GenerateTestID()}
	one := []SignedVote{{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)}}

	v, err := BuildVerifiedQuorumCert(vs, nil, Quasar, 1, 1, pos, one)
	if !errors.Is(err, ErrExportNeedsStake) {
		t.Fatalf("an export token minted over one signer with no stake source, err=%v", err)
	}
	if !errors.Is(err, ErrNoVerifiedQC) {
		t.Fatalf("the refusal must still present as the liveness answer, got %v", err)
	}
	if !v.IsZero() {
		t.Fatal("a refused build must return the zero token")
	}

	// Every export rung, not a list of tier names kept in step with another list.
	for _, rung := range []Finality{Quasar, Horizon} {
		if !rung.AuthorizesExport() {
			t.Fatalf("%s is expected to authorize export", rung)
		}
		if _, err := BuildVerifiedQuorumCert(vs, nil, rung, 1, 1, pos, one); !errors.Is(err, ErrExportNeedsStake) {
			t.Fatalf("%s minted with no stake source: %v", rung, err)
		}
	}

	// The accept rung keeps its count-only road: it authorizes local execution the
	// chain can still reorg away, and an equal-stake chain has to be able to make
	// progress on it. Three of four, declaring the count majority that road enforces.
	votes := make([]SignedVote, 0, 3)
	for i := 0; i < 3; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	nova, err := BuildVerifiedQuorumCert(vs, nil, Nova, uint32(NovaQuorum(4)), 1, pos, votes)
	if err != nil {
		t.Fatalf("the accept rung must still build without a stake source: %v", err)
	}
	if nova.IsZero() {
		t.Fatal("a nova build that returned no error must carry a witness")
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
//   - MINTING an export token from raw votes takes the floor from the CALLER's
//     alpha, so with no set to check it against there is nothing to check at all.
//     Refused outright.
//   - ADMITTING one that arrived checks N real signatures from N distinct in-set
//     validators. What was missing was only that its declared quorum was its own.
//     So the floor is derived here too — from the node's live committee, which is
//     this road's authoritative view of its set — and the equal-stake deployment
//     keeps the export rung it has always had. Not "such a chain cannot export",
//     but "it may not export on a quorum the certificate wrote for itself".
func TestGossipRoadDerivesItsFloorToo(t *testing.T) {
	for _, rung := range []Finality{Quasar, Horizon} {
		if err := exportNeedsStake(rung, nil); !errors.Is(err, ErrExportNeedsStake) {
			t.Fatalf("%s with no stake source must be refused, got %v", rung, err)
		}
	}
	if err := exportNeedsStake(Nova, nil); err != nil {
		t.Fatalf("the accept rung keeps its count-only road: %v", err)
	}
	// With a set in hand the rule says nothing — it is about the absence, not the tier.
	set := &stakeMap{w: map[ids.NodeID]uint64{}, total: 4}
	for _, rung := range []Finality{Nova, Quasar, Horizon} {
		if err := exportNeedsStake(rung, set); err != nil {
			t.Fatalf("%s with a stake source must not be refused by this clause: %v", rung, err)
		}
	}

	// AND THE OTHER DOOR, on a real stake-less engine. Its committee is K, and the
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

	honest, err := AssembleQuorumCert(pos, Quasar, Quorum(Quasar, 5), votes)
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
		if got, want := Quorum(Nova, n), uint32(NovaQuorum(n)); got != want {
			t.Fatalf("n=%d: the count-only road derives %d for nova, its assembler declares %d",
				n, got, want)
		}
		// What the weighted assembler declares (engine.go, the stake arm).
		if got, want := SignerFloor(Nova, n), uint32(NovaSignerFloor(n)); got != want {
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
