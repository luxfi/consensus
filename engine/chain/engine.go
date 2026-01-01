package chain

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// Engine defines the chain consensus engine
type Engine interface {
	// Start starts the engine
	Start(context.Context, bool) error

	// StopWithError stops the engine with an error
	StopWithError(context.Context, error) error

	// Context returns the engine's context
	Context() context.Context

	// HealthCheck performs a health check
	HealthCheck(context.Context) (interface{}, error)

	// IsBootstrapped returns whether the chain is bootstrapped
	IsBootstrapped() bool
}

// Re-export message types for convenience
type (
	MessageType = engine.MessageType
	Message     = engine.Message
)

const (
	PendingTxs    = engine.PendingTxs
	StateSyncDone = engine.StateSyncDone
)

// BlockBuilder is the minimal interface consensus needs from a VM.
// It provides the four core operations required for block consensus:
// building, fetching, parsing, and tracking the chain head.
//
// This interface is intentionally small - VMs may implement the full
// block.ChainVM interface, but consensus only depends on these methods.
type BlockBuilder interface {
	// BuildBlock creates a new block from pending transactions.
	// Returns an error if no transactions are pending or block creation fails.
	BuildBlock(context.Context) (block.Block, error)

	// GetBlock retrieves a block by its ID.
	// Returns an error if the block is not found.
	GetBlock(context.Context, ids.ID) (block.Block, error)

	// ParseBlock deserializes a block from its byte representation.
	// Used when receiving blocks from the network.
	ParseBlock(context.Context, []byte) (block.Block, error)

	// LastAccepted returns the ID of the most recently accepted block.
	// This is the current chain head from the VM's perspective.
	LastAccepted(context.Context) (ids.ID, error)
}

// PendingBlock tracks a block pending consensus decision
type PendingBlock struct {
	ConsensusBlock *Block      // Block in consensus
	VMBlock        block.Block // Original VM block (for Accept/Reject callbacks)
	ProposedAt     time.Time   // When block was proposed
	VoteCount      int         // Votes received
	Decided        bool        // Whether consensus decision was made
}

// BlockProposal contains all data needed to propose a block for consensus.
// This is what consensus produces; how it reaches validators is not its concern.
type BlockProposal struct {
	BlockID   ids.ID
	BlockData []byte
	Height    uint64
	ParentID  ids.ID
}

// VoteRequest is a request for specific validators to vote on a block.
// Used when consensus needs targeted vote collection (e.g., missing votes).
type VoteRequest struct {
	BlockID    ids.ID
	Validators []ids.NodeID
}

// Vote represents a validator's decision on a block.
type Vote struct {
	BlockID  ids.ID
	NodeID   ids.NodeID
	Accept   bool
	SignedAt time.Time
}

// BlockProposer is the interface consensus uses to propose blocks.
// Consensus expresses WHAT it wants (propose block, request votes);
// the implementation decides HOW (gossip, direct send, Warp, etc.).
//
// This separation enables:
//   - Testing consensus without network
//   - Swapping transport layers (P2P, Warp, local)
//   - Composing with different network topologies
//
// Design principles:
//   - Small interface (2 methods)
//   - Intent-based naming (Propose, not Emit/Broadcast)
//   - No network terminology in consensus layer
type BlockProposer interface {
	// Propose submits a block for validators to vote on.
	// Returns nil if the proposal was accepted for delivery.
	// The actual delivery mechanism is implementation-defined.
	Propose(ctx context.Context, proposal BlockProposal) error

	// RequestVotes asks specific validators to vote on a block.
	// Used for vote collection when some validators haven't responded.
	// Returns nil if the request was accepted for delivery.
	RequestVotes(ctx context.Context, req VoteRequest) error
}

// VoteEmitter is deprecated. Use BlockProposer instead.
// Kept for backward compatibility during migration.
//
// Deprecated: Use BlockProposer interface.
type VoteEmitter = BlockProposer

// Transitive implements chain consensus using Lux protocols.
// It coordinates block building, proposal, voting, and finalization.
type Transitive struct {
	mu sync.RWMutex

	consensus          *ChainConsensus
	params             config.Parameters
	bootstrapped       bool
	ctx                context.Context
	cancel             context.CancelFunc
	pendingBuildBlocks int
	vm                 BlockBuilder // The VM to build blocks from

	// Pending blocks awaiting consensus decision
	pendingBlocks map[ids.ID]*PendingBlock

	// Vote handling channels
	voteRequests  chan VoteRequest
	votes         chan Vote

	// Block proposer for submitting blocks to validators.
	// Consensus expresses intent; proposer handles delivery.
	proposer BlockProposer

	// Metrics for tracking consensus progress
	blocksBuilt    uint64
	blocksAccepted uint64
	blocksRejected uint64
	votesSent      uint64
	votesReceived  uint64
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
		consensus:     NewChainConsensus(params.K, params.AlphaPreference, int(params.Beta)),
		params:        params,
		bootstrapped:  false,
		pendingBlocks: make(map[ids.ID]*PendingBlock),
		voteRequests:  make(chan VoteRequest, 100),
		votes:         make(chan Vote, 1000),
	}
}

// SetProposer sets the block proposer for delivering proposals to validators.
func (t *Transitive) SetProposer(proposer BlockProposer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.proposer = proposer
}

// SetEmitter is deprecated. Use SetProposer instead.
//
// Deprecated: Use SetProposer.
func (t *Transitive) SetEmitter(emitter VoteEmitter) {
	t.SetProposer(emitter)
}

// Start starts the engine.
func (t *Transitive) Start(ctx context.Context, startReqID bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.bootstrapped = true

	// Start the consensus polling loop
	go t.consensusPollLoop()

	// Start the vote handler
	go t.voteHandler()

	return nil
}

