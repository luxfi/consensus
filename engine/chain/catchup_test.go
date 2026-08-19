// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// catchup_test.go — the cert-carrying catch-up path:
//
//   - Liveness: a validator stranded at height N converges to the network tip N+k by
//     accepting fetched (block, cert) pairs through AcceptCatchupBlock, without
//     re-voting — no live quorum exists for an already-finalized height.
//   - Safety: a forged, sub-quorum or below-α-floor cert delivered via catch-up is
//     rejected and finalizes nothing. The cert-gate holds through catch-up exactly as
//     it does on the live path, so a node cannot be force-fed a chain.
//   - Ordering: even a valid cert applied out of parent order is refused by the
//     per-height guard, since finality is contiguous. Oldest-first is enforced by the
//     engine rather than assumed of the transport.
package chain

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// catchupVM is a faithful test VM: ParseBlock returns the same *verifyOnceBlock that
// was registered for those bytes, so block identity (ID/height/parent) and AcceptCalled
// tracking survive a round-trip through bytes, as a real VM's deterministic codec does
// and unlike verifyOnceVM.ParseBlock, which discards identity. Unknown bytes parse to
// an error so AcceptCatchupBlock rejects them.
type catchupVM struct {
	mu      sync.Mutex
	byBytes map[string]*verifyOnceBlock
	byID    map[ids.ID]*verifyOnceBlock
	lastAcc ids.ID
}

func newCatchupVM() *catchupVM {
	return &catchupVM{
		byBytes: map[string]*verifyOnceBlock{},
		byID:    map[ids.ID]*verifyOnceBlock{},
	}
}

func (m *catchupVM) register(blk *verifyOnceBlock) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.byBytes[string(blk.bytes)] = blk
	m.byID[blk.id] = blk
}

func (m *catchupVM) BuildBlock(context.Context) (block.Block, error) {
	return nil, errVerifiedAlready // a behind node never builds during catch-up
}

func (m *catchupVM) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.byID[id]; ok {
		return b, nil
	}
	return nil, errVerifiedAlready
}

func (m *catchupVM) ParseBlock(_ context.Context, bytes []byte) (block.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.byBytes[string(bytes)]; ok {
		return b, nil
	}
	return nil, errVerifiedAlready
}

func (m *catchupVM) LastAccepted(context.Context) (ids.ID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastAcc, nil
}

func (m *catchupVM) SetPreference(_ context.Context, id ids.ID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastAcc = id
	return nil
}

var _ BlockBuilder = (*catchupVM)(nil)

// newCatchupRuntime builds a started stake-weighted multi-validator Runtime for
// validator `self`, wired with the test validator set (verifier + signer + stake), the
// faithful VM, and a recording gossiper so a test can assert that no votes or certs are
// emitted during catch-up. It returns the runtime, its chainID, and the recorder. Stake
// weighting is wired because catch-up must clear the same ⅔-of-stake predicate that
// live finality does; a headcount quorum is not enough.
func newCatchupRuntime(t *testing.T, vs *testValidatorSet, self int, vm BlockBuilder) (*Runtime, ids.ID, *recordingGossiper) {
	t.Helper()
	chainID := ids.GenerateTestID()
	rec := &recordingGossiper{}
	rt := NewRuntime(NetworkConfig{
		ChainID:      chainID,
		NetworkID:    ids.GenerateTestID(),
		NodeID:       vs.nodeID(self),
		Logger:       log.Noop(),
		Params:       ptrParams(params5()), // K=5, α=3
		VoteVerifier: vs,
		VoteSigner:   vs.signerFor(self),
		StakeSource:  vs, // equal unit weights → α-of-K count is also a ⅔-stake quorum
		Gossiper:     &certQuorumGossiper{rec: rec},
		VM:           vm,
	})
	if err := rt.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = rt.Stop(context.Background()) })
	return rt, chainID, rec
}

// catchupCertFor assembles a real finality cert for blk at the given chainID, signed by
// `voters` over blk's canonical position, asserting `threshold`. It is byte-identical
// to the cert a node ahead of us would have stored (CertForBlock) and gossiped at
// finalize time. Returns the marshaled cert bytes.
func catchupCertFor(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock, voters []int, threshold uint32) []byte {
	t.Helper()
	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
	votes := make([]SignedVote, 0, len(voters))
	for _, i := range voters {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	qc, err := AssembleQuorumCert(pos, Quasar, threshold, votes)
	if err != nil {
		t.Fatalf("assemble cert: %v", err)
	}
	b, err := qc.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal cert: %v", err)
	}
	return b
}

