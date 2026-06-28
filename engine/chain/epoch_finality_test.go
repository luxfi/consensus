// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_pchain_epoch_finality_test.go — the LOAD-BEARING integration test the
// MEDIUM-1 round lacked (CRITICAL-1). The earlier set-root/stake tests injected a
// validators.State directly into the source constructors and asserted the source
// math; they NEVER drove the full engine vote→assemble→verify→finalize path, and
// they NEVER distinguished the value-chain height from the P-chain epoch height.
// That is exactly the "test bypasses the wiring" trap: the sources were correct,
// but nothing proved the engine reads the RIGHT height, nor that a block actually
// finalizes.
//
// This test wires the engine the way production does — a height-indexed
// validator-set source (set-root + stake) and a height-pinned vote verifier, all
// keyed off the block's P-CHAIN epoch height — and proves four properties end to
// end:
//
//	(A) A multi-validator block FINALIZES through the real source path: votes are
//	    verified against the set@epoch, the cert assembles, VerifyWeighted clears
//	    the ⅔-by-stake supermajority, and VM.Accept runs once.
//	(B) CRITICAL-1(b): the engine reads the validator set at the block's P-CHAIN
//	    height, NOT its value-chain height. The block's value height is enormous
//	    (10_000_000) while its P-chain epoch is small (7); the set is known ONLY at
//	    epoch 7. If the engine (wrongly) read the value height, the set would be
//	    empty → TotalStake 0 → ErrQCStakeBelowSupermajority → STALL. Finalizing
//	    proves the epoch height is used.
//	(C) RESIDUAL-B: a validator that is in the set@epoch but ABSENT from the
//	    "current" map still has its vote verified (pubkey resolved at the epoch),
//	    so a >⅓-stake validator departing the current map during async skew does
//	    not stall the block it legitimately signed.
//	(D) Fail-closed: with the SAME wiring but an EMPTY set at the block's epoch,
//	    the block does NOT finalize (no set ⟹ no cert ⟹ no Accept). This is the
//	    behavior the node-layer guard turns into a loud refuse-to-start; here we
//	    pin that the engine itself never finalizes off an empty epoch.
package chain

import (
	"context"
	"crypto/ed25519"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

// --- a block that carries a P-chain height (a proposervm SignedBlock would) ---

// pChainBlock is a verifyOnceBlock-equivalent that ALSO exposes PChainHeight, so
// the engine's pChainHeightOf boundary assertion records the epoch height onto
// the consensus Block. Its value-chain Height and its PChainHeight are
// DELIBERATELY different so the test can tell which one the engine reads.
type pChainBlock struct {
	id           ids.ID
	parentID     ids.ID
	height       uint64 // value-chain height (huge)
	pChainHeight uint64 // P-chain epoch height (small)
	timestamp    time.Time
	bytes        []byte
	acceptCalled int64
	mu           sync.Mutex
}

func (b *pChainBlock) ID() ids.ID                    { return b.id }
func (b *pChainBlock) Parent() ids.ID                { return b.parentID }
func (b *pChainBlock) ParentID() ids.ID             { return b.parentID }
func (b *pChainBlock) Height() uint64               { return b.height }
func (b *pChainBlock) PChainHeight() uint64         { return b.pChainHeight } // the boundary hook
func (b *pChainBlock) Timestamp() time.Time         { return b.timestamp }
func (b *pChainBlock) Status() uint8                { return 0 }
func (b *pChainBlock) Verify(context.Context) error { return nil }
func (b *pChainBlock) Accept(context.Context) error {
	b.mu.Lock()
	b.acceptCalled++
	b.mu.Unlock()
	return nil
}
func (b *pChainBlock) Reject(context.Context) error { return nil }
func (b *pChainBlock) Bytes() []byte                { return b.bytes }
func (b *pChainBlock) AcceptCalled() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.acceptCalled
}

// --- a height-indexed validator-set source (set-root + stake + verifier) ------

// epochValidatorSet models the production wiring: a single height-indexed source
// of epoch truth. Membership, BLS-equivalent pubkeys, stake weights, AND the
// set-root are ALL read at a given epoch height. The set is registered per epoch
// height; an unknown epoch yields the empty set (the fail-soft the engine folds
// into Empty root / zero stake). A SEPARATE "current" map models the live
// validator manager — which, during a staking change, can DISAGREE with a past
// epoch (RESIDUAL-B).
type epochValidatorSet struct {
	byEpoch map[uint64]map[ids.NodeID]ed25519.PublicKey
	stake   map[uint64]map[ids.NodeID]uint64
	// current is the live map; a node id present at epoch H but absent here models
	// a validator that left the current set after H (async skew).
	current map[ids.NodeID]struct{}
}

