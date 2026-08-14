// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"sync"
	"testing"

	"github.com/luxfi/ids"
)

// recordingCertStore captures what height each cert was durably filed under.
type recordingCertStore struct {
	mu      sync.Mutex
	heights map[ids.ID]uint64
	order   []ids.ID
}

func newRecordingCertStore() *recordingCertStore {
	return &recordingCertStore{heights: map[ids.ID]uint64{}}
}

func (c *recordingCertStore) Put(blockID ids.ID, height uint64, cert []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, seen := c.heights[blockID]; !seen {
		c.order = append(c.order, blockID)
	}
	c.heights[blockID] = height
	return nil
}

func (c *recordingCertStore) Get(blockID ids.ID) ([]byte, bool) { return nil, false }

func (c *recordingCertStore) heightFor(blockID ids.ID) (uint64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	h, ok := c.heights[blockID]
	return h, ok
}

// TestPersistServedCert_FilesUnderTheCertsOwnHeight pins the one property the
// durable cert store is built on: retention is ordered BY HEIGHT — "keeps the
// most recent maxServedCerts, drops the lowest" — so a cert filed under the
// wrong height is a cert nobody can find.
//
// The height a cert certifies belongs to the CERT, and to nothing else. Taking
// it from `highestAccepted` — the top of whatever this particular call happened
// to accept — reads zero whenever there was nothing left to accept, which is the
// ordinary case for a cert about a block we already hold.
//
// A store whose keys are all zero has no order, so it answers for no height: the
// serve path finds nothing, pairs each block with an empty cert, and a node
// behind the tip can neither accept one nor reject one. The in-memory window
// covers for this until a restart drops it, so the durable height is the only
// thing standing between a lagging node and a permanent gap.
func TestPersistServedCert_FilesUnderTheCertsOwnHeight(t *testing.T) {
	const certifiedHeight = uint64(1162600)

	store := newRecordingCertStore()
	e := NewWithConfig(Config{Params: params5()}, WithCerts(store))

	blockID := ids.GenerateTestID()

	// Persist exactly as the finalizer does. The height MUST be the one the cert
	// attests, independent of what this call accepted.
	e.persistServedCert(blockID, certifiedHeight, []byte("marshaled-cert"))

	got, ok := store.heightFor(blockID)
	if !ok {
		t.Fatal("the cert was not persisted at all — a straggler has no way back")
	}
	if got == 0 {
		t.Fatalf("cert filed under height 0. Retention is ordered by height, so an "+
			"all-zero store has no order and answers for no height — the serve path "+
			"then hands out EMPTY certs and no node behind the tip can ever catch up. "+
			"Want %d", certifiedHeight)
	}
	if got != certifiedHeight {
		t.Fatalf("cert filed under height %d, want %d — the height belongs to the cert, "+
			"not to whatever the caller happened to accept", got, certifiedHeight)
	}
}

// TestPersistServedCert_SkipsWhenThereIsNoCert guards the other direction: an
// empty cert is not worth a disk write, and writing one would occupy a slot in a
// bounded, height-ordered window that a real cert needs.
func TestPersistServedCert_SkipsWhenThereIsNoCert(t *testing.T) {
	store := newRecordingCertStore()
	e := NewWithConfig(Config{Params: params5()}, WithCerts(store))

	e.persistServedCert(ids.GenerateTestID(), 42, nil)
	e.persistServedCert(ids.GenerateTestID(), 43, []byte{})

	store.mu.Lock()
	n := len(store.order)
	store.mu.Unlock()
	if n != 0 {
		t.Fatalf("persisted %d empty certs; an empty cert proves nothing and evicts a real one", n)
	}
}

// TestApplyBranchFinalization_PersistsUnderTheCertHeight is the test that would
// have caught this, and the reason the two above would not.
//
// They call persistServedCert directly and hand it the height, so they pass
// whatever the real finalizer computes. TestCertOutlivesTheProcess does the same
// — it says it runs "the same two steps applyBranchFinalization runs" and then
// writes persistServedCert(blockID, 15499, cert) with the height spelled out. A
// test that re-implements its caller cannot observe the caller being wrong.
//
// So this one drives the REAL path, with the plan EMPTY — the ordinary case for
// a cert about a block already accepted, and precisely when highestAccepted is 0.
func TestApplyBranchFinalization_PersistsUnderTheCertHeight(t *testing.T) {
	const height = uint64(1162600)

	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	blockID := ids.GenerateTestID()

	pos := VotePosition{ChainID: chainID, Height: height, Round: 0, BlockID: blockID, ParentID: ids.Empty}
	qc, err := AssembleQuorumCert(pos, Quasar, 3, []SignedVote{
		{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(0, pos)},
		{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
		{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
	})
	if err != nil {
		t.Fatalf("assemble cert: %v", err)
	}

	store := newRecordingCertStore()
	e := NewWithConfig(Config{Params: params5()}, WithCerts(store))

	// Nothing left to accept — the cert is about a block we already hold.
	_, err = e.applyBranchFinalization(t.Context(), Plan{}, blockID, VerifiedQuorumCert{qc: qc})
	if err != nil {
		t.Fatalf("applyBranchFinalization: %v", err)
	}

	got, ok := store.heightFor(blockID)
	if !ok {
		t.Fatal("the finalizer persisted no cert at all")
	}
	if got != height {
		t.Fatalf("finalizer filed the cert under height %d, want %d — retention is "+
			"ordered by height, so height 0 leaves the store with no order and no height "+
			"to answer for: the serve path hands out empty certs and every node behind "+
			"the tip stays behind it", got, height)
	}
}
