package chain

import (
	"context"
	"sync"

	"github.com/luxfi/consensus/config"
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

// Transitive implements real transitive chain consensus using Lux protocols (Photon → Wave → Focus)
type Transitive struct {
	mu sync.RWMutex

	consensus    *ChainConsensus
	params       config.Parameters
	bootstrapped bool
	ctx          context.Context
	cancel       context.CancelFunc
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
