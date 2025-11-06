// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// Package interfaces defines core consensus interfaces

package interfaces

import (
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// BCLookup provides blockchain lookup functionality
type BCLookup interface {
	// Lookup returns the blockchain ID for an alias
	Lookup(alias string) (ids.ID, error)
	// GetAlias returns the alias for a blockchain ID
	GetAlias(blockchainID ids.ID) (string, error)
	// GetBlockchainID returns the blockchain ID for an alias
	GetBlockchainID(alias string) (ids.ID, error)
	// PrimaryAlias returns the primary alias for a blockchain
	PrimaryAlias(blockchainID ids.ID) (string, error)
	// Aliases returns all aliases for a blockchain
	Aliases(blockchainID ids.ID) ([]string, error)
}

// SharedMemory provides shared memory functionality
type SharedMemory interface {
	GetDatabase(id ids.ID) (*VersionedDatabase, database.Database)
	ReleaseDatabase(id ids.ID) error
}

// VersionedDatabase provides versioned database access
type VersionedDatabase struct {
	Lock   database.Database
	Memory database.Database
}

// StateHolder holds state information
type StateHolder interface {
	// GetState returns the current state
	GetState() StateEnum
}

// StateEnum represents consensus state
type StateEnum uint8

const (
	// StateSyncing indicates the node is syncing state
	StateSyncing StateEnum = iota
	// Bootstrapping indicates the node is bootstrapping
	Bootstrapping
	// NormalOp indicates normal operation
	NormalOp
)
