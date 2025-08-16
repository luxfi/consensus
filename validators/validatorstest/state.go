// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validatorstest

import (
	"context"
	"testing"

	"github.com/luxfi/consensus/validators"
	"github.com/luxfi/ids"
)

// State is a mock validator state for testing
type State struct {
	T *testing.T

	CantGetMinimumHeight,
	CantGetCurrentHeight,
	CantGetSubnetID,
	CantGetValidatorSet,
	CantGetCurrentValidatorSet bool

	GetMinimumHeightF func(context.Context) (uint64, error)
	GetCurrentHeightF func(context.Context) (uint64, error)
	GetSubnetIDF      func(context.Context, ids.ID) (ids.ID, error)
	GetValidatorSetF  func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error)
	GetCurrentValidatorSetF func(context.Context, ids.ID) (map[ids.ID]*validators.GetCurrentValidatorOutput, uint64, error)
}

func (s *State) GetMinimumHeight(ctx context.Context) (uint64, error) {
	if s.GetMinimumHeightF != nil {
		return s.GetMinimumHeightF(ctx)
	}
	if s.CantGetMinimumHeight && s.T != nil {
		s.T.Fatal("unexpected GetMinimumHeight")
	}
	return 0, nil
}

func (s *State) GetCurrentHeight(ctx context.Context) (uint64, error) {
	if s.GetCurrentHeightF != nil {
		return s.GetCurrentHeightF(ctx)
	}
	if s.CantGetCurrentHeight && s.T != nil {
		s.T.Fatal("unexpected GetCurrentHeight")
	}
	return 0, nil
}

func (s *State) GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error) {
	if s.GetSubnetIDF != nil {
		return s.GetSubnetIDF(ctx, chainID)
	}
	if s.CantGetSubnetID && s.T != nil {
		s.T.Fatal("unexpected GetSubnetID")
	}
	return ids.Empty, nil
}

func (s *State) GetValidatorSet(ctx context.Context, height uint64, subnetID ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
	if s.GetValidatorSetF != nil {
		return s.GetValidatorSetF(ctx, height, subnetID)
	}
	if s.CantGetValidatorSet && s.T != nil {
		s.T.Fatal("unexpected GetValidatorSet")
	}
	return nil, nil
}

func (s *State) GetCurrentValidatorSet(ctx context.Context, subnetID ids.ID) (map[ids.ID]*validators.GetCurrentValidatorOutput, uint64, error) {
	if s.GetCurrentValidatorSetF != nil {
		return s.GetCurrentValidatorSetF(ctx, subnetID)
	}
	if s.CantGetCurrentValidatorSet && s.T != nil {
		s.T.Fatal("unexpected GetCurrentValidatorSet")
	}
	return nil, 0, nil
}
