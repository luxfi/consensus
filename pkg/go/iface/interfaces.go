package iface

import (
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// BCLookup provides blockchain lookup functionality
type BCLookup interface {
	Lookup(string) (ids.ID, error)
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
