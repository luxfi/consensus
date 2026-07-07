// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// multinode_clone_test.go — the vote-guard CLONE-ARTIFACT repro + the re-sign fallback.
//
// The artifact that froze testnet: all 5 validators were restored from ONE VolumeSnapshot
// taken at a voted-but-UNFINALIZED height. Each booted with committedSlot[H] durably set
// (the binding) but certVotes EMPTY — the vote-guard persisted the BINDING, not the
// SIGNATURE. The old emitConvergedVote saw hasSignedHeight(H)==true and returned early, so
// no node ever re-contributed its vote → the height never reached α-of-K → wedged forever.
// This would freeze ANY build (even finalizing mainnet v1.34.14); it is not a build bug.
//
// This test reproduces it end-to-end on a persistent-vote-guard net: build H, let every
// node BIND its slot (votes partitioned so no cert forms), KILL ALL nodes, RESTART ALL from
// the fsync'd guards (committedSlot re-seeded, certVotes empty — the exact clone state),
// heal, and assert H FINALIZES. Without the re-sign fallback the restarted fleet stays
// wedged (the assertion times out); with it, each node re-signs the DURABLE committedSlot[H]
// canonical (never a fresh winner) and one α-of-K cert forms.
package chain

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// guardedNet is a persistent-vote-guard variant of simNet: each node's engine is wired to a
// fileVoteGuard on disk, and restartAll() rebuilds every Runtime from the fsync'd guards
// (the block store / simVM persists; certVotes does NOT) — modeling a mass restart.
type guardedNet struct {
	t          *testing.T
	vs         *testValidatorSet
	bus        *simBus
	chainID    ids.ID
	params     config.Parameters
	nodes      []*simNode
	guardPaths []string
	// samplers, when set, wires a per-node ValidatorSampler into NewRuntime so the
	// engine exercises the bftCommittee floor + reclampCommitteeLocked live-count paths
	// (FIX #1). nil (the default) leaves cfg.Validators unwired — the fixed-preset
	// behavior every pre-existing guardedNet test relies on.
	samplers []ValidatorSampler
}

func newGuardedNet(t *testing.T, n int, params config.Parameters) *guardedNet {
	return newGuardedNetSampled(t, n, params, nil)
}

// newGuardedNetSampled is newGuardedNet with an optional per-node ValidatorSampler
// slice (len n, or nil to leave every node unwired). The samplers MUST exist before
// the runtimes are built because bftCommittee reads Count() at construction.
func newGuardedNetSampled(t *testing.T, n int, params config.Parameters, samplers []ValidatorSampler) *guardedNet {
	t.Helper()
	g := &guardedNet{
		t:        t,
		vs:       newTestValidatorSet(n),
		bus:      newSimBus(),
		chainID:  ids.GenerateTestID(),
		params:   params,
		samplers: samplers,
	}
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		g.guardPaths = append(g.guardPaths, filepath.Join(dir, "vote-guard-"+string(rune('0'+i))))
		vm := newSimVM()
		node := &simNode{
			nodeID: g.vs.nodeID(i),
			vm:     vm,
			inbox:  make(chan busMsg, 4096),
			stop:   make(chan struct{}),
			up:     true,
		}
		node.rt = g.buildRuntime(i, vm)
		g.bus.register(node)
		g.nodes = append(g.nodes, node)
	}
	for _, node := range g.nodes {
		node.wg.Add(1)
		go node.run()
	}
	t.Cleanup(g.shutdown)
	return g
}

// buildRuntime opens node i's guard from disk (re-seeding committedSlot + decidedFloor) and
// wires a fresh Runtime over the given (persistent) VM.
func (g *guardedNet) buildRuntime(i int, vm *simVM) *Runtime {
	store, err := OpenVoteGuard(g.guardPaths[i])
	if err != nil {
		g.t.Fatalf("node %d OpenVoteGuard: %v", i, err)
	}
	cfg := NetworkConfig{
		ChainID:      g.chainID,
		NetworkID:    ids.Empty,
		NodeID:       g.vs.nodeID(i),
		Logger:       log.Noop(),
		Gossiper:     &busGossiper{bus: g.bus, self: g.vs.nodeID(i)},
		VM:           vm,
		Params:       &g.params,
		VoteVerifier: g.vs,
		VoteSigner:   g.vs.signerFor(i),
		StakeSource:  g.vs,
		VoteGuard:    store,
	}
	if i < len(g.samplers) && g.samplers[i] != nil {
		cfg.Validators = g.samplers[i] // exercise the floor + reclamp live-count paths
	}
	rt := NewRuntime(cfg)
	if err := rt.Start(context.Background(), true); err != nil {
		g.t.Fatalf("node %d Start: %v", i, err)
	}
	return rt
}

