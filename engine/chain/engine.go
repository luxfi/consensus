package chain

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
)

// -----------------------------------------------------------------------------
// Errors
// -----------------------------------------------------------------------------

var (
	ErrNotStarted     = errors.New("engine not started")
	ErrAlreadyStarted = errors.New("engine already started")
)

// -----------------------------------------------------------------------------
// Interfaces
// -----------------------------------------------------------------------------

// Engine is the chain consensus engine interface.
type Engine interface {
	Start(context.Context, bool) error
	StopWithError(context.Context, error) error
	Context() context.Context
	HealthCheck(context.Context) (interface{}, error)
	IsBootstrapped() bool
}

// BlockBuilder is the minimal interface consensus needs from a VM.
// Intentionally small: VMs may implement more, but consensus only needs these.
type BlockBuilder interface {
	BuildBlock(context.Context) (block.Block, error)
	GetBlock(context.Context, ids.ID) (block.Block, error)
	ParseBlock(context.Context, []byte) (block.Block, error)
	LastAccepted(context.Context) (ids.ID, error)
	// SetPreference tells the VM which block to build on next.
	// This MUST be called after accepting a block to keep the VM's preferred
	// block in sync with the last accepted block. Without this, the VM's
	// Preferred() returns the old block while GetLastAccepted() returns the
	// new block, causing GetState(preferred) to fail during block building.
	SetPreference(context.Context, ids.ID) error
}

// BlockProposer submits blocks to validators.
// Consensus expresses WHAT (propose block); implementation decides HOW.
type BlockProposer interface {
	Propose(ctx context.Context, proposal BlockProposal) error
	RequestVotes(ctx context.Context, req VoteRequest) error
}

// VoteEmitter is deprecated. Use BlockProposer.
type VoteEmitter = BlockProposer

// -----------------------------------------------------------------------------
// Message types
// -----------------------------------------------------------------------------

type (
	MessageType = engine.MessageType
	Message     = engine.Message
)

const (
	PendingTxs    = engine.PendingTxs
	StateSyncDone = engine.StateSyncDone
)

// -----------------------------------------------------------------------------
// Protocol types
// -----------------------------------------------------------------------------

// BlockProposal contains data needed to propose a block.
type BlockProposal struct {
	BlockID   ids.ID
	BlockData []byte
	Height    uint64
	ParentID  ids.ID
}

// VoteRequest asks specific validators to vote.
type VoteRequest struct {
	BlockID    ids.ID
	Validators []ids.NodeID
}

// Vote represents a validator's decision.
type Vote struct {
	BlockID  ids.ID
	NodeID   ids.NodeID
	Accept   bool
	SignedAt time.Time
}

// PendingBlock tracks a block awaiting consensus.
type PendingBlock struct {
	ConsensusBlock *Block
	VMBlock        block.Block
	ProposedAt     time.Time
	VoteCount      int
	Decided        bool
}

// -----------------------------------------------------------------------------
// Configuration
// -----------------------------------------------------------------------------

// Config holds engine dependencies and settings.
type Config struct {
	Params   config.Parameters
	VM       BlockBuilder
	Proposer BlockProposer

	// Channel buffer sizes (defaults applied if zero)
	VoteRequestBuffer int
	VoteBuffer        int
}

// DefaultConfig returns sensible defaults.
func DefaultConfig() Config {
	return Config{
		Params:            config.DefaultParams(),
		VoteRequestBuffer: 100,
		VoteBuffer:        1000,
	}
}

// Validate checks config validity.
func (c Config) Validate() error {
	return c.Params.Valid()
}

// -----------------------------------------------------------------------------
// Functional options
// -----------------------------------------------------------------------------

// Option configures the engine.
type Option func(*Transitive)

// WithParams sets consensus parameters.
func WithParams(p config.Parameters) Option {
	return func(t *Transitive) {
		t.params = p
		t.consensus = NewChainConsensus(p.K, p.AlphaPreference, int(p.Beta))
	}
}

// WithVM sets the block builder.
func WithVM(vm BlockBuilder) Option {
	return func(t *Transitive) {
		t.vm = vm
	}
}

// WithProposer sets the block proposer.
func WithProposer(p BlockProposer) Option {
	return func(t *Transitive) {
		t.proposer = p
	}
}

// WithEmitter is deprecated. Use WithProposer.
func WithEmitter(e BlockProposer) Option {
	return WithProposer(e)
}

