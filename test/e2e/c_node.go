package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/luxfi/consensus/utils/ids"
)

// CNode implements NodeRunner for C implementation
// Currently uses stub implementation to avoid CGo build dependencies
type CNode struct {
	t         *testing.T
	decisions map[ids.ID]bool
	mu        sync.RWMutex
	healthy   bool
}

// NewCNode creates a new C consensus node
func NewCNode(t *testing.T) *CNode {
	return &CNode{
		t:         t,
		decisions: make(map[ids.ID]bool),
	}
}

// Start initializes and starts the C consensus engine
func (n *CNode) Start(ctx context.Context, port int) error {
	n.t.Logf("Starting C node on port %d", port)

	// Check if C library is built
	cLibPath := filepath.Join("pkg", "c", "build", "liblux_consensus.a")
	if _, err := os.Stat(cLibPath); os.IsNotExist(err) {
		n.t.Log("⚠️  C library not built, using stub implementation")
		n.healthy = true // Still mark as healthy for stub
		return nil
	}

	n.healthy = true
	n.t.Log("✅ C node started successfully (stub)")
	return nil
}

// Stop shuts down the C consensus engine
func (n *CNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.t.Log("Stopping C node")
	n.healthy = false
	return nil
}

// ProposeBlock submits a block to the C consensus engine
func (n *CNode) ProposeBlock(testBlock *Block) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.healthy {
		return fmt.Errorf("C node not healthy")
	}

	n.t.Logf("C node: proposing block %s (height %d)", testBlock.ID, testBlock.Height)

	// For E2E test stub, simulate consensus
	// In production with CGo, this would call C library functions
	blockData := map[string]interface{}{
		"id":        testBlock.ID.String(),
		"parent_id": testBlock.ParentID.String(),
		"height":    testBlock.Height,
		"data":      string(testBlock.Data),
	}

	if data, err := json.Marshal(blockData); err == nil {
		n.t.Logf("C block data: %s", string(data))
	}

	// Simulate acceptance (would use C consensus logic in production)
	n.decisions[testBlock.ID] = true

	return nil
}

// GetDecision returns whether a block was accepted
func (n *CNode) GetDecision(blockID ids.ID) (bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	decision, exists := n.decisions[blockID]
	if !exists {
		return false, fmt.Errorf("no decision for block %s", blockID)
	}

	return decision, nil
}

// IsHealthy returns whether the C node is healthy
func (n *CNode) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.healthy
}
