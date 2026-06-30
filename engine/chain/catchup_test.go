// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// catchup_convergence_test.go — proves the cert-carrying catch-up path:
//
//   - LIVENESS: a validator stranded at height N converges to the network tip
//     N+k by accepting fetched (block, cert) pairs through AcceptCatchupBlock,
//     WITHOUT re-voting (no live quorum exists for an already-finalized height).
//   - SAFETY: a forged / sub-quorum / below-α-floor cert delivered via catch-up
//     is REJECTED and finalizes nothing — the cert-gate holds through catch-up
//     exactly as it does on the live path. A node cannot be force-fed a chain.
//   - ORDERING: even a VALID cert applied out of parent order is refused by the
//     per-height guard (finality is contiguous), so the oldest-first invariant
//     is ENFORCED, not merely assumed.
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

// catchupVM is a FAITHFUL test VM: ParseBlock returns the SAME *verifyOnceBlock
// that was registered for those bytes, so block identity (ID/height/parent) and
// AcceptCalled tracking survive a round-trip through bytes — exactly as a real
// VM's deterministic codec does (unlike verifyOnceVM.ParseBlock, which discards
// identity). Unknown bytes parse to an error so AcceptCatchupBlock rejects them.
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

// newCatchupRuntime builds a STARTED stake-weighted multi-validator Runtime for
// validator `self`, wired with the test validator set (verifier + signer + stake),
// the faithful VM, and a recording gossiper (so a test can assert NO votes/certs
// are emitted during catch-up). It returns the runtime, its chainID, and the
// recorder. Stake-weighting is wired because the value (mainnet) chain that wedged
// is stake-weighted — catch-up must clear the SAME ⅔-of-stake predicate.
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