// seedBehindAt strands the runtime at finalized height N with tip block `tip`, as a
// node that fell behind the frontier would be. The gap blocks built afterward extend
// `tip`.
func seedBehindAt(t *testing.T, rt *Runtime, vm *catchupVM, tip *verifyOnceBlock) {
	t.Helper()
	vm.register(tip)
	// Establish certified finality at N: a node that legitimately finalized up to N
	// in-process. SyncState is only a non-authoritative hint and does not seed certified
	// finality, so the certified baseline has to come through the real finalize fold,
	// whose first finalize seeds at tip.height.
	if _, err := rt.Transitive.consensus.FinalizeBranch(tip.id, tip.height, ids.Empty); err != nil {
		t.Fatalf("seed behind at height %d: %v", tip.height, err)
	}
	if fh, set := rt.Transitive.consensus.GetFinalizedHeight(); !set || fh != tip.height {
		t.Fatalf("precondition: behind node must be finalized at N=%d, got (%d,%v)", tip.height, fh, set)
	}
}

// buildGap returns k blocks N+1..N+k chained on `tip`, registered in vm. Each
// block's bytes are unique (keyed on height) so the VM registry never collides.
func buildGap(vm *catchupVM, tip *verifyOnceBlock, k int) []*verifyOnceBlock {
	gap := make([]*verifyOnceBlock, 0, k)
	parent := tip
	for i := 1; i <= k; i++ {
		h := tip.height + uint64(i)
		blk := newTestBlock(h, parent.id, fmt.Sprintf("gap@%d", h))
		vm.register(blk)
		gap = append(gap, blk)
		parent = blk
	}
	return gap
}

func TestVerifyCatchupCertificate_ReadOnlyCryptographicFrontierGate(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)
	blk := newTestBlock(42, ids.GenerateTestID(), "certified-frontier")
	vm.register(blk)
	cert := catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2, 3}, 3)

	if err := rt.VerifyCatchupCertificate(context.Background(), blk.bytes, cert); err != nil {
		t.Fatalf("valid 4-of-5 certified frontier rejected: %v", err)
	}
	if blk.VerifyCalls() != 0 || blk.AcceptCalled() != 0 || rt.IsAccepted(blk.id) {
		t.Fatalf("frontier proof check mutated execution/finality: verify=%d accept=%d accepted=%v",
			blk.VerifyCalls(), blk.AcceptCalled(), rt.IsAccepted(blk.id))
	}

	forged := append([]byte(nil), cert...)
	forged[len(forged)-1] ^= 0xff
	if err := rt.VerifyCatchupCertificate(context.Background(), blk.bytes, forged); err == nil {
		t.Fatal("forged frontier certificate verified")
	}
	if blk.VerifyCalls() != 0 || blk.AcceptCalled() != 0 || rt.IsAccepted(blk.id) {
		t.Fatal("rejected frontier proof mutated execution/finality")
	}
}

// -----------------------------------------------------------------------------
// Liveness — a stranded node converges to the tip through the cert path, with no
// re-voting. The network will not re-vote an already-finalized height, so the only
// way back for a node left behind is applying the certs the network already
// assembled.
// -----------------------------------------------------------------------------

