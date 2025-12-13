package chain

import (
	"context"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/core"
	"github.com/luxfi/ids"
)

// Engine defines the chain consensus engine
type Engine interface {
	// Start starts the engine
	Start(context.Context, uint32) error

	// Stop stops the engine
	Stop(context.Context) error

	// HealthCheck performs a health check
	HealthCheck(context.Context) (interface{}, error)

	// IsBootstrapped returns whether the chain is bootstrapped
	IsBootstrapped() bool
}

// Re-export core message types
type (
	MessageType = core.MessageType
	Message     = core.Message
)

// Message type constants
const (
	PendingTxs    = core.PendingTxs
	StateSyncDone = core.StateSyncDone
)

// BlockBuilder is the interface for VMs that can build blocks
type BlockBuilder interface {
	// BuildBlock builds a new block
	BuildBlock(context.Context) (interface{}, error)
}

// Transitive implements real transitive chain consensus using Lux protocols (Photon → Wave → Focus)
type Transitive struct {
	mu sync.RWMutex

	consensus          *ChainConsensus
	params             config.Parameters
	bootstrapped       bool
	ctx                context.Context
	cancel             context.CancelFunc
	pendingBuildBlocks int
	vm                 BlockBuilder // The VM to build blocks from
}

// Transport handles message transport for consensus
type Transport[ID comparable] interface {
	// Send sends a message
	Send(ctx context.Context, to string, msg interface{}) error

	// Receive receives messages
	Receive(ctx context.Context) (interface{}, error)
}

// New creates a new chain consensus engine with real Lux consensus
func New() *Transitive {
	return NewWithParams(config.DefaultParams())
}

// NewWithParams creates an engine with specific parameters
func NewWithParams(params config.Parameters) *Transitive {
	return &Transitive{
		consensus:    NewChainConsensus(params.K, params.AlphaPreference, int(params.Beta)),
		params:       params,
		bootstrapped: false,
	}
}

// Start starts the engine
func (t *Transitive) Start(ctx context.Context, requestID uint32) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.bootstrapped = true

	return nil
}

// Stop stops the engine
func (t *Transitive) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
	}
	t.bootstrapped = false

	return nil
}

// HealthCheck performs a health check
func (t *Transitive) HealthCheck(ctx context.Context) (interface{}, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := t.consensus.Stats()
	stats["bootstrapped"] = t.bootstrapped
	stats["k"] = t.params.K
	stats["alpha"] = t.params.AlphaPreference
	stats["beta"] = t.params.Beta

	return stats, nil
}

// IsBootstrapped returns whether the chain is bootstrapped
func (t *Transitive) IsBootstrapped() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.bootstrapped
}

// AddBlock adds a block to consensus
func (t *Transitive) AddBlock(ctx context.Context, block *Block) error {
	return t.consensus.AddBlock(ctx, block)
}

// ProcessVote processes a vote for a block
func (t *Transitive) ProcessVote(ctx context.Context, blockID ids.ID, accept bool) error {
	return t.consensus.ProcessVote(ctx, blockID, accept)
}

// Poll conducts a consensus poll
func (t *Transitive) Poll(ctx context.Context, responses map[ids.ID]int) error {
	return t.consensus.Poll(ctx, responses)
}

// IsAccepted checks if a block is accepted
func (t *Transitive) IsAccepted(blockID ids.ID) bool {
	return t.consensus.IsAccepted(blockID)
}

// Preference returns the current preferred block
func (t *Transitive) Preference() ids.ID {
	return t.consensus.Preference()
}

// GetBlock gets a block by ID
func (t *Transitive) GetBlock(ctx context.Context, nodeID ids.NodeID, requestID uint32, blockID ids.ID) error {
	// In the real implementation, this would fetch the block and add to consensus
	// For now, just return success
	return nil
}

// SetVM sets the block builder (VM) for the engine
func (t *Transitive) SetVM(vm BlockBuilder) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.vm = vm
}

// Notify handles VM notifications (e.g., pending transactions)
func (t *Transitive) Notify(ctx context.Context, msg Message) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	switch msg.Type {
	case PendingTxs:
		t.pendingBuildBlocks++
		return t.buildBlocksLocked(ctx)
	case StateSyncDone:
		// State sync completed, nothing to do
		return nil
	}
	return nil
}

// buildBlocksLocked builds pending blocks (must be called with lock held)
func (t *Transitive) buildBlocksLocked(ctx context.Context) error {
	if t.vm == nil {
		return nil
	}

	// Build blocks until we have no more pending
	for t.pendingBuildBlocks > 0 {
		t.pendingBuildBlocks--

		blk, err := t.vm.BuildBlock(ctx)
		if err != nil {
			// Failed to build block, but this is not fatal
			// The VM might not have any transactions ready
			return nil
		}

		// Block was built successfully
		// In a full implementation, we would:
		// 1. Add the block to consensus
		// 2. Send it to peers for voting
		// For now, we just note that a block was built
		_ = blk
	}
	return nil
}

// PendingBuildBlocks returns the number of pending block builds
func (t *Transitive) PendingBuildBlocks() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pendingBuildBlocks
}
