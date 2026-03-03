package chain

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/slashing"
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
	BlockData  []byte // Block bytes for PushQuery (peers can immediately verify and vote)
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
	VoteCount      int // Accept votes
	RejectCount    int // Reject votes
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

// WithSlashing enables equivocation detection and slashing evidence collection.
func WithSlashing(detector *slashing.Detector, db *slashing.DB) Option {
	return func(t *Transitive) {
		t.slashingDetector = detector
		t.slashingDB = db
	}
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
	wg           sync.WaitGroup // tracks background goroutines

	// Block management
	pendingBlocks      map[ids.ID]*PendingBlock
	pendingBuildBlocks int

	// Vote channels
	voteRequests chan VoteRequest
	votes        chan Vote

	// Pipeline: signal channel for continuous block production
	pipelineSignal chan struct{}

	// Metrics
	blocksBuilt    uint64
	blocksAccepted uint64
	blocksRejected uint64
	votesSent      uint64
	votesReceived  uint64

	// Slashing: equivocation detection (optional, nil disables)
	slashingDetector *slashing.Detector
	slashingDB       *slashing.DB
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
	// Scale buffers for burst mode — 1ms blocks produce 1000 blocks/sec,
	// so vote channels need depth to avoid back-pressure stalls.
	burst := cfg.Params.BlockTime <= time.Millisecond
	if cfg.VoteRequestBuffer == 0 {
		cfg.VoteRequestBuffer = 100
		if burst {
			cfg.VoteRequestBuffer = 4096
		}
	}
	if cfg.VoteBuffer == 0 {
		cfg.VoteBuffer = 1000
		if burst {
			cfg.VoteBuffer = 16384
		}
	}

	t := &Transitive{
		consensus:      NewChainConsensus(cfg.Params.K, cfg.Params.AlphaPreference, int(cfg.Params.Beta)),
		params:         cfg.Params,
		vm:             cfg.VM,
		proposer:       cfg.Proposer,
		pendingBlocks:  make(map[ids.ID]*PendingBlock),
		voteRequests:   make(chan VoteRequest, cfg.VoteRequestBuffer),
		votes:          make(chan Vote, cfg.VoteBuffer),
		pipelineSignal: make(chan struct{}, 1),
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

	t.wg.Add(3)
	go t.pollLoopWithCtx(engineCtx)
	go t.voteHandlerWithCtx(engineCtx)
	go t.pipelineLoop(engineCtx)

	return nil
}

// StartWithID starts with a request ID.
func (t *Transitive) StartWithID(ctx context.Context, requestID uint32) error {
	return t.Start(ctx, requestID > 0)
}

// Stop stops the engine.
func (t *Transitive) Stop(ctx context.Context) error {
	// Cancel context first, outside the lock, to signal goroutines to exit.
	// This prevents deadlock where goroutines are blocked waiting for the lock
	// while we're holding the lock waiting for them to exit.
	t.mu.RLock()
	cancel := t.cancel
	t.mu.RUnlock()

	if cancel != nil {
		cancel()
	}

	// Wait for goroutines to exit before updating state.
	// This ensures clean shutdown without goroutine leaks.
	t.wg.Wait()

	t.mu.Lock()
	defer t.mu.Unlock()

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

// CheckBlockProposal checks a block proposal for double-signing.
// Call this when receiving a block from a remote validator before adding it to consensus.
// Returns the evidence if the proposer equivocated, nil otherwise.
func (t *Transitive) CheckBlockProposal(proposerID ids.NodeID, height uint64, blockID ids.ID, blockData []byte) *slashing.Evidence {
	t.mu.RLock()
	detector := t.slashingDetector
	sdb := t.slashingDB
	t.mu.RUnlock()

	if detector == nil {
		return nil
	}

	// Reject proposals from jailed validators
	if sdb != nil && sdb.IsJailed(proposerID) {
		return &slashing.Evidence{
			Type:        slashing.DoubleSign,
			ValidatorID: proposerID,
			Height:      height,
		}
	}

	ev := detector.CheckBlock(proposerID, height, blockID, blockData)
	if ev != nil && sdb != nil {
		sdb.RecordEvidence(*ev)
	}
	return ev
}

// SlashingDB returns the slashing database, or nil if slashing is disabled.
func (t *Transitive) SlashingDB() *slashing.DB {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.slashingDB
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

	switch msg.Type {
	case PendingTxs:
		t.pendingBuildBlocks++
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
		return false
	}

	select {
	case t.votes <- vote:
		return true
	default:
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
	defer t.wg.Done()

	// Use BlockTime as poll interval — the engine must check finalization
	// at least as fast as blocks are produced. For 1ms blocks this means
	// 1ms polling; for mainnet 200ms blocks, 200ms polling.
	interval := t.params.BlockTime
	if interval <= 0 {
		interval = 50 * time.Millisecond // fallback
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			t.processPendingBlocks()
		}
	}
}

// pipelineLoop implements pipelined block production: as soon as a block is
// finalized, immediately build the next block. This decouples throughput from
// latency — with a 10-stage pipeline, a 10ms round produces 1 block/ms.
func (t *Transitive) pipelineLoop(ctx context.Context) {
	defer t.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.pipelineSignal:
			if ctx.Err() != nil {
				return
			}
			// A block was just finalized — immediately try to build the next one
			t.mu.Lock()
			if t.vm != nil {
				t.pendingBuildBlocks++
				_ = t.buildBlocksLocked(ctx)
			}
			t.mu.Unlock()
		}
	}
}

