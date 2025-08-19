// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validatorstate

// GetValidatorSetRequest represents a validator set request
type GetValidatorSetRequest struct {
	SubnetId []byte
	Height   uint64
}

// GetValidatorSetResponse represents a validator set response
type GetValidatorSetResponse struct {
	Validators []*Validator
}
