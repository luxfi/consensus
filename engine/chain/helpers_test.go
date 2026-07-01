// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// -----------------------------------------------------------------------------
// nodeAuth — a deterministic per-node HMAC authenticator for tests.
//
// It models a real vote authenticator without per-test key registration: each
// validator's secret key is derived from its NodeID (key = SHA256("lux-test-vote-key"||nodeID)),
// a vote signature is HMAC-SHA256(key, message), and verification recomputes it.
// This is a genuine MAC (forging a vote requires the node's derived key), so the
// security properties under test (forged/unknown/wrong-position votes fail) hold
// — while letting existing tests keep using ids.GenerateTestNodeID() and only
// attach a signature. It is NOT used in production (the node injects a real
// signature scheme); it exists purely so the engine's quorum-cert path can be
// exercised with arbitrary node ids.
// -----------------------------------------------------------------------------

type nodeAuth struct{}

var testAuth = nodeAuth{}

func (nodeAuth) keyFor(nodeID ids.NodeID) []byte {
	h := sha256.Sum256(append([]byte("lux-test-vote-key"), nodeID[:]...))
	return h[:]
}

func (a nodeAuth) sign(nodeID ids.NodeID, message []byte) []byte {
	mac := hmac.New(sha256.New, a.keyFor(nodeID))
	mac.Write(message)
	return mac.Sum(nil)
}

// VerifyVote implements VoteVerifier: recompute the node's MAC over message.
// The MAC is keyed by nodeID, not by epoch, so epochHeight is not consulted here
// (the deterministic harness has one fixed key per node across all epochs).
func (a nodeAuth) VerifyVote(nodeID ids.NodeID, message []byte, sig []byte, _ uint64) bool {
	want := a.sign(nodeID, message)
	return hmac.Equal(want, sig)
}

// signerFor returns a VoteSigner bound to nodeID's derived key.
func (a nodeAuth) signerFor(nodeID ids.NodeID) VoteSigner {
	return voteSignerFunc(func(message []byte) ([]byte, error) { return a.sign(nodeID, message), nil })
}

// signedVoteForEngine builds a signed accept Vote for a block the engine is
// already tracking, signing the engine's OWN reconstructed position so the
// engine's handleVote verification accepts it. This is the minimal-edit helper
// for migrating legacy tests: replace an unsigned ReceiveVote(Vote{...,Accept:true})
// with ReceiveVote(signedVoteForEngine(e, blockID, nodeID)).
func signedVoteForEngine(e *Transitive, blockID ids.ID, nodeID ids.NodeID) Vote {
	e.mu.RLock()
	pending, ok := e.pendingBlocks[blockID]
	var pos VotePosition
	if ok {
		pos = e.blockPositionLocked(pending, blockID)
	} else {
		pos = VotePosition{ChainID: e.chainID, BlockID: blockID}
	}
	e.mu.RUnlock()
	return Vote{
		BlockID:   blockID,
		NodeID:    nodeID,
		Accept:    true,
		SignedAt:  time.Now(),
		Signature: testAuth.sign(nodeID, CanonicalVoteMessage(pos)),
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	}
}

// newTestEngine builds an UNSTARTED engine wired with the deterministic
// nodeAuth verifier+signer (so the K>1 Start guard is satisfied) plus any extra
// options. The drop-in replacement for `New(opts...)` in legacy lifecycle tests
// that call Start/Stop themselves. The self nodeID is a fresh test id.
func newTestEngine(opts ...Option) *Transitive {
	self := ids.GenerateTestNodeID()
	all := append([]Option{WithQuorumCert(ids.Empty, self, testAuth, nil, testAuth.signerFor(self))}, opts...)
	return New(all...)
}

// newTestEngineParams is newTestEngine with explicit params.
func newTestEngineParams(params config.Parameters, opts ...Option) *Transitive {
	self := ids.GenerateTestNodeID()
	all := append([]Option{WithParams(params), WithQuorumCert(ids.Empty, self, testAuth, nil, testAuth.signerFor(self))}, opts...)
	return New(all...)
}

