// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package bootstrap drives INITIAL SYNC for a linear (Quasar/Nova) chain: an empty
// or behind node fetches the chain from a peer's accepted frontier and re-executes
// it to the network tip, WITHOUT voting and WITHOUT requiring a stored α-of-K cert.
//
// This is a port of avalanche's proven snowman bootstrapper, adapted to the Lux
// engine's split architecture (the consensus engine owns the ACCEPT primitive —
// chain.Runtime.AcceptBootstrapBlock — and the node owns the TRANSPORT). The loop is
// the same:
//
//  1. discover the network frontier (a sampled peer's accepted tip — BlockSource.FrontierTip);
//  2. fetch the missing ancestry (BlockSource.Ancestors walks DOWN from the frontier in
//     bounded batches, descending until the segment connects to our last-accepted);
//  3. EXECUTE the fetched blocks OLDEST-FIRST (Chain.AcceptBootstrapBlock re-executes
//     each against the already-accepted parent state and finalizes on frontier-trust);
//  4. repeat until our last-accepted reaches the frontier — then bootstrap is DONE and
//     the node hands off to live consensus (the caller flips the engine to the
//     cert-gated path).
//
// Trust model (see chain/bootstrap_accept.go for the full argument): the node trusts
// the BEACON/VALIDATOR set it samples for the frontier (the weak-subjectivity anchor),
// RE-EXECUTES every fetched block (so a bad block is rejected), and accepts only the
// contiguous next block (so the chain cannot be gapped or forked). No quorum is joined
// because the network will not re-vote an already-finalized height.
package bootstrap

