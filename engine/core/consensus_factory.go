// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !cgo
// +build !cgo

package core

// ConsensusFactory creates the appropriate consensus implementation
// This is the pure Go version when CGO is disabled
type ConsensusFactory struct{}

// NewConsensusFactory creates a new consensus factory
func NewConsensusFactory() *ConsensusFactory {
	return &ConsensusFactory{}
}

// CreateConsensus creates a consensus engine instance
func (f *ConsensusFactory) CreateConsensus(params ConsensusParams) (Consensus, error) {
	// Use pure Go implementation when CGO is disabled
	return &PureGoConsensus{
		params: params,
	}, nil
}
