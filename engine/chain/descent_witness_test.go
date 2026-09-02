// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// descent_witness_test.go — the two places a node answers a question it may not
// be able to answer, and has to say so rather than guess.
//
// Descent serves history to a recovering peer. It walks upward and stops at the
// first position it cannot serve, and a SHORT run is honest — a run with a hole
// in it is the one outcome that must be impossible, because the peer takes the
// run as contiguous. So the interesting cases are all the ways a height can be
// half-known: named by the recovery index but absent from the VM, present in the
// VM but with no cert filed.
//
// ToQuasarCert is the other: it MAPS an already-verified engine cert onto the
// post-quantum witness and re-decides nothing. Every piece of validator-set
// material it needs comes from the node, and any one of them missing means the
// caller keeps the engine witness. It must never fabricate a record.
package chain

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/ids"
)

// TestDescentRefusesWhereItCannotServe walks the positions a node cannot answer
// from. Each one ends the run rather than skipping the height, because a peer
// reads a run as the contiguous chain following Base.
func TestDescentRefusesWhereItCannotServe(t *testing.T) {
	t.Run("a runtime with no engine behind it", func(t *testing.T) {
		if _, err := (&Runtime{}).From(context.Background(), 1, 4); !errors.Is(err, ErrNoDescent) {
			t.Fatalf("want ErrNoDescent, got %v", err)
		}
	})

	t.Run("a runtime with no VM to read blocks from", func(t *testing.T) {
		rt, _ := servingNode(t, 2)
		rt.config.VM = nil
		if _, err := rt.From(context.Background(), 1, 4); !errors.Is(err, ErrNoDescent) {
			t.Fatalf("want ErrNoDescent, got %v", err)
		}
	})

	t.Run("a height below anything this node finalized", func(t *testing.T) {
		rt, _ := servingNode(t, 2)
		if _, err := rt.From(context.Background(), 99, 4); !errors.Is(err, ErrNoDescent) {
			t.Fatalf("a height this node never finalized must be a refusal, got %v", err)
		}
	})

	t.Run("a request for nothing is not a refusal", func(t *testing.T) {
		rt, _ := servingNode(t, 2)
		run, err := rt.From(context.Background(), 1, 0)
		if err != nil {
			t.Fatalf("asking for zero blocks is a well-formed question: %v", err)
		}
		if run.Base != 1 || len(run.Chain) != 0 {
			t.Fatalf("a zero-length request served %d entries at base %d", len(run.Chain), run.Base)
		}
	})
}

// TestDescentStopsAtTheFirstHeightItCannotServe is the contiguity property. Two
// ways a height goes half-known — the index names a block the VM does not hold,
// and the VM holds a block no cert was filed for — and both must TRUNCATE the
// run at that height rather than serve the heights above it, which would hand
// the peer a chain with a hole in it that reads as contiguous.
func TestDescentStopsAtTheFirstHeightItCannotServe(t *testing.T) {
	t.Run("the index names a block the VM does not hold", func(t *testing.T) {
		rt, _ := servingNode(t, 3)
		// Height 2 is finalized and filed, but its block never reaches the VM.
		rt.Transitive.mu.Lock()
		rt.Transitive.recoveredAt[2] = ids.GenerateTestID()
		rt.Transitive.mu.Unlock()

		run, err := rt.From(context.Background(), 1, 3)
		if err != nil {
			t.Fatalf("height 1 is servable, so the run is not a refusal: %v", err)
		}
		if len(run.Chain) != 1 {
			t.Fatalf("served %d entries, want 1 — the run must stop at the height it cannot serve", len(run.Chain))
		}
	})

	t.Run("the VM holds a block no cert was filed for", func(t *testing.T) {
		rt, _ := servingNode(t, 3)
		rt.Transitive.mu.Lock()
		id := rt.Transitive.recoveredAt[2]
		delete(rt.Transitive.certByDecision, id)
		rt.Transitive.mu.Unlock()

		run, err := rt.From(context.Background(), 1, 3)
		if err != nil {
			t.Fatalf("height 1 is servable: %v", err)
		}
		if len(run.Chain) != 1 {
			t.Fatalf("served %d entries, want 1 — a block with no cert is not certified history", len(run.Chain))
		}
	})
}

// partialWitness is a CryptoWitnessSource missing exactly one piece of the
// material ToQuasarCert needs, so each refusal can be reached on its own.
type partialWitness struct {
	haveRoot      bool
	haveThreshold bool
	haveRecord    bool
}

func (partialWitness) Epoch(height uint64) uint64 { return height }

func (w partialWitness) ValidatorSetRoot(uint64) ([48]byte, bool) {
	return [48]byte{}, w.haveRoot
}

func (w partialWitness) QuorumThreshold(uint64) (uint64, bool) { return 3, w.haveThreshold }

func (w partialWitness) SignerRecord(ids.NodeID, []byte, []byte) (quasar.QuorumSignerRecord, bool) {
	return quasar.QuorumSignerRecord{}, w.haveRecord
}

// TestToQuasarCertFabricatesNothing holds the mapping's contract: every piece of
// validator-set material comes from the node, and any one of them missing yields
// an error and NO cert. A witness assembled around a missing root or a missing
// threshold would be a post-quantum proof of nothing, and the caller would ship
// it as one.
func TestToQuasarCertFabricatesNothing(t *testing.T) {
	f := newCertFixture(t, 4)
	cert := f.cert(t, Quasar, uint32(NovaSignerFloor(4)), 4)

	for _, row := range []struct {
		holds string
		cert  *QuorumCert
		src   CryptoWitnessSource
		want  error
	}{
		{
			holds: "no cert to upgrade",
			cert:  nil, src: partialWitness{true, true, true}, want: ErrQCNil,
		},
		{
			holds: "no validator-set material plumbed at all",
			cert:  cert, src: nil, want: ErrQuasarWitnessUnavailable,
		},
		{
			holds: "no set root at the epoch the cert finalizes in",
			cert:  cert, src: partialWitness{false, true, true}, want: ErrQuasarWitnessUnavailable,
		},
		{
			holds: "no quorum weight floor at that epoch",
			cert:  cert, src: partialWitness{true, false, true}, want: ErrQuasarWitnessUnavailable,
		},
		{
			holds: "a voter the weighted set does not hold a leaf for",
			cert:  cert, src: partialWitness{true, true, false}, want: ErrQuasarVoterNotInSet,
		},
	} {
		t.Run(row.holds, func(t *testing.T) {
			got, err := row.cert.ToQuasarCert(row.src)
			if !errors.Is(err, row.want) {
				t.Fatalf("want %v, got %v", row.want, err)
			}
			if got != nil {
				t.Fatal("a refused upgrade returned a witness — the caller would ship it")
			}
		})
	}
}
