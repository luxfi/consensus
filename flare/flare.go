// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package flare implements fast path consensus (embedded; ON by default)
package flare

import (
	"sync"
)

type Status uint8

const (
	StatusPending    Status = iota
	StatusExecutable        // 2f+1 votes observed
	StatusFinal             // anchored by a committed block OR 2f+1 certs in history
)

type VoteRef struct {
	BlockID []byte // or hash
	Index   uint32 // position in block
}

type Flare[T comparable] interface {
	Propose(tx T) VoteRef              // implicit vote in next block
	ObserveVote(v VoteRef)                 // from peers
	Status(tx T) Status
	Executable() []T                   // drain currently executable
	PendingVotes(parents [][]byte) []VoteRef // to embed in next block
}

type impl[T comparable] struct {
	mu     sync.Mutex
	votes  map[T]int
	status map[T]Status
	f      int // byzantine fault tolerance
}

func New[T comparable](f int) Flare[T] {
	return &impl[T]{
		votes:  make(map[T]int),
		status: make(map[T]Status),
		f:      f,
	}
}

func (fl *impl[T]) Propose(tx T) VoteRef {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	fl.votes[tx]++
	if fl.votes[tx] >= 2*fl.f+1 {
		if fl.status[tx] < StatusExecutable {
			fl.status[tx] = StatusExecutable
		}
	}
	return VoteRef{}
}

func (fl *impl[T]) ObserveVote(v VoteRef) { /* wire later */ }

func (fl *impl[T]) Status(tx T) Status {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	return fl.status[tx]
}

func (fl *impl[T]) Executable() []T {
	fl.mu.Lock()
	defer fl.mu.Unlock()
	var out []T
	for t, st := range fl.status {
		if st == StatusExecutable {
			out = append(out, t)
		}
	}
	return out
}

func (fl *impl[T]) PendingVotes(parents [][]byte) []VoteRef { return nil }