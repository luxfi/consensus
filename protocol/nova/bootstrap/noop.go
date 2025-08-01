// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/set"
)

var Noop Prism = noop{}

type noop struct{}

func (noop) GetPeers(context.Context) set.Set[ids.NodeID] {
	return nil
}

func (noop) RecordOpinion(context.Context, ids.NodeID, set.Set[ids.ID]) error {
	return nil
}

func (noop) Result(context.Context) ([]ids.ID, bool) {
	return nil, false
}
