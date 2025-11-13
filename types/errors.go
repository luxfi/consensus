// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

package types

import "errors"

// Common consensus errors
var (
	// ErrBlockNotFound is returned when a block is not found
	ErrBlockNotFound = errors.New("block not found")

	// ErrInvalidBlock is returned when a block is invalid
	ErrInvalidBlock = errors.New("invalid block")

	// ErrInvalidVote is returned when a vote is invalid
	ErrInvalidVote = errors.New("invalid vote")

	// ErrNoQuorum is returned when there is no quorum
	ErrNoQuorum = errors.New("no quorum")

	// ErrAlreadyVoted is returned when a validator has already voted
	ErrAlreadyVoted = errors.New("already voted")

	// ErrNotValidator is returned when the node is not a validator
	ErrNotValidator = errors.New("not a validator")

	// ErrTimeout is returned when an operation times out
	ErrTimeout = errors.New("operation timeout")

	// ErrNotInitialized is returned when the engine is not initialized
	ErrNotInitialized = errors.New("engine not initialized")
)
