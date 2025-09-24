// Package interfaces provides common consensus interfaces
package interfaces

import "github.com/luxfi/ids"

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
