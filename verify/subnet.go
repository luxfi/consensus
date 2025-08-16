// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package verify

import (
	"context"
	"errors"

	"github.com/luxfi/consensus"
	"github.com/luxfi/ids"
)

var (
	ErrSameChainID     = errors.New("same chain ID")
	ErrDifferentSubnet = errors.New("different subnet")
)

// Verifier performs consensus verification
type Verifier struct {
	VS consensus.ValidatorState
}

// SameSubnet verifies that a peer chain is in the same subnet
func (v Verifier) SameSubnet(ctx *consensus.Context, peer ids.ID) error {
	localChain := ctx.ChainID
	localSubnet := ctx.SubnetID

	if peer == localChain {
		return ErrSameChainID
	}

	peerSubnet, err := v.VS.GetSubnetID(peer)
	if err != nil {
		return err
	}

	if peerSubnet != localSubnet {
		return ErrDifferentSubnet
	}

	return nil
}
