// Package block is DEPRECATED.
// Block structures belong in the node/chain repository, not consensus.
// Consensus algorithms work with abstract IDs, not specific block structures.
//
// Migration:
//
//	OLD: import "github.com/luxfi/consensus/engine/chain/block"
//	NEW: import "github.com/luxfi/node/chain/block"
package block

import "errors"

var ErrDeprecated = errors.New("block package should be in github.com/luxfi/node/chain/block")

// Deprecated: Use node's block implementation
type Block interface {
	ID() string
	Deprecated()
}
