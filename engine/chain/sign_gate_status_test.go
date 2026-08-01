// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// sign_gate_status_test.go — teeth for the sign-gate introspection (SignGateStatus /
// ExplainHeight). Every assertion here is the answer to a question a live investigation
// (devnet + testnet, 2026-07-31) could not get out of a running node: is a guard wired, where
// does it write, is that file on disk, did the durable write succeed, and is THIS height
// bound or floored. The introspection is read-only — the tests also prove it changes nothing.
package chain

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// capturingLogger records Info lines and delegates the rest of the ~35-method log.Logger
// surface to the noop logger by embedding it — so only the one method under test is written.
type capturingLogger struct {
	log.Logger
	mu   sync.Mutex
	info []string
}

func newCapturingLogger() *capturingLogger { return &capturingLogger{Logger: log.Noop()} }

func (c *capturingLogger) Info(msg string, ctx ...interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Sprintln, not Sprint: Sprint omits the separator between two string operands, which would
	// glue every key to its value ("guardConfiguredtrue") and make the assertions meaningless.
	c.info = append(c.info, strings.TrimSpace(fmt.Sprintln(append([]interface{}{msg}, ctx...)...)))
}

// linesContaining returns the recorded Info lines mentioning substr.
func (c *capturingLogger) linesContaining(substr string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []string
	for _, l := range c.info {
		if strings.Contains(l, substr) {
			out = append(out, l)
		}
	}
	return out
}

// TestSignGateStatus_GuardConfigured proves the field that was read backwards in production:
// GuardConfigured is FALSE with no store and TRUE with one, and the implementation type +
// path + on-disk state are reported so "I looked in /data and saw no file" can never again be
// mistaken for "the guard is unwired". A fresh signer that has not yet bound a height reports
// GuardFileExists=false — OpenVoteGuard creates NO file, the first committed Persist does.
func TestSignGateStatus_GuardConfigured(t *testing.T) {
	vs := newTestValidatorSet(5)

	// --- no guard: memory-only.
	noGuard, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{})
	st := noGuard.SignGateStatus()
	if st.GuardConfigured {
		t.Fatalf("GuardConfigured must be false with no VoteGuardStore wired, got %+v", st)
	}
	if st.GuardImplementation != "" || st.GuardPath != "" || st.GuardFileExists {
		t.Fatalf("an unwired guard must report no implementation, no path and no file: %+v", st)
	}

	// --- the production file store.
	path := filepath.Join(t.TempDir(), "vote-guard")
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store))
	st = e.SignGateStatus()
	if !st.GuardConfigured {
		t.Fatalf("GuardConfigured must be true once a VoteGuardStore is wired, got %+v", st)
	}
	if st.GuardImplementation != "*chain.fileVoteGuard" {
		t.Fatalf("GuardImplementation must name the concrete store (the only way to tell the real "+
			"durable file store from a memory stub), got %q", st.GuardImplementation)
	}
	if st.GuardPath != path {
		t.Fatalf("GuardPath must report where the store actually writes: got %q want %q", st.GuardPath, path)
	}
	if st.GuardFileExists {
		t.Fatal("GuardFileExists must be false before the first binding — OpenVoteGuard creates no " +
			"file, so 'no file' here means 'nothing was ever signed', NOT 'no guard'")
	}

	// --- one real binding through the production sign gate ⇒ the file appears.
	if !e.reserveSlotForSign(9, ids.GenerateTestID()) {
		t.Fatal("reserveSlotForSign must permit the first binding at an unbound height above the floor")
	}
	if st = e.SignGateStatus(); !st.GuardFileExists {
		t.Fatalf("GuardFileExists must be true after a committed Persist — the fsync'd rename put the "+
			"file at %s: %+v", path, st)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("control: the guard file must really be on disk at the reported path: %v", err)
	}
}

