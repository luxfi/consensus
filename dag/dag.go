// Package dag provides Directed Acyclic Graph implementation for consensus
package dag

import (
	"sync"
	"time"
)

// BlockID represents a unique block identifier
type BlockID [32]byte

// Block represents a block in the DAG
type Block struct {
	ID        BlockID
	Height    uint64
	Timestamp time.Time
	Parents   []BlockID
	Payload   []byte
}

// DAG represents a directed acyclic graph for block ordering
type DAG struct {
	mu     sync.RWMutex
	blocks map[BlockID]*Block
	tips   map[BlockID]struct{}
}

// New creates a new DAG
func New() *DAG {
	return &DAG{
		blocks: make(map[BlockID]*Block),
		tips:   make(map[BlockID]struct{}),
	}
}

// AddBlock adds a block to the DAG
func (d *DAG) AddBlock(block *Block) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Add block
	d.blocks[block.ID] = block

	// Update tips
	d.tips[block.ID] = struct{}{}
	for _, parent := range block.Parents {
		delete(d.tips, parent)
	}

	return nil
}

// GetBlock retrieves a block by ID
func (d *DAG) GetBlock(id BlockID) (*Block, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	block, exists := d.blocks[id]
	return block, exists
}

// GetTips returns current DAG tips
func (d *DAG) GetTips() []BlockID {
	d.mu.RLock()
	defer d.mu.RUnlock()

	tips := make([]BlockID, 0, len(d.tips))
	for tip := range d.tips {
		tips = append(tips, tip)
	}
	return tips
}
