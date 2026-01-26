// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package validators re-exports github.com/luxfi/validators for backward compatibility.
package validators

import (
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	"github.com/luxfi/math/set"
	"github.com/luxfi/validators"
)

// Sortable is an alias for validators.Sortable
type Sortable[T any] = validators.Sortable[T]

// Re-export errors
var (
	ErrUnknownValidator = validators.ErrUnknownValidator
	ErrWeightOverflow   = validators.ErrWeightOverflow
)

// CanonicalValidatorSet is an alias for validators.CanonicalValidatorSet
type CanonicalValidatorSet = validators.CanonicalValidatorSet

// CanonicalValidator is an alias for validators.CanonicalValidator
type CanonicalValidator = validators.CanonicalValidator

// FlattenValidatorSet re-exports validators.FlattenValidatorSet
func FlattenValidatorSet(vdrSet map[ids.NodeID]*GetValidatorOutput) (CanonicalValidatorSet, error) {
	return validators.FlattenValidatorSet(vdrSet)
}

// FilterValidators re-exports validators.FilterValidators
func FilterValidators(
	indices set.Bits,
	vdrs []*CanonicalValidator,
) ([]*CanonicalValidator, error) {
	return validators.FilterValidators(indices, vdrs)
}

// SumWeight re-exports validators.SumWeight
func SumWeight(vdrs []*CanonicalValidator) (uint64, error) {
	return validators.SumWeight(vdrs)
}

// AggregatePublicKeys re-exports validators.AggregatePublicKeys
func AggregatePublicKeys(vdrs []*CanonicalValidator) (*bls.PublicKey, error) {
	return validators.AggregatePublicKeys(vdrs)
}