// consensusPollLoop runs the Wave → Focus consensus phases
// It periodically polls pending blocks and finalizes accepted ones
func (t *Transitive) consensusPollLoop() {
	ticker := time.NewTicker(50 * time.Millisecond) // Fast polling for low latency
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.processPendingBlocks()
		}
	}
}

// processPendingBlocks checks all pending blocks for consensus decisions
func (t *Transitive) processPendingBlocks() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for blockID, pending := range t.pendingBlocks {
		if pending.Decided {
			continue
		}

		// Check if consensus has been reached (Focus convergence)
		if t.consensus.IsAccepted(blockID) {
			pending.Decided = true
			t.blocksAccepted++

			// Call Accept on the VM block - this finalizes it in the chain
			if pending.VMBlock != nil {
				if err := pending.VMBlock.Accept(t.ctx); err != nil {
					// Log error but continue - the consensus decision was made
					fmt.Printf("warning: failed to accept VM block %s: %v\n", blockID, err)
				}
			}

			// Remove from pending (keep for a bit for late votes)
			delete(t.pendingBlocks, blockID)
		} else if t.consensus.IsRejected(blockID) {
			pending.Decided = true
			t.blocksRejected++

			// Call Reject on the VM block
			if pending.VMBlock != nil {
				if err := pending.VMBlock.Reject(t.ctx); err != nil {
					fmt.Printf("warning: failed to reject VM block %s: %v\n", blockID, err)
				}
			}

			delete(t.pendingBlocks, blockID)
		}
	}
}

// voteHandler processes incoming votes from validators.
func (t *Transitive) voteHandler() {
	for {
		select {
		case <-t.ctx.Done():
			return
		case vote := <-t.votes:
			t.handleVote(vote)
		}
	}
}

// handleVote processes a single vote from a validator.
func (t *Transitive) handleVote(vote Vote) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.votesReceived++

	// Record the vote in consensus
	if err := t.consensus.ProcessVote(t.ctx, vote.BlockID, vote.Accept); err != nil {
		// Block might not exist yet - that's OK for late votes
		return
	}

	// Update pending block vote count
	if pending, ok := t.pendingBlocks[vote.BlockID]; ok {
		pending.VoteCount++

		// Build poll response for consensus
		responses := map[ids.ID]int{vote.BlockID: pending.VoteCount}
		if err := t.consensus.Poll(t.ctx, responses); err != nil {
			return
		}
	}
}

// StartWithID starts the engine with a specific request ID
func (t *Transitive) StartWithID(ctx context.Context, requestID uint32) error {
	return t.Start(ctx, requestID > 0)
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

// StopWithError stops the engine with an error (for graceful shutdown on errors)
func (t *Transitive) StopWithError(ctx context.Context, err error) error {
	return t.Stop(ctx)
}

// Context returns the engine's context
func (t *Transitive) Context() context.Context {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.ctx == nil {
		return context.Background()
	}
	return t.ctx
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

// buildBlocksLocked builds pending blocks and submits them to consensus.
// Must be called with lock held.
func (t *Transitive) buildBlocksLocked(ctx context.Context) error {
	if t.vm == nil {
		return nil
	}

	// Build blocks until we have no more pending
	for t.pendingBuildBlocks > 0 {
		t.pendingBuildBlocks--

		vmBlock, err := t.vm.BuildBlock(ctx)
		if err != nil {
			// Failed to build block, but this is not fatal
			// The VM might not have any transactions ready
			return nil
		}

		t.blocksBuilt++

		// Create a consensus block from the VM block
		consensusBlock := &Block{
			id:        vmBlock.ID(),
			parentID:  vmBlock.ParentID(),
			height:    vmBlock.Height(),
			timestamp: vmBlock.Timestamp().Unix(),
			data:      vmBlock.Bytes(),
		}

		// Add the block to consensus for voting
		if err := t.consensus.AddBlock(ctx, consensusBlock); err != nil {
			// Block might already exist (reorg or duplicate notification)
			continue
		}

		// Track as pending - waiting for consensus decision
		t.pendingBlocks[vmBlock.ID()] = &PendingBlock{
			ConsensusBlock: consensusBlock,
			VMBlock:        vmBlock,
			ProposedAt:     time.Now(),
			VoteCount:      0,
			Decided:        false,
		}

		// Propose block to validators for voting
		if t.proposer != nil {
			proposal := BlockProposal{
				BlockID:   vmBlock.ID(),
				BlockData: vmBlock.Bytes(),
				Height:    vmBlock.Height(),
				ParentID:  vmBlock.ParentID(),
			}
			if err := t.proposer.Propose(ctx, proposal); err != nil {
				// Failed to propose, but block is still tracked locally
				fmt.Printf("warning: failed to propose block %s: %v\n", vmBlock.ID(), err)
			}
		}
	}
	return nil
}

// ReceiveVote receives a vote from a validator.
// This is called by the network layer when a vote arrives.
func (t *Transitive) ReceiveVote(vote Vote) {
	select {
	case t.votes <- vote:
	default:
		// Channel full, drop vote (it will be resent)
	}
}

// Stats returns engine statistics including consensus metrics
func (t *Transitive) Stats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := t.consensus.Stats()
	stats["blocks_built"] = t.blocksBuilt
	stats["blocks_accepted"] = t.blocksAccepted
	stats["blocks_rejected"] = t.blocksRejected
	stats["votes_sent"] = t.votesSent
	stats["votes_received"] = t.votesReceived
	stats["pending_blocks"] = len(t.pendingBlocks)
	stats["bootstrapped"] = t.bootstrapped

	return stats
}

// PendingBuildBlocks returns the number of pending block builds
func (t *Transitive) PendingBuildBlocks() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pendingBuildBlocks
}
