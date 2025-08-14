// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPhotonBasic(t *testing.T) {
	// Test creating a photon
	p := Photon[string]{
		Item:   "test-item",
		Prefer: true,
	}

	require.Equal(t, "test-item", p.Item)
	require.True(t, p.Prefer)
}

func TestPhotonTypes(t *testing.T) {
	// Test with different types
	intPhoton := Photon[int]{
		Item:   42,
		Prefer: false,
	}
	require.Equal(t, 42, intPhoton.Item)
	require.False(t, intPhoton.Prefer)

	type CustomID struct {
		Value string
	}

	customPhoton := Photon[CustomID]{
		Item:   CustomID{Value: "custom"},
		Prefer: true,
	}
	require.Equal(t, "custom", customPhoton.Item.Value)
	require.True(t, customPhoton.Prefer)
}

func TestPhotonZeroValue(t *testing.T) {
	// Test zero value
	var p Photon[string]
	require.Equal(t, "", p.Item)
	require.False(t, p.Prefer)
}