// catchupCertFor assembles a REAL finality cert for blk at the given chainID,
// signed by `voters` over blk's canonical position, asserting `threshold`. This is
// byte-identical to the cert an AHEAD node would have stored (CertForBlock) and
// gossiped at finalize time. Returns the marshaled cert bytes.
func catchupCertFor(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock, voters []int, threshold uint32) []byte {
	t.Helper()
	pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
	votes := make([]SignedVote, 0, len(voters))
	for _, i := range voters {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	qc, err := AssembleQuorumCert(pos, threshold, votes)
	if err != nil {
		t.Fatalf("assemble cert: %v", err)
	}
	b, err := qc.MarshalBinary()
	if err != nil {
		t.Fatalf("marshal cert: %v", err)
	}
	return b
}

// seedBehindAt strands the runtime at finalized height N with tip block `tip`,
// exactly as a node that fell behind the frontier (the incident: luxd-0 at
// 1082780). The gap blocks built afterward extend `tip`.
func seedBehindAt(t *testing.T, rt *Runtime, vm *catchupVM, tip *verifyOnceBlock) {
	t.Helper()
	vm.register(tip)
	// Establish CERTIFIED finality at N (a node that legitimately finalized up to N
	// in-process). Post-incident-1082814, SyncState is only a non-authoritative HINT —
	// it no longer seeds certified finality — so the certified baseline is set through
	// the real finalize fold (first-finalize seeds at tip.height). The distinct
	// restart-from-hint recovery is covered by the incident regression suite.
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

// -----------------------------------------------------------------------------
// LIVENESS — the stranded node converges to the tip THROUGH the cert path, with
// ZERO re-voting. This is the fix for the wedge: the network will not re-vote an
// already-finalized height, so the only way back is accepting the certs it
// already assembled.
// -----------------------------------------------------------------------------

func TestCatchup_BehindNodeConvergesViaCertPath(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, chainID, rec := newCatchupRuntime(t, vs, 0, vm)

	// Strand the node at N=1082780 (the incident height) and build the 17-block gap
	// up to the network tip N+17=1082797 (the incident delta).
	const N = uint64(1082780)
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

	// CONVERGENCE: the behind node advanced N → N+k purely through cert-accept.
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N+uint64(k) {
		t.Fatalf("convergence FAILED: finalized height %d, want %d (N=%d + k=%d)", fh, N+uint64(k), N, k)
	}

	// THE DISTINGUISHING ASSERTION: convergence happened WITHOUT re-voting. The
	// catch-up path never broadcasts a vote and never re-gossips a cert — it applies
	// FINISHED certs. (If catch-up had fallen back to the voting Put path, the node
	// would have broadcast votes for already-decided heights and still wedged.)
	rec.mu.Lock()
	votes, certs := len(rec.votes), len(rec.certs)
	rec.mu.Unlock()
	if votes != 0 {
		t.Fatalf("catch-up must NOT broadcast votes (re-voting an already-finalized height), got %d", votes)
	}
	if certs != 0 {
		t.Fatalf("catch-up must NOT re-gossip certs (it applies finished certs), got %d", certs)
	}
}

// -----------------------------------------------------------------------------
// SAFETY — a bad cert delivered via catch-up is REJECTED and finalizes nothing.
// The cert-gate (VerifyWeighted / α-floor) holds through the catch-up path with
// the SAME rigor as live finality. A node cannot be force-fed a bad chain.
// -----------------------------------------------------------------------------

func TestCatchup_RejectsForgedAndSubQuorumCerts(t *testing.T) {
	const N = uint64(1082780)

	// Each sub-case strands a FRESH node at N and tries to push block N+1 with a
	// DEFECTIVE cert. None may finalize. Fresh runtimes keep verifyOnceBlock.Verify
	// single-shot and isolate the per-height ledger.
	cases := []struct {
		name  string
		cert  func(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock) []byte
	}{
		{
			// FORGED signature: 4 voters (count ok, stake ok) but voter 0's slot
			// carries voter 1's signature → clause-6 (signature) fails → no cert.
			name: "forged-signature",
			cert: func(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock) []byte {
				pos := VotePosition{ChainID: chainID, Height: blk.height, Round: 0, BlockID: blk.id, ParentID: blk.parentID}
				votes := []SignedVote{
					{NodeID: vs.nodeID(0), Accept: true, Signature: vs.sign(1, pos)}, // 0 claims, 1 signed
					{NodeID: vs.nodeID(1), Accept: true, Signature: vs.sign(1, pos)},
					{NodeID: vs.nodeID(2), Accept: true, Signature: vs.sign(2, pos)},
					{NodeID: vs.nodeID(3), Accept: true, Signature: vs.sign(3, pos)},
				}
				qc, err := AssembleQuorumCert(pos, 3, votes)
				if err != nil {
					t.Fatalf("assemble forged: %v", err)
				}
				b, _ := qc.MarshalBinary()
				return b
			},
		},
		{
			// SUB-QUORUM by stake: 3 of 5 validators (count=3≥α=3 passes the COUNT
			// predicate) but 3/5 = 60% ≤ ⅔ → VerifyWeighted's strict supermajority
			// fails → no finality. This is the HIGH-3 headcount-vs-stake gap, enforced
			// through catch-up.
			name: "sub-quorum-stake",
			cert: func(t *testing.T, vs *testValidatorSet, chainID ids.ID, blk *verifyOnceBlock) []byte {
				return catchupCertFor(t, vs, chainID, blk, []int{0, 1, 2}, 3)
			},
		},
		{
			// BELOW α-FLOOR: a cert that asserts a LOWER threshold (1) than the chain's
			// α (3). HandleIncomingCert rejects it at the MinThreshold floor even though
			// its 4 signatures verify — sub-quorum finality-forgery defence.
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
				t.Fatalf("SAFETY VIOLATION: a %s cert was accepted via catch-up", tc.name)
			}
			if rt.IsAccepted(blk.id) {
				t.Fatalf("SAFETY VIOLATION: %s cert finalized block N+1 (IsAccepted)", tc.name)
			}
			if got := blk.AcceptCalled(); got != 0 {
				t.Fatalf("SAFETY VIOLATION: %s cert ran VM.Accept %d×", tc.name, got)
			}
			if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N {
				t.Fatalf("%s: finalized height moved off N=%d to %d on a bad cert", tc.name, N, fh)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ORDERING — even a VALID cert applied OUT OF PARENT ORDER is refused. The
// per-height guard requires height == finalizedHeight+1 AND parent == finalizedTip,
// so the oldest-first invariant is ENFORCED by the engine, not merely assumed by
// the transport. After the gap is filled in order, the same block finalizes.
// -----------------------------------------------------------------------------

func TestCatchup_OutOfOrderRefusedThenInOrderConverges(t *testing.T) {
	vs := newTestValidatorSet(5)
	vm := newCatchupVM()
	rt, chainID, _ := newCatchupRuntime(t, vs, 0, vm)

	const N = uint64(1082780)
	tip := newTestBlock(N, ids.Empty, "tip@N")
	seedBehindAt(t, rt, vm, tip)
	gap := buildGap(vm, tip, 2) // N+1, N+2 (kept pristine for step 2)

	// (1) Try to skip ahead: a DISTINCT block at height N+2 (parent = the real N+1)
	// with a perfectly VALID 4-of-5 cert, applied while still finalized at N. The
	// per-height guard refuses it (height N+2 != finalizedHeight+1 == N+1) — a valid
	// cert does NOT license a non-contiguous finalize.
	ooo := newTestBlock(N+2, gap[0].id, "ooo@N+2")
	vm.register(ooo)
	certOoo := catchupCertFor(t, vs, chainID, ooo, []int{0, 1, 2, 3}, 3)
	if err := rt.AcceptCatchupBlock(context.Background(), ooo.bytes, certOoo); err == nil {
		t.Fatal("ORDERING VIOLATION: a height-N+2 block accepted while finalized at N (contiguity guard bypassed)")
	}
	if rt.IsAccepted(ooo.id) {
		t.Fatal("ORDERING VIOLATION: out-of-order block finalized")
	}
	if fh, _ := rt.Transitive.consensus.GetFinalizedHeight(); fh != N {
		t.Fatalf("out-of-order accept moved finalized height off N=%d to %d", N, fh)
	}

	// (2) Now apply N+1 then N+2 IN ORDER → both finalize. The earlier refusal was
	// the contiguity guard, not a stuck path.
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
// SERVE — the ahead side. A node that finalized a block retains and SERVES its
// cert (CertForBlock), and the served bytes are exactly what a behind node needs
// to finalize the same block. Closes the loop: store-on-finalize ⇄ serve.
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

	// After finalize: the node serves a cert that decodes+verifies to the SAME
	// finality witness — a peer can finalize on it with zero trust in this node.
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