// signedRejectForEngine builds a signed REJECT vote for a tracked block (signs
// the reject-bound canonical message). The reject counterpart of
// signedVoteForEngine.
func signedRejectForEngine(e *Transitive, blockID ids.ID, nodeID ids.NodeID) Vote {
	e.mu.RLock()
	pending, ok := e.pendingBlocks[blockID]
	var pos VotePosition
	if ok {
		pos = e.blockPositionLocked(pending, blockID)
	} else {
		pos = VotePosition{ChainID: e.chainID, BlockID: blockID}
	}
	e.mu.RUnlock()
	return Vote{
		BlockID:   blockID,
		NodeID:    nodeID,
		Accept:    false,
		SignedAt:  time.Now(),
		Signature: testAuth.sign(nodeID, canonicalVoteMessageFor(pos, false)),
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	}
}

// newAuthEngine builds a started multi-validator engine wired with the
// deterministic nodeAuth verifier+signer for `self`. Used by migrated legacy
// tests that need Start to succeed under the K>1 guard.
func newAuthEngine(t *testing.T, params config.Parameters, self ids.NodeID, opts ...Option) *Transitive {
	t.Helper()
	all := append([]Option{WithQuorumCert(ids.Empty, self, testAuth, nil, testAuth.signerFor(self))}, opts...)
	e := NewWithConfig(Config{Params: params}, all...)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	return e
}

// testValidatorSet is a deterministic Ed25519-backed validator key set used to
// exercise the real quorum-cert path in tests. It is a faithful stand-in for a
// node-layer VoteVerifier/VoteSigner: each validator has an Ed25519 keypair,
// signatures are over the canonical vote message, and verification rejects
// unknown validators and bad signatures. Ed25519 is a proven primitive — the
// tests do NOT roll custom crypto.
type testValidatorSet struct {
	mu   sync.RWMutex
	keys map[ids.NodeID]ed25519.PrivateKey
	pub  map[ids.NodeID]ed25519.PublicKey
	ids  []ids.NodeID

	// committed models each honest validator running the fixed engine's per-height
	// vote-once discipline (reserveSlotForSign): keyed (validatorIndex,height) →
	// the canonical it first signed. Asking a validator to sign a DIFFERENT block at
	// a height it already signed returns an unsigned (refused) vote — an honest
	// validator never equivocates. So a real cross-node fork needs ≥ intersection
	// (≥3 of 5 here) ACTUAL Byzantine equivocators — f ≥ n/3, beyond the guarantee.
	committed map[[2]uint64]ids.ID
}

func newTestValidatorSet(n int) *testValidatorSet {
	s := &testValidatorSet{
		keys:      make(map[ids.NodeID]ed25519.PrivateKey, n),
		pub:       make(map[ids.NodeID]ed25519.PublicKey, n),
		committed: make(map[[2]uint64]ids.ID),
	}
	for i := 0; i < n; i++ {
		nodeID := ids.GenerateTestNodeID()
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			panic(err)
		}
		s.keys[nodeID] = priv
		s.pub[nodeID] = pub
		s.ids = append(s.ids, nodeID)
	}
	return s
}

func (s *testValidatorSet) nodeID(i int) ids.NodeID { return s.ids[i] }

// VerifyVote implements VoteVerifier. Unknown node → false (never an error).
// The fixed test set is the same at every epoch, so epochHeight is not consulted
// here; the height-pinned-resolution behavior (RESIDUAL-B) is exercised by the
// dedicated epochValidatorSet in quorum_finality_test.go.
func (s *testValidatorSet) VerifyVote(nodeID ids.NodeID, message []byte, sig []byte, _ uint64) bool {
	s.mu.RLock()
	pub, ok := s.pub[nodeID]
	s.mu.RUnlock()
	if !ok {
		return false
	}
	return ed25519.Verify(pub, message, sig)
}

// Weight implements StakeSource: every validator in the test set carries EQUAL unit
// weight (1), so a count-α quorum is also a ⅔-stake supermajority — the test set is a
// valid stake source for the value-DEX quorum-finality gate (Mode() requires one). An
// unknown node has weight 0 (it contributes no stake).
func (s *testValidatorSet) Weight(nodeID ids.NodeID, _ uint64) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.pub[nodeID]; ok {
		return 1
	}
	return 0
}

// TotalStake implements StakeSource: the total active stake is the validator count
// (equal unit weights). Non-zero for a non-empty set, so the stake-weighted check is
// usable (a zero total would be treated as unusable / fail-closed by VerifyWeighted).
func (s *testValidatorSet) TotalStake(_ uint64) uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return uint64(len(s.ids))
}

