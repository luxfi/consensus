// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beamtest

import (
	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/choices"
)

func RequireStatusIs(require *require.Assertions, status choices.Status, blks ...*Block) {
	for i, blk := range blks {
		require.Equal(status, blk.Status(), i)
	}
}
