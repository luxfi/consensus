package e2e

import (
	"context"
	"os/exec"
	"testing"

	"github.com/luxfi/consensus/utils/ids"
)

// Block represents a consensus block shared across all implementations
type Block struct {
	ID       ids.ID
	ParentID ids.ID
	Height   uint64
	Data     []byte
}

// NodeRunner interface for different language implementations
type NodeRunner interface {
	Start(ctx context.Context, port int) error
	Stop() error
	ProposeBlock(block *Block) error
	GetDecision(blockID ids.ID) (bool, error)
	IsHealthy() bool
}

// checkBuild attempts to run a build command and returns true if successful
func checkBuild(t *testing.T, lang string, buildCmd []string) bool {
	t.Logf("Building %s implementation: %v", lang, buildCmd)
	//nolint:gosec // buildCmd is from test configuration, not user input
	cmd := exec.Command(buildCmd[0], buildCmd[1:]...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Logf("⚠️  %s build failed: %v\n%s", lang, err, output)
		return false
	}
	return true
}
