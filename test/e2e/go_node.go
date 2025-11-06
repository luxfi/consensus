package e2e

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/luxfi/consensus/utils/ids"
)

// GoNode implements NodeRunner for Go implementation
type GoNode struct {
	t         *testing.T
	blocks    map[ids.ID]*Block
	decisions map[ids.ID]bool
	mu        sync.RWMutex
	healthy   bool
}

// NewGoNode creates a new Go consensus node
func NewGoNode(t *testing.T) *GoNode {
	return &GoNode{
		t:         t,
		blocks:    make(map[ids.ID]*Block),
		decisions: make(map[ids.ID]bool),
	}
}

// Start initializes and starts the Go consensus engine
func (n *GoNode) Start(ctx context.Context, port int) error {
	n.t.Logf("Starting Go node on port %d", port)

	// For E2E stub, just mark as healthy
	// In production, this would create a full consensus engine
	n.healthy = true
	n.t.Log("✅ Go node started successfully (stub)")
	return nil
}

// Stop shuts down the Go consensus engine
func (n *GoNode) Stop() error {
	n.t.Log("Stopping Go node")
	n.healthy = false
	return nil
}

// ProposeBlock submits a block to the Go consensus engine
func (n *GoNode) ProposeBlock(testBlock *Block) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.t.Logf("Go node: proposing block %s (height %d)", testBlock.ID, testBlock.Height)

	// Store block
	n.blocks[testBlock.ID] = testBlock

	// For E2E test stub, simulate accepting the block
	// In production, this would go through the full consensus protocol
	n.decisions[testBlock.ID] = true

	return nil
}

// GetDecision returns whether a block was accepted
func (n *GoNode) GetDecision(blockID ids.ID) (bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	decision, exists := n.decisions[blockID]
	if !exists {
		return false, fmt.Errorf("no decision for block %s", blockID)
	}

	return decision, nil
}

// IsHealthy returns whether the Go node is healthy
func (n *GoNode) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.healthy
}
