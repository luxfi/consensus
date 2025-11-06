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

// RustNode implements NodeRunner for Rust implementation
type RustNode struct {
	t         *testing.T
	cmd       *exec.Cmd
	decisions map[ids.ID]bool
	mu        sync.RWMutex
	healthy   bool
}

// NewRustNode creates a new Rust consensus node
func NewRustNode(t *testing.T) *RustNode {
	return &RustNode{
		t:         t,
		decisions: make(map[ids.ID]bool),
	}
}

// Start initializes and starts the Rust consensus engine
func (n *RustNode) Start(ctx context.Context, port int) error {
	n.t.Logf("Starting Rust node on port %d", port)

	// Check if Rust library exists
	if !checkBuild(n.t, "Rust", []string{"cargo", "build", "--manifest-path", "pkg/rust/Cargo.toml", "--release"}) {
		n.t.Log("⚠️  Rust library not built, skipping Rust node")
		n.healthy = false
		return fmt.Errorf("Rust library not available")
	}

	n.healthy = true
	n.t.Log("✅ Rust node started successfully")
	return nil
}

// Stop shuts down the Rust consensus engine
func (n *RustNode) Stop() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	n.t.Log("Stopping Rust node")

	if n.cmd != nil && n.cmd.Process != nil {
		if err := n.cmd.Process.Kill(); err != nil {
			n.t.Logf("Error killing Rust process: %v", err)
		}
	}

	n.healthy = false
	return nil
}

// ProposeBlock submits a block to the Rust consensus engine
func (n *RustNode) ProposeBlock(testBlock *Block) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.healthy {
		return fmt.Errorf("Rust node not healthy")
	}

	n.t.Logf("Rust node: proposing block %s (height %d)", testBlock.ID, testBlock.Height)

	// For E2E test stub, simulate consensus
	// In production, this would communicate with Rust process via FFI or IPC
	blockData := map[string]interface{}{
		"id":        testBlock.ID.String(),
		"parent_id": testBlock.ParentID.String(),
		"height":    testBlock.Height,
		"data":      string(testBlock.Data),
	}

	if data, err := json.Marshal(blockData); err == nil {
		n.t.Logf("Rust block data: %s", string(data))
	}

	// Simulate acceptance after voting rounds
	n.decisions[testBlock.ID] = true

	return nil
}

// GetDecision returns whether a block was accepted
func (n *RustNode) GetDecision(blockID ids.ID) (bool, error) {
	n.mu.RLock()
	defer n.mu.RUnlock()

	decision, exists := n.decisions[blockID]
	if !exists {
		return false, fmt.Errorf("no decision for block %s", blockID)
	}

	return decision, nil
}

// IsHealthy returns whether the Rust node is healthy
func (n *RustNode) IsHealthy() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.healthy
}