// TestSignGateStatus_PersistCounters proves the counters move on BOTH outcomes of the durable
// write. A failing store is FAIL-CLOSED: it silently costs this node every vote it would ever
// cast, forever. PersistFailures is the only aggregate signal of that fault, and
// PersistAttempts==0 on a running signer is the DIFFERENT fault — the sign path was never
// reached at all, which is exactly what the stuck height showed fleet-wide.
func TestSignGateStatus_PersistCounters(t *testing.T) {
	vs := newTestValidatorSet(5)

	// --- a healthy store: attempt + success, no failure.
	store, err := OpenVoteGuard(filepath.Join(t.TempDir(), "vote-guard"))
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	ok, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store))
	if st := ok.SignGateStatus(); st.PersistAttempts != 0 || st.PersistSuccesses != 0 || st.PersistFailures != 0 {
		t.Fatalf("no durable write has happened yet — all counters must be zero: %+v", st)
	}
	if !ok.reserveSlotForSign(11, ids.GenerateTestID()) {
		t.Fatal("reserveSlotForSign must permit the first binding at height 11")
	}
	st := ok.SignGateStatus()
	if st.PersistAttempts != 1 || st.PersistSuccesses != 1 || st.PersistFailures != 0 {
		t.Fatalf("a successful bind must count exactly one attempt and one success: %+v", st)
	}
	if st.BindingCount != 1 {
		t.Fatalf("BindingCount must report the LIVE committedSlot size (1 after one bind): %+v", st)
	}
	// The idempotent re-solicit of the SAME canonical does NOT write — the binding is already
	// durable — so the counters must not move. (Proves the seam counts WRITES, not calls.)
	bound, isBound := ok.committedCanonical(11)
	if !isBound {
		t.Fatal("precondition: height 11 must be bound")
	}
	if !ok.reserveSlotForSign(11, bound) {
		t.Fatal("the SAME canonical must be admitted idempotently")
	}
	if st = ok.SignGateStatus(); st.PersistAttempts != 1 {
		t.Fatalf("an idempotent re-solicit performs no durable write — attempts must stay 1: %+v", st)
	}

	// --- a store whose durable write always fails: attempt + failure, no success, and the
	// signature is refused (fail-closed, unchanged behaviour).
	bad, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(failingGuard{}))
	if bad.reserveSlotForSign(11, ids.GenerateTestID()) {
		t.Fatal("a failed durable write must still REFUSE the signature (fail-closed) — the " +
			"observability must not have changed the decision")
	}
	st = bad.SignGateStatus()
	if st.PersistAttempts != 1 || st.PersistFailures != 1 || st.PersistSuccesses != 0 {
		t.Fatalf("a failing guard must count exactly one attempt and one failure: %+v", st)
	}
	if st.BindingCount != 0 {
		t.Fatalf("the refused binding must have been rolled back — BindingCount must be 0: %+v", st)
	}
}

// TestSignGateStatus_LoadedBootState proves the two OPEN-TIME values survive as the boot
// answer even after later writes move the live ones. LoadedFinalizedThrough is not
// re-derivable from the store: every Persist advances FinalizedThrough(), so without capturing
// it at open, "what did this node BOOT with" — the first question of any equivocation
// post-mortem — is unanswerable.
func TestSignGateStatus_LoadedBootState(t *testing.T) {
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")

	// --- run 1: bind two heights, then certify through 5 so the store carries a floor.
	store1, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store1): %v", err)
	}
	e1, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store1))
	if st := e1.SignGateStatus(); st.LoadedBindingCount != 0 || st.LoadedFinalizedThrough != 0 {
		t.Fatalf("a fresh store must report a genesis boot state (0 bindings, floor 0): %+v", st)
	}
	for _, h := range []uint64{6, 7} {
		if !e1.reserveSlotForSign(h, ids.GenerateTestID()) {
			t.Fatalf("bind at height %d must be permitted", h)
		}
	}
	if err := e1.compactVoteGuardThroughQuasar(5); err != nil {
		t.Fatalf("compactVoteGuardThroughQuasar(5): %v", err)
	}
	// The LOADED values are open-time facts and must NOT have moved with the live ones.
	st := e1.SignGateStatus()
	if st.LoadedFinalizedThrough != 0 || st.LoadedBindingCount != 0 {
		t.Fatalf("the loaded boot state must stay the OPEN-TIME values after later writes: %+v", st)
	}
	if st.GuardFloor != 5 {
		t.Fatalf("GuardFloor must track the live decidedFloor (5 after certifying): %+v", st)
	}

	// --- run 2 on the SAME durable file: the boot state is now what run 1 left behind.
	store2, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store2): %v", err)
	}
	e2, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store2))
	st = e2.SignGateStatus()
	if st.LoadedFinalizedThrough != 5 {
		t.Fatalf("LoadedFinalizedThrough must be the floor recovered at open (5), got %d: %+v",
			st.LoadedFinalizedThrough, st)
	}
	if st.LoadedBindingCount != 2 {
		t.Fatalf("LoadedBindingCount must be the number of bindings recovered at open (6 and 7 "+
			"survive a floor of 5 — the prune is STRICTLY below), got %d: %+v", st.LoadedBindingCount, st)
	}
	if st.BindingCount != st.LoadedBindingCount {
		t.Fatalf("before any new bind the live count must equal the loaded count: %+v", st)
	}
	if !st.GuardFileExists {
		t.Fatalf("the guard file written by run 1 must be reported present on reopen: %+v", st)
	}
}

