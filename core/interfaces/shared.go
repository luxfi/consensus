// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

// Package interfaces defines core consensus interfaces.
package interfaces

import (
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// BCLookup provides blockchain lookup functionality
type BCLookup interface {
	Lookup(alias string) (ids.ID, error)
	GetAlias(blockchainID ids.ID) (string, error)
	GetBlockchainID(alias string) (ids.ID, error)
	PrimaryAlias(blockchainID ids.ID) (string, error)
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