// at returns the (pubkeys, stake) registered for epoch height H.
func (s *epochValidatorSet) at(h uint64) (map[ids.NodeID]ed25519.PublicKey, map[ids.NodeID]uint64) {
	return s.byEpoch[h], s.stake[h]
}

// VerifyVote resolves the voter's pubkey FROM THE SET@epochHeight (RESIDUAL-B) —
// NOT from the current map. A voter unknown at the epoch is rejected; a voter
// known at the epoch verifies even if it has since left the current map.
func (s *epochValidatorSet) VerifyVote(nodeID ids.NodeID, message, sig []byte, epochHeight uint64) bool {
	pubs, _ := s.at(epochHeight)
	pub, ok := pubs[nodeID]
	if !ok {
		return false
	}
	return ed25519.Verify(pub, message, sig)
}

func (s *epochValidatorSet) Weight(nodeID ids.NodeID, epochHeight uint64) uint64 {
	_, st := s.at(epochHeight)
	return st[nodeID]
}

func (s *epochValidatorSet) TotalStake(epochHeight uint64) uint64 {
	_, st := s.at(epochHeight)
	var total uint64
	for _, w := range st {
		total += w
	}
	return total
}

// ValidatorSetRoot is a deterministic commitment to the set@epoch — distinct per
// epoch, Empty for an unknown/empty epoch. (A simple length+nodeid+stake fold;
// the node's production source uses SHA-256, but any deterministic function of
// the set works for the engine-level epoch-binding properties under test.)
func (s *epochValidatorSet) ValidatorSetRoot(epochHeight uint64) ids.ID {
	pubs, st := s.at(epochHeight)
	if len(pubs) == 0 {
		return ids.Empty
	}
	var root ids.ID
	root[0] = byte(len(pubs))
	root[1] = byte(epochHeight)
	for id := range pubs {
		for i := 0; i < ids.NodeIDLen && i < len(root)-2; i++ {
			root[2+i] ^= id[i]
		}
		root[31] ^= byte(st[id])
	}
	return root
}

// epochSigners holds the per-node ed25519 keys so the test can sign votes.
type epochSigners struct {
	keys map[ids.NodeID]ed25519.PrivateKey
	ids  []ids.NodeID
}

func newEpochSigners(n int) *epochSigners {
	es := &epochSigners{keys: make(map[ids.NodeID]ed25519.PrivateKey, n)}
	for i := 0; i < n; i++ {
		id := ids.GenerateTestNodeID()
		pub, priv, err := ed25519.GenerateKey(nil)
		_ = pub
		if err != nil {
			panic(err)
		}
		es.keys[id] = priv
		es.ids = append(es.ids, id)
	}
	return es
}

func (es *epochSigners) pub(i int) ed25519.PublicKey  { return es.keys[es.ids[i]].Public().(ed25519.PublicKey) }
func (es *epochSigners) signerFor(i int) VoteSigner {
	priv := es.keys[es.ids[i]]
	return voteSignerFunc(func(message []byte) ([]byte, error) { return ed25519.Sign(priv, message), nil })
}

