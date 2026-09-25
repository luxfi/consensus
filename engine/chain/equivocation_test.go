// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// equivocation_test.go — one faulty validator that signs two blocks at one height,
// on the multi-node harness, and the votes parked for blocks a validator lacks.
// A block is accepted only on a ⅔ certificate, and two of those at one height share
// more signers than one equivocator, so no run may decide two blocks at a height.
package chain

import (
	"bytes"
	"context"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// voteBytesFor is seat `seat`'s signed accept vote on blk, over the position node
// `on` derives for it (every honest node derives the same one).
func voteBytesFor(t *testing.T, net *simNet, on, seat int, blk *simBlock) []byte {
	t.Helper()
	e := net.nodes[on].rt.Transitive
	if !waitFor(emergeTO, func() bool { return net.nodes[on].rt.HasPendingBlock(blk.ID()) }) {
		t.Fatalf("seat %d never tracked %s", on, blk.ID())
	}
	e.mu.RLock()
	pos := e.blockPositionLocked(e.pendingBlocks[blk.ID()], blk.ID())
	e.mu.RUnlock()
	vb, err := encodeSignedVote(net.vs.nodeID(seat), net.vs.sign(seat, pos))
	if err != nil {
		t.Fatal(err)
	}
	return vb
}

func deliverVote(net *simNet, from int, blk *simBlock, vb []byte, to ...int) {
	for _, i := range to {
		net.nodes[i].enqueue(busMsg{kind: msgVote, from: net.vs.nodeID(from), blockID: blk.ID(), payload: vb})
	}
}

// A faulty validator shows seats 1,2 block A and seats 3,4 block B, signs each to its
// half, and a partition longer than the settle window keeps the halves apart.
func TestEquivocation_PartitionDecidesNoTwoBlocks(t *testing.T) {
	vs := newTestValidatorSet(5)
	net := newSimNetStaked(t, vs, vs, siblingParams())
	net.down(0)
	x := vs.nodeID(0)
	left := map[ids.NodeID]bool{vs.nodeID(1): true, vs.nodeID(2): true}
	net.bus.setLink(func(from, to ids.NodeID, _ busMsgKind) bool { return left[from] == left[to] })

	a := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, "A"))
	b := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, "B"))
	net.send(0, a, 1, 2)
	net.send(0, b, 3, 4)
	deliverVote(net, 0, a, voteBytesFor(t, net, 1, 0, a), 1, 2)
	deliverVote(net, 0, b, voteBytesFor(t, net, 3, 0, b), 3, 4)

	time.Sleep(2 * time.Second) // ten settle windows apart
	net.bus.setLink(nil)
	time.Sleep(2 * time.Second)
	if heads := net.headsAtHeight(1); len(heads) > 1 {
		t.Fatalf("two blocks decided at height 1: %v (A=%s B=%s)", heads, a.ID(), b.ID())
	}
}

// The same faulty validator with no partition: every seat holds both blocks before it
// settles, because each half passes on the block it took up, and the four honest seats
// decide one.
func TestEquivocation_HalvesDecideOneBlock(t *testing.T) {
	vs := newTestValidatorSet(5)
	net := newSimNetStaked(t, vs, vs, siblingParams())
	net.down(0)
	x := vs.nodeID(0)
	a := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, "A"))
	b := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, "B"))
	net.send(0, a, 1, 2)
	net.send(0, b, 3, 4)
	deliverVote(net, 0, a, voteBytesFor(t, net, 1, 0, a), 1, 2)
	deliverVote(net, 0, b, voteBytesFor(t, net, 3, 0, b), 3, 4)

	want := a
	if b.ID().Compare(a.ID()) < 0 {
		want = b
	}
	if !waitFor(emergeTO, func() bool { return net.decided(want, 1, 2, 3, 4) }) {
		t.Fatalf("seats 1-4 did not decide %s: heads=%v", want.ID(), net.headsAtHeight(1))
	}
}

