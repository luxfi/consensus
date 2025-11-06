package e2e

import (
	"context"

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
