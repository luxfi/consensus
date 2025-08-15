// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import "errors"

var (
	// ErrWrongPhase is returned when an operation is attempted in the wrong phase.
	ErrWrongPhase = errors.New("wrong phase for operation")

	// ErrInvalidProposal is returned when a proposal is invalid.
	ErrInvalidProposal = errors.New("invalid proposal")

	// ErrInvalidCommit is returned when a commit is invalid.
	ErrInvalidCommit = errors.New("invalid commit")

	// ErrNoQuorum is returned when quorum is not reached.
	ErrNoQuorum = errors.New("no quorum")

	// ErrAlreadyFinalized is returned when trying to finalize an already finalized item.
	ErrAlreadyFinalized = errors.New("already finalized")

	// ErrNotFound is returned when an item is not found.
	ErrNotFound = errors.New("not found")

	// ErrInvalidVertex is returned when a vertex is invalid.
	ErrInvalidVertex = errors.New("invalid vertex")

	// ErrInvalidBlock is returned when a block is invalid.
	ErrInvalidBlock = errors.New("invalid block")
)
