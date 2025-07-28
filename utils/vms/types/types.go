// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package types

// PermissionlessValidator represents a validator without permissions
type PermissionlessValidator interface {
	// NodeID returns the node ID of the validator
	NodeID() string
	
	// Weight returns the weight of the validator
	Weight() uint64
	
	// StartTime returns when the validator starts
	StartTime() uint64
	
	// EndTime returns when the validator ends
	EndTime() uint64
}