// signerFor returns a VoteSigner bound to validator i's key.
func (s *testValidatorSet) signerFor(i int) VoteSigner {
	nodeID := s.ids[i]
	return voteSignerFunc(func(message []byte) ([]byte, error) {
		s.mu.RLock()
		priv := s.keys[nodeID]
		s.mu.RUnlock()
		return ed25519.Sign(priv, message), nil
	})
}

// sign produces validator i's signature over the position's canonical message.
func (s *testValidatorSet) sign(i int, pos VotePosition) []byte {
	s.mu.RLock()
	priv := s.keys[s.ids[i]]
	s.mu.RUnlock()
	return ed25519.Sign(priv, CanonicalVoteMessage(pos))
}

// signedVote builds validator i's signed accept Vote for a position, HONORING the
// per-validator vote-once discipline: an honest validator signs at most one canonical
// per height. If validator i was already asked to sign a DIFFERENT canonical at this
// height, it REFUSES — returns an unsigned vote (empty signature) that fails
// verification and is never counted, exactly as a fixed-engine honest validator would.
// The same canonical is idempotent. This makes the test set model reality post-fix, so
// a cross-node fork can only be forced by ACTUAL Byzantine equivocators (f ≥ n/3).
func (s *testValidatorSet) signedVote(i int, pos VotePosition) Vote {
	key := [2]uint64{uint64(i), pos.Height}
	canonical := slotCanonical(pos)
	s.mu.Lock()
	if bound, ok := s.committed[key]; ok && bound != canonical {
		s.mu.Unlock()
		// honest validator refuses to equivocate: unsigned vote (won't verify).
		return Vote{BlockID: pos.BlockID, NodeID: s.ids[i], Accept: true, SignedAt: time.Now(), ParentID: pos.ParentID, Round: pos.Round}
	}
	s.committed[key] = canonical
	s.mu.Unlock()
	return Vote{
		BlockID:   pos.BlockID,
		NodeID:    s.ids[i],
		Accept:    true,
		SignedAt:  time.Now(),
		Signature: s.sign(i, pos),
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	}
}

type voteSignerFunc func(message []byte) ([]byte, error)

func (f voteSignerFunc) SignVote(message []byte) ([]byte, error) { return f(message) }

// recordingGossiper captures gossiped certs/votes for assertions and can relay
// them to a set of follower runtimes (to drive an end-to-end multi-node test in
// a single process).
type recordingGossiper struct {
	mu        sync.Mutex
	certs     [][]byte
	votes     [][]byte
	certBlock []ids.ID
}

func (g *recordingGossiper) GossipPut(ids.ID, ids.ID, []byte) int { return 0 }
func (g *recordingGossiper) SendPullQuery(ids.ID, ids.ID, ids.ID, []ids.NodeID) int {
	return 0
}
func (g *recordingGossiper) SendPushQuery(ids.ID, ids.ID, []byte, []ids.NodeID) int { return 0 }
func (g *recordingGossiper) SendVote(ids.ID, ids.NodeID, ids.ID) error              { return nil }

func (g *recordingGossiper) BroadcastVote(_ ids.ID, _ ids.ID, _ ids.ID, voteBytes []byte) int {
	g.mu.Lock()
	g.votes = append(g.votes, append([]byte(nil), voteBytes...))
	g.mu.Unlock()
	return 1
}

func (g *recordingGossiper) GossipCert(_ ids.ID, blockID ids.ID, certBytes []byte) error {
	g.mu.Lock()
	g.certs = append(g.certs, append([]byte(nil), certBytes...))
	g.certBlock = append(g.certBlock, blockID)
	g.mu.Unlock()
	return nil
}

// also satisfy the engine-level CertGossiper used by tryFinalizeBlock.
var _ CertGossiper = (*recordingGossiper)(nil)
var _ QuorumGossiper = (*certQuorumGossiper)(nil)

// certQuorumGossiper adapts recordingGossiper to QuorumGossiper (network-level)
// signature (chainID, networkID, blockID, bytes).
type certQuorumGossiper struct{ rec *recordingGossiper }

