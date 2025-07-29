// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package flare

import (
	"context"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

// Tx is a transaction that can be included in a vertex
type Tx interface {
	choices.Decidable

	// ID returns the unique identifier of this transaction
	ID() ids.ID

	// Verify that this transaction is well-formed
	Verify(context.Context) error

	// Conflicts returns the set of transactions that this transaction conflicts with
	Conflicts() ([]ids.ID, error)

	// Dependencies returns the set of transactions that this transaction depends on
	Dependencies() ([]ids.ID, error)

	// Bytes returns the binary representation of this transaction
	Bytes() []byte
}