import (
	"context"
	"errors"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

const (
	// defaultMaxBlocksPerFetch is the batch size for one Ancestors round — matches the
	// node's GetContext served-window (maxContextBlocks). The descent fetches this many
	// blocks per round walking down from the frontier.
	defaultMaxBlocksPerFetch = 256

	// defaultMaxBuffer bounds the in-memory descent buffer: the most blocks held
	// between "fetched from frontier" and "executed up to here" in ONE sync pass. A
	// gap larger than this is NOT a runtime catch-up case — the node state-syncs to
	// within the window first, then this finishes the tail. 64Ki blocks is a generous
	// catch-up window while keeping the transient buffer bounded.
	defaultMaxBuffer = 64 * 1024

	// defaultMaxBufferBytes bounds the descent buffer by BYTES as well as by block
	// count (M3). A block count alone does not bound memory: 64Ki large blocks could
	// still OOM the node. A peer that streams oversized blocks during the descent now
	// hits this budget and the pass fails ErrGapTooLarge (state-sync first) instead of
	// exhausting memory. 1 GiB is generous for a transient catch-up window.
	defaultMaxBufferBytes = 1 << 30

	// defaultRetryInterval is the pause after a round that made NO progress (a peer
	// did not serve, or none is ahead yet) before re-sampling — so a dead/withholding
	// peer is retried against a fresh sample, never hot-looped.
	defaultRetryInterval = time.Second

	// maxDescentRounds bounds the descent per sync pass (defence against a peer that
	// serves blocks that never connect). MaxBuffer also bounds it; this caps the
	// request count independently.
	maxDescentRounds = 4096

	// maxStallRounds is how many consecutive no-progress rounds the driver tolerates
	// before giving up (the peer set is unreachable / not serving). The caller
	// (monitorBootstrap) then surfaces a real bootstrap failure rather than masking it.
	maxStallRounds = 60

	// defaultConnectDeadline bounds how long the loop WAITS for the beacon set to become
	// reachable (enough connected stake to FORM a ⅔ quorum) before it gives up and fails
	// SAFE (ErrBeaconsUnreachable). A freshly-booted node whose beacons have not finished
	// the peer handshake is NOT caught up — it is not yet able to ASK the network for the
	// frontier, so concluding "caught up" at the local (stale) height would be the
	// false-complete the canary hit. This is GENEROUS (a healthy node connects to its
	// beacon set in seconds) yet strictly LESS than monitorBootstrap's no-progress
	// watchdog (5 min), so the loop returns a clean named error before the watchdog
	// force-stops the engine.
	defaultConnectDeadline = 3 * time.Minute

	// defaultMaxNoQuorumRounds bounds how many CONSECUTIVE FrontierNoQuorum rounds Run RETRIES
	// before terminally failing SAFE with ErrNoBeaconQuorum (F2). On a LIVE chain (1–2 s blocks)
	// the beacon-named frontier is a MOVING TARGET: a single GetAcceptedFrontier round at the
	// instant connectivity first crosses the ⅔ floor can catch honest beacons momentarily SPLIT
	// across ADJACENT finalized tips (finality-propagation skew) — a transient no-quorum that one
	// more poll clears. Retrying this many rounds lets a transient split CONVERGE; a PERSISTENT
	// no-quorum (genuine eclipse / ≥⅓ Byzantine beacon stake) still fails safe after the bound.
	// 10 rounds × (the node's ~3 s frontier window + ~1 s pause) ≈ 40 s — a sane fail-safe window,
	// far below monitorBootstrap's 5-min no-progress watchdog, yet ample for a transient (which
	// clears in a round or two) to settle.
	defaultMaxNoQuorumRounds = 10
)

var (
	// ErrGapTooLarge is returned when the descent buffer would exceed MaxBuffer before
	// connecting to the local tip — the node is too far behind for in-memory block
	// bootstrap and must state-sync to within the window first.
	ErrGapTooLarge = errors.New("bootstrap: gap exceeds the in-memory window — state-sync to within the window first")

	// ErrStalled is returned when no peer serves usable blocks for maxStallRounds in a
	// row, so the sync cannot progress. Surfaces a real failure (not a silent stop).
	ErrStalled = errors.New("bootstrap: stalled — no peer served the missing ancestry")

	// ErrCannotConnect is returned when a descent exhausts its round budget without the
	// fetched segment reaching the local tip (a peer serving a disjoint chain).
	ErrCannotConnect = errors.New("bootstrap: fetched ancestry never connected to the local tip")

	// ErrFrontierNotDescendedFromCheckpoint is returned when an operator pinned a
	// weak-subjectivity checkpoint (a recent finalized block id + height) and the
	// α-agreed frontier's ancestry does NOT pass through it — the frontier is on a
	// chain that does not descend from the trusted checkpoint, so it is refused. This
	// is the defense-in-depth anchor for the empty-genesis case (where there is no
	// local history for the contiguity guard to bind the first block to).
	ErrFrontierNotDescendedFromCheckpoint = errors.New("bootstrap: α-agreed frontier does not descend from the weak-subjectivity checkpoint")

	// ErrBeaconsUnreachable is returned when a beacon set IS configured but not enough of
	// its stake ever connects (within the connect deadline) to FORM a ⅔ quorum — the node
	// cannot even ASK the network for the frontier. This is fail-SAFE: the node stays
	// un-bootstrapped (it does NOT false-complete at its local stale height) and the caller
	// surfaces a health failure for the operator to fix peering / state-sync / restart. It
	// is the bounded outcome of the canary boot race (FrontierTip ran before any beacon
	// handshaked) when connectivity never recovers.
	ErrBeaconsUnreachable = errors.New("bootstrap: beacon set never reached the connectivity needed to form a ⅔ quorum within the connect deadline — eclipsed/partitioned, failing safe (NOT caught up)")

	// ErrNoBeaconQuorum is returned when enough beacon stake IS connected to ask, but no
	// ⅔-by-stake supermajority names a single frontier (a genuine eclipse / partition /
	// disagreement above the ⅓ stall threshold). Like ErrBeaconsUnreachable this is
	// fail-SAFE: the node does NOT conclude caught-up at its stale height.
	ErrNoBeaconQuorum = errors.New("bootstrap: beacons connected but no ⅔-by-stake quorum named a frontier — eclipsed/partitioned, failing safe (NOT caught up)")
)

// FrontierStatus decomplects the THREE distinct reasons a FrontierTip query may NOT yield
// a named tip — which the loop MUST treat differently. Collapsing them into a single
// "ok=false" (the pre-fix bug) made a freshly-booted node conclude "caught up" at its local
// stale height the instant it asked the frontier BEFORE any beacon had connected: ok=false
// meant "no beacon quorum reachable", but the loop read it as "nothing ahead, done". These
// statuses keep "I cannot ask yet" (wait), "I asked and no one is ahead" (done), and "I
// asked and the network disagrees / is unreachable" (fail safe) as the distinct values they
// are.
type FrontierStatus int

const (
	// FrontierInvalid is the ZERO value — a deliberately MEANINGLESS status (F4). Making the
	// zero value non-permissive is defense-in-depth: an uninitialized or out-of-range status can
	// never be mistaken for "FrontierNamed" or "caught up". syncOnce's switch maps it (via the
	// fail-safe default) to a bounded-retry-then-fail-safe, so a future bug that returns a zero
	// status can NEVER false-complete a node at its stale height. It is never returned deliberately.
	FrontierInvalid FrontierStatus = iota

	// FrontierNamed: a ⅔-by-stake beacon quorum agreed on the returned tip — sync to it. This is
	// the ONLY status the tip is meaningful in, and the ONLY one that proceeds to descend+execute.
	FrontierNamed

	// FrontierNoBeacons: NO beacon set is configured (single-node / dev / --skip-bootstrap).
	// There is no network frontier to reach, so the node is genuinely caught up — COMPLETE
	// (never wait, never stall — a legit single node must not hang).
	FrontierNoBeacons

	// FrontierConnecting: beacons ARE configured but not enough of their stake is connected
	// YET to FORM a ⅔ quorum (the boot race — peers still handshaking). The node is NOT
	// caught up: it cannot even ask the network. KEEP POLLING until connectivity comes up
	// (then the quorum names the frontier) or the connect deadline elapses (then fail safe).
	FrontierConnecting

	// FrontierNoQuorum: enough beacon stake IS connected to ask, but THIS round saw no
	// ⅔-by-stake agreement on a single tip — either a TRANSIENT finality-skew split (honest
	// beacons momentarily on adjacent finalized tips; the live frontier is a moving target)
	// that a retry clears, or a PERSISTENT eclipse / partition. The node is NOT caught up: the
	// loop RETRIES a bounded number of rounds (a transient converges) then fails SAFE (a
	// persistent no-quorum never false-completes at the stale height). See defaultMaxNoQuorumRounds.
	FrontierNoQuorum

	// FrontierCaughtUp: the BlockSource has PROVEN the node is already AT OR ABOVE the network
	// frontier — DONE — even though no ⅔-by-stake beacon quorum actively NAMED a tip this round.
	// This decomplects "a quorum named a tip I hold" (FrontierNamed, which still drives the descend+
	// execute + Accepted() shortcut) from "I proved there is nothing ahead of me" (this), which the
	// pre-fix code CONFLATED by returning FrontierNamed(ownTip) for the caught-up case.
	//
	// THE FRESH-NET CASE this fixes: on a fresh network every validator — INCLUDING this node, which
	// is itself a beacon and knows its own accepted tip with certainty — holds only genesis. A node
	// whose own stake makes the PEER-ONLY responder weight fall below the stake-majority NAMING floor
	// (a heavy validator, a small beacon set, or skewed stake) can then NEVER name a frontier even
	// with the WHOLE validator set connected — it hangs in FrontierConnecting forever. But when the
	// node has reached its ENTIRE beacon set (so no eclipse can be hiding an ahead-tip) and that full
	// set PLUS itself unanimously report a tip it has ALREADY ACCEPTED, it IS at the frontier. The
	// BlockSource returns this ONLY after proving caught-up under that FULL-connectivity condition, so
	// it can NEVER false-complete a partially-connected (eclipsed) node — a suppressed beacon breaks
	// full-connectivity and the source falls back to the partition-capture floor (fail safe). The
	// stale-go-live guard is preserved: an eclipse cannot tip a partial view into a false caught-up.
	FrontierCaughtUp
)

// BlockSource is the peer-fetch transport the bootstrapper drives. The node
// implements it over its GetAcceptedFrontier / GetAncestors wire; a test implements
// it in-memory. It is the ONLY network dependency — the loop has no other I/O.
type BlockSource interface {
	// FrontierTip names the network frontier from a ⅔-by-stake quorum of the configured
	// BEACONS, returning the agreed tip and a FrontierStatus that decomplects WHY a tip
	// might not be named (see FrontierStatus): FrontierNamed (tip valid — sync to it),
	// FrontierNoBeacons (single-node — done), FrontierConnecting (beacons not connected
	// yet — wait), FrontierNoQuorum (connected but no agreement — fail safe), and
	// FrontierCaughtUp (the full beacon set plus the node itself prove nothing is ahead —
	// done, even though no quorum NAMED a tip; the fresh-net / tip-holder case). The tip is
	// meaningful (the named frontier to sync to) ONLY when status == FrontierNamed; for
	// FrontierCaughtUp the source has already established the node is at/above the frontier.
	FrontierTip(ctx context.Context) (tipID ids.ID, status FrontierStatus)

	// Ancestors returns up to maxBlocks blocks ending at blockID, OLDEST-FIRST (the
	// deepest ancestor first, blockID last) — the order the receiver must execute them
	// in. An empty result with no error means the peer did not serve; the caller
	// retries against a fresh peer.
	Ancestors(ctx context.Context, blockID ids.ID, maxBlocks int) ([][]byte, error)
}

// Chain is the local node the bootstrapper executes fetched blocks into. It is the
// receive side of chain.Runtime: parse, query sync state, and accept-on-frontier-trust.
type Chain interface {
	// ParseBlock decodes block bytes identity-preservingly so the loop can read each
	// block's height + parent for ordering and the descent.
	ParseBlock(ctx context.Context, b []byte) (block.Block, error)
	// LastAccepted returns the local last-accepted block id and height.
	LastAccepted(ctx context.Context) (id ids.ID, height uint64, err error)
	// Accepted reports whether block id is on the node's ACCEPTED chain (finalized) —
	// NOT merely present in the block store. This is the convergence predicate: the node
	// has REACHED the named frontier only when it has ACCEPTED it. The mainnet luxd-2
	// freeze was a store-vs-acceptance conflation: a frontier GOSSIPED into the store but
	// not yet accepted (height ABOVE last-accepted) reported "present", short-circuiting
	// the loop to caught-up at the stale last-accepted and never descending to accept it.
	// A stored-but-unaccepted block MUST return false here so the descent drives its
	// acceptance from last-accepted+1 up to the frontier through the cert/Verify path.
	Accepted(ctx context.Context, id ids.ID) bool
	// AcceptBootstrapBlock re-executes + finalizes a fetched block on frontier-trust
	// (chain.Runtime.AcceptBootstrapBlock). A reject (invalid bytes, failed Verify, or
	// out-of-order) returns a non-nil error and the loop stops advancing at that block.
	AcceptBootstrapBlock(ctx context.Context, b []byte) error
}

// Config wires a Bootstrapper. Source and Chain are required; the rest default.
type Config struct {
	Source BlockSource
	Chain  Chain
	Log    log.Logger

	MaxBlocksPerFetch int           // default 256
	MaxBuffer         int           // default 64Ki blocks
	MaxBufferBytes    int           // default 1 GiB (M3 — byte budget on the descent buffer)
	RetryInterval     time.Duration // default 1s

	// ConnectDeadline bounds how long Run WAITS for the beacon set to become reachable
	// (FrontierConnecting) before failing SAFE with ErrBeaconsUnreachable. Default 3 min
	// (generous for a healthy handshake, strictly below monitorBootstrap's 5-min
	// no-progress watchdog). A freshly-booted node with zero connected beacons is NOT
	// caught up — it must wait until it can actually ask the network.
	ConnectDeadline time.Duration // default 3m

	// MaxNoQuorumRounds bounds how many CONSECUTIVE FrontierNoQuorum rounds Run RETRIES before
	// terminally failing SAFE with ErrNoBeaconQuorum (F2). A LIVE frontier is a MOVING TARGET:
	// one tally at the instant connectivity first crosses the ⅔ floor can catch honest beacons
	// momentarily SPLIT across adjacent finalized tips (finality-propagation skew) — a transient
	// no-quorum that one more poll clears. Retrying a bounded number of rounds lets a transient
	// split CONVERGE while a PERSISTENT no-quorum (genuine eclipse / ≥⅓ Byzantine beacon stake)
	// still fails safe after the bound. The counter RESETS the instant a frontier IS named (the
	// no-quorum condition cleared). Default 10 (a seconds-to-low-minutes window, well below the
	// 5-min watchdog). The transient/persistent boundary is exercised in the bounded-retry tests.
	MaxNoQuorumRounds int // default 10

	// WeakSubjectivityID + WeakSubjectivityHeight pin an OPTIONAL operator-supplied
	// recent finalized checkpoint (block id at height). When both are set (id != Empty
	// AND height > 0) and the checkpoint is AHEAD of the local last-accepted, the
	// content-addressed descent from the α-agreed frontier MUST pass through this id at
	// this height — otherwise the frontier is on a chain that does not descend from the
	// trusted checkpoint and is refused (ErrFrontierNotDescendedFromCheckpoint). This is
	// the defense-in-depth anchor for an EMPTY (genesis) node, where there is no local
	// history for the per-height guard to bind the first fetched block to. Zero value =
	// disabled (the α-beacon-quorum on the frontier remains the primary safety anchor).
	WeakSubjectivityID     ids.ID
	WeakSubjectivityHeight uint64
}

// Bootstrapper runs the fetch+execute loop to converge a node to the frontier.
type Bootstrapper struct {
	cfg Config
}

// New builds a Bootstrapper, applying defaults for any unset Config field.
func New(cfg Config) *Bootstrapper {
	if cfg.MaxBlocksPerFetch <= 0 {
		cfg.MaxBlocksPerFetch = defaultMaxBlocksPerFetch
	}
	if cfg.MaxBuffer <= 0 {
		cfg.MaxBuffer = defaultMaxBuffer
	}
	if cfg.MaxBufferBytes <= 0 {
		cfg.MaxBufferBytes = defaultMaxBufferBytes
	}
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = defaultRetryInterval
	}
	if cfg.ConnectDeadline <= 0 {
		cfg.ConnectDeadline = defaultConnectDeadline
	}
	if cfg.MaxNoQuorumRounds <= 0 {
		cfg.MaxNoQuorumRounds = defaultMaxNoQuorumRounds
	}
	if cfg.Log == nil {
		cfg.Log = log.NewNoOpLogger()
	}
	return &Bootstrapper{cfg: cfg}
}

