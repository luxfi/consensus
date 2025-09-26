// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo
// +build cgo

package core

// ConsensusFactory creates the appropriate consensus implementation
// This is the CGO version when CGO is enabled
type ConsensusFactory struct{}

// NewConsensusFactory creates a new consensus factory
func NewConsensusFactory() *ConsensusFactory {
	return &ConsensusFactory{}
}

// CreateConsensus creates a consensus engine instance
func (f *ConsensusFactory) CreateConsensus(params ConsensusParams) (Consensus, error) {
	// Use the existing consensus implementation
	return NewCGOConsensus(params)
}
