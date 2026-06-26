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
)

// BlockSource is the peer-fetch transport the bootstrapper drives. The node
// implements it over its GetAcceptedFrontier / GetAncestors wire; a test implements
// it in-memory. It is the ONLY network dependency — the loop has no other I/O.
type BlockSource interface {
	// FrontierTip returns the accepted-tip block id advertised by a sampled peer (the
	// network frontier). ok=false when no peer answered (no peers, or all timed out) —
	// the driver treats that as "nothing to sync to" and finishes.
	FrontierTip(ctx context.Context) (tipID ids.ID, ok bool)

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
	// Has reports whether the node already holds block id — used to detect that the
	// node has reached (already has) the frontier.
	Has(ctx context.Context, id ids.ID) bool
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
	MaxBuffer         int           // default 64Ki
	RetryInterval     time.Duration // default 1s
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
	if cfg.RetryInterval <= 0 {
		cfg.RetryInterval = defaultRetryInterval
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
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		advanced, caughtUp, err := b.syncOnce(ctx)
		if err != nil {
			return err
		}
		if caughtUp {
			b.cfg.Log.Info("bootstrap complete — reached the network frontier")
			return nil
		}

		if advanced {
			stalls = 0
			continue // immediately fetch the next segment toward the frontier
		}

		// No progress this round: a peer did not serve, or none is ahead yet. Re-sample
		// after a short pause (never a hot loop); give up after maxStallRounds so a real
		// unreachable-peer failure surfaces instead of spinning forever.
		stalls++
		if stalls >= maxStallRounds {
			return ErrStalled
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(b.cfg.RetryInterval):
		}
	}
}

// syncOnce performs ONE sync pass: discover the frontier, descend-and-buffer the
// missing ancestry until it connects to our tip, then execute it oldest-first.
//
//	advanced — at least one block was accepted this pass.
//	caughtUp — our last-accepted has reached the frontier (or no peer is ahead): DONE.
func (b *Bootstrapper) syncOnce(ctx context.Context) (advanced bool, caughtUp bool, err error) {
	tipID, ok := b.cfg.Source.FrontierTip(ctx)
	if !ok {
		// No peer answered the frontier query. Nothing to sync to — treat as caught up
		// (e.g. a single-node / dev chain, or a momentarily peerless start; the live
		// frontier poller keeps watching once we are live).
		return false, true, nil
	}
	if tipID == ids.Empty || b.cfg.Chain.Has(ctx, tipID) {
		// We already hold the frontier tip — synced.
		return false, true, nil
	}

	lastID, lastH, err := b.cfg.Chain.LastAccepted(ctx)
	if err != nil {
		return false, false, err
	}
	if tipID == lastID {
		return false, true, nil
	}

	// DESCEND from the frontier in bounded batches, buffering by height, until the
	// fetched segment reaches our tip (lowest height ≤ lastH+1). Each batch is
	// oldest-first; the lowest block's parent is the next descent target.
	buffer := make(map[uint64][]byte)
	want := tipID
	connected := false
	for round := 0; round < maxDescentRounds; round++ {
		if ctx.Err() != nil {
			return advanced, false, ctx.Err()
		}
		batch, ferr := b.cfg.Source.Ancestors(ctx, want, b.cfg.MaxBlocksPerFetch)
		if ferr != nil || len(batch) == 0 {
			// Peer did not serve this segment; abandon the pass (no advance) so Run
			// re-samples a fresh peer. Not fatal — another peer may have it.
			return advanced, false, nil
		}

		// batch is oldest-first: batch[0] is the lowest. Buffer all, by height.
		var lowest block.Block
		for _, raw := range batch {
			blk, perr := b.cfg.Chain.ParseBlock(ctx, raw)
			if perr != nil {
				// Unparseable bytes from a peer — abandon the pass and re-sample.
				return advanced, false, nil
			}
			if lowest == nil || blk.Height() < lowest.Height() {
				lowest = blk
			}
			buffer[blk.Height()] = raw
		}
		if lowest == nil {
			return advanced, false, nil
		}

		if lowest.Height() <= lastH+1 {
			connected = true
			break
		}
		if len(buffer) >= b.cfg.MaxBuffer {
			return advanced, false, ErrGapTooLarge
		}
		// Descend: fetch the parent of the lowest block we have.
		want = lowest.ParentID()
	}
	if !connected {
		return advanced, false, ErrCannotConnect
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
	return advanced, false, nil
}
