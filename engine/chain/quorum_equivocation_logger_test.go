// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_equivocation_logger_test.go — regression for the equivocation-handler
// fleet-wide liveness kill (RED CRITICAL-2).
//
// The bug: reportCertEquivocation logged the conflicting-cert event with
// Logger.Crit. In luxfi/log, Crit → Fatal → os.Exit(1). The per-height guard had
// ALREADY preserved safety (the second, conflicting cert is REJECTED — no double
// Accept, no fork), so the Crit converted a CORRECTLY-HANDLED safety event into a
// self-DoS: every honest node that merely OBSERVES the gossiped conflicting cert
// would os.Exit, taking the whole fleet down at once. The slashing-evidence loop
// right after the Crit was dead code (the process had already exited).
//
// Why the existing two-cert tests (TestCriticalFork_TwoCertsOneHeightAcrossRounds,
// TestConsensus_EquivocatingProposerCannotFinalizeBothForks) did NOT catch it:
// they construct the Runtime with log.Noop(), whose IsZero()==true makes
// reportCertEquivocation SKIP the log call entirely — so the os.Exit path was
// never exercised. This test drives the IDENTICAL two-cert-at-one-height scenario
// through a REAL logger (log.NewWriter → IsZero()==false) so the (formerly fatal)
// log call actually runs, and isolates it in a SUBPROCESS so an os.Exit(1) is
// observable as a non-zero child exit code rather than silently killing the test
// binary. With the fix (Crit→Error) the child runs the scenario to completion:
// the second cert is rejected, evidence is recorded, the survival sentinel is
// printed, and the child exits 0.
package chain

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/luxfi/consensus/core/slashing"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// equivSurvivedSentinel is printed by the child ONLY after it has survived the
// equivocation and asserted the safety reject + evidence. The parent asserts on
// it to prove the scenario actually ran to completion (not that the child merely
// happened to exit 0 for some unrelated reason).
const equivSurvivedSentinel = "EQUIV_SURVIVED_NO_FLEET_EXIT"

// subprocessEnvKey gates the child body so the re-exec does not recurse.
const subprocessEnvKey = "LUX_EQUIV_NOEXIT_SUBPROCESS"

// TestEquivocation_RealLogger_NoFleetWideExit proves a second, conflicting
// finality cert at an already-finalized height is REJECTED + slashed but does NOT
// terminate the process, EVEN under a real (non-Noop) logger whose Crit path is
// os.Exit(1). This is the load-bearing regression for RED CRITICAL-2.
func TestEquivocation_RealLogger_NoFleetWideExit(t *testing.T) {
	if os.Getenv(subprocessEnvKey) == "1" {
		// Child: run the real-logger two-cert scenario. If the equivocation
		// handler still exits the process, this never returns and the child exits
		// non-zero; if fixed, it returns and the test binary exits 0.
		runTwoCertEquivocationRealLogger(t)
		return
	}

	// Parent: re-exec THIS test in a subprocess with the gate set.
	cmd := exec.Command(os.Args[0], "-test.run=^TestEquivocation_RealLogger_NoFleetWideExit$", "-test.v")
	cmd.Env = append(os.Environ(), subprocessEnvKey+"=1")
	out, err := cmd.CombinedOutput()

	// A non-zero child exit is PRECISELY the regression: the equivocation log path
	// terminated the process (Crit→Fatal→os.Exit(1)) instead of logging+slashing.
	if err != nil {
		t.Fatalf("CRITICAL-2 REGRESSION: the two-cert-at-one-height scenario terminated the process "+
			"under a real logger (equivocation became a fleet-wide self-DoS instead of a logged + "+
			"slashed reject): %v\n--- child output ---\n%s", err, out)
	}
	// The child exited 0 — confirm it actually ran the scenario to the end (the
	// reject + evidence assertions live in the child; the sentinel proves they
	// were reached).
	if !bytes.Contains(out, []byte(equivSurvivedSentinel)) {
		t.Fatalf("subprocess exited 0 but did not run the equivocation scenario to completion "+
			"(survival sentinel %q absent):\n%s", equivSurvivedSentinel, out)
	}
}