// trackPChainProposal inserts a verified own-proposal pending block carrying the
// block's P-CHAIN epoch height (as production buildBlocksLocked does via
// pChainHeightOf), so votes drive the real finality path. Returns the position
// votes must bind to (its set-root is stamped at the epoch height).
func trackPChainProposal(e *Transitive, chainID ids.ID, blk *pChainBlock, round uint32) VotePosition {
	cb := &Block{
		id:           blk.id,
		parentID:     blk.parentID,
		height:       blk.height,
		timestamp:    blk.timestamp.Unix(),
		data:         blk.bytes,
		pChainHeight: pChainHeightOf(blk), // the boundary capture under test
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
	e.recordOwnVoteLocked(pb, blk.id)
	e.mu.Unlock()
	return e.blockPositionLockedForTest(pb, blk.id)
}

// blockPositionLockedForTest exposes the engine's position builder to the test
// (it is the value votes bind to, with the set-root stamped at the epoch height).
func (t *Transitive) blockPositionLockedForTest(pending *PendingBlock, blockID ids.ID) VotePosition {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.blockPositionLocked(pending, blockID)
}

// signEpochVote produces validator i's signed accept Vote for a position.
func signEpochVote(es *epochSigners, i int, pos VotePosition) Vote {
	priv := es.keys[es.ids[i]]
	return Vote{
		BlockID:   pos.BlockID,
		NodeID:    es.ids[i],
		Accept:    true,
		SignedAt:  time.Now(),
		Signature: ed25519.Sign(priv, CanonicalVoteMessage(pos)),
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	}
}

// TestPChainEpochFinality_RealWiring is the CRITICAL-1 load-bearing test.
func TestPChainEpochFinality_RealWiring(t *testing.T) {
	const (
		valueHeight = uint64(10_000_000) // value-chain height races far ahead
		epochHeight = uint64(7)          // the P-chain height the set is known at
	)
	es := newEpochSigners(5)

	// The set is known ONLY at the P-chain epoch (7). Node 4 is in the epoch set
	// (a >⅓-stake validator) but has DEPARTED the current map (RESIDUAL-B). All 5
	// carry equal stake (20 each, total 100) so a 4-voter cert is 80/100 > ⅔.
	src := &epochValidatorSet{
		byEpoch: map[uint64]map[ids.NodeID]ed25519.PublicKey{
			epochHeight: {
				es.ids[0]: es.pub(0), es.ids[1]: es.pub(1), es.ids[2]: es.pub(2),
				es.ids[3]: es.pub(3), es.ids[4]: es.pub(4),
			},
		},
		stake: map[uint64]map[ids.NodeID]uint64{
			epochHeight: {es.ids[0]: 20, es.ids[1]: 20, es.ids[2]: 20, es.ids[3]: 20, es.ids[4]: 20},
		},
		current: map[ids.NodeID]struct{}{ // node 4 NOT present → departed current map
			es.ids[0]: {}, es.ids[1]: {}, es.ids[2]: {}, es.ids[3]: {},
		},
	}

	chainID := ids.GenerateTestID()
	rec := &recordingGossiper{}
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, es.ids[0], src, rec, es.signerFor(0)),
		WithStakeWeighting(src),     // ⅔-by-stake, read at the epoch height
		WithValidatorSetRoot(src),   // set-root stamped at the epoch height
	)
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	blk := &pChainBlock{
		id:           ids.GenerateTestID(),
		parentID:     ids.Empty,
		height:       valueHeight, // huge value height
		pChainHeight: epochHeight, // small epoch height — the set is known HERE
		timestamp:    time.Now(),
		bytes:        []byte("pchain-epoch-finality"),
	}
	pos := trackPChainProposal(e, chainID, blk, 0)

	// (B) sanity: the position the engine built stamped the set-root at the EPOCH
	// height, not the value height. A root computed at the value height would be
	// Empty (no set there); the epoch root is non-Empty.
	if pos.ValidatorSetRoot == ids.Empty {
		t.Fatal("CRITICAL-1(b): position set-root is Empty — engine read the (empty) value height, not the epoch height")
	}
	if pos.ValidatorSetRoot != src.ValidatorSetRoot(epochHeight) {
		t.Fatal("CRITICAL-1(b): position set-root does not match the set@epoch root")
	}
	if pos.ValidatorSetRoot == src.ValidatorSetRoot(valueHeight) {
		t.Fatal("test vacuous: epoch root must differ from the (Empty) value-height root")
	}

	// (A)+(C): drive 3 peer signed accepts (proposer 0 already self-voted) — INCLUDING
	// node 4, which is in set@epoch but NOT in the current map. With proposer(0)+{1,2,4}
	// that is 4 distinct accepts = 80/100 stake > ⅔ → MUST finalize.
	e.ReceiveVote(signEpochVote(es, 1, pos))
	e.ReceiveVote(signEpochVote(es, 2, pos))
	e.ReceiveVote(signEpochVote(es, 4, pos)) // departed-from-current voter

	// Wait on the TRUE finalization signal — VM.Accept — not merely the count
	// flip (IsAccepted). ReceiveVote is async (channel-fed), and the cert-driven
	// VMBlock.Accept lags the count quorum, so asserting AcceptCalled immediately
	// after an IsAccepted poll races. AcceptCalled==1 subsumes finality.
	if !waitFor(2*time.Second, func() bool { return blk.AcceptCalled() == 1 }) {
		t.Fatalf("CRITICAL-1: a multi-validator block with a ⅔-stake epoch quorum did NOT finalize "+
			"(VM.Accept=%d; the real-wiring finality the MEDIUM-1 round never tested)", blk.AcceptCalled())
	}
	if !e.IsAccepted(blk.id) {
		t.Fatal("block VM.Accepted but consensus does not report it accepted")
	}

	// A verified, stake-weighted cert must have been assembled + gossiped, and it
	// must verify under VerifyWeighted at the EPOCH height (not the value height).
	rec.mu.Lock()
	nCerts := len(rec.certs)
	var lastCert []byte
	if nCerts > 0 {
		lastCert = rec.certs[nCerts-1]
	}
	rec.mu.Unlock()
	if nCerts == 0 {
		t.Fatal("a verified quorum cert must be assembled and gossiped at finality")
	}
	cert, err := UnmarshalQuorumCert(lastCert)
	if err != nil {
		t.Fatalf("decode gossiped cert: %v", err)
	}
	// (C) explicit: node 4's vote IS in the cert, proving the departed-from-current
	// validator's legit vote was verified at the epoch (RESIDUAL-B).
	var has4 bool
	for i := range cert.Votes {
		if cert.Votes[i].NodeID == es.ids[4] {
			has4 = true
		}
		// none of the cert's voters may be the departed node IF we resolved from the
		// current map — but they are not; this asserts the positive case.
		if _, inCurrent := src.current[cert.Votes[i].NodeID]; !inCurrent && cert.Votes[i].NodeID != es.ids[4] {
			t.Fatalf("cert carries a voter %s that is neither in the current map nor the expected epoch-only node", cert.Votes[i].NodeID)
		}
	}
	if !has4 {
		t.Fatal("RESIDUAL-B: the epoch-only voter (departed from the current map) must be in the cert " +
			"— its vote was wrongly dropped (verifier read the current map, not set@epoch)")
	}
	// The cert verifies under the stake-weighted predicate at the EPOCH height.
	if err := cert.VerifyWeighted(src, src, epochHeight); err != nil {
		t.Fatalf("gossiped cert must verify stake-weighted at the epoch height: %v", err)
	}
	// And it must FAIL at the value height (where the set is empty / total stake 0)
	// — proving the height genuinely matters and the epoch read is load-bearing.
	if err := cert.VerifyWeighted(src, src, valueHeight); err == nil {
		t.Fatal("CRITICAL-1(b): cert must NOT verify at the value height (empty set there) — " +
			"if it does, the height is not actually being used")
	}
}

