// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package photon

import (
    "testing"
    
    "github.com/stretchr/testify/require"
    "github.com/luxfi/consensus/config"
)

func TestPhoton(t *testing.T) {
    require := require.New(t)
    
    params := config.DefaultParameters
    p := NewPhoton(params)
    require.NotNil(p)
}
