// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
    "testing"
    
    "github.com/stretchr/testify/require"
)

func TestQuasar(t *testing.T) {
    require := require.New(t)
    
    // Test engine creation
    e := &Engine{}
    require.NotNil(e)
}
