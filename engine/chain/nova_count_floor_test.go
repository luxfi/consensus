// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// nova_count_floor_test.go — Nova's accept threshold has two doors and they do not
// agree on what a Nova cert has to carry.
//
// assembleCertLocked freezes a locally-built Nova cert at NovaQuorum(n) when no
// stake source is wired, because on an equal-stake chain the count majority IS the
// stake majority and there is no second predicate to be the authority. The
// incoming-cert door instead admits any cert whose SELF-DECLARED Threshold reaches
// NovaSignerFloor(K) — which is capped at NovaQuorum(minBFTCommittee)=3 for every K
// — and then hands it to Verify, whose only count clause is `distinct votes >=
// c.Threshold`. Nothing between the wire and VM.Accept re-derives the majority from
// the live set, so on an equal-stake chain the number a Nova cert must reach is the
// number the cert says it must reach.
//
// The comment on that floor argues it is unweakened because "on a stake-weighted
// chain the Nova majority is measured in stake". That is the arm where
// verifyNovaMajority recomputes both predicates from the authoritative set. On the
// arm where no stake source is wired there is no recomputation, and
// WithStakeWeighting's own doc names that a supported configuration: omit it when
// equal stake is enforced at admission.
//
// The properties here are about the LIVE SET, not about the cert: a Nova accept
// needs a majority of the validators the chain actually has, by whatever unit the
// chain weighs them in.
package chain

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/slashing"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// novaFleet is n validators with equal stake, the configuration WithStakeWeighting
// documents as not needing a stake source.
type novaFleet struct {
	vs      *testValidatorSet
	chainID ids.ID
	n       int
}

func newNovaFleet(n int) *novaFleet {
	return &novaFleet{vs: newTestValidatorSet(n), chainID: ids.GenerateTestID(), n: n}
}

// node builds one equal-stake engine — verifier and gossip wired, NO stake source.
func (f *novaFleet) node(t *testing.T, self int, opts ...Option) *Runtime {
	t.Helper()
	alpha := bftAlpha(f.n)
	base := []Option{WithQuorumCert(f.chainID, f.vs.nodeID(self), f.vs, &recordingGossiper{}, f.vs.signerFor(self))}
	e := NewWithConfig(Config{Params: config.Parameters{
		K: f.n, Alpha: 0.75, AlphaPreference: alpha, AlphaConfidence: alpha, Beta: 1,
	}}, append(base, opts...)...)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	return &Runtime{Transitive: e, config: NetworkConfig{ChainID: f.chainID, Logger: log.Noop()}}
}

