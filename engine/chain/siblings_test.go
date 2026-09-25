// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// siblings_test.go — a height with more than one live block, on the multi-node
// harness: a validator coming back to a height others have signed, a proposer
// showing each half of the committee a different block, one sending junk, and one
// streaming siblings. Each ends with one block decided on every up validator.
package chain

import (
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// siblingParams is the 5-seat, α=4 committee with a 400 ms round: a 200 ms settle
// window, and a retry from 400 ms doubling to maxRePollBackoff.
func siblingParams() config.Parameters {
	p := prodParams5()
	p.RoundTO = 400 * time.Millisecond
	return p
}

// weighted is a validator set whose members carry unequal stake.
type weighted struct {
	*testValidatorSet
	w     map[ids.NodeID]uint64
	total uint64
}

func weigh(vs *testValidatorSet, units []uint64) weighted {
	s := weighted{testValidatorSet: vs, w: map[ids.NodeID]uint64{}}
	for i, u := range units {
		s.w[vs.nodeID(i)] = u
		s.total += u
	}
	return s
}

func (s weighted) Weight(node ids.NodeID, _ uint64) uint64 { return s.w[node] }
func (s weighted) SignerStake(uint64) uint64               { return s.total }
func (s weighted) CarriedStake(uint64) uint64              { return s.total }

// below is a block at a's height and parent, proposed by seat and named by tag, whose
// id sorts below every one of `than`.
func below(seat ids.NodeID, tag string, a *simBlock, than ...*simBlock) *simBlock {
	for n := 0; ; n++ {
		b := newHonestBlock(a.parentID, a.parentStateRoot, a.height, proposedBy(seat, fmt.Sprintf("%s-%d", tag, n)))
		lowest := true
		for _, o := range than {
			if b.ID().Compare(o.ID()) >= 0 {
				lowest = false
				break
			}
		}
		if lowest {
			return b
		}
	}
}

// decided reports whether every one of seats has decided blk at its height.
func (net *simNet) decided(blk *simBlock, seats ...int) bool {
	for _, i := range seats {
		if got, ok := net.nodes[i].rt.FinalizedBlockAtHeight(blk.height); !ok || got != blk.ID() {
			return false
		}
	}
	return true
}

// trackedFrom counts the undecided blocks seat n tracks at height from proposer who.
func trackedFrom(n *simNode, height uint64, who ids.NodeID) int {
	t := n.rt.Transitive
	t.mu.RLock()
	defer t.mu.RUnlock()
	c := 0
	for _, pb := range t.pendingBlocks {
		sb, ok := pb.VMBlock.(*simBlock)
		if cb := pb.ConsensusBlock; ok && cb != nil && !pb.Decided && cb.height == height && sb.Proposer() == who {
			c++
		}
	}
	return c
}

// A validator comes back to a height the others have signed, and its proposer
// window opens before the signed block reaches it. It builds B, lower than A. The
// height decides only if it signs A: B cannot reach α, and A can.
//
// A block is accepted on the ⅔ certificate, four of five seats, so three signing A
// leave it one short. The second row is rule 2 on its own: a validator holding A when
// its window opens proposes nothing beside it.
func TestSiblings_ReturningValidatorSignsTheBlockThatCanReachAlpha(t *testing.T) {
	for _, row := range []struct {
		name    string
		stake   []uint64
		signers []int // up while A is signed; everyone else is down
		holds   bool  // the returning seat holds A before its window opens
	}{
		{"three sign A", []uint64{1, 1, 1, 1, 1}, []int{0, 1, 2}, false},
		{"the returning seat holds A when its window opens", []uint64{1, 1, 1, 1, 1}, []int{0, 1, 2}, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			vs := newTestValidatorSet(5)
			net := newSimNetStaked(t, vs, weigh(vs, row.stake), siblingParams())
			for i := 2; i < 5; i++ {
				net.down(i)
			}
			for _, i := range row.signers {
				net.up(i)
			}

			a := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(vs.nodeID(0), "A"))
			net.build(0, a)
			if !waitFor(emergeTO, func() bool {
				for _, i := range row.signers {
					if !net.nodes[i].rt.hasSignedHeight(1) {
						return false
					}
				}
				return true
			}) {
				t.Fatal("the up validators never signed A")
			}
			time.Sleep(100 * time.Millisecond) // their votes land
			if _, ok := net.nodes[0].rt.FinalizedBlockAtHeight(1); ok {
				t.Fatal("A decided one short: the row does not set up the case")
			}

			// Seat 3 returns. Its window opens now, before a quiet height's backoff pushes A.
			net.up(3)
			if row.holds && !waitFor(emergeTO, func() bool { return net.nodes[3].rt.HasPendingBlock(a.ID()) }) {
				t.Fatal("A never reached the returning seat")
			}
			b := below(vs.nodeID(3), "B", a, a)
			net.build(3, b)

			seats := append(append([]int{}, row.signers...), 3)
			if !waitFor(emergeTO, func() bool { return net.decided(a, seats...) }) {
				t.Fatalf("height 1 never decided: A=%s (signed by %v) and B=%s < A; heads=%v",
					a.ID(), row.signers, b.ID(), net.headsAtHeight(1))
			}
			if row.holds {
				net.nodes[0].vm.mu.Lock()
				_, saw := net.nodes[0].vm.blockByID[b.ID()]
				net.nodes[0].vm.mu.Unlock()
				if saw {
					t.Fatal("the returning seat proposed B beside A, which it held")
				}
			}
		})
	}
}

