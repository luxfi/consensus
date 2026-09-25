// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// nova_count_floor_test.go — the accept threshold is read off the live set, on both
// roads a certificate arrives by.
//
// A block is accepted only on a ⅔ certificate (the Quasar tier): on the count-only road
// verifyCert derives Quorum(Quasar, K) = TwoThirdsCount(K) from this node's own committee
// and demands the certificate's declared quorum equal it; on the stake-weighted road
// VerifyWeighted derives it from the set. A certificate's tier and quorum are labels no
// signature covers, so a Nova-labeled one is read at the same ⅔ floor. The properties
// here are about the LIVE SET, not about the cert: a certificate that names a smaller
// quorum for itself accepts nothing and slashes nobody.
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

// cert assembles a certificate of the given tier over pos signed by the named validators,
// declaring exactly as many signatures as it carries — what an assembler holding those
// votes would produce. Votes are broadcast to every validator, so anyone on the gossip
// path holds enough of them to build this.
func (f *novaFleet) cert(t *testing.T, tier Finality, pos VotePosition, voters ...int) []byte {
	t.Helper()
	votes := make([]SignedVote, 0, len(voters))
	for _, i := range voters {
		votes = append(votes, SignedVote{NodeID: f.vs.nodeID(i), Accept: true, Signature: f.vs.sign(i, pos)})
	}
	cert, err := AssembleQuorumCert(pos, tier, uint32(len(voters)), votes)
	if err != nil {
		t.Fatalf("assemble %s cert %v: %v", tier, voters, err)
	}
	b, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// seats returns the validator indexes 0..k-1.
func seats(k int) []int {
	out := make([]int, k)
	for i := range out {
		out[i] = i
	}
	return out
}

// TestAcceptFloor_BelowTwoThirdsCertRefused: on an equal-stake chain an accept needs ⅔ of
// the LIVE SET, TwoThirdsCount(9)=7. A ⅔ certificate one signer short, declaring its own
// quorum of six, is refused; so is a Nova-labeled certificate carrying the majority
// NovaQuorum(9)=5, read at the ⅔ floor.
//
// The positive control runs first so the refusals below are attributable to the count and
// the tier, and not to the chain id, the height gate, the canonical match or the signatures.
func TestAcceptFloor_BelowTwoThirdsCertRefused(t *testing.T) {
	const n = 9
	f := newNovaFleet(n)
	tt := int(config.TwoThirdsStakeFloor(n)) + 1

	// CONTROL — ⅔ of the set finalizes.
	ctl := f.node(t, 0)
	good := newTestBlock(1, ids.Empty, "two-thirds")
	trackVerifiedBlock(ctl, good, 0)
	if !ctl.HandleIncomingCert(f.cert(t, Quasar, posFor(f.chainID, good), seats(tt)...)) {
		t.Fatalf("control broke: a ⅔ cert with TwoThirdsCount(%d)=%d signers must finalize", n, tt)
	}
	if got := good.AcceptCalled(); got != 1 {
		t.Fatalf("control broke: VM.Accept=%d want 1", got)
	}

	for _, c := range []struct {
		name   string
		tier   Finality
		voters []int
	}{
		{"a ⅔ certificate one signer short, naming its own quorum", Quasar, seats(tt - 1)},
		{"a Nova-labeled certificate carrying the bare majority", Nova, seats(NovaQuorum(n))},
	} {
		rt := f.node(t, 0)
		blk := newTestBlock(1, ids.Empty, c.name)
		trackVerifiedBlock(rt, blk, 0)
		finalized := rt.HandleIncomingCert(f.cert(t, c.tier, posFor(f.chainID, blk), c.voters...))
		if finalized || blk.AcceptCalled() != 0 {
			t.Fatalf("SAFETY BREAK: %s — %d of %d validators finalized a block (finalized=%v VM.Accept=%d); "+
				"an accept is ⅔ of the live set, TwoThirdsCount(%d)=%d",
				c.name, len(c.voters), n, finalized, blk.AcceptCalled(), n, tt)
		}
	}
}

// TestNovaCountFloor_TwoDisjointQuorumsBothCertify is the safety property: two quorums
// that certify conflicting blocks at one height must intersect. Two disjoint sets of
// three in a nine-validator chain each carry a Nova-labeled certificate, and neither accepts.
//
// No validator equivocates here. Each of the six signs one block, once — the split
// an honest set produces when two proposals reach it in different orders.
func TestNovaCountFloor_TwoDisjointQuorumsBothCertify(t *testing.T) {
	const n = 9
	f := newNovaFleet(n)

	a := f.node(t, 0)
	x := newTestBlock(1, ids.Empty, "branch-X")
	trackVerifiedBlock(a, x, 0)
	aFinal := a.HandleIncomingCert(f.cert(t, Nova, posFor(f.chainID, x), 0, 1, 2))

	b := f.node(t, 1)
	y := newTestBlock(1, ids.Empty, "branch-Y")
	trackVerifiedBlock(b, y, 0)
	bFinal := b.HandleIncomingCert(f.cert(t, Nova, posFor(f.chainID, y), 3, 4, 5))

	if aFinal || bFinal {
		ha, _ := a.Transitive.consensus.GetFinalizedHeight()
		hb, _ := b.Transitive.consensus.GetFinalizedHeight()
		t.Fatalf("a disjoint set of 3 in a %d-validator chain finalized a block at height 1 — "+
			"node A on %s (VM.Accept=%d, finalized height %d), node B on %s (VM.Accept=%d, "+
			"finalized height %d). Two such sets share no validator, so accepting on either lets "+
			"an honest split decide two blocks.",
			n, x.id, x.AcceptCalled(), ha, y.id, y.AcceptCalled(), hb)
	}
}

// TestNovaCountFloor_StakeWeightedArmRefuses: the same sub-majority Nova-labeled cert
// against an engine with a stake source is refused too. The two arms reach the same
// verdict on the same cert.
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
	if rt.HandleIncomingCert(f.cert(t, Nova, posFor(f.chainID, blk), short...)) || blk.AcceptCalled() != 0 {
		t.Fatalf("the stake-weighted arm admitted a Nova-labeled cert of %d of %d", len(short), n)
	}
}