// WithVoteBuffers sets channel buffer sizes.
func WithVoteBuffers(requests, votes int) Option {
	return func(t *Transitive) {
		if requests > 0 {
			t.voteRequests = make(chan VoteRequest, requests)
		}
		if votes > 0 {
			t.votes = make(chan Vote, votes)
		}
	}
}

// -----------------------------------------------------------------------------
// Transitive consensus engine
// -----------------------------------------------------------------------------

// Transitive implements chain consensus using Lux protocols.
//
// Construction:
//
//	New()                              // defaults
//	New(WithVM(vm), WithProposer(p))   // with options
//	NewWithConfig(cfg)                 // explicit config
//	NewWithConfig(cfg, WithVM(vm))     // config + option overrides
//
// Lifecycle: New -> Start -> (running) -> Stop
type Transitive struct {
	mu sync.RWMutex

	// Core consensus
	consensus *ChainConsensus
	params    config.Parameters

	// Dependencies
	vm       BlockBuilder
	proposer BlockProposer

	// Runtime state
	ctx          context.Context
	cancel       context.CancelFunc
	bootstrapped bool
	started      bool

	// Block management
	pendingBlocks      map[ids.ID]*PendingBlock
	pendingBuildBlocks int

	// Vote channels
	voteRequests chan VoteRequest
	votes        chan Vote

	// Metrics
	blocksBuilt    uint64
	blocksAccepted uint64
	blocksRejected uint64
	votesSent      uint64
	votesReceived  uint64
}

// New creates an engine with default parameters.
// Apply options to configure dependencies.
//
// Example:
//
//	engine := New(WithVM(vm), WithProposer(proposer))
func New(opts ...Option) *Transitive {
	return NewWithConfig(DefaultConfig(), opts...)
}

// NewWithConfig creates an engine from explicit config plus options.
// Options are applied after config, allowing overrides.
//
// Example:
//
//	cfg := Config{Params: config.MainnetParams(), VM: vm}
//	engine := NewWithConfig(cfg, WithProposer(proposer))
func NewWithConfig(cfg Config, opts ...Option) *Transitive {
	if cfg.VoteRequestBuffer == 0 {
		cfg.VoteRequestBuffer = 100
	}
	if cfg.VoteBuffer == 0 {
		cfg.VoteBuffer = 1000
	}

	t := &Transitive{
		consensus:     NewChainConsensus(cfg.Params.K, cfg.Params.AlphaPreference, int(cfg.Params.Beta)),
		params:        cfg.Params,
		vm:            cfg.VM,
		proposer:      cfg.Proposer,
		pendingBlocks: make(map[ids.ID]*PendingBlock),
		voteRequests:  make(chan VoteRequest, cfg.VoteRequestBuffer),
		votes:         make(chan Vote, cfg.VoteBuffer),
	}

	for _, opt := range opts {
		opt(t)
	}

	return t
}

// NewWithParams creates an engine with specific parameters.
// Deprecated: Use NewWithConfig or New(WithParams(p)).
func NewWithParams(params config.Parameters) *Transitive {
	cfg := DefaultConfig()
	cfg.Params = params
	return NewWithConfig(cfg)
}

// -----------------------------------------------------------------------------
// Lifecycle
// -----------------------------------------------------------------------------

// Start starts the engine.
func (t *Transitive) Start(ctx context.Context, _ bool) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.started {
		return ErrAlreadyStarted
	}

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.bootstrapped = true
	t.started = true

	// Capture ctx in local variable to avoid race with struct field access
	engineCtx := t.ctx
	go t.pollLoopWithCtx(engineCtx)
	go t.voteHandlerWithCtx(engineCtx)

	return nil
}

// StartWithID starts with a request ID.
func (t *Transitive) StartWithID(ctx context.Context, requestID uint32) error {
	return t.Start(ctx, requestID > 0)
}

// Stop stops the engine.
func (t *Transitive) Stop(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.cancel != nil {
		t.cancel()
	}
	t.bootstrapped = false
	t.started = false
	return nil
}

// StopWithError stops with an error.
func (t *Transitive) StopWithError(ctx context.Context, _ error) error {
	return t.Stop(ctx)
}

// Context returns the engine's context.
func (t *Transitive) Context() context.Context {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.ctx == nil {
		return context.Background()
	}
	return t.ctx
}

// IsBootstrapped returns bootstrap status.
func (t *Transitive) IsBootstrapped() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.bootstrapped
}