// No partition, a 150 ms link delay, and a faulty validator that has seat 1 sign A alone
// before it shows the others a lower B, then signs A to seats 1,2 and B to seats 3,4.
func TestEquivocation_DelayDecidesNoTwoBlocks(t *testing.T) {
	vs := newTestValidatorSet(5)
	net := newSimNetStaked(t, vs, vs, siblingParams()) // settle 200 ms
	net.down(0)
	x := vs.nodeID(0)
	net.bus.setDelay(func() time.Duration { return 150 * time.Millisecond })

	a := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, "A"))
	b := below(x, "B", a, a)
	start := time.Now()
	net.send(0, a, 1)
	aVote := voteBytesFor(t, net, 1, 0, a)
	time.Sleep(250*time.Millisecond - time.Since(start))
	net.send(0, b, 2, 3, 4)
	bVote := voteBytesFor(t, net, 3, 0, b)
	deliverVote(net, 0, a, aVote, 1, 2)
	deliverVote(net, 0, b, bVote, 3, 4)

	time.Sleep(4 * time.Second)
	if heads := net.headsAtHeight(1); len(heads) > 1 {
		t.Fatalf("two blocks decided at height 1: %v (A=%s B=%s)", heads, a.ID(), b.ID())
	}
}

// Three siblings with votes 2, 1 and 1 and one signer unheard: none can reach the ⅔ a
// block is accepted on, so the winner is the second tier's — the lowest of all.
func TestEquivocation_NoSiblingCanReachAlphaTheLowestWins(t *testing.T) {
	vs := newTestValidatorSet(5)
	e, _ := newQuorumEngine(t, prodParams5(), vs, 4, &recordingGossiper{}) // seat 4 is the open seat
	blks := []*verifyOnceBlock{newTestBlock(1, ids.Empty, "s0"), newTestBlock(1, ids.Empty, "s1"), newTestBlock(1, ids.Empty, "s2")}
	sort.Slice(blks, func(i, j int) bool { return bytes.Compare(blks[i].id[:], blks[j].id[:]) < 0 })
	lo, mid, hi := blks[0], blks[1], blks[2]
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, b := range blks {
		cb := &Block{id: b.id, parentID: b.parentID, height: 1, timestamp: b.timestamp.Unix(), data: b.bytes}
		_ = e.consensus.AddBlock(context.Background(), cb)
		e.pendingBlocks[b.id] = &PendingBlock{ConsensusBlock: cb, VMBlock: b, ProposedAt: time.Now()}
	}
	vote := func(b *verifyOnceBlock, seats ...int) {
		pb := e.pendingBlocks[b.id]
		pos := e.blockPositionLocked(pb, b.id)
		for _, s := range seats {
			e.recordCertVoteLocked(pb, Vote{BlockID: b.id, NodeID: vs.nodeID(s), Accept: true, Signature: vs.sign(s, pos)})
		}
	}
	vote(hi, 0, 1) // 2 + 1 open = 3 < 4
	vote(lo, 2)    // 1 + 1 = 2
	vote(mid, 3)   // 1 + 1 = 2
	if w, n, _ := e.convergedWinnerAtHeightLocked(1, ids.Empty); n != 3 || w != lo.id {
		t.Fatalf("winner %s over %d siblings, want the lowest %s", w, n, lo.id)
	}
	// One signer fewer heard, and the two-vote sibling can reach ⅔ again: the first tier.
	delete(e.pendingBlocks[lo.id].certVotes, vs.nodeID(2))
	if w, _, _ := e.convergedWinnerAtHeightLocked(1, ids.Empty); w != hi.id {
		t.Fatalf("winner %s, want %s, the only sibling that can still reach ⅔", w, hi.id)
	}
}

type countingVerifier struct {
	*testValidatorSet
	n atomic.Int64
}

func (c *countingVerifier) VerifyVote(id ids.NodeID, m, s []byte, h uint64) bool {
	c.n.Add(1)
	return c.testValidatorSet.VerifyVote(id, m, s, h)
}

func parkedCount(e *Transitive) (keys, votes int) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for _, v := range e.bufferedVotes {
		keys++
		votes += len(v)
	}
	return keys, votes
}