// restartAll models "kill ALL nodes, restart ALL": stop every delivery loop + engine, then
// rebuild each Runtime from its fsync'd guard over the SAME simVM (blocks persist; the
// in-memory certVotes are gone). This is the exact clone state — committedSlot re-seeded,
// certVotes empty.
func (g *guardedNet) restartAll() {
	for _, n := range g.nodes {
		close(n.stop)
	}
	for _, n := range g.nodes {
		n.wg.Wait()
		_ = n.rt.Stop(context.Background())
	}
	for i, n := range g.nodes {
		n.rt = g.buildRuntime(i, n.vm) // same VM, reopened guard
		n.stop = make(chan struct{})
		n.inbox = make(chan busMsg, 4096)
	}
	for _, n := range g.nodes {
		n.wg.Add(1)
		go n.run()
	}
}

func (g *guardedNet) shutdown() {
	for _, n := range g.nodes {
		select {
		case <-n.stop:
		default:
			close(n.stop)
		}
	}
	for _, n := range g.nodes {
		n.wg.Wait()
		_ = n.rt.Stop(context.Background())
	}
}

func (g *guardedNet) deliverBlockToAll(blk *simBlock) {
	for _, n := range g.nodes {
		n.enqueue(busMsg{kind: msgBlock, from: g.vs.nodeID(0), payload: blk.Bytes()})
	}
}

func (g *guardedNet) allBound(height uint64) bool {
	for _, n := range g.nodes {
		if _, ok := n.rt.Transitive.committedCanonical(height); !ok {
			return false
		}
	}
	return true
}

func (g *guardedNet) allFinalized(blk *simBlock) bool {
	for _, n := range g.nodes {
		got, ok := n.rt.FinalizedBlockAtHeight(blk.height)
		if !ok || got != blk.ID() {
			return false
		}
	}
	return true
}

// TestMultiNode_CloneArtifact_ResignFallback_Finalizes is the mass-restart re-seed proof.
func TestMultiNode_CloneArtifact_ResignFallback_Finalizes(t *testing.T) {
	g := newGuardedNet(t, 5, prodParams5())

	h1 := newHonestBlock(ids.Empty, simGenesisRoot(), 1, "clone-h1")

	// PARTITION VOTES + CERTS: blocks and prevotes gossip, but accept-votes / certs do NOT.
	// Every node builds/tracks H1 and BINDS committedSlot[1] (durably persisted by
	// reserveSlotForSign) + records its OWN vote — but no cross-node vote arrives, so no
	// α-of-K cert forms. This is the mid-vote snapshot state.
	g.bus.setLink(func(_, _ ids.NodeID, kind busMsgKind) bool {
		return kind != msgVote && kind != msgCert
	})
	g.deliverBlockToAll(h1)

	if !waitFor(emergeTO, func() bool { return g.allBound(1) }) {
		t.Fatalf("precondition: all nodes must BIND committedSlot[1] before restart")
	}
	if g.allFinalized(h1) {
		t.Fatal("precondition: H1 must NOT be finalized yet (votes partitioned)")
	}
	t.Logf("BEFORE restart: all 5 nodes bound committedSlot[1] (mid-vote), H1 UNfinalized")

	// KILL ALL + RESTART ALL from the fsync'd guards: committedSlot[1] re-seeds, certVotes
	// empty — the exact clone artifact. HEAL votes/certs.
	g.restartAll()
	g.bus.setLink(nil)
	g.deliverBlockToAll(h1) // re-gossip so each node re-tracks H1's pending block

	start := time.Now()
	if !waitFor(reconvergeTO, func() bool { return g.allFinalized(h1) }) {
		t.Fatalf("CLONE ARTIFACT: H1 did NOT finalize after mass-restart within %s — the re-sign fallback did not fire (all 5 bound but no re-contributed votes → no α-of-K)", reconvergeTO)
	}
	t.Logf("AFTER restart: H1 FINALIZED at 5/5 in %s via re-sign fallback (each node re-signed its DURABLE committedSlot[1] canonical — no re-vote of a fresh winner, no equivocation)", time.Since(start).Round(time.Millisecond))
}
