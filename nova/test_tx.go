// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package nova

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/consensus/utils/set"
)

var _ Tx = (*TestTx)(nil)

// TestTx is a useful test tx
type TestTx struct {
	choices.TestDecidable

	DependenciesV    set.Set[ids.ID]
	DependenciesErrV error
	VerifyV          error
	BytesV           []byte
}

func (t *TestTx) MissingDependencies() (set.Set[ids.ID], error) {
	return t.DependenciesV, t.DependenciesErrV
}

func (t *TestTx) Verify(context.Context) error {
	return t.VerifyV
}

func (t *TestTx) Bytes() []byte {
	return t.BytesV
}