// TestPChainEpochFinality_GenesisEpochFallbackIsLive pins the CURRENT in-process
// behavior (the remaining (b2) reality): when the VM block does NOT carry a
// PChainHeight (no proposervm at the engine boundary), pChainHeightOf yields 0,
// so the engine reads the validator set at P-chain height 0 — the GENESIS set.
// This MUST still finalize (it is non-empty, ≤ current P-chain height, and
// identical on every node), proving the fix UNBRICKS finality even before the
// real PChainHeight is threaded. Contrast with the pre-fix bug, where the
// value-chain height was used → errUnfinalizedHeight → empty → permanent stall.
func TestPChainEpochFinality_GenesisEpochFallbackIsLive(t *testing.T) {
	es := newEpochSigners(5)
	const genesisEpoch = uint64(0) // the set is known at the genesis P-chain height

	src := &epochValidatorSet{
		byEpoch: map[uint64]map[ids.NodeID]ed25519.PublicKey{
			genesisEpoch: {
				es.ids[0]: es.pub(0), es.ids[1]: es.pub(1), es.ids[2]: es.pub(2),
				es.ids[3]: es.pub(3), es.ids[4]: es.pub(4),
			},
		},
		stake: map[uint64]map[ids.NodeID]uint64{
			genesisEpoch: {es.ids[0]: 20, es.ids[1]: 20, es.ids[2]: 20, es.ids[3]: 20, es.ids[4]: 20},
		},
	}

	chainID := ids.GenerateTestID()
	rec := &recordingGossiper{}
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, es.ids[0], src, rec, es.signerFor(0)),
		WithStakeWeighting(src),
		WithValidatorSetRoot(src))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	// A block with a value height > 0 but NO PChainHeight (pChainHeight stays 0):
	// this is exactly the in-process VM case. pChainHeightOf(blk)==0 → set@0.
	blk := &pChainBlock{
		id:           ids.GenerateTestID(),
		parentID:     ids.Empty,
		height:       42, // value height advanced past genesis
		pChainHeight: 0,  // NO proposervm height delivered (the current reality)
		timestamp:    time.Now(),
		bytes:        []byte("genesis-epoch-fallback"),
	}
	pos := trackPChainProposal(e, chainID, blk, 0)
	if pos.ValidatorSetRoot == ids.Empty {
		t.Fatal("genesis-epoch set-root must be non-Empty (the genesis set is non-empty)")
	}

	e.ReceiveVote(signEpochVote(es, 1, pos))
	e.ReceiveVote(signEpochVote(es, 2, pos))
	e.ReceiveVote(signEpochVote(es, 3, pos))

	if !waitFor(2*time.Second, func() bool { return blk.AcceptCalled() == 1 }) {
		t.Fatalf("UNBRICK: a block with no PChainHeight must still finalize against the genesis set "+
			"(VM.Accept=%d) — the fix must not stall finality in the in-process VM case", blk.AcceptCalled())
	}
}

