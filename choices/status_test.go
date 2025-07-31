// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package choices

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusValid(t *testing.T) {
	require := require.New(t)

	require.True(Accepted.Valid())
	require.True(Rejected.Valid())
	require.True(Processing.Valid())
	require.True(Unknown.Valid())
	require.False(Status(math.MaxInt32).Valid())
}

func TestStatusDecided(t *testing.T) {
	require := require.New(t)

	require.True(Accepted.Decided())
	require.True(Rejected.Decided())
	require.False(Processing.Decided())
	require.False(Unknown.Decided())
	require.False(Status(math.MaxInt32).Decided())
}

func TestStatusFetched(t *testing.T) {
	require := require.New(t)

	require.True(Accepted.Fetched())
	require.True(Rejected.Fetched())
	require.True(Processing.Fetched())
	require.False(Unknown.Fetched())
	require.False(Status(math.MaxInt32).Fetched())
}

func TestStatusString(t *testing.T) {
	require := require.New(t)

	require.Equal("Accepted", Accepted.String())
	require.Equal("Rejected", Rejected.String())
	require.Equal("Processing", Processing.String())
	require.Equal("Unknown", Unknown.String())
	require.Equal("Invalid status", Status(math.MaxInt32).String())
}