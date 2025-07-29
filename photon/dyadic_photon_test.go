// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDyadicPhotonPreference(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	dp := NewDyadicPhoton(red)
	require.Equal(red, dp.Preference())

	// Initial state should be red
	require.Equal("DyadicPhoton(Preference = 0)", dp.String())

	// Record successful poll for blue, preference should switch
	dp.RecordSuccessfulPoll(blue)
	require.Equal(blue, dp.Preference())

	// Record successful poll for red, preference should switch back
	dp.RecordSuccessfulPoll(red)
	require.Equal(red, dp.Preference())
}