// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validator

import (
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// Validator is a struct that contains the base values representing a validator
// of the Network.
type Validator struct {
	NodeID    ids.NodeID
	PublicKey *bls.PublicKey
	TxID      ids.ID
	Weight    uint64

	// index is used to efficiently remove validators from the validator set. It
	// represents the index of this validator in the vdrSlice and weights
	// arrays.
	index int
}

// GetValidatorOutput is a struct that contains the publicly relevant values of
// a validator of the Network for the output of GetValidator.
type GetValidatorOutput struct {
	NodeID    ids.NodeID
	PublicKey *bls.PublicKey
	Weight    uint64
}

type GetCurrentValidatorOutput struct {
	ValidationID  ids.ID
	NodeID        ids.NodeID
	PublicKey     *bls.PublicKey
	Weight        uint64
	StartTime     uint64
	MinNonce      uint64
	IsActive      bool
	IsL1Validator bool
}
