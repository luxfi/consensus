// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// runtime_seam_test.go — the runtime's read-only seams onto the node, at the
// two states each of them has: wired, and not.
//
// These are small and none of them decides anything, which is why they are worth
// stating. Each answers a question the engine then acts on, and each has an
// UNWIRED answer that has to be the safe one: a sampler that is not there means
// broadcast rather than poll nobody, and a ledger that is not there means "not
// finalized" rather than the zero id — which would otherwise read as a real
// block at a real height.
//
// The wired rows check WHICH network id the sampler is asked about. The runtime
// holds two, the chain's and the network's, and asking about the chain would
// sample a different validator set — an error that returns a plausible answer
// every time and the wrong one.
package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// recordingSampler answers with a fixed set and remembers what it was asked.
type recordingSampler struct {
	asked  ids.ID
	askedK int
	ids    []ids.NodeID
	err    error
}

func (s *recordingSampler) Sample(networkID ids.ID, k int) ([]ids.NodeID, error) {
	s.asked, s.askedK = networkID, k
	return s.ids, s.err
}

func (s *recordingSampler) Count(networkID ids.ID) int {
	s.asked = networkID
	return len(s.ids)
}

// TestAnUnwiredSamplerMeansBroadcast holds the fallback. A node with no
// validator sampler must get "no sample" and no error, because the caller reads
// nil as "send to everyone" — an error here would stop the poll instead.
func TestAnUnwiredSamplerMeansBroadcast(t *testing.T) {
	rt := &Runtime{config: NetworkConfig{NetworkID: ids.GenerateTestID(), Logger: log.Noop()}}

	sample, err := rt.SampleValidators(21)
	if err != nil {
		t.Fatalf("an unwired sampler must not be an error: %v", err)
	}
	if sample != nil {
		t.Fatalf("an unwired sampler returned a sample: %v", sample)
	}
	if n := rt.ValidatorCount(); n != 0 {
		t.Fatalf("an unwired sampler counts %d validators, want 0", n)
	}
}

// TestTheSamplerIsAskedAboutTheNetworkNotTheChain is the wired half, and the
// property that matters: both seams pass the NETWORK id. A chain id here would
// sample the wrong validator set and answer plausibly every time.
func TestTheSamplerIsAskedAboutTheNetworkNotTheChain(t *testing.T) {
	networkID, chainID := ids.GenerateTestID(), ids.GenerateTestID()
	s := &recordingSampler{ids: []ids.NodeID{ids.GenerateTestNodeID(), ids.GenerateTestNodeID()}}
	rt := &Runtime{
		config:     NetworkConfig{ChainID: chainID, NetworkID: networkID, Logger: log.Noop()},
		validators: s,
	}

	sample, err := rt.SampleValidators(2)
	if err != nil {
		t.Fatalf("SampleValidators: %v", err)
	}
	if len(sample) != 2 {
		t.Fatalf("sampled %d validators, want 2", len(sample))
	}
	if s.asked != networkID {
		t.Fatal("the sampler was asked about the chain, not the network whose validators secure it")
	}
	if s.askedK != 2 {
		t.Fatalf("the sampler was asked for %d, want 2", s.askedK)
	}

	s.asked = ids.Empty
	if n := rt.ValidatorCount(); n != 2 {
		t.Fatalf("counted %d validators, want 2", n)
	}
	if s.asked != networkID {
		t.Fatal("the count was taken over the chain, not the network")
	}

	// A sampler that fails answers with the failure rather than a short sample:
	// a poll built on a partial set is a poll against a quorum that was never
	// sized for it.
	s.err = errors.New("the validator set is not resolved at this height")
	if _, err := rt.SampleValidators(2); !errors.Is(err, s.err) {
		t.Fatalf("a sampler's failure must reach the caller, got %v", err)
	}
}

// TestAnUnwiredLedgerIsNotFinalizedAtAnyHeight is the fail-closed answer for the
// two ledger reads. A runtime with no engine behind it must say "nothing is
// finalized" — returning the zero id with ok=true would name a block at every
// height, and the acceptance check asks these.
func TestAnUnwiredLedgerIsNotFinalizedAtAnyHeight(t *testing.T) {
	for _, row := range []struct {
		holds string
		rt    *Runtime
	}{
		{"a runtime with no engine", &Runtime{config: NetworkConfig{Logger: log.Noop()}}},
		{"a runtime whose engine has no consensus", &Runtime{Transitive: &Transitive{}, config: NetworkConfig{Logger: log.Noop()}}},
	} {
		t.Run(row.holds, func(t *testing.T) {
			tip, height, set := row.rt.FinalizedLedger()
			if set || height != 0 || tip != ids.Empty {
				t.Fatalf("an unwired ledger reported a tip: %v %d %v", tip, height, set)
			}
			if id, ok := row.rt.FinalizedBlockAtHeight(1); ok || id != ids.Empty {
				t.Fatalf("an unwired ledger named a block at height 1: %v %v", id, ok)
			}
		})
	}
}

// TestTheLedgerAdvancesWithFinality is the wired half of the same two reads, and
// it is the reason they exist as a pair: the ledger is the single advancing
// source of truth for where the accepted chain is, and a VM's LastAccepted can
// sit frozen at the boot snapshot while it moves.
func TestTheLedgerAdvancesWithFinality(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()

	e := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithStakeWeighting(vs))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	rt := &Runtime{Transitive: e, config: NetworkConfig{ChainID: chainID, Logger: log.Noop()}}

	if _, _, set := rt.FinalizedLedger(); set {
		t.Fatal("a node that has finalized nothing reported a tip")
	}

	blk := newTestBlock(1, ids.Empty, "ledger")
	trackVerifiedBlock(rt, blk, 0)

	pos := VotePosition{ChainID: chainID, Height: 1, Round: 0, BlockID: blk.id, ParentID: ids.Empty}
	cert, err := AssembleQuorumCert(pos, Quasar, 3, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	})
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	wire, err := cert.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !rt.HandleIncomingCert(wire) {
		t.Fatal("a valid export cert did not finalize — the rest of this test says nothing")
	}

	tip, height, set := rt.FinalizedLedger()
	if !set || height != 1 || tip != blk.id {
		t.Fatalf("the ledger did not advance to the finalized block: %v %d %v", tip, height, set)
	}
	if id, ok := rt.FinalizedBlockAtHeight(1); !ok || id != blk.id {
		t.Fatalf("height 1 names %v (%v), want the finalized block", id, ok)
	}
	// A height above the tip is not finalized — the ledger does not extrapolate.
	if id, ok := rt.FinalizedBlockAtHeight(2); ok {
		t.Fatalf("a height above the tip named a block: %v", id)
	}

	// The epoch seam reads the tracked block's bound epoch, and answers 0 for a
	// height it is not tracking rather than guessing one.
	if got := rt.epochForHeight(1); got != 0 {
		t.Fatalf("a fixed-set chain resolves epoch %d at a tracked height, want 0", got)
	}
	if got := rt.epochForHeight(99); got != 0 {
		t.Fatalf("an untracked height resolved to epoch %d, want 0", got)
	}
}