// Run drives initial sync until the node has reached the discovered frontier (or no
// peer is ahead), then returns nil — the node is synced and the caller hands off to
// live consensus. Returns ctx.Err() on cancellation, or a bootstrap error
// (ErrStalled / ErrGapTooLarge / ErrCannotConnect) that the caller surfaces as a real
// failure (so it is NOT masked as "ready").
func (b *Bootstrapper) Run(ctx context.Context) error {
	stalls := 0
	// noQuorums counts CONSECUTIVE FrontierNoQuorum rounds (F2). A live frontier is a MOVING
	// TARGET — a single tally at the instant connectivity crosses the ⅔ floor can catch honest
	// beacons split across adjacent finalized tips. We RETRY a bounded number of rounds so a
	// transient split CONVERGES; a persistent no-quorum still fails SAFE after the bound. It
	// resets the instant a frontier IS named (passWorking) — the no-quorum condition cleared.
	noQuorums := 0
	// connectingSince times the WAIT for beacon connectivity. It starts at the first
	// FrontierConnecting pass and resets the instant we get PAST connecting (a named
	// frontier), so it bounds only the contiguous "cannot ask the network yet" window.
	var connectingSince time.Time
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		advanced, outcome, err := b.syncOnce(ctx)
		if err != nil {
			return err
		}

		switch outcome {
		case passCaughtUp:
			// Genuinely synced: a ⅔-beacon quorum names a tip we already hold (or none is
			// ahead), OR there is no beacon set at all (single-node / dev). DONE.
			b.cfg.Log.Info("bootstrap complete — reached the network frontier")
			return nil

		case passConnecting:
			// THE CANARY CASE. Beacons ARE configured but not enough stake is connected yet
			// to FORM a ⅔ quorum — the node cannot even ASK the network for the frontier, so
			// it is NOT caught up (concluding so here would false-complete at the stale
			// height). WAIT for connectivity, bounded by ConnectDeadline; if it never comes
			// the node is eclipsed/partitioned and we fail SAFE.
			if connectingSince.IsZero() {
				connectingSince = time.Now()
				b.cfg.Log.Info("bootstrap waiting for beacon connectivity before naming the frontier (not caught up)")
			}
			if time.Since(connectingSince) >= b.cfg.ConnectDeadline {
				return ErrBeaconsUnreachable
			}
			if err := b.pause(ctx); err != nil {
				return err
			}
			continue

		case passNoQuorum:
			// Enough beacon stake IS connected to ask, but THIS GetAcceptedFrontier round found
			// no ⅔-by-stake supermajority on a single tip. On a live chain (1–2 s blocks) the
			// beacon-named frontier MOVES: honest beacons momentarily split across ADJACENT
			// finalized tips (finality-propagation skew), so a single tally at the instant
			// connectivity first crosses the ⅔ floor can miss — a transient one more poll clears.
			// RETRY a bounded number of rounds (the same shape as the stall/connect waits): a
			// transient split CONVERGES on a later round (more beacons connect, finality settles);
			// only a PERSISTENT no-quorum (genuine eclipse / ≥⅓ Byzantine beacon stake) exhausts
			// the bound and fails SAFE. Either way the node NEVER false-completes at its stale
			// height. The bound keeps a real eclipse failing within a sane window (≪ the watchdog).
			noQuorums++
			if noQuorums == 1 {
				b.cfg.Log.Info("bootstrap: beacons connected but split with no ⅔ quorum — retrying (frontier is a moving target; not caught up)")
			}
			if noQuorums >= b.cfg.MaxNoQuorumRounds {
				return ErrNoBeaconQuorum
			}
			if err := b.pause(ctx); err != nil {
				return err
			}
			continue

		case passWorking:
			// A frontier IS named; the descent ran. We are past BOTH the connect phase and any
			// transient no-quorum, so reset both bounded waits — the next no-quorum (if any) is a
			// fresh transient, counted from zero again.
			connectingSince = time.Time{}
			noQuorums = 0
			if advanced {
				stalls = 0
				continue // immediately fetch the next segment toward the frontier
			}
			// No progress this round: the named-frontier peer did not serve the ancestry.
			// Re-sample after a short pause (never a hot loop); give up after maxStallRounds
			// so a real unreachable-peer failure surfaces instead of spinning forever.
			stalls++
			if stalls >= maxStallRounds {
				return ErrStalled
			}
			if err := b.pause(ctx); err != nil {
				return err
			}
		}
	}
}