// SyncState synchronizes consensus state with VM state.
// This is called by the syncer after RLP import or state sync to reconcile
// the consensus engine's lastAccepted pointer with the VM's actual state.
//
// This method:
//  1. Updates the consensus finalizedTip to match the VM's last accepted block
//  2. Clears any stale pending blocks that conflict with the new chain tip
//  3. Marks the engine as bootstrapped
//
// This is safe to call multiple times - it's idempotent.
func (t *Transitive) SyncState(ctx context.Context, lastAcceptedID ids.ID, height uint64) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Update consensus state
	if t.consensus != nil {
		t.consensus.SyncState(lastAcceptedID, height)
	}

	// Clear any pending blocks that are now stale (below the synced height)
	for blockID, pending := range t.pendingBlocks {
		if pending.ConsensusBlock != nil && pending.ConsensusBlock.height <= height {
			delete(t.pendingBlocks, blockID)
		}
	}

	// Ensure we're marked as bootstrapped
	t.bootstrapped = true

	fmt.Printf("[CONSENSUS] SyncState: updated to lastAccepted=%s height=%d\n",
		lastAcceptedID, height)

	return nil
}

// HealthCheck returns health stats.
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

// -----------------------------------------------------------------------------
// Deprecated setters (backward compatibility)
// -----------------------------------------------------------------------------

// SetProposer sets the block proposer.
// Deprecated: Use WithProposer option at construction.
func (t *Transitive) SetProposer(proposer BlockProposer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.proposer = proposer
}

// SetEmitter sets the proposer.
// Deprecated: Use WithProposer option at construction.
func (t *Transitive) SetEmitter(e BlockProposer) {
	t.SetProposer(e)
}

// SetVM sets the block builder.
// Deprecated: Use WithVM option at construction.
func (t *Transitive) SetVM(vm BlockBuilder) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.vm = vm
}

// -----------------------------------------------------------------------------
// Consensus operations
// -----------------------------------------------------------------------------

// AddBlock adds a block to consensus.
func (t *Transitive) AddBlock(ctx context.Context, blk *Block) error {
	return t.consensus.AddBlock(ctx, blk)
}

// ProcessVote processes a vote.
func (t *Transitive) ProcessVote(ctx context.Context, blockID ids.ID, accept bool) error {
	return t.consensus.ProcessVote(ctx, blockID, accept)
}

// Poll conducts a poll.
func (t *Transitive) Poll(ctx context.Context, responses map[ids.ID]int) error {
	return t.consensus.Poll(ctx, responses)
}

// IsAccepted checks if block is accepted.
func (t *Transitive) IsAccepted(blockID ids.ID) bool {
	return t.consensus.IsAccepted(blockID)
}

// Preference returns preferred block.
func (t *Transitive) Preference() ids.ID {
	return t.consensus.Preference()
}

// GetBlock handles a block request.
func (t *Transitive) GetBlock(ctx context.Context, nodeID ids.NodeID, requestID uint32, blockID ids.ID) error {
	return nil
}

// Notify handles VM notifications.
func (t *Transitive) Notify(ctx context.Context, msg Message) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	fmt.Printf("[CONSENSUS DEBUG] Notify: msg.Type=%v\n", msg.Type)

	switch msg.Type {
	case PendingTxs:
		t.pendingBuildBlocks++
		fmt.Printf("[CONSENSUS DEBUG] PendingTxs received, pendingBuildBlocks=%d\n", t.pendingBuildBlocks)
		return t.buildBlocksLocked(ctx)
	case StateSyncDone:
		return nil
	}
	return nil
}

// ReceiveVote queues a vote for processing.
// Returns false if the engine is not started (vote is dropped).
func (t *Transitive) ReceiveVote(vote Vote) bool {
	t.mu.RLock()
	started := t.started
	t.mu.RUnlock()

	if !started {
		// Engine not started - drop vote to prevent state corruption
		fmt.Printf("[VOTE DEBUG] ReceiveVote DROPPED: engine not started, blockID=%s from=%s accept=%v\n",
			vote.BlockID, vote.NodeID, vote.Accept)
		return false
	}

	select {
	case t.votes <- vote:
		fmt.Printf("[VOTE DEBUG] ReceiveVote QUEUED: blockID=%s from=%s accept=%v channelLen=%d\n",
			vote.BlockID, vote.NodeID, vote.Accept, len(t.votes))
		return true
	default:
		// Channel full; vote will be resent
		fmt.Printf("[VOTE DEBUG] ReceiveVote DROPPED: channel full, blockID=%s from=%s accept=%v\n",
			vote.BlockID, vote.NodeID, vote.Accept)
		return false
	}
}