// A faulty proposer shows seats 1 and 2 block A and seats 3 and 4 block B, both valid,
// and signs nothing. Each half passes its block on as it takes it up, so every seat
// weighs both before its settle window closes and signs the lower.
func TestSiblings_ProposerShowsEachHalfAnotherBlock(t *testing.T) {
	vs := newTestValidatorSet(5)
	net := newSimNetStaked(t, vs, vs, siblingParams())
	net.down(0) // the faulty proposer: it sends only what the test sends as it
	x := vs.nodeID(0)

	a := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, "A"))
	b := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, "B"))
	net.send(0, a, 1, 2)
	net.send(0, b, 3, 4)

	want := a
	if b.ID().Compare(a.ID()) < 0 {
		want = b
	}
	if !waitFor(emergeTO, func() bool { return net.decided(want, 1, 2, 3, 4) }) {
		t.Fatalf("the halves never converged: want %s decided on seats 1-4; heads=%v", want.ID(), net.headsAtHeight(1))
	}
}

// A faulty proposer sends twenty blocks that do not execute and twenty that do. No
// validator tracks more than maxSiblingsPerProposer of its blocks at the height, the
// lowest kept; the junk is refused block by block, so the next block it sends is
// weighed on its own: one below all the others is taken, and decided.
func TestSiblings_JunkIsRefusedPerBlockAndCapped(t *testing.T) {
	vs := newTestValidatorSet(5)
	net := newSimNetStaked(t, vs, vs, siblingParams())
	net.down(0)
	x := vs.nodeID(0)
	honest := []int{1, 2, 3, 4}

	var most atomic.Int64
	stop := make(chan struct{})
	watched := make(chan struct{})
	go func() {
		defer close(watched)
		for {
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
			for _, i := range honest {
				if c := int64(trackedFrom(net.nodes[i], 1, x)); c > most.Load() {
					most.Store(c)
				}
			}
		}
	}()

	var valid []*simBlock
	for k := 0; k < 20; k++ {
		net.send(0, newForkedBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, fmt.Sprintf("junk-%d", k))), honest...)
		v := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, fmt.Sprintf("valid-%d", k)))
		valid = append(valid, v)
		net.send(0, v, honest...)
	}
	last := below(x, "last", valid[0], valid...)
	net.send(0, last, honest...)

	ok := waitFor(emergeTO, func() bool { return net.decided(last, honest...) })
	close(stop)
	<-watched
	if !ok {
		t.Fatalf("the proposer's last block, below all it sent, was not decided: heads=%v", net.headsAtHeight(1))
	}
	if m := most.Load(); m > maxSiblingsPerProposer {
		t.Fatalf("a validator tracked %d of one proposer's blocks at a height; the cap is %d", m, maxSiblingsPerProposer)
	}
}

// A faulty proposer streams siblings every third of a settle window, each below the
// highest of the lowest four it has sent, so each one takes room. The settle window
// runs from the latest block until one deadline for the height, so every validator
// signs within about two windows of the first block while the stream goes on.
//
// Whether the height then decides is not asserted: a block the stream lands between
// two validators' signatures can be the lower one for the later signer, and at five
// seats with one faulty the ⅔ certificate needs all four honest signatures on one
// block. That split is the exposure one signature per height keeps.
func TestSiblings_StreamingProposerHoldsTheVoteAboutOneWindow(t *testing.T) {
	vs := newTestValidatorSet(5)
	params := siblingParams()
	net := newSimNetStaked(t, vs, vs, params)
	net.down(0)
	x := vs.nodeID(0)
	honest := []int{1, 2, 3, 4}
	settle := params.RoundTO / 2

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		var lowest []*simBlock // the four lowest sent so far
		first := newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, "stream"))
		for k := 0; ; k++ {
			b := first
			if k > 0 {
				b = newHonestBlock(ids.Empty, simGenesisRoot(), 1, proposedBy(x, fmt.Sprintf("stream-%d", k)))
			}
			if len(lowest) >= maxSiblingsPerProposer {
				highest := lowest[0]
				for _, l := range lowest {
					if l.ID().Compare(highest.ID()) > 0 {
						highest = l
					}
				}
				b = below(x, fmt.Sprintf("stream-%d", k), first, highest)
				kept := lowest[:0]
				for _, l := range lowest {
					if l != highest {
						kept = append(kept, l)
					}
				}
				lowest = kept
			}
			lowest = append(lowest, b)
			net.send(0, b, honest...)
			select {
			case <-stop:
				return
			case <-time.After(settle / 3):
			}
		}
	}()
	defer func() { close(stop); <-done }()

	began := time.Now()
	if !waitFor(8*settle, func() bool {
		for _, i := range honest {
			if !net.nodes[i].rt.hasSignedHeight(1) {
				return false
			}
		}
		return true
	}) {
		t.Fatalf("a validator had not signed height 1 %s after a stream began", 8*settle)
	}
	// Two settle windows, a convergence tick, and scheduling slack.
	if held := time.Since(began); held > 3*settle {
		t.Fatalf("the stream held the vote %s; one deadline a height bounds it near two windows (%s)", held, 2*settle)
	}
	if heads := net.headsAtHeight(1); len(heads) > 1 {
		t.Fatalf("two blocks decided at height 1: %v", heads)
	}
}