// runTwoCertEquivocationRealLogger is the CHILD body. It is the SAME
// two-cert-at-one-height scenario as TestCriticalFork_TwoCertsOneHeightAcrossRounds
// (quorum_height_guard_test.go) — reusing the same 4-validator harness — but the
// Runtime is wired with a REAL logger so reportCertEquivocation's log call is
// actually executed. If that call is still Crit, the process exits inside
// HandleIncomingCert(certB) and never reaches the assertions below.
func runTwoCertEquivocationRealLogger(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()
	db := slashing.NewDB(time.Hour)

	follower := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(3), vs, &recordingGossiper{}, vs.signerFor(3)),
		WithSlashing(slashing.NewDetector(64, 0.5), db))
	if err := follower.Start(context.Background(), true); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = follower.Stop(context.Background()) })

	// REAL logger (NOT log.Noop()). IsZero()==false, so reportCertEquivocation's
	// log call runs for real — exercising the path that used to os.Exit(1).
	// io.Discard keeps stderr quiet while still constructing a non-zero logger.
	realLogger := log.NewWriter(io.Discard)
	if realLogger.IsZero() {
		t.Fatal("test invariant broken: logger must be non-zero to exercise the equivocation log path")
	}
	rt := &Runtime{Transitive: follower, config: NetworkConfig{ChainID: chainID, Logger: realLogger}}

	// Two conflicting blocks at the SAME height 1 (genesis parent), distinct IDs.
	blkA := newTestBlock(1, ids.Empty, "height1-A")
	blkB := newTestBlock(1, ids.Empty, "height1-B")
	trackVerifiedBlock(rt, blkA, 0)
	trackVerifiedBlock(rt, blkB, 7) // attacker-chosen non-zero round

	// Two valid 3-of-4 certs for the conflicting blocks (different rounds).
	certA := buildCertAtRound(t, vs, chainID, blkA.id, ids.Empty, 1, 0, 3)
	certB := buildCertAtRound(t, vs, chainID, blkB.id, ids.Empty, 1, 7, 3)

	// First cert finalizes A.
	if !rt.HandleIncomingCert(certA) {
		t.Fatal("first cert must finalize block A")
	}

	// The conflicting SECOND cert. Under the OLD Crit this call NEVER RETURNS — the
	// process exits inside reportCertEquivocation, before printing the sentinel,
	// and the parent observes a non-zero child exit. Under the fix it returns false
	// (rejected) and execution proceeds to the assertions below.
	if rt.HandleIncomingCert(certB) {
		t.Fatal("CRITICAL FORK: second cert at an already-finalized height was accepted")
	}
	if follower.IsAccepted(blkB.id) {
		t.Fatal("CRITICAL FORK: block B finalized at an already-finalized height")
	}
	if blkB.AcceptCalled() != 0 {
		t.Fatalf("conflicting block B must never be VM.Accepted, got %d calls", blkB.AcceptCalled())
	}
	// Exactly one block finalized at height 1, and it is A.
	if fin, ok := follower.consensus.FinalizedBlockAtHeight(1); !ok || fin != blkA.id {
		t.Fatalf("height 1 must remain finalized to A, got (%s, ok=%v)", fin, ok)
	}
	// Evidence must ACTUALLY be recorded now. Under the old Crit the process exited
	// before this loop ran — RED's "slashing-evidence loop is dead code". The fix
	// makes the slashing path reachable, so the Byzantine voters are recorded.
	if len(db.GetAllRecords()) == 0 {
		t.Fatal("equivocation evidence must be recorded for the conflicting cert's voters")
	}

	// Reaching here proves the process SURVIVED a correctly-handled safety event
	// under a real logger. Emit the sentinel the parent asserts on. Write straight
	// to stdout so it survives regardless of test-output buffering.
	if _, err := os.Stdout.WriteString(equivSurvivedSentinel + "\n"); err != nil {
		t.Fatalf("failed to emit survival sentinel: %v", err)
	}
}
