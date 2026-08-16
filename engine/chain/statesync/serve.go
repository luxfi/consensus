// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package statesync answers a peer's state-summary requests out of the local VM.
//
// Two of the op-space's four summary ops are requests a node must answer — the
// frontier, and the summaries held at named heights; the other two are replies,
// which belong to the collection running on the requesting node. This package is the
// answering half: it reads the VM and returns what to send, while framing and sending
// stay with the node, which owns the transport. The handler surface also carries a
// singular by-height request that no op-space wire can carry, so nothing here answers
// it.
//
// Serving is unconditional, including on a node that will never sync itself: it reads
// summaries the VM already holds, and a node's own preference says nothing about what
// its peers may ask. The VM narrowing below therefore omits StateSyncEnabled, putting
// that preference out of reach rather than trusting a caller not to consult it.
//
// Silence and an empty reply are distinct answers. No summary, or an error, sends
// nothing, and the requester closes the round on its deadline. A request naming no
// heights gets an empty reply: an affirmative "none of those" that completes the
// tally at once. Swap them and every round either stalls or under-counts.
package statesync

import (
	"context"
	"errors"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// Summary is what the serving path reads off a state summary: the identity a
// requester tallies stake against, and the bytes it parses. Height and Accept are the
// requester's business and are deliberately absent, so no policy that belongs to the
// adopting node can be expressed here.
type Summary interface {
	ID() ids.ID
	Bytes() []byte
}

// Summaries is the serving half of a state-syncable VM. The summary type is a
// parameter because a VM's summary interface is declared where the VM is built; this
// package reads only ID and Bytes, so any declaration carrying those two serves
// without an intermediate type.
type Summaries[S Summary] interface {
	// GetLastStateSummary returns the VM's most recent summary, or
	// database.ErrNotFound when it holds none.
	GetLastStateSummary(context.Context) (S, error)

	// GetStateSummary returns the summary at the given height, or
	// database.ErrNotFound when the VM does not hold that height.
	GetStateSummary(context.Context, uint64) (S, error)
}

// Server answers one chain's summary requests.
type Server[S Summary] struct {
	vm Summaries[S]
}

// NewServer narrows vm to its serving half. A vm that does not serve summaries — any
// type, including nil — yields a server that is silent by construction, so the caller
// keeps one path for every chain and never has to decide whether to install a handler.
func NewServer[S Summary](vm any) *Server[S] {
	summaries, _ := vm.(Summaries[S])
	return &Server[S]{vm: summaries}
}

// Frontier answers a frontier request with the bytes of the VM's last summary.
//
// A non-nil error means send nothing. Holding no summary, failing to read one, and not
// serving summaries at all are one answer on the wire, because a requester reads them
// identically: as a peer that named no summary before the deadline.
func (s *Server[S]) Frontier(ctx context.Context) ([]byte, error) {
	if s.vm == nil {
		return nil, block.ErrStateSyncableVMNotImplemented
	}

	summary, err := s.vm.GetLastStateSummary(ctx)
	if err != nil {
		return nil, err
	}
	// A VM that names no summary and reports no error has still named no summary,
	// and a peer must not be able to reach a nil dereference by asking.
	if any(summary) == nil {
		return nil, database.ErrNotFound
	}
	return summary.Bytes(), nil
}

// Accepted answers a request for the summaries held at the named heights with the ids
// of those the VM holds. No ids and a nil error is the affirmative "none of those":
// the requester folds it in as a vote for nothing and its round completes without
// waiting out a deadline. A non-nil error means send nothing.
//
// A request naming no heights is answered before the VM is read, and is answered the
// same way by a VM that does not serve summaries at all — there is nothing to look up,
// so there is nothing that can fail.
//
// A height the VM does not hold is skipped and the remaining heights are still served.
// Any other read failure abandons the request: a VM that could not read cannot separate
// holding none of those heights from never having looked, and an empty reply asserts the
// first. A height named more than once is read once, because the reply is a set — a
// repeated id would let a tally that weights each named id count one stake twice.
func (s *Server[S]) Accepted(ctx context.Context, heights []uint64) ([]ids.ID, error) {
	if len(heights) == 0 {
		return nil, nil
	}
	if s.vm == nil {
		return nil, block.ErrStateSyncableVMNotImplemented
	}

	held := make([]ids.ID, 0, len(heights))
	asked := make(map[uint64]struct{}, len(heights))
	for _, height := range heights {
		if _, repeated := asked[height]; repeated {
			continue
		}
		asked[height] = struct{}{}

		summary, err := s.vm.GetStateSummary(ctx, height)
		switch {
		case errors.Is(err, database.ErrNotFound):
			continue
		case err != nil:
			return nil, err
		case any(summary) == nil:
			continue
		}
		held = append(held, summary.ID())
	}
	return held, nil
}
