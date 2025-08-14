// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package beam implements the assembler/proposer
package beam

import (
	"time"

	"github.com/luxfi/consensus/flare"
)

type Header struct {
	Parents [][]byte
	Round   uint64
	Ts      time.Time
}

type Entry struct {
	Payload []byte // tx/command
}

type ProposedBlock struct {
	Header  Header
	Entries []Entry
	Votes   []flare.VoteRef
	BLSSig  []byte
	PQSig   []byte
	Binding []byte
}

type Builder[T comparable] interface {
	Propose(parents [][]byte, decided [][]byte, execOwned []T) (*ProposedBlock, error)
}

type DefaultBuilder[T comparable] struct{}

func (b *DefaultBuilder[T]) Propose(parents [][]byte, decided [][]byte, execOwned []T) (*ProposedBlock, error) {
	// pack entries from decided + execOwned; add votes later
	return &ProposedBlock{
		Header: Header{Parents: parents, Ts: time.Now()},
	}, nil
}