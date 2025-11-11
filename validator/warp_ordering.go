// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validators

import (
	"bytes"
	"errors"
	"fmt"

	"golang.org/x/exp/maps"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils"
	"github.com/luxfi/node/utils/crypto/bls"
	"github.com/luxfi/node/utils/math"
	"github.com/luxfi/math/set"
)

var (
	ErrUnknownValidator = errors.New("unknown validator")
	ErrWeightOverflow   = errors.New("weight overflowed")
)

// CanonicalValidatorSet represents a validator set in canonical ordering
type CanonicalValidatorSet struct {
	// Validators slice in canonical ordering of the validators that has public key
	Validators []*CanonicalValidator
	// The total weight of all the validators, including the ones that doesn't have a public key
	TotalWeight uint64
}

// CanonicalValidator represents a single validator with BLS public key in canonical form
type CanonicalValidator struct {
	PublicKey      *bls.PublicKey
	PublicKeyBytes []byte // Uncompressed bytes for canonical ordering
	Weight         uint64
	NodeIDs        []ids.NodeID // Can have multiple NodeIDs with same public key
}

// Compare implements utils.Sortable for canonical ordering
func (v *CanonicalValidator) Compare(o *CanonicalValidator) int {
	return bytes.Compare(v.PublicKeyBytes, o.PublicKeyBytes)
}

var _ utils.Sortable[*CanonicalValidator] = (*CanonicalValidator)(nil)

// FlattenValidatorSet converts the provided [vdrSet] into a canonical ordering.
// Also returns the total weight of the validator set.
func FlattenValidatorSet(vdrSet map[ids.NodeID]*GetValidatorOutput) (CanonicalValidatorSet, error) {
	var (
		// Map public keys to validators to handle duplicates
		pkToValidator = make(map[string]*CanonicalValidator)
		totalWeight   uint64
		err           error
	)
	for _, vdr := range vdrSet {
		totalWeight, err = math.Add(totalWeight, vdr.Weight)
		if err != nil {
			return CanonicalValidatorSet{}, fmt.Errorf("%w: %w", ErrWeightOverflow, err)
		}

		// Skip validators without public keys
		if len(vdr.PublicKey) == 0 {
			continue
		}

		// Convert []byte to *bls.PublicKey
		blsPK, err := bls.PublicKeyFromCompressedBytes(vdr.PublicKey)
		if err != nil {
			continue // Skip invalid public keys
		}

		// Use uncompressed bytes as the canonical key representation
		pkBytes := bls.PublicKeyToUncompressedBytes(blsPK)
		pkKey := string(pkBytes)

		// Check if we already have a validator with this public key
		if existingVdr, exists := pkToValidator[pkKey]; exists {
			// Merge validators with duplicate public keys
			existingVdr.Weight, err = math.Add(existingVdr.Weight, vdr.Weight)
			if err != nil {
				return CanonicalValidatorSet{}, fmt.Errorf("%w: %w", ErrWeightOverflow, err)
			}
			existingVdr.NodeIDs = append(existingVdr.NodeIDs, vdr.NodeID)
		} else {
			// Create new validator
			newVdr := &CanonicalValidator{
				PublicKey:      blsPK,
				PublicKeyBytes: pkBytes,
				Weight:         vdr.Weight,
				NodeIDs:        []ids.NodeID{vdr.NodeID},
			}
			pkToValidator[pkKey] = newVdr
		}
	}

	// Sort validators by public key
	vdrList := maps.Values(pkToValidator)
	utils.Sort(vdrList)
	return CanonicalValidatorSet{Validators: vdrList, TotalWeight: totalWeight}, nil
}

// FilterValidators returns the validators in [vdrs] whose bit is set to 1 in
// [indices].
//
// Returns an error if [indices] references an unknown validator.
func FilterValidators(
	indices set.Bits,
	vdrs []*CanonicalValidator,
) ([]*CanonicalValidator, error) {
	// Verify that all alleged signers exist
	if indices.BitLen() > len(vdrs) {
		return nil, fmt.Errorf(
			"%w: NumIndices (%d) >= NumFilteredValidators (%d)",
			ErrUnknownValidator,
			indices.BitLen()-1, // -1 to convert from length to index
			len(vdrs),
		)
	}

	filteredVdrs := make([]*CanonicalValidator, 0, len(vdrs))
	for i, vdr := range vdrs {
		if !indices.Contains(i) {
			continue
		}

		filteredVdrs = append(filteredVdrs, vdr)
	}
	return filteredVdrs, nil
}

// SumWeight returns the total weight of the provided validators.
func SumWeight(vdrs []*CanonicalValidator) (uint64, error) {
	var (
		weight uint64
		err    error
	)
	for _, vdr := range vdrs {
		weight, err = math.Add(weight, vdr.Weight)
		if err != nil {
			return 0, fmt.Errorf("%w: %w", ErrWeightOverflow, err)
		}
	}
	return weight, nil
}

// AggregatePublicKeys returns the public key of the provided validators.
//
// Invariant: All of the public keys in [vdrs] are valid.
func AggregatePublicKeys(vdrs []*CanonicalValidator) (*bls.PublicKey, error) {
	pks := make([]*bls.PublicKey, len(vdrs))
	for i, vdr := range vdrs {
		pks[i] = vdr.PublicKey
	}
	return bls.AggregatePublicKeys(pks)
}