// pause sleeps one RetryInterval or returns ctx.Err() on cancellation — the single place
// the loop backs off so a dead/withholding peer or a not-yet-connected beacon set is never
// hot-looped.
func (b *Bootstrapper) pause(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(b.cfg.RetryInterval):
		return nil
	}
}

// passOutcome is the result of ONE syncOnce pass — the decomplected signal Run acts on.
// It separates the three FrontierStatus "not named" reasons (wait / done / fail safe) that
// the pre-fix code collapsed into a single caughtUp bool.
type passOutcome int

const (
	passWorking    passOutcome = iota // a frontier IS named; the descent ran (see advanced)
	passCaughtUp                      // reached the frontier, or no beacon set: DONE
	passConnecting                    // beacons not connected yet to form a quorum: WAIT
	passNoQuorum                      // beacons connected but no ⅔ agreement: FAIL SAFE
)

// syncOnce performs ONE sync pass: discover the frontier, descend-and-buffer the
// missing ancestry until it connects to our tip, then execute it oldest-first.
//
//	advanced — at least one block was accepted this pass.
//	outcome  — passCaughtUp (synced / single-node), passConnecting (wait for beacons),
//	           passNoQuorum (eclipse — fail safe), or passWorking (frontier named, descent ran).
func (b *Bootstrapper) syncOnce(ctx context.Context) (advanced bool, outcome passOutcome, err error) {
	tipID, status := b.cfg.Source.FrontierTip(ctx)
	switch status {
	case FrontierNamed:
		// A ⅔-stake beacon quorum named tipID — the ONLY permissive status. Fall through to
		// descend + execute below.
	case FrontierNoBeacons:
		// No beacon set configured (single-node / dev / --skip-bootstrap). There is no
		// network frontier to reach — genuinely caught up. A legit single node must not hang.
		return false, passCaughtUp, nil
	case FrontierConnecting:
		// Beacons configured but not enough connected YET to form a ⅔ quorum (the canary
		// boot race). The node cannot even ASK — it is NOT caught up. Run waits (bounded).
		return false, passConnecting, nil
	case FrontierNoQuorum:
		// Beacons connected, but no ⅔-by-stake agreement reachable THIS round (transient
		// finality skew or a genuine eclipse). NOT caught up — Run retries bounded then fails safe.
		return false, passNoQuorum, nil
	case FrontierCaughtUp:
		// The BlockSource PROVED the node is at/above the network frontier (the fresh-net / tip-holder
		// case: the FULL beacon set plus the node itself unanimously report a tip the node already
		// holds, so nothing is ahead) even though no quorum NAMED a tip. There is nothing to descend —
		// DONE. Distinct from FrontierNamed, which falls through to the descend+execute path; here the
		// source has already established caught-up under its FULL-connectivity (eclipse-free) condition.
		return false, passCaughtUp, nil
	default:
		// FrontierInvalid (the zero value) or any out-of-range status (F4). A status the type
		// does not define can NEVER be read as caught-up — that would be the false-complete the
		// whole decomplecting exists to prevent. Fail SAFE, identical to FrontierNoQuorum
		// (bounded retry then ErrNoBeaconQuorum), so even an uninitialized status finalizes nothing.
		b.cfg.Log.Warn("bootstrap: FrontierTip returned an undefined status — failing safe (not caught up)",
			log.Int("status", int(status)))
		return false, passNoQuorum, nil
	}

	lastID, lastH, err := b.cfg.Chain.LastAccepted(ctx)
	if err != nil {
		return false, passWorking, err
	}

	// CAUGHT-UP iff the named frontier is ACCEPTED (it IS our last-accepted, or it is on our
	// accepted chain at/below our last-accepted height) — NOT merely PRESENT in the block
	// store. THE FREEZE: luxd-2 had the named frontier (1082796) and its run-up (1082781..95)
	// GOSSIPED into its store but UNACCEPTED at last-accepted 1082780; the old Chain.Has(tip)
	// returned true for that stored-but-unaccepted block and short-circuited to caught-up at
	// the stale 1082780, never descending to accept it — self-reinforcing across restarts. The
	// acceptance predicate falls through for a stored-unaccepted frontier, so the content-
	// addressed descent below DRIVES ACCEPTANCE from last-accepted+1 up to the named frontier
	// through the per-height-guarded Verify/finality path (it accepts the blocks already in the
	// store — re-Verify + commit — advancing last-accepted, never marking them accepted merely
	// because they are present).
	if tipID == ids.Empty || tipID == lastID || b.cfg.Chain.Accepted(ctx, tipID) {
		return false, passCaughtUp, nil
	}

	// CONTENT-ADDRESSED DESCENT from the α-agreed frontier (the C1 safety core).
	// Starting at tipID, walk the parent chain DOWNWARD, buffering ONLY blocks that
	// cryptographically link from tipID — each buffered block's id is the parent-id of
	// the block one height above it on the path. This is what guarantees the executed
	// chain actually REACHES the α-agreed frontier: the per-height accept guard alone
	// enforces only LOCAL parent linkage (height == finalized+1, parent == tip), NOT
	// that the top of the synced chain equals the agreed tip. Without content
	// addressing a malicious Ancestors peer could serve a forged-but-Verify-passing
	// sidechain that connects to our local tip yet never reaches the honest frontier —
	// finalizing forged blocks and bricking the node. Here a forged block cannot be
	// substituted: its id will not equal the `want` we derived from tipID's parent
	// chain, so it is ignored (off-path), and a batch that does not even contain `want`
	// is abandoned (re-sample a fresh peer). Frontier-naming (node FrontierTip,
	// beacon + α-weighted-stake quorum) closes the OTHER half — a forged FRONTIER can
	// never be named.
	buffer := make(map[uint64][]byte)
	var bufferBytes int
	want := tipID // the next block id we require, descending from the agreed frontier
	// sawCheckpoint is pre-satisfied unless an operator pinned a weak-subjectivity
	// checkpoint that is AHEAD of our local tip (so the descent must pass through it).
	checkpointActive := b.cfg.WeakSubjectivityID != ids.Empty &&
		b.cfg.WeakSubjectivityHeight > 0 &&
		b.cfg.WeakSubjectivityHeight > lastH
	sawCheckpoint := !checkpointActive
	connected := false
	for round := 0; round < maxDescentRounds; round++ {
		if ctx.Err() != nil {
			return advanced, passWorking, ctx.Err()
		}
		batch, ferr := b.cfg.Source.Ancestors(ctx, want, b.cfg.MaxBlocksPerFetch)
		if ferr != nil || len(batch) == 0 {
			// Peer did not serve this segment; abandon the pass (no advance) so Run
			// re-samples a fresh peer. Not fatal — another peer may have it.
			return advanced, passWorking, nil
		}

		// Index the batch by content (block id). Unparseable bytes ⇒ a malformed peer
		// response: abandon and re-sample (never trust junk into the buffer).
		type idxEntry struct {
			raw    []byte
			height uint64
			parent ids.ID
		}
		index := make(map[ids.ID]idxEntry, len(batch))
		for _, raw := range batch {
			blk, perr := b.cfg.Chain.ParseBlock(ctx, raw)
			if perr != nil {
				return advanced, passWorking, nil
			}
			index[blk.ID()] = idxEntry{raw: raw, height: blk.Height(), parent: blk.ParentID()}
		}

		// Walk the VERIFIED parent chain from `want` downward, buffering on-path blocks
		// only. Off-path blocks present in the batch are ignored — a forged block cannot
		// masquerade as an ancestor of the α-agreed frontier because its id is not on
		// tipID's parent chain.
		haveLowest := false
		var lowestHeight uint64
		var lowestParent ids.ID
		cur := want
		for {
			e, present := index[cur]
			if !present {
				break // the batch does not extend the path further down this round
			}
			if _, dup := buffer[e.height]; !dup {
				buffer[e.height] = e.raw
				bufferBytes += len(e.raw)
			}
			if cur == b.cfg.WeakSubjectivityID && e.height == b.cfg.WeakSubjectivityHeight {
				sawCheckpoint = true
			}
			haveLowest = true
			lowestHeight = e.height
			lowestParent = e.parent
			if e.height <= lastH+1 {
				break // reached our tip's successor — connected
			}
			cur = e.parent
		}
		if !haveLowest {
			// The batch did not contain the block we asked for (`want`): the peer served
			// off-path junk. Abandon the pass and re-sample a fresh peer.
			return advanced, passWorking, nil
		}

		if lowestHeight <= lastH+1 {
			connected = true
			break
		}
		if len(buffer) >= b.cfg.MaxBuffer || bufferBytes >= b.cfg.MaxBufferBytes {
			return advanced, passWorking, ErrGapTooLarge
		}
		// Descend: require the parent of the lowest on-path block next round.
		want = lowestParent
	}
	if !connected {
		return advanced, passWorking, ErrCannotConnect
	}

	// WEAK SUBJECTIVITY (defense-in-depth). If an operator pinned a recent finalized
	// checkpoint AHEAD of our tip, the α-agreed frontier MUST descend from it — we must
	// have encountered that id at that height on the content-addressed path from the
	// frontier down. If not, the frontier is on a chain that does not descend from the
	// trusted checkpoint: refuse it (do not execute a single block of it).
	if !sawCheckpoint {
		b.cfg.Log.Warn("bootstrap: α-agreed frontier does not descend from the weak-subjectivity checkpoint — refusing the frontier",
			log.Stringer("checkpoint", b.cfg.WeakSubjectivityID),
			log.Uint64("checkpointHeight", b.cfg.WeakSubjectivityHeight))
		return advanced, passWorking, ErrFrontierNotDescendedFromCheckpoint
	}

	// EXECUTE oldest-first from lastH+1 up through the contiguous buffer. Each accept
	// re-executes (Verify) against the already-accepted parent and finalizes on
	// frontier-trust. A reject (invalid/out-of-order) STOPS the run at that block — the
	// sync does not advance past an unverifiable block; the next pass re-fetches it.
	for h := lastH + 1; ; h++ {
		raw, ok := buffer[h]
		if !ok {
			break // reached the top of this contiguous run
		}
		if aerr := b.cfg.Chain.AcceptBootstrapBlock(ctx, raw); aerr != nil {
			b.cfg.Log.Warn("bootstrap: block rejected during execute — sync paused at this height",
				log.Uint64("height", h), log.Err(aerr))
			break
		}
		advanced = true
	}
	return advanced, passWorking, nil
}
