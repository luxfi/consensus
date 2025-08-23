package core

import (
	"context"
	"github.com/luxfi/ids"
	"time"
)

// ConsensusContext provides context for consensus operations
type ConsensusContext interface {
	// Context returns the underlying Go context
	Context() context.Context

	// NodeID returns the node ID
	NodeID() ids.NodeID

	// ChainID returns the chain ID
	ChainID() ids.ID

	// Deadline returns the deadline for operations
	Deadline() time.Time
}
