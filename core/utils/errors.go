// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package utils

import "errors"

var (
	// ErrNotRunning is returned when an operation is attempted on a stopped component
	ErrNotRunning = errors.New("not running")

	// ErrNotImplemented is returned when a method is not implemented
	ErrNotImplemented = errors.New("not implemented")

	// ErrConflict is returned when there's a conflict in consensus
	ErrConflict = errors.New("conflicting operation")
)