// TestExplainHeight_Binding proves the per-height explain reports IsBound=false before a bind
// and the EXACT bound canonical after — the difference between "this node has not voted here
// yet" and "this node is welded to a candidate the fleet may have abandoned", which is the
// stall shape that produced zero signed votes at one height while other heights signed fine.
func TestExplainHeight_Binding(t *testing.T) {
	vs := newTestValidatorSet(5)
	store, err := OpenVoteGuard(filepath.Join(t.TempDir(), "vote-guard"))
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store))

	const H = uint64(31)
	ex := e.ExplainHeight(H)
	if ex.Height != H {
		t.Fatalf("ExplainHeight must echo the height asked about: %+v", ex)
	}
	if ex.IsBound || ex.Bound != ids.Empty {
		t.Fatalf("an unbound height must report IsBound=false and Bound=Empty: %+v", ex)
	}

	canonical := ids.GenerateTestID()
	if !e.reserveSlotForSign(H, canonical) {
		t.Fatalf("reserveSlotForSign must permit the first binding at height %d", H)
	}
	ex = e.ExplainHeight(H)
	if !ex.IsBound {
		t.Fatalf("after a bind ExplainHeight must report IsBound=true: %+v", ex)
	}
	if ex.Bound != canonical {
		t.Fatalf("Bound must be the EXACT canonical committed: got %s want %s", ex.Bound, canonical)
	}
	// A neighbouring height must be unaffected — the slot is per-height, not per-node.
	if other := e.ExplainHeight(H + 1); other.IsBound {
		t.Fatalf("binding height %d must not bind height %d: %+v", H, H+1, other)
	}
	// Read-only: explaining must not have created a binding anywhere.
	if st := e.SignGateStatus(); st.BindingCount != 1 || st.PersistAttempts != 1 {
		t.Fatalf("ExplainHeight must be side-effect free — exactly one binding and one write "+
			"should exist: %+v", st)
	}
}

