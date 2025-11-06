package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"testing"

	"github.com/luxfi/consensus/utils/ids"
)

// CppNode implements NodeRunner for C++ implementation
type CppNode struct {
	t         *testing.T
	cmd       *exec.Cmd
	decisions map[ids.ID]bool
	mu        sync.RWMutex
	healthy   bool
}

// NewCppNode creates a new C++ consensus node
func NewCppNode(t *testing.T) *CppNode {
	return &CppNode{
		t:         t,
		decisions: make(map[ids.ID]bool),
	}
}

// Start initializes and starts the C++ consensus engine
func (n *CppNode) Start(ctx context.Context, port int) error {
	n.t.Logf("Starting C++ node on port %d", port)

	// Check if C++ binary exists
	if !checkBuild(n.t, "C++", []string{"cmake", "--build", "pkg/cpp/build"}) {
		n.t.Log("⚠️  C++ binary not built, skipping C++ node")
		n.healthy = false
		return fmt.Errorf("C++ binary not available")
	}

	// For now, just mark as healthy - actual IPC would be implemented here
	n.healthy = true
	n.t.Log("✅ C++ node started successfully (stub)")
	return nil
}

// Stop shuts down the C++ consensus engine
func (n *CppNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.t.Log("Stopping C++ node")

	if n.cmd != nil && n.cmd.Process != nil {
		if err := n.cmd.Process.Kill(); err != nil {
			n.t.Logf("Error killing C++ process: %v", err)
		}
	}

	n.healthy = false
	return nil
}

// ProposeBlock submits a block to the C++ consensus engine
func (n *CppNode) ProposeBlock(testBlock *Block) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.healthy {
		return fmt.Errorf("C++ node not healthy")
	}

	n.t.Logf("C++ node: proposing block %s (height %d)", testBlock.ID, testBlock.Height)

	// For E2E test stub, simulate consensus
	// In production, this would send to C++ process via IPC
	blockData := map[string]interface{}{
		"id":        testBlock.ID.String(),
		"parent_id": testBlock.ParentID.String(),
		"height":    testBlock.Height,
		"data":      string(testBlock.Data),
	}

	if data, err := json.Marshal(blockData); err == nil {
		n.t.Logf("C++ block data: %s", string(data))
	}

	// Simulate acceptance
	n.decisions[testBlock.ID] = true

	return nil
}

// GetDecision returns whether a block was accepted
func (n *CppNode) GetDecision(blockID ids.ID) (bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	decision, exists := n.decisions[blockID]
	if !exists {
		return false, fmt.Errorf("no decision for block %s", blockID)
	}

	return decision, nil
}

// IsHealthy returns whether the C++ node is healthy
func (n *CppNode) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.healthy
}
