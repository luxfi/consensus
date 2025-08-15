// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package beam implements the assembler/proposer
package beam

import (
	"time"
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
	Header   Header
	Entries  []Entry
	Votes    []VoteRef // Fast-path votes
	BLSSig   []byte
	PQSig    []byte
	Binding  []byte
	// FPC additions
	FPCVotes [][]byte // Embedded fast-path vote references
	EpochBit bool     // Epoch fence bit
}

// VoteRef represents a vote reference
type VoteRef []byte

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