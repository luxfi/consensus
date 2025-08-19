// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

func TestBootstrapper(t *testing.T) {
	require := require.New(t)

	b := NewBootstrapper(10)
	require.NotNil(b)

	// Add a vertex
	vertexID := ids.GenerateTestID()
	b.Add(vertexID)

	// Process
	err := b.Process(context.Background())
	require.NoError(err)
}