// novaCert assembles a Nova cert over pos signed by the named validators, declaring
// exactly as many signatures as it carries — what an assembler holding those votes
// would honestly produce. Votes are broadcast to every validator, so anyone on the
// gossip path holds enough of them to build this.
func (f *novaFleet) novaCert(t *testing.T, pos VotePosition, voters ...int) []byte {
	t.Helper()
	votes := make([]SignedVote, 0, len(voters))
	for _, i := range voters {
		votes = append(votes, SignedVote{NodeID: f.vs.nodeID(i), Accept: true, Signature: f.vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, Nova, uint32(len(voters)), votes)
	if err != nil {
		t.Fatalf("assemble nova cert %v: %v", voters, err)
	}
	b, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestNovaCountFloor_SubMajorityCertRefused: on an equal-stake chain a Nova accept
// must need a majority of the LIVE SET. NovaQuorum(9)=5; NovaSignerFloor(9)=3.
//
// The positive control runs first so the refusal below is attributable to the count
// and not to the chain id, the height gate, the canonical match or the signatures.
func TestNovaCountFloor_SubMajorityCertRefused(t *testing.T) {
	const n = 9
	f := newNovaFleet(n)
	if NovaQuorum(n) <= NovaSignerFloor(n) {
		t.Fatalf("test vacuous at n=%d: NovaQuorum=%d must exceed NovaSignerFloor=%d",
			n, NovaQuorum(n), NovaSignerFloor(n))
	}

	// CONTROL — the honest majority finalizes.
	ctl := f.node(t, 0)
	good := newTestBlock(1, ids.Empty, "nova-majority")
	trackVerifiedBlock(ctl, good, 0)
	majority := make([]int, NovaQuorum(n))
	for i := range majority {
		majority[i] = i
	}
	if !ctl.HandleIncomingCert(f.novaCert(t, posFor(f.chainID, good), majority...)) {
		t.Fatalf("control broke: a Nova cert with NovaQuorum(%d)=%d signers must finalize", n, NovaQuorum(n))
	}
	if got := good.AcceptCalled(); got != 1 {
		t.Fatalf("control broke: VM.Accept=%d want 1", got)
	}

	// THE CLAIM — one below the majority must not.
	rt := f.node(t, 0)
	blk := newTestBlock(1, ids.Empty, "nova-sub-majority")
	trackVerifiedBlock(rt, blk, 0)
	short := make([]int, NovaQuorum(n)-1)
	for i := range short {
		short[i] = i
	}
	finalized := rt.HandleIncomingCert(f.novaCert(t, posFor(f.chainID, blk), short...))
	if finalized || blk.AcceptCalled() != 0 {
		t.Fatalf("SAFETY BREAK: %d of %d validators finalized a block (finalized=%v VM.Accept=%d). "+
			"A Nova accept is a majority of the live set — NovaQuorum(%d)=%d. The incoming door's "+
			"floor is NovaSignerFloor(K)=%d, capped at NovaQuorum(minBFTCommittee) for every K, and "+
			"with no stake source Verify's only count clause compares against the cert's OWN "+
			"Threshold. Locally, assembleCertLocked would not have built this cert.",
			len(short), n, finalized, blk.AcceptCalled(), n, NovaQuorum(n), NovaSignerFloor(n))
	}
}

// TestNovaCountFloor_TwoDisjointQuorumsBothCertify is the consequence stated as a
// safety property: two quorums that certify conflicting blocks at one height must
// intersect. NovaSignerFloor(9)=3 leaves room for three disjoint certifying sets in
// a nine-validator chain, so the intersection argument that makes a majority safe
// is not the argument the door is enforcing.
//
// No validator equivocates here. Each of the six signs one block, once — the split
// an honest set produces when two proposals reach it in different orders.
func TestNovaCountFloor_TwoDisjointQuorumsBothCertify(t *testing.T) {
	const n = 9
	f := newNovaFleet(n)

	a := f.node(t, 0)
	x := newTestBlock(1, ids.Empty, "branch-X")
	trackVerifiedBlock(a, x, 0)
	aFinal := a.HandleIncomingCert(f.novaCert(t, posFor(f.chainID, x), 0, 1, 2))

	b := f.node(t, 1)
	y := newTestBlock(1, ids.Empty, "branch-Y")
	trackVerifiedBlock(b, y, 0)
	bFinal := b.HandleIncomingCert(f.novaCert(t, posFor(f.chainID, y), 3, 4, 5))

	if aFinal && bFinal {
		ha, _ := a.Transitive.consensus.GetFinalizedHeight()
		hb, _ := b.Transitive.consensus.GetFinalizedHeight()
		t.Fatalf("FORK: two DISJOINT sets of 3 in a %d-validator chain each finalized a different "+
			"block at height 1 — node A on %s (VM.Accept=%d, finalized height %d), node B on %s "+
			"(VM.Accept=%d, finalized height %d). Neither set intersects the other, so no "+
			"non-equivocating validator is shared and no validator misbehaved: the two accepts are "+
			"both permitted by the rule the door applies.",
			n, x.id, x.AcceptCalled(), ha, y.id, y.AcceptCalled(), hb)
	}
}

// TestNovaCountFloor_StakeWeightedArmRefuses scopes the defect. The same
// sub-majority cert against an engine with a stake source is refused, because
// verifyNovaMajority recomputes the majority from the live set instead of reading
// the cert's own Threshold. The two arms must reach the same verdict on the same
// cert; this one is the arm that already does.
func TestNovaCountFloor_StakeWeightedArmRefuses(t *testing.T) {
	const n = 9
	f := newNovaFleet(n)
	rt := f.node(t, 0, WithStakeWeighting(f.vs)) // equal unit weights

	blk := newTestBlock(1, ids.Empty, "nova-sub-majority-weighted")
	trackVerifiedBlock(rt, blk, 0)
	short := make([]int, NovaQuorum(n)-1)
	for i := range short {
		short[i] = i
	}
	if rt.HandleIncomingCert(f.novaCert(t, posFor(f.chainID, blk), short...)) || blk.AcceptCalled() != 0 {
		t.Fatalf("the stake-weighted arm admitted %d of %d — verifyNovaMajority must recompute the "+
			"majority from the live set, never from the cert's Threshold", len(short), n)
	}
}

// TestNovaCountFloor_SubMajorityCertCannotSlash: a cert that cannot finalize a
// block must not be able to attribute a fork to the validators who signed it.
//
// The equivocation path runs the same verifyCert the finalize path runs, so a
// sub-majority Nova cert clears it too — and reportCertEquivocation then records a
// DoubleVote against every signer. The three validators here signed one block, at
// one height, once. Their signatures are on the losing branch of an honest split,
// which is not a fault; naming them costs an attacker nothing beyond relaying votes
// that were broadcast to it.
func TestNovaCountFloor_SubMajorityCertCannotSlash(t *testing.T) {
	const n = 9
	f := newNovaFleet(n)
	db := slashing.NewDB(time.Hour)
	rt := f.node(t, 0, WithSlashing(slashing.NewDetector(64, 0.5), db))

	// Height 1 is finalized by the honest majority.
	winner := newTestBlock(1, ids.Empty, "slash-winner")
	trackVerifiedBlock(rt, winner, 0)
	majority := make([]int, NovaQuorum(n))
	for i := range majority {
		majority[i] = i
	}
	if !rt.HandleIncomingCert(f.novaCert(t, posFor(f.chainID, winner), majority...)) {
		t.Fatal("control broke: the majority cert must finalize height 1")
	}

	// A sub-majority cert for a losing sibling at the same height, signed by three
	// validators that never voted for the winner.
	loser := newTestBlock(1, ids.Empty, "slash-loser")
	trackVerifiedBlock(rt, loser, 0)
	accused := []int{6, 7, 8}
	rt.HandleIncomingCert(f.novaCert(t, posFor(f.chainID, loser), accused...))

	for _, i := range accused {
		if rec := db.GetRecord(f.vs.nodeID(i)); rec != nil {
			t.Fatalf("validator %d carries %d slash(es) and is jailed=%v after signing ONE block at "+
				"ONE height. A cert too small to finalize is too small to attribute a fork: "+
				"reportCertEquivocation runs behind the same verifyCert, which on an equal-stake chain "+
				"checks the count only against the cert's own Threshold. Three relayed votes jail "+
				"three honest validators, and jailing NovaQuorum-many of them halts the chain.",
				i, rec.SlashCount, db.IsJailed(f.vs.nodeID(i)))
		}
	}
}