// Stats returns engine statistics.
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

// PendingBuildBlocks returns pending build count.
func (t *Transitive) PendingBuildBlocks() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pendingBuildBlocks
}

// HasPendingBlock checks if a block is in the pending blocks map (built or received but not yet finalized).
// This is used by the Vote handler to determine if votes should be processed immediately
// (block exists) or buffered (block not yet available).
func (t *Transitive) HasPendingBlock(blockID ids.ID) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, exists := t.pendingBlocks[blockID]
	return exists
}

// GetPendingBlock returns the VMBlock for a pending block if it exists.
// This allows the Vote handler to process votes for blocks that are in consensus
// but not yet verified/stored in the VM.
func (t *Transitive) GetPendingBlock(blockID ids.ID) (block.Block, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	pending, exists := t.pendingBlocks[blockID]
	if !exists || pending.VMBlock == nil {
		return nil, false
	}
	return pending.VMBlock, true
}

// -----------------------------------------------------------------------------
// Internal
// -----------------------------------------------------------------------------

func (t *Transitive) pollLoopWithCtx(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.processPendingBlocks()
		}
	}
}

func (t *Transitive) processPendingBlocks() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if len(t.pendingBlocks) > 0 {
		fmt.Printf("[CONSENSUS DEBUG] processPendingBlocks: %d pending blocks\n", len(t.pendingBlocks))
	}

	for blockID, pending := range t.pendingBlocks {
		if pending.Decided {
			continue
		}

		fmt.Printf("[CONSENSUS DEBUG] checking block %s: votes=%d isAccepted=%v isRejected=%v\n",
			blockID, pending.VoteCount, t.consensus.IsAccepted(blockID), t.consensus.IsRejected(blockID))

		if t.consensus.IsAccepted(blockID) {
			pending.Decided = true
			t.blocksAccepted++
			if pending.VMBlock != nil {
				if err := pending.VMBlock.Accept(t.ctx); err != nil {
					fmt.Printf("warning: accept failed for %s: %v\n", blockID, err)
				}
			}
			// After accepting a block, update the VM's preferred block to build on
			// This is critical: without this, the VM's Preferred() returns the old block
			// while GetLastAccepted() returns the newly accepted block, causing
			// GetState(preferred) to fail when building the next block
			if t.vm != nil {
				if err := t.vm.SetPreference(t.ctx, blockID); err != nil {
					fmt.Printf("warning: SetPreference failed for %s: %v\n", blockID, err)
				} else {
					fmt.Printf("[CONSENSUS DEBUG] SetPreference updated to %s\n", blockID)
				}
			}
			delete(t.pendingBlocks, blockID)
		} else if t.consensus.IsRejected(blockID) {
			pending.Decided = true
			t.blocksRejected++
			if pending.VMBlock != nil {
				if err := pending.VMBlock.Reject(t.ctx); err != nil {
					fmt.Printf("warning: reject failed for %s: %v\n", blockID, err)
				}
			}
			delete(t.pendingBlocks, blockID)
		}
	}
}

func (t *Transitive) voteHandlerWithCtx(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case vote := <-t.votes:
			t.handleVote(vote)
		}
	}
}

func (t *Transitive) handleVote(vote Vote) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.votesReceived++

	fmt.Printf("[VOTE DEBUG] handleVote: blockID=%s from=%s accept=%v pendingBlocksCount=%d\n",
		vote.BlockID, vote.NodeID, vote.Accept, len(t.pendingBlocks))

	// Only process votes for blocks we're tracking
	pending, exists := t.pendingBlocks[vote.BlockID]
	if !exists {
		// DEBUG: List all pending blocks to see what we ARE tracking
		fmt.Printf("[VOTE DEBUG] handleVote DROPPED: block NOT in pendingBlocks. Tracking blocks:\n")
		for id, p := range t.pendingBlocks {
			fmt.Printf("[VOTE DEBUG]   - blockID=%s voteCount=%d decided=%v\n", id, p.VoteCount, p.Decided)
		}
		return
	}

	fmt.Printf("[VOTE DEBUG] handleVote: block found in pending, currentVoteCount=%d\n", pending.VoteCount)

	if err := t.consensus.ProcessVote(t.ctx, vote.BlockID, vote.Accept); err != nil {
		fmt.Printf("[VOTE DEBUG] handleVote: ProcessVote error: %v\n", err)
		return
	}

	// Only count accept votes toward quorum
	if vote.Accept {
		pending.VoteCount++
		fmt.Printf("[VOTE DEBUG] handleVote: incremented VoteCount to %d for block=%s\n",
			pending.VoteCount, vote.BlockID)
		responses := map[ids.ID]int{vote.BlockID: pending.VoteCount}
		_ = t.consensus.Poll(t.ctx, responses)
	}
}