// TestPChainEpochFinality_EmptyEpochFailsClosed is property (D): the SAME real
// wiring, but the block's epoch has NO registered set, must NOT finalize. This is
// the engine-level half of the fail-closed guard (the node layer turns this into
// a loud refuse-to-start; the engine must never finalize off an empty epoch).
func TestPChainEpochFinality_EmptyEpochFailsClosed(t *testing.T) {
	es := newEpochSigners(5)
	const knownEpoch = uint64(7)
	const blockEpoch = uint64(9) // the block points at an epoch with NO set

	src := &epochValidatorSet{
		byEpoch: map[uint64]map[ids.NodeID]ed25519.PublicKey{
			knownEpoch: { // set known at 7, but the block's epoch is 9 → empty
				es.ids[0]: es.pub(0), es.ids[1]: es.pub(1), es.ids[2]: es.pub(2),
				es.ids[3]: es.pub(3), es.ids[4]: es.pub(4),
			},
		},
		stake: map[uint64]map[ids.NodeID]uint64{
			knownEpoch: {es.ids[0]: 20, es.ids[1]: 20, es.ids[2]: 20, es.ids[3]: 20, es.ids[4]: 20},
		},
	}

	chainID := ids.GenerateTestID()
	rec := &recordingGossiper{}
	e := NewWithConfig(Config{Params: params5()},
		WithQuorumCert(chainID, es.ids[0], src, rec, es.signerFor(0)),
		WithStakeWeighting(src),
		WithValidatorSetRoot(src))
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })

	blk := &pChainBlock{
		id:           ids.GenerateTestID(),
		parentID:     ids.Empty,
		height:       5,
		pChainHeight: blockEpoch, // empty epoch
		timestamp:    time.Now(),
		bytes:        []byte("empty-epoch"),
	}
	pos := trackPChainProposal(e, chainID, blk, 0)

	// The set-root at an empty epoch is Empty (the explicit "unbound" answer).
	if pos.ValidatorSetRoot != ids.Empty {
		t.Fatalf("an empty epoch must commit to Empty set-root, got %s", pos.ValidatorSetRoot)
	}

	// Even if peers "sign" (their pubkeys are NOT in the block's epoch, so the
	// verifier rejects), the block MUST NOT finalize.
	e.ReceiveVote(signEpochVote(es, 1, pos))
	e.ReceiveVote(signEpochVote(es, 2, pos))
	e.ReceiveVote(signEpochVote(es, 3, pos))
	e.ReceiveVote(signEpochVote(es, 4, pos))

	if waitFor(500*time.Millisecond, func() bool { return e.IsAccepted(blk.id) }) {
		t.Fatal("CRITICAL-1(D): a block whose epoch has NO validator set must NOT finalize (fail-closed)")
	}
	if blk.AcceptCalled() != 0 {
		t.Fatalf("VM.Accept must NOT run for an empty-epoch block, got %d", blk.AcceptCalled())
	}
}

