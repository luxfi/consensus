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
	"github.com/luxfi/log"
)

// -----------------------------------------------------------------------------
// Errors
// -----------------------------------------------------------------------------

var (
	ErrNotStarted     = errors.New("engine not started")
	ErrAlreadyStarted = errors.New("engine already started")

	// ErrQuorumVerifierRequired is returned by Start when a multi-validator
	// engine (K>1) is started without a VoteVerifier. Multi-validator finality
	// MUST be gated on a verifiable α-of-K quorum cert; without a verifier
	// there is no way to tell a real quorum from forged votes. Fail-closed.
	ErrQuorumVerifierRequired = errors.New("chain: multi-validator engine (K>1) requires a vote verifier for quorum-cert finality (use WithQuorumCert / WithVoteVerifier)")
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

	// Signature is the validator's signature over CanonicalVoteMessage of the
	// block's position (ChainID, Height, Round, BlockID, ParentID). It is the
	// material the engine collects into a QuorumCert — the portable, verifiable
	// α-of-K finality witness. May be empty for a single-validator (K==1)
	// engine, where the sole validator's local accept is the quorum and no cert
	// is gossiped; MUST be present and valid for multi-validator finality.
	Signature []byte
	// ParentID binds the parent into the vote's signed position. Carried so the
	// engine can rebuild CanonicalVoteMessage when assembling a cert even if it
	// is not separately tracking the block's parent.
	ParentID ids.ID
	// Round is the consensus round the vote was cast in (0 for the first round
	// at a height). Bound into the signed position.
	Round uint32
}