func TestCatchup_BehindNodeConvergesViaCertPath(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, chainID, rec := newCatchupRuntime(t, vs, 0, vm)

	// Strand the node at N, deep in a long chain, and build a k-block gap up to the
	// network tip N+k.
	const N = uint64(1_000_000)
	const k = 17
	tip := newTestBlock(N, ids.Empty, "tip@N")
	seedBehindAt(t, rt, vm, tip)
	gap := buildGap(vm, tip, k)

	// Feed each (block, cert) oldest-first, as the node-side catch-up transport
	// delivers fetched ancestors. Each cert is a genuine 4-of-5 (≥⅔ stake) witness.
	for i, blk := range gap {
		cert := catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2, 3}, 3)
		if err := rt.AcceptCatchupBlock(context.Background(), blk.bytes, cert); err != nil {
			t.Fatalf("gap[%d] (height %d) cert-accept failed: %v", i, blk.height, err)
		}
		if !rt.IsAccepted(blk.id) {
			t.Fatalf("gap[%d] (height %d) not finalized via cert path", i, blk.height)
		}
		if got := blk.AcceptCalled(); got != 1 {
			t.Fatalf("gap[%d] must VM.Accept exactly once, got %d", i, got)
		}
	}

	// Convergence: the behind node advanced N → N+k purely through cert-accept.
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N+uint64(k) {
		t.Fatalf("convergence failed: finalized height %d, want %d (N=%d + k=%d)", fh, N+uint64(k), N, k)
	}

	// The distinguishing assertion: convergence happened without re-voting. The
	// catch-up path never broadcasts a vote and never re-gossips a cert — it applies
	// finished ones. Falling back to the voting Put path would broadcast votes for
	// already-decided heights, which no peer will answer, leaving the node stuck.
	rec.mu.Lock()
	votes, certs := len(rec.votes), len(rec.certs)
	rec.mu.Unlock()
	if votes != 0 {
		t.Fatalf("catch-up must not broadcast votes (re-voting an already-finalized height), got %d", votes)
	}
	if certs != 0 {
		t.Fatalf("catch-up must not re-gossip certs (it applies finished certs), got %d", certs)
	}
}

// -----------------------------------------------------------------------------
// Safety — a bad cert delivered via catch-up is rejected and finalizes nothing. The
// cert-gate (VerifyWeighted and the α-floor) holds through the catch-up path with
// the same rigor as live finality, so a node cannot be force-fed a bad chain.
// -----------------------------------------------------------------------------

