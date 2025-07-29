// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrapper

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/utils/math"
	"github.com/luxfi/consensus/utils/sampler"
	"github.com/luxfi/consensus/utils/set"
)

var errUnexpectedSamplerFailure = errors.New("unexpected sampler failure")

// Sample keys from [elements] uniformly by weight without replacement. The
// returned set will have size less than or equal to [maxSize]. This function
// will error if the sum of all weights overflows.
func Sample[T comparable](elements map[T]uint64, maxSize int) (set.Set[T], error) {
	var (
		keys        = make([]T, len(elements))
		weights     = make([]uint64, len(elements))
		totalWeight uint64
		err         error
	)
	i := 0
	for key, weight := range elements {
		keys[i] = key
		weights[i] = weight
		totalWeight, err = math.Add(totalWeight, weight)
		if err != nil {
			return nil, err
		}
		i++
	}

	source := sampler.NewSource(0) // Use deterministic source for bootstrapping
	weightedSampler := sampler.NewWeightedWithoutReplacement(source)
	if err := weightedSampler.Initialize(weights); err != nil {
		return nil, err
	}

	maxSize = int(min(uint64(maxSize), totalWeight))
	indices, ok := weightedSampler.Sample(maxSize)
	if !ok {
		return nil, fmt.Errorf("failed to sample %d elements", maxSize)
	}

	sampledElements := set.NewSet[T](maxSize)
	for _, index := range indices {
		sampledElements.Add(keys[index])
	}
	return sampledElements, nil
}