// PendingBlock tracks a block awaiting consensus.
type PendingBlock struct {
	ConsensusBlock *Block
	VMBlock        block.Block
	ProposedAt     time.Time
	VoteCount      int // Accept votes
	RejectCount    int // Reject votes
	Decided        bool

	// certVotes collects the distinct SIGNED accept votes observed for this
	// block, keyed by voter NodeID (de-dup: one vote per validator). When the
	// count reaches alpha the engine assembles a QuorumCert from these — the
	// α-of-K finality witness. Empty for single-validator (K==1) finality.
	certVotes map[ids.NodeID]SignedVote
	// Round is the consensus round the block was proposed in. Bound into every
	// vote's signed position so a cert binds the exact round.
	Round uint32
	// cert is the assembled+verified finality witness once the quorum is
	// reached (nil until then). Retained so the engine can re-gossip it on
	// request and so a follower's accept is gated on holding it.
	cert *QuorumCert

	// lastRePoll is when the re-poll loop last re-solicited votes for this block
	// (zero until the first re-poll). The re-poll loop only re-drives a block once
	// per RoundTO so a stuck block recovers without a gossip storm. See
	// rePollAllPending — this is the liveness retry the topology doc promises
	// ("vote-broadcast + cert-gossip + the poll-timeout re-request").
	lastRePoll time.Time

	// IsOwnProposal is true when this node built and proposed the block. It now
	// selects ONLY the finalization ENTRY POINT (finalizeOwnProposal vs.
	// tryFinalizeBlock); it does NOT alter how votes are counted. The former
	// REJECT→ACCEPT laundering it used to gate ("a peer's reject counts as accept
	// for my own block") is DELETED — vote.Accept is authoritative once the
	// signature verifies (handleVote), so an own-proposal finalizes only on a
	// real α-of-K cert (K>1) or the 1-of-1 force (K==1), never on self-promises.
	IsOwnProposal bool
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

// WithLogger sets the engine logger.
func WithLogger(l log.Logger) Option {
	return func(t *Transitive) {
		t.log = l
	}
}

// WithQuorumCert wires multi-validator α-of-K cert-witnessed finality. The node
// supplies a VoteVerifier (mandatory for cert finality — verifies every vote
// signature and every incoming cert), and optionally a CertGossiper (to
// distribute assembled certs) and a VoteSigner (to sign this node's own votes).
// chainID and nodeID identify this node's position for vote/cert binding.
//
// Without this option the engine runs in single-validator (K==1) mode: the sole
// validator's local accept is the quorum, finality uses the ForceAccept path,
// and no certs or signatures are produced. The engine REFUSES to start a
// multi-validator (K>1) configuration without a verifier (fail-closed) — see
// Start.
func WithQuorumCert(chainID ids.ID, nodeID ids.NodeID, verifier VoteVerifier, gossiper CertGossiper, signer VoteSigner) Option {
	return func(t *Transitive) {
		t.chainID = chainID
		t.nodeID = nodeID
		t.voteVerifier = verifier
		t.certGossiper = gossiper
		t.voteSigner = signer
	}
}

// WithVoteVerifier sets only the vote/cert signature verifier. Convenience for
// callers that verify but neither sign nor gossip (e.g. a verifying-only node
// or a test).
func WithVoteVerifier(verifier VoteVerifier) Option {
	return func(t *Transitive) {
		t.voteVerifier = verifier
	}
}

// WithStakeWeighting makes finality STAKE-WEIGHTED (HIGH-3): a cert is accepted
// only when its voters hold a strict ⅔-of-stake supermajority at the cert's
// height, in addition to the α-of-K count. The node supplies a StakeSource
// backed by the chain's validator set (weights from the same set the verifier
// authenticates against). REQUIRED for a PoS value chain with unequal stake;
// omit it only when equal stake is enforced at admission (then count-α is the
// correct, equivalent rule). It is ALSO a precondition for Mode() to report
// ModeQuorumFinality (the engine's stake-weighted finality regime) — see Mode().
func WithStakeWeighting(stake StakeSource) Option {
	return func(t *Transitive) {
		t.stakeSource = stake
	}
}

// WithStrictPQ marks the engine as running under a STRICT post-quantum security
// profile (the node derives this from the chain's consensus profile —
// config.Profile.IsStrict()). When set, Mode() additionally requires a PQ
// cryptoWitness (WithCryptoWitness) before it reports ModeQuorumFinality, so the
// engine cannot report a quorum-finality regime the chain cannot witness
// post-quantum. A non-strict chain leaves this false (the requirement is vacuous).
func WithStrictPQ(strict bool) Option {
	return func(t *Transitive) {
		t.strictPQ = strict
	}
}

// WithCryptoWitness wires the post-quantum finality witness source a strict-PQ
// chain uses. It is REQUIRED for Mode() to report ModeQuorumFinality on a strict-PQ
// chain; on a non-strict chain it is unused. The node supplies a source whose
// Scheme() names the PQ witness scheme actually in force.
func WithCryptoWitness(w CryptoWitnessSource) Option {
	return func(t *Transitive) {
		t.cryptoWitness = w
	}
}

// WithValidatorSetRoot binds every vote/cert this engine produces to the active
// weighted validator set at the block's height (the MEDIUM fix). The node
// supplies a ValidatorSetRootSource backed by the chain's validator set; the
// engine stamps the root into each VotePosition so a cert is pinned to the exact
// set it was certified under — a cross-epoch cert (votes cast under set R
// re-presented as certifying under set R') fails signature verification because
// every signature was over R. REQUIRED alongside WithStakeWeighting on a chain
// whose validator set / stake can change across epochs, so the ⅔-by-stake
// predicate is enforced at the cert-position epoch rather than assumed. Omit it
// only on a fixed-set chain (then Empty-root binding is the correct no-op).
func WithValidatorSetRoot(src ValidatorSetRootSource) Option {
	return func(t *Transitive) {
		t.setRootSource = src
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

	// Quorum-cert finality (multi-validator). These are the engine's sole
	// dependencies for α-of-K cert-witnessed finality:
	//
	//   - voteVerifier verifies each collected vote's signature before it is
	//     counted toward a cert and verifies an incoming cert's signatures.
	//     The node injects a real scheme (BLS / ML-DSA / secp256k1). When nil,
	//     the engine is in single-validator (K==1) mode and finality uses the
	//     local-accept force path (no cert, no signatures).
	//   - certGossiper re-broadcasts an assembled cert to all validators so
	//     followers can finalize on a verifiable α-of-K proof rather than
	//     fast-following an unverified block. Optional (nil disables cert
	//     gossip; finality still holds locally via the α-of-K count).
	//   - voteSigner signs this node's own accept votes (used when it votes as
	//     a follower, so its signature can be collected into a cert). Optional.
	//   - chainID / nodeID identify this node's position for vote/cert binding.
	voteVerifier VoteVerifier
	certGossiper CertGossiper
	voteSigner   VoteSigner
	chainID      ids.ID
	nodeID       ids.NodeID

	// stakeSource (optional) makes finality STAKE-WEIGHTED instead of a raw voter
	// count (HIGH-3). When set (a value/PoS chain with unequal stake), a cert is
	// accepted only if its voters hold a strict ⅔ supermajority of stake at the
	// cert's height (VerifyWeighted), in addition to the α-of-K count. When nil,
	// finality is count-α and the chain MUST enforce equal stake at validator
	// admission (the documented invariant) — the node wires this for value chains.
	stakeSource StakeSource

	// setRootSource (optional) supplies the commitment to the active weighted
	// validator set at a block's height (the MEDIUM fix). When set, every
	// VotePosition this engine signs/assembles carries that set-root, so a cert
	// is cryptographically pinned to the exact set it was certified under and
	// cannot be re-verified against a different epoch's set. When nil, positions
	// carry ids.Empty (behavior identical to before set-root binding) — a chain
	// without epoch-versioned sets needs no binding.
	setRootSource ValidatorSetRootSource

	// strictPQ records that this chain runs under a STRICT post-quantum security
	// profile (set via WithStrictPQ, from config.Profile.IsStrict()). When true,
	// Mode() additionally requires a PQ cryptoWitness before reporting
	// ModeQuorumFinality, so the engine cannot report a quorum-finality
	// regime the chain cannot witness post-quantum.
	strictPQ bool

	// cryptoWitness (optional) is the post-quantum finality witness source a strict-PQ
	// chain wires (the SAME node-layer CryptoWitnessSource that upgrades an engine
	// QuorumCert into a quasar.WeightedQuorumCert — see quorum_cert_quasar.go). It is
	// REQUIRED for Mode() to report ModeQuorumFinality on a strict-PQ chain: without it the
	// cert path cannot produce the PQ (quasar) witness the profile demands, so the value-
	// DEX gate must not certify a quorum-finality regime that cannot be witnessed post-
	// quantum. On a non-strict chain it is unused. Injected like voteVerifier/stakeSource so
	// the gate reads the SAME field that delivers the witness — nil means "PQ witness not
	// plumbed", exactly the semantics ToQuasarCert already relies on.
	cryptoWitness CryptoWitnessSource

	// Logger for consensus events (nil-safe: uses log.Noop() if unset)
	log log.Logger
}

// CertGossiper broadcasts an assembled finality cert to validators. The node
// supplies the network implementation; the engine expresses WHAT (gossip this
// proof of α-of-K finality), the node decides HOW. Optional — a nil gossiper
// means the proposer finalizes locally on the α-of-K count without distributing
// the cert (followers then reach finality via their own collected votes once
// the topology gossips votes to all).
type CertGossiper interface {
	// GossipCert broadcasts the encoded finality cert for blockID to validators.
	GossipCert(chainID ids.ID, blockID ids.ID, certBytes []byte) error
}

// VoteSigner signs this node's accept vote over the canonical vote message so
// the signature can be collected into a QuorumCert. Backed by the node's
// validator key (the same key the VoteVerifier checks against). Optional: a
// single-validator engine does not gossip votes and needs no signer.
type VoteSigner interface {
	// SignVote returns this node's signature over message (the canonical vote
	// message for a position). The returned bytes are what a peer's
	// VoteVerifier will verify.
	SignVote(message []byte) ([]byte, error)
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

	if t.log == nil {
		t.log = log.Noop()
	}

	return t
}

// NewWithParams creates an engine with specific parameters.
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

	// FAIL-CLOSED: a multi-validator engine (K>1) MUST have a vote verifier so
	// finality can be gated on a verifiable α-of-K quorum cert. Starting K>1
	// without one would leave no way to distinguish a real quorum from forged
	// votes — exactly the hole this change closes. A single-validator engine
	// (K==1) needs no verifier: its own accept is the quorum.
	if t.params.K > 1 && t.voteVerifier == nil {
		return ErrQuorumVerifierRequired
	}

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.bootstrapped = true
	t.started = true

	// Capture ctx in local variable to avoid race with struct field access
	engineCtx := t.ctx

	t.wg.Add(4)
	go t.pollLoopWithCtx(engineCtx)
	go t.voteHandlerWithCtx(engineCtx)
	go t.pipelineLoop(engineCtx)
	go t.rePollLoopWithCtx(engineCtx)

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

	// Update consensus state. A backward import is refused (ErrSyncStateRegression)
	// and must abort the whole reconcile — we do NOT clear pending blocks or flip
	// bootstrapped on a refused import, so a rejected regression is a clean no-op
	// rather than a partial state mutation.
	if t.consensus != nil {
		if err := t.consensus.SyncState(lastAcceptedID, height); err != nil {
			return err
		}
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

// SetProposer sets the block proposer.
func (t *Transitive) SetProposer(proposer BlockProposer) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.proposer = proposer
}

// SetEmitter sets the proposer (alias for SetProposer).
func (t *Transitive) SetEmitter(e BlockProposer) {
	t.SetProposer(e)
}

// SetVM sets the block builder.
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

// rePollLoopWithCtx is the LIVENESS retry that prevents a terminal first-poll
// stall. The proposer issues exactly ONE RequestVotes when it builds a block
// (buildBlocksLocked) and runs finalizeOwnProposal ONCE right after; if at that
// instant the α-of-K signed votes have not yet arrived (the common case at
// genesis — peers are still bootstrapping, or the first PushQuery was dropped at
// the network boundary), the proposer's block sits in pendingBlocks with only its
// own self-vote and NOTHING re-solicits the missing votes. The finality poll loop
// (processPendingBlocks) only CHECKS consensus.IsAccepted; it never re-requests.
// So a single lagging validator at height 0 wedged finality forever — the devnet
// freeze. This loop implements the "poll-timeout re-request" the topology doc
// (quorum_topology.go) already promises but that was never wired.
//
// It is a pure liveness retry: it re-solicits votes and re-attempts cert
// assembly, and changes NOTHING about the finality predicate. A block still
// finalizes only on a verified α-of-K cert (multi-validator) or the 1-of-1 force
// (single-validator); a genuine minority still cannot and does not finalize.
//
// Interval = RoundTO (the round budget) so a stuck block is re-driven once per
// round, not every block tick — bounded work, no gossip storm. Each block is
// re-driven at most once per interval via PendingBlock.lastRePoll.
func (t *Transitive) rePollLoopWithCtx(ctx context.Context) {
	defer t.wg.Done()

	// Re-poll on the round budget. RoundTO is the time a poll is expected to
	// complete in; a block still undecided after a round is a candidate for
	// re-solicitation. Fall back to a conservative 250ms if unset.
	interval := t.params.RoundTO
	if interval <= 0 {
		interval = 250 * time.Millisecond
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
			t.rePollAllPending(ctx, interval)
		}
	}
}

// rePollAllPending re-drives every undecided pending block that has gone a full
// re-poll interval without being re-solicited. For each such block it:
//
//  1. re-attempts finalization (tryFinalizeBlock) — idempotent; assembles +
//     gossips the cert if α signed votes are now present, so a follower that
//     missed the proposer's single cert-gossip still finalizes; and
//  2. if this node PROPOSED the block and a proposer transport is wired, re-issues
//     RequestVotes — re-sending the PushQuery so a laggard/peer that missed the
//     first poll receives the block + vote request again and can sign.
//
// Single-validator (K==1) engines never stall here (their own accept is the
// quorum, finalized synchronously), so the re-poll is a no-op for them.
func (t *Transitive) rePollAllPending(ctx context.Context, interval time.Duration) {
	// K==1: no peer votes are ever needed; nothing to re-solicit.
	if t.consensus.K() <= 1 {
		return
	}

	now := time.Now()

	// Snapshot the blocks due for a re-poll under the lock, then act without it
	// (RequestVotes / tryFinalizeBlock take their own locks). A block is due once
	// it has been undecided for a full interval since the later of ProposedAt and
	// its last re-poll — so the FIRST re-poll waits one interval after proposal
	// (giving the normal fast path time to finalize), then at most one per interval.
	type due struct {
		blockID   ids.ID
		blockData []byte
		ownProp   bool
	}
	var dueBlocks []due
	t.mu.Lock()
	for blockID, pending := range t.pendingBlocks {
		if pending.Decided {
			continue
		}
		last := pending.lastRePoll
		if last.IsZero() {
			last = pending.ProposedAt
		}
		if now.Sub(last) < interval {
			continue
		}
		pending.lastRePoll = now
		var data []byte
		if pending.VMBlock != nil {
			data = pending.VMBlock.Bytes()
		}
		dueBlocks = append(dueBlocks, due{blockID: blockID, blockData: data, ownProp: pending.IsOwnProposal})
	}
	proposer := t.proposer
	t.mu.Unlock()

	for _, d := range dueBlocks {
		// (1) Re-attempt finalization first: if α signed votes already arrived but
		// the single finalize attempt raced (or a follower missed the cert gossip),
		// this assembles + gossips the cert and commits now. Idempotent.
		t.tryFinalizeBlock(ctx, d.blockID)

		// (2) Proposer re-poll: re-send the vote request so a laggard re-receives
		// the block and votes. Only the proposer polls peers (followers learn the
		// block via gossip and broadcast their own votes); a follower short of
		// quorum recovers via the cert-gossip path that step (1) re-runs.
		if d.ownProp && proposer != nil {
			_ = proposer.RequestVotes(ctx, VoteRequest{
				BlockID:   d.blockID,
				BlockData: d.blockData,
			})
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
				if err := vm.SetPreference(ctx, action.blockID); err != nil {
					t.log.Crit("SetPreference failed after Accept — forcing preference",
						"blockID", action.blockID,
						"error", err)
					// Force consensus to match the accepted block so the
					// engine and VM don't diverge on preferred tip.
					t.consensus.ForcePreference(action.blockID)
				}
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

	// AUTHENTICATE THE VOTE (multi-validator). A vote is counted toward the
	// quorum ONLY if its signature verifies over the block's position AND the
	// vote's own decision (accept→accept-message, reject→reject-message). This:
	//   - makes consensus.acceptVotes count only REAL validator accepts, so
	//     IsAccepted() is truthful (it cannot be inflated by forged/unsigned
	//     votes) and stays in lock-step with the assemblable cert;
	//   - blocks forged-accept finality (an outsider key, or a real validator's
	//     signature lifted from a DIFFERENT position/decision, fails here);
	//   - blocks forged-reject censorship (an unauthenticated reject is dropped,
	//     strictly safer than the prior unauthenticated reject path).
	//
	// The former `effectiveAccept = vote.Accept || IsOwnProposal` REJECT→ACCEPT
	// flip is DELETED: vote.Accept is authoritative once the signature checks
	// out. Single-validator engines (no verifier) skip authentication — the sole
	// validator's self-vote is the quorum and carries no signature.
	if t.voteVerifier != nil {
		pos := t.blockPositionLocked(pending, vote.BlockID)
		msg := canonicalVoteMessageFor(pos, vote.Accept)
		// Resolve the voter's pubkey at the block's P-CHAIN epoch height (RESIDUAL-B),
		// the same height the position's set-root commits to.
		if len(vote.Signature) == 0 || !t.voteVerifier.VerifyVote(vote.NodeID, msg, vote.Signature, t.epochHeightLocked(pending)) {
			// Unsigned or invalid: not a real vote from this validator at this
			// position/decision. Drop it — count nothing.
			t.mu.Unlock()
			return
		}
	}

	accept := vote.Accept
	var voteCount int
	if accept {
		pending.VoteCount++
		voteCount = pending.VoteCount
		// Record the signed accept vote toward this block's quorum cert so the
		// engine can assemble + gossip the α-of-K witness once the threshold is
		// reached. (Reject votes are not certifiable — a finality cert proves
		// acceptance — they only drive the rejection path.)
		t.recordCertVoteLocked(pending, vote)
	} else {
		pending.RejectCount++
	}
	t.mu.Unlock()

	if err := t.consensus.ProcessVote(ctx, vote.BlockID, accept); err != nil {
		return
	}
	_ = t.consensus.Poll(ctx, map[ids.ID]int{vote.BlockID: voteCount})

	// Finalize: if consensus reached the α-of-K accept quorum, assemble the
	// cert, gossip it, and call VM.Accept().
	t.tryFinalizeBlock(ctx, vote.BlockID)
}

// recordCertVoteLocked records a distinct SIGNED accept vote toward this
// block's quorum cert. Caller holds t.mu. A vote with no signature is ignored
// for cert purposes (it still counts toward the plain accept tally in
// handleVote) — only signed votes can witness a cert. Verification of the
// signature happens at assembly time (assembleCertLocked) so a single bad
// signature cannot poison the map; de-dup is by NodeID.
func (t *Transitive) recordCertVoteLocked(pending *PendingBlock, vote Vote) {
	if len(vote.Signature) == 0 {
		return
	}
	if pending.certVotes == nil {
		pending.certVotes = make(map[ids.NodeID]SignedVote)
	}
	pending.certVotes[vote.NodeID] = SignedVote{
		NodeID:    vote.NodeID,
		Accept:    true,
		Signature: append([]byte(nil), vote.Signature...),
	}
}

// recordOwnVoteLocked signs THIS node's accept vote for blockID and records it
// into the block's cert set. Caller holds t.mu. No-op when no voteSigner is
// configured (single-validator / K==1 finality needs no cert). The proposer
// (and any node casting its own accept locally) is one of the α signers, so its
// signature belongs in the cert.
func (t *Transitive) recordOwnVoteLocked(pending *PendingBlock, blockID ids.ID) {
	if t.voteSigner == nil {
		return
	}
	pos := t.blockPositionLocked(pending, blockID)
	sig, err := t.voteSigner.SignVote(CanonicalVoteMessage(pos))
	if err != nil {
		t.log.Warn("failed to sign own accept vote for cert", "blockID", blockID, "error", err)
		return
	}
	t.recordCertVoteLocked(pending, Vote{
		BlockID:   blockID,
		NodeID:    t.nodeID,
		Accept:    true,
		Signature: sig,
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	})
}

// pChainHeighter is the subset of block.SignedBlock the engine needs to pin a
// block's validator-set epoch: the P-CHAIN height the block was proposed at. A
// proposervm block satisfies it; a bare VM block does not (epoch height 0 →
// fail-closed on the K>1 finality path). Defined locally so the engine depends
// only on the one method it reads, not the whole SignedBlock surface.
type pChainHeighter interface {
	PChainHeight() uint64
}

// pChainHeightOf extracts the P-chain height a VM block was proposed at, or 0 if
// the block does not carry one (pre-fork / no proposervm wrapper). This is the
// SOLE place the engine reads PChainHeight off a VM block, so every consensus
// Block records the same epoch the proposervm signed.
func pChainHeightOf(b block.Block) uint64 {
	if ph, ok := b.(pChainHeighter); ok {
		return ph.PChainHeight()
	}
	return 0
}

// PChainHeightOfForTest exposes the engine's block→P-chain-height boundary read to
// a test in ANOTHER module (the node's chains package), so it can prove the
// node-layer wrapper delivers the REAL epoch height (not 0) through the EXACT
// function the engine uses. Exported only for that cross-module test reach.
func PChainHeightOfForTest(b block.Block) uint64 { return pChainHeightOf(b) }

// epochHeightLocked returns the P-CHAIN height the block's weighted validator set
// is pinned to — the SINGLE height used for the set-root commitment, the
// ⅔-by-stake tally, AND per-voter pubkey resolution (membership, pubkey,
// set-root, stake ALL read set@H, MEDIUM-1/CRITICAL-1/RESIDUAL-B). It is the
// block's recorded P-chain height, NOT its value-chain height. Caller holds t.mu.
func (t *Transitive) epochHeightLocked(pending *PendingBlock) uint64 {
	if pending != nil && pending.ConsensusBlock != nil {
		return pending.ConsensusBlock.pChainHeight
	}
	return 0
}

// blockPositionLocked returns the consensus position a block's votes/cert bind
// to. Caller holds t.mu.
func (t *Transitive) blockPositionLocked(pending *PendingBlock, blockID ids.ID) VotePosition {
	var parentID ids.ID
	var height uint64
	if pending.ConsensusBlock != nil {
		parentID = pending.ConsensusBlock.parentID
		height = pending.ConsensusBlock.height
	}
	// Stamp the active weighted-validator-set commitment at the block's P-CHAIN
	// EPOCH height (the MEDIUM-1 / CRITICAL-1 fix) — NOT its value-chain height.
	// Every path that builds a position — sign (recordOwnVoteLocked), assemble +
	// verify (assembleCertLocked), incoming-vote/cert verify — routes through
	// here, so they all bind the SAME root for a given block: a cert is pinned to
	// the exact set it was certified under. The epoch height is the proposervm's
	// PChainHeight, the only height that is (i) ≤ the current P-chain height (so
	// platformvm.GetValidatorSet does NOT errUnfinalizedHeight) and (ii) embedded
	// in the signed block so every honest node derives the IDENTICAL set/root.
	// nil source ⟹ Empty root (the fixed-set no-op).
	var setRoot ids.ID
	if t.setRootSource != nil {
		setRoot = t.setRootSource.ValidatorSetRoot(t.epochHeightLocked(pending))
	}
	return VotePosition{
		ChainID:          t.chainID,
		Height:           height,
		Round:            pending.Round,
		BlockID:          blockID,
		ParentID:         parentID,
		ValidatorSetRoot: setRoot,
	}
}

// TrackOwnProposalForTest inserts blk as a verified own-proposal pending block —
// the SAME state buildBlocksLocked establishes for a locally built block — and
// returns the canonical VotePosition followers must sign. It exists so a test in
// ANOTHER module (the node's chains package) can drive a REAL VM block (e.g. the
// node's P-chain-height-stamping wrapper block) through the engine's actual
// vote→assemble→verify→finalize path. It is exported ONLY for that cross-module
// test reach; it is not part of the consensus runtime surface.
//
// It is NOT a finality shortcut: it records the proposer's own signed accept
// (recordOwnVoteLocked) and a single self-vote toward the count exactly as
// production does, and it NEVER calls ForceAccept. A block tracked here finalizes
// (K>1) only when enough real signed peer votes arrive to assemble a cert that
// VERIFIES under the wired verifier (and clears the ⅔-stake predicate when a
// stake source is wired) — the genuine BFT path.
//
// The load-bearing line is `pChainHeightOf(blk)`: it captures the block's P-CHAIN
// epoch height off the VM block through the SAME boundary the production
// buildBlocksLocked uses, so a test can prove the boundary delivers the real
// height (not 0) end to end. The returned position's set-root is stamped at that
// epoch height (blockPositionLocked), so a follower signs — and the verifier
// resolves pubkeys at — the LIVE set@H.
func (t *Transitive) TrackOwnProposalForTest(ctx context.Context, blk block.Block, round uint32) VotePosition {
	cb := &Block{
		id:           blk.ID(),
		parentID:     blk.ParentID(),
		height:       blk.Height(),
		timestamp:    blk.Timestamp().Unix(),
		data:         blk.Bytes(),
		pChainHeight: pChainHeightOf(blk), // the boundary capture under test (b2)
	}
	_ = t.consensus.AddBlock(ctx, cb)
	_ = t.consensus.ProcessVote(ctx, blk.ID(), true)
	t.mu.Lock()
	pb := &PendingBlock{
		ConsensusBlock: cb,
		VMBlock:        blk,
		ProposedAt:     time.Now(),
		VoteCount:      1,
		Round:          round,
		Decided:        false,
		IsOwnProposal:  true,
	}
	t.pendingBlocks[blk.ID()] = pb
	t.recordOwnVoteLocked(pb, blk.ID())
	pos := t.blockPositionLocked(pb, blk.ID())
	t.mu.Unlock()
	return pos
}

// assembleCertLocked attempts to assemble a verified QuorumCert from the signed
// accept votes collected for blockID. Caller holds t.mu. Returns the cert (and
// caches it on pending) iff:
//   - a vote verifier is configured (multi-validator finality), AND
//   - at least alpha distinct votes verify under it.
//
// Each collected vote's signature is verified here; votes that fail are dropped
// from the candidate set so one forged vote cannot block a real quorum, and the
// cert is only built from VERIFIED votes — Assemble + the subsequent Verify
// then re-check distinctness and the threshold. Returns nil if the verified
// quorum is not yet present (the proposer keeps waiting / re-requesting — this
// is the liveness path, NOT a force).
func (t *Transitive) assembleCertLocked(pending *PendingBlock, blockID ids.ID) *QuorumCert {
	if pending.cert != nil {
		return pending.cert
	}
	if t.voteVerifier == nil {
		return nil
	}
	alpha := t.consensus.Alpha()
	if alpha <= 0 {
		return nil
	}
	pos := t.blockPositionLocked(pending, blockID)
	message := CanonicalVoteMessage(pos)
	// The epoch height pins every per-voter pubkey resolution + the stake tally to
	// the SAME P-chain height the position's set-root commits to (MEDIUM-1).
	epochHeight := t.epochHeightLocked(pending)

	verified := make([]SignedVote, 0, len(pending.certVotes))
	for _, sv := range pending.certVotes {
		if t.voteVerifier.VerifyVote(sv.NodeID, message, sv.Signature, epochHeight) {
			verified = append(verified, sv)
		}
	}
	if uint32(len(verified)) < uint32(alpha) {
		return nil
	}
	cert, err := AssembleQuorumCert(pos, uint32(alpha), verified)
	if err != nil {
		return nil
	}
	// Defence in depth: the cert we just built must verify under our own
	// verifier before we treat it as a finality witness (catches any assembly
	// invariant drift). Assemble already enforced distinctness + threshold.
	// On a stake-weighted chain it must ALSO clear the ⅔-of-stake supermajority
	// (HIGH-3) — the count quorum alone is not finality when stake is unequal, so
	// we keep WAITING (return nil) until enough STAKE has voted, never forcing.
	if t.stakeSource != nil {
		if err := cert.VerifyWeighted(t.voteVerifier, t.stakeSource, epochHeight); err != nil {
			return nil
		}
	} else if err := cert.Verify(t.voteVerifier, epochHeight); err != nil {
		return nil
	}
	pending.cert = cert
	return cert
}

// tryFinalizeBlock finalizes a block once the α-of-K accept quorum is reached.
//
// Multi-validator (K>1): finality requires a verified QuorumCert. consensus
// .IsAccepted only flips true once acceptVotes>=alpha (the α-of-K count), so
// reaching it means alpha distinct accepts arrived. We assemble the cert from
// the collected SIGNED votes as the portable witness, GOSSIP it so followers
// finalize on a verifiable proof (not a fast-follow guess), then commit. If the
// count says accepted but we cannot yet assemble a verified cert (signatures
// still in flight), we WAIT — we never force. This is what makes the change
// BFT: no value finalizes without a verifiable α-of-K witness.
//
// Single-validator (K==1): there are no peer signatures; IsAccepted reflects
// the sole validator's own accept, which IS the 1-of-1 quorum. Commit directly.
func (t *Transitive) tryFinalizeBlock(ctx context.Context, blockID ids.ID) {
	if !t.consensus.IsAccepted(blockID) {
		return
	}

	t.mu.Lock()
	pending, exists := t.pendingBlocks[blockID]
	if !exists || pending.Decided {
		t.mu.Unlock()
		return
	}
	multiValidator := t.consensus.K() > 1
	var cert *QuorumCert
	var certBytes []byte
	if multiValidator {
		cert = t.assembleCertLocked(pending, blockID)
		if cert == nil {
			// Quorum count reached but no verified cert yet — wait for signed
			// votes. Do NOT finalize: BFT safety requires the witness.
			t.mu.Unlock()
			return
		}
		if b, err := cert.MarshalBinary(); err == nil {
			certBytes = b
		}
	}
	chainID := t.chainID
	gossiper := t.certGossiper
	t.mu.Unlock()

	// Distribute the finality proof so followers can finalize on it. Best
	// effort: local finality is already established by the verified cert.
	if multiValidator && gossiper != nil && certBytes != nil {
		_ = gossiper.GossipCert(chainID, blockID, certBytes)
	}

	t.finalizePendingLocked(ctx, blockID)
}

// finalizeOwnProposal commits a self-proposed block once finality is legitimate.
//
// THE FREEZE THIS USED TO "FIX" — AND HOW IT IS NOW FIXED WITHOUT SELF-FINALITY:
//
// The old version FORCE-ACCEPTED the proposer's own block on its lone self-vote
// (consensus.ForceAccept) because peer Chits arrived late/dropped, pinning
// acceptVotes at 1 < alpha and freezing the node. That was self-finality — a
// value could finalize with NO α-of-K agreement, so an equivocating proposer
// could fork the chain. DELETED for K>1.
//
// The freeze is now solved STRUCTURALLY by the vote-distribution topology
// (integration.go): followers gossip their SIGNED accept votes to ALL
// validators (not only back to the proposer), and the proposer assembles +
// gossips the cert. So the proposer collects alpha distinct signed votes and
// finalizes via the cert (tryFinalizeBlock) — late/dropped Chits are handled by
// re-request + cert gossip + the poll timeout, NOT by self-finalizing. If the
// quorum genuinely is not present, the node correctly does NOT finalize (a real
// minority cannot and must not finalize).
//
// Here we only do two things, both fail-closed:
//   - K==1: ForceAccept (1-of-1 quorum: the sole validator's accept is final).
//   - K>1: re-run tryFinalizeBlock, which finalizes IFF a verified cert exists.
//     Never forces.
func (t *Transitive) finalizeOwnProposal(ctx context.Context, blockID ids.ID) {
	t.mu.RLock()
	pending, exists := t.pendingBlocks[blockID]
	t.mu.RUnlock()
	if !exists || pending.Decided || !pending.IsOwnProposal {
		return
	}

	if t.consensus.K() == 1 {
		// Single-validator: the sole validator's accept IS the quorum. Force is
		// the correct 1-of-1 finalization (ForceAccept refuses K>1).
		if err := t.consensus.ForceAccept(blockID); err != nil {
			t.log.Crit("ForceAccept refused — engine misconfigured (K>1 reached single-validator path)",
				"blockID", blockID, "error", err)
			return
		}
		t.finalizePendingLocked(ctx, blockID)
		return
	}

	// Multi-validator: finalize only via a verified α-of-K cert. No force.
	t.tryFinalizeBlock(ctx, blockID)
}

// finalizePendingLocked is the shared finalization path used by both
// tryFinalizeBlock (peer-quorum-driven) and finalizeOwnProposal
// (proposer-self-driven). It assumes consensus.IsAccepted has been satisfied
// either naturally (alpha-of-K signals) or via ForceAccept (own proposal).
//
// Idempotent: subsequent calls find pending.Decided=true and no-op.
func (t *Transitive) finalizePendingLocked(ctx context.Context, blockID ids.ID) {
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

	// SetPreference must follow Accept to keep VM and consensus in sync.
	t.mu.RLock()
	vm := t.vm
	t.mu.RUnlock()
	if vm != nil {
		if err := vm.SetPreference(ctx, blockID); err != nil {
			t.log.Crit("SetPreference failed after Accept — forcing preference",
				"blockID", blockID,
				"error", err)
			t.consensus.ForcePreference(blockID)
		}
	}

	// Pipeline: block finalized → immediately build next
	t.signalPipeline()

	// MED-6: bound the slashing detector's per-height vote/block maps to a
	// sliding window below the finalized height. Equivocation is only
	// actionable near the tip; retaining every height's records unboundedly is a
	// memory-exhaustion vector. Prune everything older than the window.
	t.pruneSlashingBelowWindow()
}

// slashingRetentionHeights is how many heights below the finalized tip the
// slashing detector retains vote/block records for. Equivocation evidence is
// only useful near the tip (a fork is attempted at or above the last finalized
// height); older records cannot prove a NEW double-vote and are pruned to bound
// memory. 1024 heights is ample for cross-validator timing skew at any block
// time while keeping the maps O(window·validators).
const slashingRetentionHeights = uint64(1024)

// pruneSlashingBelowWindow drops slashing records older than the retention
// window below the finalized height. No-op when no detector is wired or when the
// chain has not advanced past the window.
func (t *Transitive) pruneSlashingBelowWindow() {
	t.mu.RLock()
	detector := t.slashingDetector
	t.mu.RUnlock()
	if detector == nil {
		return
	}
	fh, set := t.consensus.GetFinalizedHeight()
	if !set || fh <= slashingRetentionHeights {
		return
	}
	detector.PruneBelow(fh - slashingRetentionHeights)
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

	t.mu.RLock()
	vm := t.vm
	t.mu.RUnlock()

	for _, a := range toAccept {
		if a.blk != nil {
			_ = a.blk.Accept(ctx)
		}
		if vm != nil {
			if err := vm.SetPreference(ctx, a.id); err != nil {
				t.log.Crit("SetPreference failed after Accept — forcing preference",
					"blockID", a.id,
					"error", err)
				t.consensus.ForcePreference(a.id)
			}
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
		vmBlock, err := t.vm.BuildBlock(ctx)
		if err != nil {
			t.log.Error("BuildBlock failed, will retry next tick",
				"error", err,
				"pendingBuildBlocks", t.pendingBuildBlocks)
			// Do NOT decrement pendingBuildBlocks — the request is still
			// outstanding and will be retried on the next Notify or pipeline tick.
			return nil
		}
		t.pendingBuildBlocks--

		t.blocksBuilt++

		// Skip if already tracked
		if _, exists := t.pendingBlocks[vmBlock.ID()]; exists {
			continue
		}

		consensusBlock := &Block{
			id:           vmBlock.ID(),
			parentID:     vmBlock.ParentID(),
			height:       vmBlock.Height(),
			timestamp:    vmBlock.Timestamp().Unix(),
			data:         vmBlock.Bytes(),
			pChainHeight: pChainHeightOf(vmBlock), // epoch for the weighted set (MEDIUM-1)
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
				if err := t.vm.SetPreference(ctx, vmBlock.ID()); err != nil {
					t.log.Crit("SetPreference failed after Accept — forcing preference",
						"blockID", vmBlock.ID(),
						"error", err)
					t.consensus.ForcePreference(vmBlock.ID())
				}
			}
			t.mu.Lock()
		} else {
			pb := &PendingBlock{
				ConsensusBlock: consensusBlock,
				VMBlock:        vmBlock,
				ProposedAt:     time.Now(),
				VoteCount:      1,
				Decided:        false,
				IsOwnProposal:  true,
			}
			t.pendingBlocks[vmBlock.ID()] = pb
			// The proposer is one of the α signers: record its OWN signed accept
			// vote into the cert set so the assembled cert includes it (its
			// ProcessVote above counted it toward acceptVotes; this puts the
			// matching SIGNED record in certVotes so count and cert agree).
			t.recordOwnVoteLocked(pb, vmBlock.ID())
		}

		// Gossip the block + request peer votes. These calls are done while
		// holding t.mu — keep them short (msg creation + queue, no waiting).
		proposerWired := t.proposer != nil
		if proposerWired {
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

		// Proposer-self-accept: once the proposal has been broadcast (or
		// the engine is running without a network proposer in tests), the
		// proposer locally finalizes its own block. The block has already
		// been verified locally (line 1001) so the proposer has committed
		// to its correctness; waiting on peer Chits to drive the local
		// alpha-of-K threshold causes the lux-devnet stall when Chits
		// arrive late or are dropped at the network boundary. See
		// finalizeOwnProposal for the safety argument.
		//
		// alreadyAccepted=true means K=1 single-node mode already called
		// VMBlock.Accept above — skip the self-finalize to avoid double-Accept.
		if !alreadyAccepted && proposerWired {
			blockID := vmBlock.ID()
			t.mu.Unlock()
			t.finalizeOwnProposal(ctx, blockID)
			t.mu.Lock()
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
