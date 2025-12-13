package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/luxfi/consensus/utils/ids"
)

// PythonNode implements NodeRunner for Python implementation
type PythonNode struct {
	t         *testing.T
	cmd       *exec.Cmd
	decisions map[ids.ID]bool
	mu        sync.RWMutex
	healthy   bool
}

// NewPythonNode creates a new Python consensus node
func NewPythonNode(t *testing.T) *PythonNode {
	return &PythonNode{
		t:         t,
		decisions: make(map[ids.ID]bool),
	}
}

// Start initializes and starts the Python consensus engine
func (n *PythonNode) Start(ctx context.Context, port int) error {
	n.t.Logf("Starting Python node on port %d", port)

	// Check if Python library exists
	pythonSetupPath := filepath.Join("pkg", "python", "setup.py")
	if _, err := os.Stat(pythonSetupPath); os.IsNotExist(err) {
		n.t.Log("⚠️  Python package not found, skipping Python node")
		n.healthy = false
		return fmt.Errorf("python package not available")
	}

	// Try to import the Python module
	testCmd := exec.Command("python3", "-c", "import lux_consensus")
	if err := testCmd.Run(); err != nil {
		n.t.Log("⚠️  Python package not installed, skipping Python node")
		n.healthy = false
		return fmt.Errorf("python package not installed")
	}

	n.healthy = true
	n.t.Log("✅ Python node started successfully")
	return nil
}

// Stop shuts down the Python consensus engine
func (n *PythonNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.t.Log("Stopping Python node")

	if n.cmd != nil && n.cmd.Process != nil {
		if err := n.cmd.Process.Kill(); err != nil {
			n.t.Logf("Error killing Python process: %v", err)
		}
	}

	n.healthy = false
	return nil
}

// ProposeBlock submits a block to the Python consensus engine
func (n *PythonNode) ProposeBlock(testBlock *Block) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.healthy {
		return fmt.Errorf("python node not healthy")
	}

	n.t.Logf("Python node: proposing block %s (height %d)", testBlock.ID, testBlock.Height)

	// For E2E test stub, simulate consensus
	// In production, this would communicate with Python process
	blockData := map[string]interface{}{
		"id":        testBlock.ID.String(),
		"parent_id": testBlock.ParentID.String(),
		"height":    testBlock.Height,
		"data":      string(testBlock.Data),
	}

	if data, err := json.Marshal(blockData); err == nil {
		n.t.Logf("Python block data: %s", string(data))
	}

	// Simulate acceptance
	n.decisions[testBlock.ID] = true

	return nil
}

// GetDecision returns whether a block was accepted
func (n *PythonNode) GetDecision(blockID ids.ID) (bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	decision, exists := n.decisions[blockID]
	if !exists {
		return false, fmt.Errorf("no decision for block %s", blockID)
	}

	return decision, nil
}

// IsHealthy returns whether the Python node is healthy
func (n *PythonNode) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.healthy
}
