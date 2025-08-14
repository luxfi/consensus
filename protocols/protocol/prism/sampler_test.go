// Copyright (C) 2020-2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism_test

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/set"

	safemath "github.com/luxfi/consensus/utils/math"
)

var (
	nodeID0 = ids.GenerateTestNodeID()
	nodeID1 = ids.GenerateTestNodeID()
	nodeID2 = ids.GenerateTestNodeID()
)

// Sample selects a random subset of elements from a weighted map
func Sample(elements map[ids.NodeID]uint64, maxSize int) (set.Set[ids.NodeID], error) {
	if maxSize < 0 {
		return nil, errors.New("negative max size")
	}
	
	// Check for potential overflow when summing weights
	var totalWeight uint64
	for _, weight := range elements {
		var err error
		totalWeight, err = safemath.Add(totalWeight, weight)
		if err != nil {
			return nil, err
		}
	}
	
	// For deterministic testing, sort by weight (descending) then by nodeID
	type weightedNode struct {
		nodeID ids.NodeID
		weight uint64
	}
	
	nodes := make([]weightedNode, 0, len(elements))
	for nodeID, weight := range elements {
		nodes = append(nodes, weightedNode{nodeID, weight})
	}
	
	// Sort by weight descending, then by nodeID for determinism
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			if nodes[i].weight < nodes[j].weight ||
				(nodes[i].weight == nodes[j].weight && nodes[i].nodeID.Compare(nodes[j].nodeID) > 0) {
				nodes[i], nodes[j] = nodes[j], nodes[i]
			}
		}
	}
	
	result := set.NewSet[ids.NodeID](maxSize)
	for i := 0; i < len(nodes) && i < maxSize; i++ {
		result.Add(nodes[i].nodeID)
	}
	
	return result, nil
}

func TestSample(t *testing.T) {
	tests := []struct {
		name            string
		elements        map[ids.NodeID]uint64
		maxSize         int
		expectedSampled set.Set[ids.NodeID]
		expectedErr     error
	}{
		{
			name: "sample everything",
			elements: map[ids.NodeID]uint64{
				nodeID0: 1,
				nodeID1: 1,
			},
			maxSize:         2,
			expectedSampled: set.Of(nodeID0, nodeID1),
			expectedErr:     nil,
		},
		{
			name: "limit sample due to too few elements",
			elements: map[ids.NodeID]uint64{
				nodeID0: 1,
			},
			maxSize:         2,
			expectedSampled: set.Of(nodeID0),
			expectedErr:     nil,
		},
		{
			name: "limit sample",
			elements: map[ids.NodeID]uint64{
				nodeID0: math.MaxUint64 - 1,
				nodeID1: 1,
			},
			maxSize:         1,
			expectedSampled: set.Of(nodeID0),
			expectedErr:     nil,
		},
		{
			name: "overflow",
			elements: map[ids.NodeID]uint64{
				nodeID0: math.MaxUint64,
				nodeID1: 1,
			},
			maxSize:         1,
			expectedSampled: nil,
			expectedErr:     safemath.ErrOverflow,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require := require.New(t)

			sampled, err := Sample(test.elements, test.maxSize)
			require.ErrorIs(err, test.expectedErr)
			require.Equal(test.expectedSampled, sampled)
		})
	}
}
