// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package replog is a replicated, totally-ordered command log built on a
// Lux linear-chain consensus [consensus.Engine]. It is the replicated state
// machine primitive a coordinator needs — submit a command, and once
// consensus finalizes it, the command is applied exactly once, in commit
// order, on every replica.
//
// It exists to retire leader-based Raft (e.g. seaweedfs/raft, hashicorp/raft)
// from services that only need an ordered replicated log: the Hanzo S3
// master replicating its monotonic volume-id + topology-id, cluster
// membership logs, and the like. Compared to Raft this is ZAP-native (the
// block/vote gossip rides the zap-proto transport, no gRPC) and
// post-quantum-final (Quasar BLS + ML-DSA), with no separate leader-election
// FSM — finality is the consensus engine's job.
//
// The log is transport-agnostic: [Log] drives the local engine and applies
// finalized blocks; the votes that finalize them arrive via the engine's
// network layer (in production, peer votes gossiped over the transport; in a
// test, injected directly). Apply is the only place commands touch state, so
// it is the natural seam for a service's state machine.
package replog

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"

	"github.com/luxfi/consensus"
)

// Apply is invoked once per finalized command, in strict commit order, with
// the command's payload. Returning an error halts draining at that command
// (it will be retried on the next [Log.Advance]) so application stays
// gap-free and ordered.
type Apply func(payload []byte) error

// Log is a replicated, totally-ordered command log over a linear-chain
// [consensus.Engine]. Safe for concurrent Submit/Advance.
type Log struct {
	engine consensus.Engine
	apply  Apply

	mu      sync.Mutex
	height  uint64
	lastID  consensus.ID
	pending []pendingBlock // submitted, not yet applied, in ascending height
}

type pendingBlock struct {
	id      consensus.ID
	payload []byte
}

// New returns a Log over engine; apply is called for each finalized command
// in order. Call [Log.Start] before submitting.
func New(engine consensus.Engine, apply Apply) *Log {
	return &Log{engine: engine, apply: apply, lastID: consensus.GenesisID}
}

// Start starts the underlying consensus engine.
func (l *Log) Start(ctx context.Context) error { return l.engine.Start(ctx) }

// Stop stops the underlying consensus engine.
func (l *Log) Stop() error { return l.engine.Stop() }

// Submit appends payload as the next command in the log and proposes it to
// consensus. It returns the command's block ID. The command is NOT applied
// until consensus finalizes it and [Log.Advance] drains it — Submit is the
// proposal, not the commit.
func (l *Log) Submit(ctx context.Context, payload []byte) (consensus.ID, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	height := l.height + 1
	id := blockID(height, l.lastID, payload)
	blk := &consensus.Block{
		ID:       id,
		ParentID: l.lastID,
		Height:   height,
		Time:     time.Now(),
		Payload:  payload,
	}
	if err := l.engine.Add(ctx, blk); err != nil {
		return consensus.ID{}, err
	}
	l.height = height
	l.lastID = id
	l.pending = append(l.pending, pendingBlock{id: id, payload: append([]byte(nil), payload...)})
	return id, nil
}

// Advance applies every finalized command at the head of the pending queue,
// in order, stopping at the first command consensus has not yet accepted (so
// the applied log is always a gap-free prefix). It returns the number
// applied. A coordinator calls Advance from a ticker or whenever the engine
// signals new finality.
func (l *Log) Advance(ctx context.Context) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	applied := 0
	for len(l.pending) > 0 {
		head := l.pending[0]
		if l.engine.GetStatus(head.id) != consensus.StatusAccepted {
			break // preserve total order: don't skip an unfinalized command
		}
		if err := l.apply(head.payload); err != nil {
			return applied, err
		}
		l.pending = l.pending[1:]
		applied++
	}
	return applied, nil
}

// Pending reports how many submitted commands have not yet been applied.
func (l *Log) Pending() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pending)
}

// blockID deterministically derives a block ID from the chain position so
// the same command at the same height/parent yields the same ID on every
// replica (content-addressed, collision-resistant via SHA-256).
func blockID(height uint64, parent consensus.ID, payload []byte) consensus.ID {
	h := sha256.New()
	var hb [8]byte
	binary.BigEndian.PutUint64(hb[:], height)
	h.Write(hb[:])
	h.Write(parent[:])
	h.Write(payload)
	var id consensus.ID
	copy(id[:], h.Sum(nil))
	return id
}
