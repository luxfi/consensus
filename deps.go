// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"context"
	
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// Heavy dependencies that should NOT be stored in context
// Use the luxfi/log and luxfi/metric packages directly instead

// ValidatorState is the interface for querying validator sets
type ValidatorState interface {
	GetMinimumHeight(ctx context.Context) (uint64, error)
	GetCurrentHeight(ctx context.Context) (uint64, error)
	GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error)
	GetValidatorSet(ctx context.Context, height uint64, subnetID ids.ID) (map[ids.NodeID]*GetValidatorOutput, error)
}

// AliasLookup provides chain alias lookups
type AliasLookup interface {
	Lookup(alias string) (ids.ID, error)
	PrimaryAlias(id ids.ID) (string, error)
}

// GetValidatorOutput contains validator information
type GetValidatorOutput struct {
	NodeID    ids.NodeID
	PublicKey *bls.PublicKey
	Weight    uint64
}

// State represents the consensus state
type State uint8

const (
	// Bootstrapping indicates the chain is bootstrapping
	Bootstrapping State = iota
	// NormalOp indicates normal operation
	NormalOp
)