func (g *certQuorumGossiper) GossipPut(c, n ids.ID, b []byte) int { return g.rec.GossipPut(c, n, b) }
func (g *certQuorumGossiper) SendPullQuery(c, n, b ids.ID, v []ids.NodeID) int {
	return g.rec.SendPullQuery(c, n, b, v)
}
func (g *certQuorumGossiper) SendPushQuery(c, n ids.ID, b []byte, v []ids.NodeID) int {
	return g.rec.SendPushQuery(c, n, b, v)
}
func (g *certQuorumGossiper) SendVote(c ids.ID, to ids.NodeID, b ids.ID) error {
	return g.rec.SendVote(c, to, b)
}
func (g *certQuorumGossiper) BroadcastVote(c, n, b ids.ID, vb []byte) int {
	return g.rec.BroadcastVote(c, n, b, vb)
}
func (g *certQuorumGossiper) GossipCert(c, n, b ids.ID, cb []byte) int {
	_ = g.rec.GossipCert(c, b, cb)
	return 1
}

// newQuorumEngine builds a started multi-validator engine for validator index
// `self`, wired with the test validator set as verifier+signer and the given
// cert gossiper. K/alpha come from params.
func newQuorumEngine(t *testing.T, params config.Parameters, vs *testValidatorSet, self int, gossiper CertGossiper) (*Transitive, ids.ID) {
	t.Helper()
	return newQuorumEngineOpts(t, params, vs, self, gossiper)
}

// newQuorumEngineOpts builds the engine and applies any EXTRA options on top of the
// base quorum-cert wiring. The base harness deliberately does NOT wire a stake source —
// the finality tests rely on COUNT-α finality (cert.Verify), and stake-weighting
// (cert.VerifyWeighted) would change the threshold (⅔-of-stake) and break those tests.
// A test that exercises the value-DEX MODE gate (which requires a stake source) passes
// WithStakeWeighting explicitly, scoped to that test, so finality math stays count-α for
// everyone else.
func newQuorumEngineOpts(t *testing.T, params config.Parameters, vs *testValidatorSet, self int, gossiper CertGossiper, extra ...Option) (*Transitive, ids.ID) {
	t.Helper()
	chainID := ids.GenerateTestID()
	opts := append([]Option{
		WithQuorumCert(chainID, vs.nodeID(self), vs, gossiper, vs.signerFor(self)),
	}, extra...)
	e := NewWithConfig(Config{Params: params}, opts...)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	return e, chainID
}

// trackProposal inserts a verified own-proposal pending block into the engine
// (as buildBlocksLocked would) so a test can drive votes at it. Returns the
// position votes must bind to.
func trackProposal(e *Transitive, chainID ids.ID, blk *verifyOnceBlock, round uint32) VotePosition {
	cb := &Block{
		id:        blk.id,
		parentID:  blk.parentID,
		height:    blk.height,
		timestamp: blk.timestamp.Unix(),
		data:      blk.bytes,
	}
	_ = e.consensus.AddBlock(context.Background(), cb)
	_ = e.consensus.ProcessVote(context.Background(), blk.id, true)
	e.mu.Lock()
	pb := &PendingBlock{
		ConsensusBlock: cb,
		VMBlock:        blk,
		ProposedAt:     time.Now(),
		VoteCount:      1,
		Round:          round,
		Decided:        false,
		IsOwnProposal:  true,
	}
	e.pendingBlocks[blk.id] = pb
	// Mirror production buildBlocksLocked: record the proposer's OWN signed
	// accept vote so the assembled cert includes it.
	e.recordOwnVoteLocked(pb, blk.id)
	e.mu.Unlock()
	return VotePosition{
		ChainID:  chainID,
		Height:   blk.height,
		Round:    round,
		BlockID:  blk.id,
		ParentID: blk.parentID,
	}
}

// waitFor polls cond until true or the deadline elapses. Returns cond's final value.
func waitFor(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

// newTestBlock is a small constructor for verifyOnceBlock used across tests.
func newTestBlock(height uint64, parent ids.ID, tag string) *verifyOnceBlock {
	return &verifyOnceBlock{
		id:        ids.GenerateTestID(),
		parentID:  parent,
		height:    height,
		timestamp: time.Now(),
		bytes:     []byte(tag),
	}
}

// ensure block.Block is referenced (harness builds VM-less engines in some tests).
var _ = block.Block(nil)