func TestCatchup_RejectsForgedAndSubQuorumCerts(t *testing.T) {
	const N = uint64(1_000_000)

	// Each sub-case strands a fresh node at N and tries to push block N+1 with a
	// defective cert. None may finalize. Fresh runtimes keep verifyOnceBlock.Verify
	// single-shot and isolate the per-height ledger.
	cases := []struct {
		name string
		cert func(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock) []byte
	}{
		{
			// Forged signature: 4 voters (count ok, stake ok) but voter 0's slot
			// carries voter 1's signature, so the signature clause fails and no cert
			// is formed.
			name: "forged-signature",
			cert: func(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock) []byte {
				pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
				votes := []SignedVote{
					{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(1, pos)}, // 0 claims, 1 signed
					{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
					{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
					{NodeID: vs.nodeID(3), Accept: true, Signature: vs.sign(3, pos)},
				}
				qc, err := AssembleQuorumCert(pos, Quasar, 3, votes)
				if err != nil {
					t.Fatalf("assemble forged: %v", err)
				}
				b, _ := qc.MarshalBinary()
				return b
			},
		},
		{
			// Sub-quorum by stake: 3 of 5 validators clears the count predicate
			// (count=3 ≥ α=3) but 3/5 = 60% ≤ ⅔, so VerifyWeighted's strict
			// supermajority fails and nothing finalizes. This is the gap between a
			// headcount quorum and a stake quorum, enforced through catch-up.
			name: "sub-quorum-stake",
			cert: func(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock) []byte {
				return catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2}, 3)
			},
		},
		{
			// Below the α-floor: a cert asserting a lower threshold (1) than the
			// chain's α (3). HandleIncomingCert rejects it at the MinThreshold floor
			// even though its 4 signatures verify, so a cert cannot buy finality by
			// naming a weaker quorum than the chain runs.
			name: "below-alpha-floor",
			cert: func(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock) []byte {
				return catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2, 3}, 1)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vs := newTestValidatorSet(5)
			vm := newCatchupVM()
			rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)
			tip := newTestBlock(N, ids.Empty, "tip@N")
			seedBehindAt(t, rt, vm, tip)

			blk := newTestBlock(N+1, tip.id, "forced@N+1")
			vm.register(blk)
			bad := tc.cert(t, vs, chainID, blk)

			err := rt.AcceptCatchupBlock(context.Background(), blk.bytes, bad)
			if err == nil {
				t.Fatalf("a %s cert was accepted via catch-up", tc.name)
			}
			if rt.IsAccepted(blk.id) {
				t.Fatalf("%s cert finalized block N+1 (IsAccepted)", tc.name)
			}
			if got := blk.AcceptCalled(); got != 0 {
				t.Fatalf("%s cert ran VM.Accept %d×", tc.name, got)
			}
			if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N {
				t.Fatalf("%s: finalized height moved off N=%d to %d on a bad cert", tc.name, N, fh)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Ordering — even a valid cert applied out of parent order is refused. The
// per-height guard requires height == finalizedHeight+1 and parent == finalizedTip,
// so the oldest-first invariant is enforced by the engine rather than assumed of the
// transport. After the gap is filled in order, the same block finalizes.
// -----------------------------------------------------------------------------

func TestCatchup_OutOfOrderRefusedThenInOrderConverges(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1_000_000)
	tip := newTestBlock(N, ids.Empty, "tip@N")
	seedBehindAt(t, rt, vm, tip)
	gap := buildGap(vm, tip, 2) // N+1, N+2 (kept pristine for step 2)

	// Try to skip ahead: a distinct block at height N+2 (parent = the real N+1) with a
	// perfectly valid 4-of-5 cert, applied while still finalized at N. The per-height
	// guard refuses it (height N+2 != finalizedHeight+1 == N+1) — a valid cert does not
	// license a non-contiguous finalize.
	ooo := newTestBlock(N+2, gap[0].id, "ooo@N+2")
	vm.register(ooo)
	certOoo := catchupCertFor(t, vs, chainID, ooo, []int{0, 1, 2, 3}, 3)
	if err := rt.AcceptCatchupBlock(context.Background(), ooo.bytes, certOoo); err == nil {
		t.Fatal("a height-N+2 block was accepted while finalized at N, bypassing the contiguity guard")
	}
	if rt.IsAccepted(ooo.id) {
		t.Fatal("out-of-order block finalized")
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N {
		t.Fatalf("out-of-order accept moved finalized height off N=%d to %d", N, fh)
	}

	// Now apply N+1 then N+2 in order and both finalize. The earlier refusal was the
	// contiguity guard, not a stuck path.
	for i, blk := range gap {
		cert := catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2, 3}, 3)
		if err := rt.AcceptCatchupBlock(context.Background(), blk.bytes, cert); err != nil {
			t.Fatalf("in-order gap[%d] (height %d) cert-accept failed: %v", i, blk.height, err)
		}
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N+2 {
		t.Fatalf("did not converge to N+2 after in-order apply, got %d", fh)
	}
}

// -----------------------------------------------------------------------------
// Serve — the ahead side. A node that finalized a block retains and serves its cert
// (CertForBlock), and the served bytes are exactly what a node behind it needs to
// finalize the same block. That closes the loop: store-on-finalize ⇄ serve.
// -----------------------------------------------------------------------------

func TestCatchup_CertForBlockServesWhatWasFinalized(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(100)
	tip := newTestBlock(N, ids.Empty, "tip@N")
	seedBehindAt(t, rt, vm, tip)
	blk := buildGap(vm, tip, 1)[0] // N+1

	// Before finalize: nothing to serve.
	if _, ok := rt.CertForBlock(blk.id); ok {
		t.Fatal("CertForBlock returned a cert for an unfinalized block")
	}

	cert := catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2, 3}, 3)
	if err := rt.AcceptCatchupBlock(context.Background(), blk.bytes, cert); err != nil {
		t.Fatalf("finalize N+1: %v", err)
	}

	// After finalize: the node serves a cert that decodes and verifies to the same
	// finality witness, so a peer can finalize on it with zero trust in this node.
	served, ok := rt.CertForBlock(blk.id)
	if !ok {
		t.Fatal("CertForBlock did not serve the finalized block's cert")
	}
	qc, err := UnmarshalQuorumCert(served)
	if err != nil {
		t.Fatalf("served cert does not decode: %v", err)
	}
	if qc.Position.BlockID != blk.id || qc.Position.Height != blk.height {
		t.Fatalf("served cert binds the wrong position: %+v", qc.Position)
	}
	if err := qc.VerifyWeighted(vs, vs, 0); err != nil {
		t.Fatalf("served cert does not clear the ⅔-stake predicate: %v", err)
	}
}
