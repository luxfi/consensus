// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// source.go — the two things the round needs from outside itself: the network
// (Source) and the local VM. Both are interfaces, so the round runs against an
// in-memory network in a test and so the consensus module carries NO transport —
// no request ids, no pending or failed sets, no outstanding-request window, no
// timeout manager, no validator set. Every wire concern sits behind Source, the
// same division the bootstrapper draws at BlockSource.
package summary

import (
	"context"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// Offer is one beacon's answer in the discovery round: the raw bytes of the newest
// summary that beacon holds. The bytes stay opaque here — only the VM can read
// them — and a beacon that answers with junk is still a beacon that answered.
type Offer struct {
	NodeID ids.NodeID
	Bytes  []byte
}

// Ballot is one beacon's answer in the ratification round: which of the requested
// heights it actually holds, named by summary id, with that beacon's stake already
// attached. The node attaches the stake because the consensus module has no
// validator set — it can weigh a vote but it cannot look one up.
//
// A beacon that holds none of the requested heights sends an empty Held; a beacon
// that never answers has no Ballot at all. Both are a vote for nothing, which is
// why neither needs a path of its own.
//
// Held is carried per beacon rather than as a flat list of (id, weight) pairs so
// that a beacon which can prove a height already carries a ⅔-stake certificate has
// somewhere to say so — a certificate settles ratification outright, and adding it
// is then a field, not a new shape.
type Ballot struct {
	NodeID ids.NodeID
	Weight uint64
	Held   []ids.ID
}

// Source is the network, as the round sees it: two blocking calls that each ask a
// question and return whatever arrived before their window closed.
type Source interface {
	// Offers asks the connected beacons which summary each holds and returns the
	// replies that arrived inside the collection window — at most one per beacon,
	// returning early once every connected beacon has answered.
	//
	// An empty result with a nil error is an answer: the beacons were asked and
	// none of them had anything. A non-nil error means the beacon set could not be
	// ASKED at all — not enough of it ever connected — and that is the one
	// condition the caller must not read as "the network has nothing for me". Wait
	// for connectivity here, bounded by a deadline, exactly as the bootstrapper
	// waits before it will name a frontier.
	Offers(ctx context.Context) ([]Offer, error)

	// Ballots asks EVERY beacon — not a sample — which of these heights it holds,
	// and returns the ballots that arrived inside the collection window together
	// with total, the stake of the whole beacon set.
	//
	// The whole set is the denominator because this is where safety lives.
	// Discovery only produced a list of heights, and a wrong height there costs one
	// extra entry in this request; everything that decides what the node adopts is
	// weighed here. Weighing it against a sample would let an adversary who happens
	// to be over-represented in that sample decide what state the node keeps.
	//
	// total counts exactly the beacons this call is willing to collect from, and
	// this node's own stake belongs in it if and only if this node is a beacon and
	// its own ballot comes back. Answering your own question while leaving yourself
	// out of total — or the reverse — judges a summary against a set that is not
	// the set that voted.
	Ballots(ctx context.Context, heights []uint64) (ballots []Ballot, total uint64, err error)
}

// VM is the local state-syncable VM, narrowed to what this round actually calls. It
// is a strict subset of block.StateSyncableVM, so a VM that supports state sync
// already satisfies it and the node writes no adapter.
//
// The caller decides whether the round runs at all (block.StateSyncableVM's
// StateSyncEnabled) and owns VM state across it: the VM is already syncing when Run
// is called and stays where the caller put it whichever Outcome comes back. This
// package never moves the VM between states. Two owners for one lifecycle is how a
// VM ends up live on state it never finished fetching.
type VM interface {
	// GetOngoingSyncStateSummary returns the summary this node was already syncing
	// toward when it stopped, so a restart can finish the trie it has partly
	// fetched instead of throwing it away. database.ErrNotFound means there is no
	// such sync, which is an ordinary answer and not a failure.
	GetOngoingSyncStateSummary(context.Context) (block.StateSummary, error)

	// ParseStateSummary turns a beacon's reply into a summary. It must preserve
	// identity — ParseStateSummary(s.Bytes()).ID() == s.ID() — because the id it
	// yields is the name stake is counted against in ratification. A summary whose
	// id changed on the way through parsing would collect votes cast for a
	// different one.
	ParseStateSummary(context.Context, []byte) (block.StateSummary, error)

	// LastAccepted and GetBlock give the height this node already stands at. It is
	// the floor no candidate may sit at or below: adopting a summary throws away
	// everything below it, so adopting one at or under the local tip trades real
	// history for nothing. A majority that happens to be behind must never be able
	// to drag a node backwards.
	LastAccepted(context.Context) (ids.ID, error)
	GetBlock(context.Context, ids.ID) (block.Block, error)
}

// A state-syncable VM satisfies VM as it stands. The narrowing bounds what this
// package may call; it is not a second interface for the node to implement.
var _ VM = (block.StateSyncableVM)(nil)
