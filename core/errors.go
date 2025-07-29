// Copyright (C) 2019-2024, Lux Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package core

import "errors"

var (
	// ErrNotRunning is returned when an operation is attempted on a stopped engine
	ErrNotRunning = errors.New("engine not running")

	// ErrNotImplemented is returned when a method is not yet implemented
	ErrNotImplemented = errors.New("not implemented")

	// ErrInvalidBlock is returned when a block is invalid
	ErrInvalidBlock = errors.New("invalid block")

	// ErrInvalidVertex is returned when a vertex is invalid
	ErrInvalidVertex = errors.New("invalid vertex")

	// ErrConflictingParents is returned when a vertex has conflicting parents
	ErrConflictingParents = errors.New("conflicting parents")

	// ErrUnknownBlock is returned when a block is not found
	ErrUnknownBlock = errors.New("unknown block")

	// ErrUnknownVertex is returned when a vertex is not found
	ErrUnknownVertex = errors.New("unknown vertex")

	// ErrTimeout is returned when an operation times out
	ErrTimeout = errors.New("operation timed out")
)