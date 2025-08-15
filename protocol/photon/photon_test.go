// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/stretchr/testify/require"
)

func TestPhoton(t *testing.T) {
	require := require.New(t)

	params := config.DefaultParameters
	p := NewPhoton(params)
	require.NotNil(p)
}