// TestExplainHeight_CertifiedFloor proves BelowCertifiedFloor tracks reserveSlotForSign's floor
// predicate EXACTLY (height <= floor, at-or-below) on both sides of the floor, and folds BOTH
// durable sources — the guard floor and the consensus Quasar export frontier — the same way the
// gate does. Getting this wrong in either direction is a fault: too low and a certified height
// looks signable, too high and a live height looks permanently closed.
func TestExplainHeight_CertifiedFloor(t *testing.T) {
	vs := newTestValidatorSet(5)
	store, err := OpenVoteGuard(filepath.Join(t.TempDir(), "vote-guard"))
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	e, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store))

	// --- floor 0: nothing is closed yet (height 0 itself is at the floor).
	if ex := e.ExplainHeight(1); ex.BelowCertifiedFloor || ex.CertifiedFloor != 0 {
		t.Fatalf("with no certified floor, height 1 must be signable: %+v", ex)
	}

	// --- advance the GUARD floor to 20 (a ⅔-stake certified height).
	const F = uint64(20)
	if err := e.compactVoteGuardThroughQuasar(F); err != nil {
		t.Fatalf("compactVoteGuardThroughQuasar(%d): %v", F, err)
	}
	for _, h := range []uint64{F - 1, F} {
		ex := e.ExplainHeight(h)
		if !ex.BelowCertifiedFloor {
			t.Fatalf("height %d is AT OR BELOW the certified floor %d and must report closed "+
				"(the gate refuses at <=, not <): %+v", h, F, ex)
		}
		if ex.CertifiedFloor != F {
			t.Fatalf("CertifiedFloor must be %d, got %d: %+v", F, ex.CertifiedFloor, ex)
		}
	}
	if ex := e.ExplainHeight(F + 1); ex.BelowCertifiedFloor {
		t.Fatalf("height %d is ABOVE the certified floor %d and must remain signable — over-refusal "+
			"here is the permanent-halt liveness fault: %+v", F+1, F, ex)
	}
	// Cross-check against the gate itself: the explain must agree with the decision.
	if e.reserveSlotForSign(F, ids.GenerateTestID()) {
		t.Fatal("control: reserveSlotForSign must refuse AT the certified floor — the explain and " +
			"the gate must not disagree")
	}

	// --- the consensus Quasar export frontier is the OTHER durable source; the enforced floor
	// is the max of the two, and SignGateStatus reports all three.
	const Q = uint64(44)
	e.consensus.SyncQuasarFrontier(ids.GenerateTestID(), Q)
	ex := e.ExplainHeight(Q)
	if !ex.BelowCertifiedFloor || ex.CertifiedFloor != Q {
		t.Fatalf("the export frontier %d must raise the enforced floor: %+v", Q, ex)
	}
	if ex := e.ExplainHeight(Q + 1); ex.BelowCertifiedFloor {
		t.Fatalf("height %d is above BOTH floors and must stay signable: %+v", Q+1, ex)
	}
	st := e.SignGateStatus()
	if st.GuardFloor != F || st.QuasarFloor != Q || st.CertifiedFloor != Q {
		t.Fatalf("SignGateStatus must report both floor sources and the enforced max "+
			"(guard=%d quasar=%d enforced=%d): %+v", F, Q, Q, st)
	}
}

// TestSignGateBootState_LoggedOnce proves Start states the sign gate's durable memory
// UNCONDITIONALLY and exactly once. Before this, the only boot-time evidence was a warning
// that fires solely in the memoryless case — so a healthy boot said nothing, and that warning's
// own precondition (a guard IS wired) was read backwards in production as proof the guard was
// missing. The positive statement is what makes "is the guard wired, and what did it recover"
// answerable from a log tail instead of a code read.
func TestSignGateBootState_LoggedOnce(t *testing.T) {
	vs := newTestValidatorSet(5)
	path := filepath.Join(t.TempDir(), "vote-guard")

	// --- run 1 leaves a binding at height 8 and a certified floor of 3 on disk.
	store1, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store1): %v", err)
	}
	seed, _ := newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{}, WithVoteGuard(store1))
	if !seed.reserveSlotForSign(8, ids.GenerateTestID()) {
		t.Fatal("bind at height 8 must be permitted")
	}
	if err := seed.compactVoteGuardThroughQuasar(3); err != nil {
		t.Fatalf("compactVoteGuardThroughQuasar(3): %v", err)
	}

	// --- run 2 boots on that file with a capturing logger.
	store2, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard(store2): %v", err)
	}
	cl := newCapturingLogger()
	newQuorumEngineOpts(t, params5Prod(), vs, 0, &recordingGossiper{},
		WithVoteGuard(store2), WithLogger(cl))

	lines := cl.linesContaining("sign-gate boot state")
	if len(lines) != 1 {
		t.Fatalf("Start must state the sign-gate boot condition EXACTLY once, got %d line(s): %v",
			len(lines), lines)
	}
	boot := lines[0]
	for _, want := range []string{
		"guardConfigured true",     // the field that was read backwards in production
		"*chain.fileVoteGuard",     // the real durable store, not a memory stub
		path,                       // WHERE it writes — the wrong-path trap
		"guardFileExists true",     // the file run 1 left really is on disk
		"loadedBindings 1",         // height 8 survived the floor-3 prune
		"loadedFinalizedThrough 3", // the open-time floor, not the live one
	} {
		if !strings.Contains(boot, want) {
			t.Fatalf("the boot statement must carry %q — without it the operator is back to "+
				"guessing at a filesystem path; got: %s", want, boot)
		}
	}
}
