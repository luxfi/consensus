// Copyright (C) 2019-2024, Lux Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

import (
	"time"

	"github.com/luxfi/consensus/choices"
	"github.com/luxfi/ids"
)

// Block represents a block in the blockchain
type Block interface {
	choices.Decidable

	// Parent returns the ID of this block's parent
	Parent() ids.ID

	// Verify that the state transition this block would make if accepted is
	// valid. If the state transition is invalid, a non-nil error should be
	// returned.
	//
	// It is guaranteed that the Parent has been successfully verified.
	//
	// If nil is returned, it is guaranteed that either Accept or Reject will be
	// called on this block, unless the VM is shut down.
	Verify() error

	// Bytes returns the binary representation of this block
	//
	// This is the representation that a VM uses to check cryptographic
	// validity of the block. This is the representation that a VM should
	// emit to send a block to consensus.
	//
	// Note: Bytes should be cached by the VM, as it may be called multiple
	// times.
	Bytes() []byte

	// Height returns the height of this block
	Height() uint64

	// Timestamp returns the timestamp of this block
	Timestamp() time.Time
}