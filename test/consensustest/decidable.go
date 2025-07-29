// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"errors"
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/choices"
)

var (
	_ choices.Decidable = (*Decidable)(nil)

	ErrInvalidStateTransition = errors.New("invalid state transition")
)

type Decidable struct {
	IDV        ids.ID
	AcceptV    error
	RejectV    error
	StatusV    choices.Status
}

func (d *Decidable) ID() ids.ID {
	return d.IDV
}

func (d *Decidable) Accept(context.Context) error {
	if d.StatusV == choices.Rejected {
		return fmt.Errorf("%w from %s to %s",
			ErrInvalidStateTransition,
			choices.Rejected,
			choices.Accepted,
		)
	}

	d.StatusV = choices.Accepted
	return d.AcceptV
}

func (d *Decidable) Reject(context.Context) error {
	if d.StatusV == choices.Accepted {
		return fmt.Errorf("%w from %s to %s",
			ErrInvalidStateTransition,
			choices.Accepted,
			choices.Rejected,
		)
	}

	d.StatusV = choices.Rejected
	return d.RejectV
}

func (d *Decidable) Status() choices.Status {
	return d.StatusV
}