func (t *Transitive) buildBlocksLocked(ctx context.Context) error {
	if t.vm == nil {
		fmt.Println("[CONSENSUS DEBUG] buildBlocksLocked: vm is nil")
		return nil
	}

	fmt.Printf("[CONSENSUS DEBUG] buildBlocksLocked: pendingBuildBlocks=%d\n", t.pendingBuildBlocks)

	for t.pendingBuildBlocks > 0 {
		t.pendingBuildBlocks--

		fmt.Println("[CONSENSUS DEBUG] calling vm.BuildBlock")
		vmBlock, err := t.vm.BuildBlock(ctx)
		if err != nil {
			fmt.Printf("[CONSENSUS DEBUG] vm.BuildBlock error: %v\n", err)
			return nil
		}
		fmt.Printf("[CONSENSUS DEBUG] vm.BuildBlock success: block=%s height=%d\n", vmBlock.ID(), vmBlock.Height())

		t.blocksBuilt++

		consensusBlock := &Block{
			id:        vmBlock.ID(),
			parentID:  vmBlock.ParentID(),
			height:    vmBlock.Height(),
			timestamp: vmBlock.Timestamp().Unix(),
			data:      vmBlock.Bytes(),
		}

		if err := t.consensus.AddBlock(ctx, consensusBlock); err != nil {
			// Check if block already exists in consensus - this is OK, just means
			// we've already added it. We still need to track it in pendingBlocks
			// so the polling/finalization logic can proceed.
			if _, exists := t.pendingBlocks[vmBlock.ID()]; exists {
				fmt.Printf("[CONSENSUS DEBUG] AddBlock: block already tracked in pendingBlocks, skipping: %s\n", vmBlock.ID())
				continue
			}
			// Block exists in consensus but not in pendingBlocks - fall through to add it
			fmt.Printf("[CONSENSUS DEBUG] AddBlock: block exists in consensus but not in pendingBlocks, will track: %s\n", vmBlock.ID())
		} else {
			fmt.Printf("[CONSENSUS DEBUG] AddBlock success for block=%s\n", vmBlock.ID())
		}

		t.pendingBlocks[vmBlock.ID()] = &PendingBlock{
			ConsensusBlock: consensusBlock,
			VMBlock:        vmBlock,
			ProposedAt:     time.Now(),
			VoteCount:      1, // Count proposer's own vote
			Decided:        false,
		}

		// CRITICAL: Process proposer's own vote through consensus Poll
		// With quorum-size=1, this single vote should finalize the block
		responses := map[ids.ID]int{vmBlock.ID(): 1}
		if err := t.consensus.Poll(t.ctx, responses); err != nil {
			fmt.Printf("[CONSENSUS DEBUG] Poll error for proposer vote: %v\n", err)
		} else {
			fmt.Printf("[CONSENSUS DEBUG] Poll processed proposer's vote for block=%s\n", vmBlock.ID())
		}

		fmt.Printf("[CONSENSUS DEBUG] proposer=%v\n", t.proposer != nil)
		if t.proposer != nil {
			proposal := BlockProposal{
				BlockID:   vmBlock.ID(),
				BlockData: vmBlock.Bytes(),
				Height:    vmBlock.Height(),
				ParentID:  vmBlock.ParentID(),
			}
			if err := t.proposer.Propose(ctx, proposal); err != nil {
				fmt.Printf("warning: propose failed for %s: %v\n", vmBlock.ID(), err)
			}
			// Request votes from all validators (nil = all validators)
			// This sends PullQuery messages asking validators to vote on this block
			voteReq := VoteRequest{
				BlockID:    vmBlock.ID(),
				Validators: nil, // nil means request from all validators
			}
			if err := t.proposer.RequestVotes(ctx, voteReq); err != nil {
				fmt.Printf("warning: request votes failed for %s: %v\n", vmBlock.ID(), err)
			}
		}
	}
	return nil
}

// -----------------------------------------------------------------------------
// Transport (network layer interface)
// -----------------------------------------------------------------------------

// Transport handles message transport.
type Transport[ID comparable] interface {
	Send(ctx context.Context, to string, msg interface{}) error
	Receive(ctx context.Context) (interface{}, error)
}