// TestNovaCountFloor_SubMajorityCertCannotSlash: a cert that cannot finalize a block
// must not be able to attribute a fork to the validators who signed it.
//
// The equivocation path runs the same verifyCert the finalize path runs. The two
// validators here signed one block, at one height, once, on the losing branch of an
// honest split, which is not a fault; naming them costs an attacker nothing beyond
// relaying votes that were broadcast to it. Neither a ⅔-labeled cert naming its own quorum
// of two nor a Nova-labeled one clears the door, so neither records a slash.
func TestNovaCountFloor_SubMajorityCertCannotSlash(t *testing.T) {
	const n = 9
	f := newNovaFleet(n)
	db := slashing.NewDB(time.Hour)
	rt := f.node(t, 0, WithSlashing(slashing.NewDetector(64, 0.5), db))

	// Height 1 is finalized by ⅔ of the set, {0..6}.
	winner := newTestBlock(1, ids.Empty, "slash-winner")
	trackVerifiedBlock(rt, winner, 0)
	if !rt.HandleIncomingCert(f.cert(t, Quasar, posFor(f.chainID, winner), seats(int(config.TwoThirdsStakeFloor(n))+1)...)) {
		t.Fatal("control broke: the ⅔ cert must finalize height 1")
	}

	// Short certs for a losing sibling at the same height, signed by two validators that
	// never voted for the winner.
	loser := newTestBlock(1, ids.Empty, "slash-loser")
	trackVerifiedBlock(rt, loser, 0)
	accused := []int{7, 8}
	rt.HandleIncomingCert(f.cert(t, Quasar, posFor(f.chainID, loser), accused...))
	rt.HandleIncomingCert(f.cert(t, Nova, posFor(f.chainID, loser), accused...))

	for _, i := range accused {
		if rec := db.GetRecord(f.vs.nodeID(i)); rec != nil {
			t.Fatalf("validator %d carries %d slash(es) and is jailed=%v after signing ONE block at "+
				"ONE height. A cert too small to finalize is too small to attribute a fork.",
				i, rec.SlashCount, db.IsJailed(f.vs.nodeID(i)))
		}
	}
}