// A vote for a block this node lacks is parked only on its voter's own word, bounded
// per voter, until its height is decided; and an arriving block checks a bounded
// number of the votes parked for it.
func TestParkedVotes_OnlyTheVoterParksItsOwnBounded(t *testing.T) {
	vs := newTestValidatorSet(5)
	cv := &countingVerifier{testValidatorSet: vs}
	chainID := ids.GenerateTestID()
	e := NewWithConfig(Config{Params: prodParams5()},
		WithQuorumCert(chainID, vs.nodeID(1), cv, &recordingGossiper{}, vs.signerFor(1)),
		WithStakeWeighting(vs))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	rt := &Runtime{Transitive: e}
	sig := make([]byte, vs.SignatureLen())

	for i := 0; i < 64; i++ { // another's id, relayed by seat 0
		vb, _ := encodeSignedVote(ids.GenerateTestNodeID(), sig)
		rt.HandleIncomingVote(vs.nodeID(0), ids.GenerateTestID(), vb)
	}
	outsider := ids.GenerateTestNodeID() // its own id, but no validator
	vb, _ := encodeSignedVote(outsider, sig)
	rt.HandleIncomingVote(outsider, ids.GenerateTestID(), vb)
	vb, _ = encodeSignedVote(vs.nodeID(2), make([]byte, 4096)) // a validator, the wrong length
	rt.HandleIncomingVote(vs.nodeID(2), ids.GenerateTestID(), vb)
	if keys, _ := parkedCount(e); keys != 0 {
		t.Fatalf("%d votes parked that their voter did not send, or no validator could have signed", keys)
	}

	for i := 0; i < 3*maxParkedPerVoter; i++ { // a validator's own votes
		vb, _ := encodeSignedVote(vs.nodeID(2), sig)
		rt.HandleIncomingVote(vs.nodeID(2), ids.GenerateTestID(), vb)
	}
	if keys, _ := parkedCount(e); keys != maxParkedPerVoter {
		t.Fatalf("one validator parked %d votes; the cap is %d", keys, maxParkedPerVoter)
	}
	e.mu.Lock()
	e.expireParkedLocked(1) // the height they were parked at is decided
	e.mu.Unlock()
	if keys, _ := parkedCount(e); keys != 0 {
		t.Fatalf("%d parked votes outlived their height", keys)
	}

	// A proposer's quota full, and every parked slot for its next block taken.
	p := vs.nodeID(0)
	x := &Block{id: ids.GenerateTestID(), parentID: ids.Empty, height: 1}
	e.mu.Lock()
	for i := 0; i < maxSiblingsPerProposer; i++ {
		b := newTestBlock(1, ids.Empty, "q")
		e.pendingBlocks[b.id] = &PendingBlock{ConsensusBlock: &Block{id: b.id, parentID: ids.Empty, height: 1, data: b.bytes}, VMBlock: b, ProposedAt: time.Now(), proposer: p}
	}
	for i := 0; i < maxBufferedVotesPerBlock; i++ {
		e.bufferedVotes[x.id] = append(e.bufferedVotes[x.id], Vote{BlockID: x.id, NodeID: ids.GenerateTestNodeID(), Accept: true, Signature: sig})
	}
	e.mu.Unlock()
	before := cv.n.Load()
	const arrivals = 20
	for i := 0; i < arrivals; i++ {
		e.mu.RLock()
		e.roomLocked(x, p)
		e.mu.RUnlock()
	}
	if n := cv.n.Load() - before; n > arrivals*maxNamedChecks {
		t.Fatalf("%d arrivals checked %d parked signatures; the bound is %d each", arrivals, n, maxNamedChecks)
	}
}

// novaCertFor is a certificate labeled at the majority rung — what an engine that
// accepted on a bare majority assembled and still serves for the heights it decided.
func novaCertFor(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock, voters []int) []byte {
	t.Helper()
	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
	votes := make([]SignedVote, 0, len(voters))
	for _, i := range voters {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	qc, err := AssembleQuorumCert(pos, Nova, uint32(SignerFloor(Nova, len(vs.ids))), votes)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	b, err := qc.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// A certificate's tier and threshold are labels no signature covers, so one labeled at
// the majority rung is read at the ⅔ rung: four of five signers prove the height, three
// do not — the served certificates of heights decided before this rule stay usable,
// and none of them decides a height on a bare majority.
func TestMajorityLabeledCertificateIsReadAtTwoThirds(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)
	blk := newTestBlock(42, ids.GenerateTestID(), "served-before-the-rule")
	vm.register(blk)

	if err := rt.VerifyCatchupCertificate(context.Background(), blk.bytes, novaCertFor(t, vs, chainID, blk, []int{0, 1, 2})); err == nil {
		t.Fatal("a bare-majority certificate proved a height")
	}
	if err := rt.VerifyCatchupCertificate(context.Background(), blk.bytes, novaCertFor(t, vs, chainID, blk, []int{0, 1, 2, 3})); err != nil {
		t.Fatalf("a majority-labeled certificate carrying four of five signers was refused: %v", err)
	}
}
