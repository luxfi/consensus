// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"github.com/luxfi/consensus/types"
)

// BlockWithFPC extends a block with FPC voting fields
type BlockWithFPC interface {
	types.Block

	// FPCVotes returns the transaction references this block votes for
	FPCVotes() []TxRef

	// EpochBit returns true if this block signals epoch closing
	EpochBit() bool

	// Author returns the block author/proposer identity
	Author() []byte
}

// BlockPayloadExt can be embedded in block payloads to add FPC fields
type BlockPayloadExt struct {
	// Array of TxRef (32B each), owned-only, no dupes
	FPCVotes [][]byte `json:"fpcVotes,omitempty"`

	// True when starting epoch-close; pauses new fast finality
	EpochBit bool `json:"epochBit,omitempty"`
}

// ToTxRefs converts raw bytes to TxRef array
func ToTxRefs(votes [][]byte) []TxRef {
	if len(votes) == 0 {
		return nil
	}
	refs := make([]TxRef, 0, len(votes))
	for _, v := range votes {
		if len(v) == 32 {
			var ref TxRef
			copy(ref[:], v)
			refs = append(refs, ref)
		}
	}
	return refs
}

// FromTxRefs converts TxRef array to raw bytes
func FromTxRefs(refs []TxRef) [][]byte {
	if len(refs) == 0 {
		return nil
	}
	votes := make([][]byte, len(refs))
	for i, ref := range refs {
		votes[i] = make([]byte, 32)
		copy(votes[i], ref[:])
	}
	return votes
}
