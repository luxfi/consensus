// Package chain provides chain consensus implementations
package chain

// Chain represents a blockchain
type Chain interface {
	// GetHeight returns the current height of the chain
	GetHeight() uint64
	
	// GetBlockByHeight returns a block at a specific height
	GetBlockByHeight(height uint64) (Block, error)
	
	// AddBlock adds a new block to the chain
	AddBlock(block Block) error
}

// Block represents a block in the chain
type Block interface {
	// ID returns the block's unique identifier
	ID() string
	
	// Height returns the block's height
	Height() uint64
	
	// Parent returns the parent block's ID
	Parent() string
}