// signalPipeline wakes the pipeline goroutine to build the next block.
func (t *Transitive) signalPipeline() {
	select {
	case t.pipelineSignal <- struct{}{}:
	default: // already signaled
	}
}

func (t *Transitive) processPendingBlocks() {
	// Phase 1: Snapshot pending block IDs under t.mu (fast read lock).
	type candidate struct {
		blockID ids.ID
		vmBlock block.Block
	}
	t.mu.RLock()
	var candidates []candidate
	for blockID, pending := range t.pendingBlocks {
		if !pending.Decided {
			candidates = append(candidates, candidate{blockID: blockID, vmBlock: pending.VMBlock})
		}
	}
	t.mu.RUnlock()

	if len(candidates) == 0 {
		return
	}

	// Phase 2: Check consensus state WITHOUT holding t.mu (avoids nested lock).
	type blockAction struct {
		blockID ids.ID
		vmBlock block.Block
		accept  bool
	}
	var actions []blockAction
	for _, c := range candidates {
		if t.consensus.IsRejected(c.blockID) {
			actions = append(actions, blockAction{blockID: c.blockID, vmBlock: c.vmBlock, accept: false})
		} else if t.consensus.IsAccepted(c.blockID) {
			actions = append(actions, blockAction{blockID: c.blockID, vmBlock: c.vmBlock, accept: true})
		}
	}

	if len(actions) == 0 {
		return
	}

	// Phase 3: Update bookkeeping under t.mu write lock.
	// Track which actions were actually found (not already finalized by tryFinalizeBlock).
	t.mu.Lock()
	var vm BlockBuilder
	var ctx context.Context
	found := make([]bool, len(actions))
	for i, action := range actions {
		if pending, exists := t.pendingBlocks[action.blockID]; exists {
			found[i] = true
			pending.Decided = true
			if action.accept {
				t.blocksAccepted++
			} else {
				t.blocksRejected++
			}
			delete(t.pendingBlocks, action.blockID)
		}
	}
	vm = t.vm
	ctx = t.ctx
	t.mu.Unlock()

	// Phase 4: Execute VM operations ONLY for blocks found in phase 3.
	// Blocks already finalized by tryFinalizeBlock are skipped to prevent
	// double Accept/Reject calls which could corrupt VM state.
	accepted := false
	for i, action := range actions {
		if !found[i] {
			continue // already finalized by vote handler
		}
		if action.accept {
			accepted = true
			if action.vmBlock != nil {
				action.vmBlock.Accept(ctx)
			}
			if vm != nil {
				vm.SetPreference(ctx, action.blockID)
			}
		} else {
			if action.vmBlock != nil {
				action.vmBlock.Reject(ctx)
			}
		}
	}

	// Pipeline: if any block was accepted, immediately build next
	if accepted {
		t.signalPipeline()
	}
}

func (t *Transitive) voteHandlerWithCtx(ctx context.Context) {
	defer t.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case vote := <-t.votes:
			// Check context again before processing to avoid work after shutdown
			if ctx.Err() != nil {
				return
			}
			t.handleVote(vote)
		}
	}
}

func (t *Transitive) handleVote(vote Vote) {
	// Collect state under t.mu, release before calling consensus methods
	// to avoid nested lock (t.mu -> c.mu) deadlock.
	t.mu.Lock()
	t.votesReceived++
	pending, exists := t.pendingBlocks[vote.BlockID]
	detector := t.slashingDetector
	sdb := t.slashingDB
	ctx := t.ctx

	if !exists {
		t.mu.Unlock()
		return
	}

	var height uint64
	if pending.ConsensusBlock != nil {
		height = pending.ConsensusBlock.height
	}

	// Equivocation + jail checks before counting the vote.
	// Detector and DB use their own locks; safe to call under t.mu
	// since there is no lock ordering conflict (t.mu -> detector.mu is one direction only).
	if detector != nil {
		if ev := detector.CheckVote(vote.NodeID, height, vote.BlockID, vote.Accept); ev != nil {
			if sdb != nil {
				sdb.RecordEvidence(*ev)
			}
			t.mu.Unlock()
			return
		}
		if sdb != nil && sdb.IsJailed(vote.NodeID) {
			t.mu.Unlock()
			return
		}
	}

	var voteCount int
	if vote.Accept {
		pending.VoteCount++
		voteCount = pending.VoteCount
	} else {
		pending.RejectCount++
	}
	t.mu.Unlock()

	if err := t.consensus.ProcessVote(ctx, vote.BlockID, vote.Accept); err != nil {
		return
	}

	if vote.Accept {
		responses := map[ids.ID]int{vote.BlockID: voteCount}
		_ = t.consensus.Poll(ctx, responses)
	} else {
		// Trigger poll to check for rejection quorum
		_ = t.consensus.Poll(ctx, map[ids.ID]int{vote.BlockID: voteCount})
	}

	// Finalize: if consensus accepted this block, call VM.Accept() and update state.
	t.tryFinalizeBlock(ctx, vote.BlockID)
}

