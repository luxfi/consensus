// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import "errors"

var (
	// ErrInvalidK is returned when K is invalid
	ErrInvalidK = errors.New("invalid K value")

	// ErrInvalidAlpha is returned when Alpha is invalid
	ErrInvalidAlpha = errors.New("invalid Alpha value")

	// ErrInvalidBeta is returned when Beta is invalid
	ErrInvalidBeta = errors.New("invalid Beta value")

	// ErrNoSampler is returned when no sampler is provided
	ErrNoSampler = errors.New("no sampler provided")
)