// tryFinalizeBlock checks if consensus has accepted a block and calls VM.Accept().
func (t *Transitive) tryFinalizeBlock(ctx context.Context, blockID ids.ID) {
	accepted := t.consensus.IsAccepted(blockID)
	if !accepted {
		return
	}

	t.mu.Lock()
	pending, exists := t.pendingBlocks[blockID]
	if !exists || pending.Decided {
		t.mu.Unlock()
		return
	}
	pending.Decided = true
	t.blocksAccepted++
	delete(t.pendingBlocks, blockID)
	t.mu.Unlock()

	if pending.VMBlock != nil {
		_ = pending.VMBlock.Accept(ctx)
	}

	// Pipeline: block finalized → immediately build next
	t.signalPipeline()
}

// DrainAccepted finalizes any pending blocks that consensus has accepted.
// Called from the ForwardVMNotifications loop after each Notify.
func (t *Transitive) DrainAccepted(ctx context.Context) {
	t.mu.Lock()
	var toAccept []struct {
		id  ids.ID
		blk block.Block
	}
	for id, pending := range t.pendingBlocks {
		if !pending.Decided && t.consensus.IsAccepted(id) {
			pending.Decided = true
			t.blocksAccepted++
			toAccept = append(toAccept, struct {
				id  ids.ID
				blk block.Block
			}{id, pending.VMBlock})
			delete(t.pendingBlocks, id)
		}
	}
	t.mu.Unlock()

	for _, a := range toAccept {
		if a.blk != nil {
			_ = a.blk.Accept(ctx)
		}
	}

	if len(toAccept) > 0 {
		t.signalPipeline()
	}
}

func (t *Transitive) buildBlocksLocked(ctx context.Context) error {
	if t.vm == nil {
		return nil
	}

	for t.pendingBuildBlocks > 0 {
		t.pendingBuildBlocks--

		vmBlock, err := t.vm.BuildBlock(ctx)
		if err != nil {
			return nil
		}

		t.blocksBuilt++

		// Skip if already tracked
		if _, exists := t.pendingBlocks[vmBlock.ID()]; exists {
			continue
		}

		consensusBlock := &Block{
			id:        vmBlock.ID(),
			parentID:  vmBlock.ParentID(),
			height:    vmBlock.Height(),
			timestamp: vmBlock.Timestamp().Unix(),
			data:      vmBlock.Bytes(),
		}

		// Verify BEFORE consensus — prevents accepting invalid blocks in K=1 mode
		// where self-vote causes immediate acceptance. If Verify fails, the block
		// is never added to consensus, so IsAccepted cannot return true for it.
		t.mu.Unlock()
		if err := vmBlock.Verify(ctx); err != nil {
			t.mu.Lock()
			continue
		}

		// Now add to consensus and self-vote.
		addErr := t.consensus.AddBlock(ctx, consensusBlock)
		if addErr == nil {
			_ = t.consensus.ProcessVote(ctx, vmBlock.ID(), true)
			_ = t.consensus.Poll(ctx, map[ids.ID]int{vmBlock.ID(): 1})
		}
		t.mu.Lock()

		if addErr != nil {
			continue
		}

		// Check if consensus already accepted this block (K=1 single-node mode).
		alreadyAccepted := t.consensus.IsAccepted(vmBlock.ID())
		if alreadyAccepted {
			// Block already verified above — safe to accept.
			t.blocksAccepted++
			t.mu.Unlock()
			_ = vmBlock.Accept(ctx)
			if t.vm != nil {
				_ = t.vm.SetPreference(ctx, vmBlock.ID())
			}
			t.mu.Lock()
		} else {
			t.pendingBlocks[vmBlock.ID()] = &PendingBlock{
				ConsensusBlock: consensusBlock,
				VMBlock:        vmBlock,
				ProposedAt:     time.Now(),
				VoteCount:      1,
				Decided:        false,
			}
		}

		if t.proposer != nil {
			proposal := BlockProposal{
				BlockID:   vmBlock.ID(),
				BlockData: vmBlock.Bytes(),
				Height:    vmBlock.Height(),
				ParentID:  vmBlock.ParentID(),
			}
			t.proposer.Propose(ctx, proposal)
			voteReq := VoteRequest{
				BlockID:    vmBlock.ID(),
				BlockData:  vmBlock.Bytes(),
				Validators: nil,
			}
			t.proposer.RequestVotes(ctx, voteReq)
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
