package chain

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// PreferenceReconciler is an OPTIONAL VM capability, type-asserted from the wired
// BlockBuilder. It force-aligns the VM's accepted/preferred head to `certified` — a
// block the consensus finality ledger has just recorded — by DROPPING an UNCERTIFIED
// provisional tip that ran ahead of (or diverged from) consensus finality. This is the
// live counterpart of the offline accepted-tip rewind (luxfi/evm core.FinalizeRewind):
// the EVM's SetPreference refuses to move its head below its own accepted tip ("cannot
// orphan finalized block"), so a divergence requires an explicit reconcile primitive to
// resolve WITHOUT crashing the node.
//
// SAFETY CONTRACT (two independent gates, both must hold):
//   - Consensus calls ReconcilePreference ONLY after proving, against its OWN finality
//     ledger (byHeight), that the tip being dropped is NOT the certified block at its
//     height — so it is never asked to orphan a consensus-finalized block.
//   - The implementer MUST additionally refuse (return an error) if aligning to
//     `certified` would drop a block at or below its own irreversible finality floor
//     (the ⅔-by-stake Quasar-certified height it tracks). `certified` is Nova-final, and
//     Quasar ≤ Nova always, so reconciling TO it never crosses that floor on the honest
//     path; the gate exists so a wrong caller can never induce an unsafe rewind.
//
// A VM that does not implement this interface keeps the pre-reconcile behaviour: the
// divergence is surfaced (non-fatal) and left to offline recovery.
type PreferenceReconciler interface {
	ReconcilePreference(ctx context.Context, certified ids.ID) error
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
	// (zero until the first re-poll). The re-poll loop re-drives a block at most
	// once per its CURRENT backoff window (rePollBackoff), so a stuck block
	// recovers without a gossip storm. See rePollAllPending — this is the liveness
	// retry the topology doc promises ("vote-broadcast + cert-gossip + the
	// poll-timeout re-request"), now with exponential backoff + a hard cap.
	lastRePoll time.Time

	// rePollBackoff is the CURRENT re-poll interval for this block. It starts at
	// the base RoundTO and DOUBLES after each re-poll (capped at maxRePollBackoff),
	// so a block that is stuck because peers are behind is re-solicited on a
	// geometric schedule (RoundTO, 2·RoundTO, 4·RoundTO, …), not a 250ms hot loop.
	// Zero ⇒ "use the base interval for the first re-poll".
	rePollBackoff time.Duration

	// rePollAttempts counts how many times the re-poll loop has re-solicited this
	// block. For a NON-OWN (gossiped) block, once it reaches maxRePollAttempts the
	// block is ABANDONED for re-poll purposes (rePollAbandoned) — re-soliciting a
	// gossiped block to peers who cannot vote (they are behind its parent) is pure
	// spam and never recovers it; the catch-up path (requestCatchup) is what recovers
	// a behind frontier. An OWN proposal is never abandoned (the proposer drives it to
	// finality), so this counter only paces its bounded-backoff re-solicitation.
	rePollAttempts int

	// rePollAbandoned is set once a NON-OWN block's rePollAttempts hits the cap. An
	// abandoned block is NEVER re-polled again (no infinite spam), but is NOT deleted:
	// it remains pending and recoverable — a late cert (HandleIncomingCert) or a
	// catch-up that fills its parent can still finalize it. An OWN proposal never sets
	// this flag: it is re-solicited until it decides, so a down/wedged/forked
	// designated proposer cannot halt the chain by starving the substitute's votes.
	rePollAbandoned bool

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
// validator's local accept is the quorum, finality goes through the 1-of-1 cert →
// FinalizeBranch, and no peer certs or signatures are produced. The engine REFUSES to start a
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

// WithQuasarObserver registers a callback fired when the EXPORT (Quasar, ⅔-by-stake) frontier
// advances — the ONE seam an export surface uses to track the exportable tip. The node wires it
// to push the export-final height into the VM (EVM `finalized`/`safe`, warp export gating), so
// those surfaces read the Quasar tip and NEVER the reorgable Nova/accept tip. Fires strictly
// after the block's Nova accept, monotonically. nil is a no-op (no export surface).
func WithQuasarObserver(fn func(canonical ids.ID, height uint64)) Option {
	return func(t *Transitive) {
		t.quasarObserver = fn
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

// WithCatchup wires the engine's runtime auto-recovery seam (Catchup): the
// transport the engine uses to fetch ancestors it is missing when a gossiped
// child or a verified cert references a parent it does not have. Without it a
// follower that falls behind during normal operation is stranded (it can neither
// vote on nor finalize a block whose parent it lacks); with it the follower
// self-heals by fetching the missing chain and rejoining the frontier. Optional;
// nil keeps the legacy no-self-heal behaviour.
func WithCatchup(c Catchup) Option {
	return func(t *Transitive) {
		t.catchup = c
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
	// Consensus lock discipline:
	// Do not call VM.Accept, ReceiveVote, network send, poll delivery,
	// certificate callbacks, or any external callback while holding Transitive.mu.
	// Queue work or release the lock before call-out.
	//
	// WHY (the deadlock class this bans, not one instance): mu is a sync.RWMutex — NOT reentrant.
	// A call-out invoked while a goroutine holds mu (write) that reenters the engine and takes
	// mu.RLock self-deadlocks that goroutine forever (the live single-validator freeze: the
	// selfVoter called ReceiveVote from RequestVotes under mu). Network sends under mu are equally
	// banned — they must never sit behind the engine lock. The pattern everywhere is: capture what
	// the call-out needs UNDER mu, Unlock, do EVERY call-out (VM.Accept/Reject, Propose,
	// RequestVotes, GossipCert, SetPreference, ReceiveVote, the finalizer, signalPipeline) with mu
	// RELEASED, then re-Lock. Enforced by TestBlue_NoCallOutUnderEngineLock (a lock-instrumented
	// runtime run) + the source audit in that file.
	mu engineMutex // sync.RWMutex + a test-time write-holder instrument (lock_invariant.go)

	// Core consensus
	consensus *ChainConsensus
	params    config.Parameters

	// Dependencies
	vm       BlockBuilder
	proposer BlockProposer

	// convergenceVoter, when wired by the Runtime, is the SOLE per-height accept-vote
	// emitter for a multi-validator (K>1) engine. Rather than binding this node's one
	// signature to whatever block it BUILT or FIRST-SAW — which fragments the vote
	// across conflicting siblings during a fresh-net storm and stalls the quorum with
	// no single block reaching α — it emits the signature for the DETERMINISTICALLY
	// CONVERGED winner at a height: the lowest signed-canonical among the live
	// siblings. Every honest node with the same tracked set picks the SAME winner, so
	// they converge their one vote onto ONE block and exactly one α-of-K cert forms
	// per height (the cert thus CERTIFIES the converged decision — it is never an
	// independent finality path that could certify a conflicting sibling). nil in
	// single-engine tests (which inject votes directly); wired in NewRuntime.
	convergenceVoter ConvergenceVoter

	// Runtime state
	ctx          context.Context
	cancel       context.CancelFunc
	bootstrapped bool
	started      bool
	wg           sync.WaitGroup // tracks background goroutines

	// bootstrapPhase is true while the node is INITIAL-SYNCING — fetching and
	// executing the chain from a peer's accepted frontier down to its local tip —
	// and false once it has reached the frontier and entered live consensus. It is
	// the SOLE gate on the bootstrap accept authority (Runtime.AcceptBootstrapBlock):
	// a block fetched-from-frontier-and-re-executed may finalize WITHOUT an α-of-K
	// cert ONLY while this is true. The instant the node goes live (FinishBootstrap),
	// the bootstrap path is fail-closed and finality flows ONLY through the
	// cert-witnessed α-of-K road — so the weak-subjectivity-on-the-beacon-set trust
	// of bootstrap can never be used to bypass the live cert-gate. Defaults true at
	// construction (a fresh engine is bootstrapping); the node flips it false exactly
	// when it signals the chain bootstrapped.
	bootstrapPhase bool

	// Two views of the same *PendingBlock: pendingBlocks by outer envelope ID
	// (transport), pendingOwnProposals by ProposalKey (consensus position).
	// One own proposal per ProposalKey — this is what stops proposervm
	// re-wraps from splitting votes. dropPendingBlockLocked is the sole unwrite.
	pendingBlocks       map[ids.ID]*PendingBlock
	pendingOwnProposals map[ProposalKey]*PendingBlock
	pendingBuildBlocks  int

	// finalizedByCert is the engine's authoritative finality record: the set of
	// block IDs that were committed through the SOLE cert-gated finalizer
	// (AcceptWithCert, which requires a VerifiedQuorumCert). It is deliberately
	// SEPARATE from the consensus core's block.accepted / finalizedByHeight, which
	// the α-of-K COUNT path populates directly (consensus.go marks a block accepted
	// on acceptVotes>=alpha). A block sitting at count-α but lacking the verified
	// cert is "accepted" in the consensus core but is NOT in this set — and
	// Transitive.IsAccepted reports THIS set, so the engine never claims finality
	// for a block it refused to finalize. Bounded by the same finalize cadence as
	// the chain (one entry per finalized height); a production node prunes it
	// alongside the slashing window if retention ever matters.
	finalizedByCert map[ids.ID]struct{}

	// certBytesByBlock persists the marshaled finality cert for each block this node
	// finalized, so it can SERVE that cert to a peer catching up (CertForBlock). It
	// is written at the SOLE finalizer (acceptWithCertCore), so every finalize path
	// captures its cert in ONE place. certServedOrder is the companion FIFO of block
	// ids in finalize (== ascending height) order, used to evict the oldest cert once
	// the store passes maxServedCerts — a bounded sliding window, never an unbounded
	// map. A node lagging beyond the window bootstraps instead of catching up.
	certBytesByBlock map[ids.ID][]byte
	certServedOrder  []ids.ID

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

	// presetK is the TARGET committee sample size — the configured K BEFORE the construction-time
	// clamp to the live validator count (bftCommittee). reclampCommitteeLocked re-clamps the live
	// committee UP toward it as the validator set grows, so a chain that launched single-validator
	// tracks its set instead of staying stuck at K=1 (the 1→N decentralization fork). 0 ⇒ no
	// re-clamp (K stays exactly as constructed — the path for tests / --dev / a chain with no
	// preset). Wired by the Runtime.
	presetK int
	// liveValidatorCount reports the CURRENT validator count for THIS chain's network (wired by the
	// Runtime from the validator sampler); nil for tests / --dev without a sampler. It is read at
	// each finalize decision so both the committee (reclampCommitteeLocked) and the single-validator
	// synthesis guard (buildSingleValidatorCertLocked) track the LIVE set rather than the frozen
	// construction-time count. A wired sampler returns 0 when the set is not yet resolved for this
	// network, ≥1 once loaded.
	liveValidatorCount func() int

	// committedSlot enforces the per-HEIGHT NON-EQUIVOCATION safety rule (SlotKey =
	// {height}): this node signs an accept vote for AT MOST ONE canonical block per
	// value-chain height — regardless of the block's validator-set epoch. Signing two
	// conflicting siblings at one height would place this node's stake in two
	// conflicting ⅔-quorums and break the quorum-intersection argument that gives
	// f<n/3 safety — the exact cross-node fork (two valid siblings each gathering a
	// legit cert). The epoch was DELIBERATELY removed from the key: it is a
	// proposer-chosen axis (ValidatorSetRoot(block.pChainHeight)) that differs between
	// honest siblings at one height, so keying on it FRAGMENTED the slot and let one
	// validator sign both siblings → the fresh-net double-finalization fatal. The
	// epoch binding remains in the SIGNED message + cert verification, where it stops
	// cross-epoch cert forgery. Keyed SlotKey → bound canonical id:
	// a second DIFFERENT canonical at a bound slot is REFUSED (never signed); the SAME
	// one is idempotent (safe re-solicit). Guarded by its own slotMu so BOTH signing
	// sites can call it — recordOwnVoteLocked (under t.mu) and the follower path in
	// followVerifiedBlock (t.mu released). Pruned below the finalized height (a
	// finalized height can never legitimately re-sign — pruneCommittedSlotsBelow, run
	// from the sole finalizer acceptWithCertCore).
	//
	// voteGuard (optional) is the DURABLE backing (HIGH-1): every new binding is
	// fsync'd BEFORE the vote is cast and reloaded on startup, so the guard's memory
	// spans a crash/restart. nil = memory-only (verify-only nodes and tests; Start
	// warns a signer that has none). See vote_guard.go.
	slotMu        sync.Mutex
	committedSlot map[SlotKey]ids.ID
	// decidedFloor is the DURABLE, MONOTONIC decided-through height: the highest height
	// this node has ever finalized. The sign gate refuses signing at any height <= it, so a
	// decided height is permanently unsignable EVEN AFTER its committedSlot entry is pruned
	// (the strictly-below prune persists the removal of below-tip slots). It is seeded on
	// boot from the vote-guard file's fsync'd floor (WithVoteGuard) — the authoritative
	// durable source that never lags the consensus decision — and advanced at each finalize
	// (pruneCommittedSlotsBelow). The gate also folds in consensus.GetDecidedFloor() (the
	// certified height OR the vm.LastAccepted hint) as a complementary lower bound. This is
	// the CROSS-RESTART backstop: without it, after a rolling restart mid-storm the certified
	// ledger.Height() is a (0,false) hint (PART-A) and the below-tip slots are gone, so a
	// re-gossiped sibling at a decided height could collect a SECOND signature. SIGN-GATE
	// ONLY — never enters the finality ledger, byHeight, or the equivocation index (PART-A).
	// Guarded by slotMu (lives with committedSlot).
	decidedFloor uint64
	voteGuard    VoteGuardStore

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
	// QuorumCert into a quasar.WeightedQuorumCert — see quasar.go). It is
	// REQUIRED for Mode() to report ModeQuorumFinality on a strict-PQ chain: without it the
	// cert path cannot produce the PQ (quasar) witness the profile demands, so the value-
	// DEX gate must not certify a quorum-finality regime that cannot be witnessed post-
	// quantum. On a non-strict chain it is unused. Injected like voteVerifier/stakeSource so
	// the gate reads the SAME field that delivers the witness — nil means "PQ witness not
	// plumbed", exactly the semantics ToQuasarCert already relies on.
	cryptoWitness CryptoWitnessSource

	// quasarAttestor (optional) is the trailing EXPORT-cert assembler — the ONE
	// producer of the ⅔-by-stake Quasar certificate, run strictly AFTER a Nova accept.
	// Nova (the majority accept cert, assembleCertLocked) is the SOLE decider of VM.Accept
	// and chain liveness; this sidecar is an ARTIFACT builder that NEVER gates acceptance
	// (an absent/late/insufficient Quasar cert leaves a block Nova-only — the degraded
	// mode). promoteQuasarLocked feeds it the just-accepted block's own verified accept
	// votes (which are byte-identical attestations of the same canonical position), and
	// when a ⅔-stake supermajority has attested it emits the export cert and advances the
	// consensus Quasar frontier. Wired ONLY when both voteVerifier and stakeSource are
	// present (a stake-weighted chain that can export); nil on an equal-stake/dev/K==1 chain
	// (Nova-only, no cross-chain export). See attestation.go.
	quasarAttestor *QuasarAttestor
	// quasarObserver (optional) is notified when the EXPORT (Quasar, ⅔-by-stake) frontier
	// advances — the ONE seam an export surface (a bridge, the EVM `finalized`/`safe` tag, the
	// warp export gate) subscribes to so it reads the Quasar tip, NEVER the reorgable Nova/accept
	// tip. Called by ingestAttestation strictly AFTER the block's Nova accept, monotonically, WITHOUT
	// t.mu. nil on a chain with no export surface. Set via WithQuasarObserver.
	quasarObserver func(canonical ids.ID, height uint64)
	// acceptedPosMu guards acceptedPos. acceptedPos remembers the signed VotePosition + P-chain
	// epoch of a Nova-accepted block, keyed by its outer id, so a LATE accept vote — the ⅔-th
	// stake vote necessarily trails the bare-majority accept and arrives when the pending block is
	// already dropped — can still be attested (verified against the exact signed bytes, at the
	// exact epoch, both carried as VALUES on the record) and complete the export cert. Bounded to
	// the attestor window (pruneQuasarBelow).
	acceptedPosMu sync.Mutex
	acceptedPos   map[ids.ID]acceptedPos
	// responsiveStakeNum/Den record the stake that voted on the most recently accepted block
	// (numerator) out of the epoch's total (denominator) — the degraded-mode RPC signal
	// (responsiveStakePct + certificateAvailable). Guarded by t.mu. Zero denominator ⇒
	// "unknown" (no stake-weighted accept observed yet).
	responsiveStakeNum uint64
	responsiveStakeDen uint64

	// catchup (optional) is the engine's seam for runtime auto-recovery when it
	// falls behind — see Catchup. When a gossiped child or a verified cert
	// references a parent this node does not have, the engine asks catchup to
	// fetch the missing ancestors (requestCatchupLocked) instead of silently
	// dropping the child (the old behaviour that stranded a behind follower). nil
	// disables self-healing (legacy). The engine owns idempotency + rate-limiting
	// so the implementation can be a thin adapter onto the network getter.
	catchup Catchup

	// catchupRequested rate-limits ancestor fetches: it remembers the missing
	// parent IDs we have already asked for and when, so a re-gossip of the same
	// orphan (or many children of one missing parent) issues ONE fetch per
	// catchupCooldown, never a fetch storm. Keyed by the MISSING block ID.
	//
	// BOUNDED two ways (both fail-closed), because a Byzantine validator can stream
	// votes for forged random IDs that never arrive:
	//   - Reclaim-on-known: an entry is deleted the moment its block becomes TRACKED
	//     or DECIDED — at the accept span, the reject site, the sync reset, and at
	//     claimCatchupLocked's early returns (already-tracked / known-to-consensus).
	//     A block that actually arrives reclaims its slot; honest entries never pile
	//     up.
	//   - Hard cap + TTL: a forged ID that never arrives is never reclaimed above, so
	//     claimCatchupLocked refuses to grow the map past maxCatchupRequested — at
	//     the cap it sweeps entries older than catchupRequestTTL and, if still full
	//     (an active young flood), refuses the new claim. The map can never exceed
	//     maxCatchupRequested.
	catchupRequested map[ids.ID]time.Time

	// certCatchupRequested rate-limits the STUCK-ROUND cert-catch-up (the one-lagging-
	// validator freeze fix). Keyed by HEIGHT, not block ID, because that case fetches the
	// finality CERT for a block we ALREADY hold (tracked, but unfinalizable because the
	// fleet finalized this height and will not re-vote it): the block-ID gate
	// (claimCatchupLocked) would wrongly SUPPRESS it as "already tracked". Same cooldown +
	// hard cap as catchupRequested; reaped at/below the finalized floor (a height, once
	// decided, is never fetched again). See claimCertCatchupLocked / Runtime.requestCertCatchup.
	certCatchupRequested map[uint64]time.Time

	// bufferedVotes parks signed accept/reject votes that arrived for a block this
	// node does not yet TRACK (the gossip race: a peer's vote can outrun the block
	// bytes). The old handleVote DROPPED such a vote — and because votes are only
	// solicited once, a dropped vote was lost forever, so a follower that missed
	// the block bytes could never reach the α-of-K quorum and the block wedged. We
	// instead BUFFER the vote (no signature work yet) and fetch the missing block
	// via the SAME catch-up seam used for a missing parent; when the block lands at
	// a tracking site, drainBufferedVotes replays these through the normal channel
	// path so each is signature-verified exactly as a live vote (buffering never
	// bypasses the gate). Keyed by the voted-on (missing) BlockID, and within each
	// block deduped by NodeID: at most ONE buffered vote per (BlockID, validator)
	// — a repeat from the same NodeID REPLACES its parked vote, never appends — so
	// the per-block slice is bounded by DISTINCT voters, not raw arrivals (the dual
	// of certVotes' NodeID keying; defeats single-Byzantine-ID buffer crowd-out).
	// Bounded by maxBufferedVoteBlocks distinct keys and maxBufferedVotesPerBlock
	// distinct NodeIDs per key (fail-closed: a new vote past a cap is dropped,
	// never evicting an existing one). Drained on track, deleted on decide — it
	// cannot leak.
	bufferedVotes map[ids.ID][]Vote

	// requestMissing is the engine's hook into the runtime's catch-up TRANSPORT
	// (Runtime.requestCatchup): "I am missing block `id` — fetch it from `from`".
	// It is the SAME one mechanism the missing-PARENT self-heal uses; the engine
	// (which does not hold the networkID) signals WHAT to fetch and the runtime
	// supplies the networkID + RequestAncestors round-trip. nil when no runtime is
	// wired (a bare engine in a unit test that drives delivery itself), in which
	// case a buffered vote simply waits for the block to be delivered by other
	// means. Set by NewRuntime; idempotency + rate-limiting stay in the engine
	// (claimCatchupLocked), so this is a thin signal, never a second transport.
	requestMissing func(missingID ids.ID, from ids.NodeID)

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

// Catchup is the engine's SOLE seam for "I am behind — fetch the block(s) I am
// missing". It is the one mechanism for runtime auto-recovery (decomplected from
// the finality path): the engine expresses WHAT (fetch the ancestors rooted at a
// missing block ID), the node decides HOW (a GetAncestors/Get round-trip on its
// existing network transport, delivering the fetched blocks back through
// HandleIncomingBlock).
//
// It is wired by the node (WithCatchup); when nil the engine simply does not
// self-heal a behind state (the legacy behaviour — a follower that falls behind
// at runtime is stranded). Idempotency and rate-limiting live in the ENGINE
// (requestCatchupLocked), so an implementation may be a thin, stateless adapter
// onto the network getter.
type Catchup interface {
	// RequestAncestors asks a peer to deliver the chain of blocks ending at
	// missingBlockID (the parent a gossiped child / verified cert referenced but
	// which this node does not have). `from` is the peer that advertised the
	// child — the natural source to fetch its parent from. chainID/networkID
	// scope the request to this chain's validator network. The fetched blocks are
	// expected to arrive via HandleIncomingBlock, at which point the formerly
	// orphaned child can be tracked, voted, and finalized.
	RequestAncestors(chainID ids.ID, networkID ids.ID, missingBlockID ids.ID, from ids.NodeID) error
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
		consensus:            NewChainConsensus(cfg.Params.K, cfg.Params.AlphaPreference, int(cfg.Params.Beta)),
		params:               cfg.Params,
		vm:                   cfg.VM,
		proposer:             cfg.Proposer,
		bootstrapPhase:       true, // a fresh engine is initial-syncing until it reaches the frontier
		pendingBlocks:        make(map[ids.ID]*PendingBlock),
		pendingOwnProposals:  make(map[ProposalKey]*PendingBlock),
		finalizedByCert:      make(map[ids.ID]struct{}),
		certBytesByBlock:     make(map[ids.ID][]byte),
		committedSlot:        make(map[SlotKey]ids.ID),
		catchupRequested:     make(map[ids.ID]time.Time),
		certCatchupRequested: make(map[uint64]time.Time),
		bufferedVotes:        make(map[ids.ID][]Vote),
		voteRequests:         make(chan VoteRequest, cfg.VoteRequestBuffer),
		votes:                make(chan Vote, cfg.VoteBuffer),
		pipelineSignal:       make(chan struct{}, 1),
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

	// Wire the trailing EXPORT-cert assembler (the ONE Quasar producer) when the chain can
	// export: a stake-weighted chain with a vote verifier. It runs strictly AFTER Nova accept
	// and NEVER gates it. An equal-stake/dev/K==1 chain has no cross-chain export surface, so
	// it stays Nova-only (nil attestor) — GetQuasarTip is simply ids.Empty there. The epoch
	// map bridges the attestor's height→epoch resolution to the block's recorded P-chain
	// epoch (identity for a fixed set). Constructed once, here, so the Quasar frontier has a
	// single producer.
	if t.voteVerifier != nil && t.stakeSource != nil && t.quasarAttestor == nil {
		t.quasarAttestor = NewQuasarAttestor(t.voteVerifier, t.stakeSource)
	}

	// A signing validator SHOULD have a durable equivocation guard so a crash between
	// signing and finalizing cannot forget a per-height binding and permit a fork
	// (HIGH-1). Memory-only is correct for verify-only nodes and tests; in production
	// the node wires WithVoteGuard. Warn (don't fail) so tests/dev keep working while
	// the gap is visible.
	if t.voteSigner != nil && t.voteGuard == nil {
		t.log.Warn("vote-once: signing WITHOUT a durable equivocation guard — a crash between " +
			"signing and finalizing may permit equivocation; wire WithVoteGuard in production")
	}

	// BOOT SEED (v1.35.5): seed the durable decided-height floor DIRECTLY from the VM's
	// last-accepted height BEFORE the signing goroutines launch. This makes the sign gate
	// safe from the first instant of boot — even for a node upgrading in place from a legacy
	// v1 vote-guard file (finalizedThrough=0) whose certified ledger is a (0,false) hint
	// until the first post-upgrade finalize (PART-A). Without it, the floor would be 0 in
	// that window and a re-gossiped sibling at a decided-below-tip height could be signed
	// (the mainnet v1→v2 in-place upgrade window). vm.LastAccepted is a durable, sound lower
	// bound on the decided height (every accepted block was finalized). SIGN-GATE ONLY — it
	// only advances t.decidedFloor, never the finality ledger / byHeight (PART-A intact).
	// UNLOCK-BEFORE-CALL-OUT: the seed reads the VM (LastAccepted/GetBlock). Release t.mu around it
	// (the signing goroutines have not launched yet, but the discipline is global — no external
	// call-out under the engine lock). The seed touches only t.vm (stable pre-Start) + slotMu.
	t.mu.Unlock()
	t.seedDecidedFloorFromVM(ctx)
	t.mu.Lock()

	t.ctx, t.cancel = context.WithCancel(ctx)
	t.bootstrapped = true
	t.started = true

	// Capture ctx in local variable to avoid race with struct field access
	engineCtx := t.ctx

	t.wg.Add(5)
	go t.pollLoopWithCtx(engineCtx)
	go t.voteHandlerWithCtx(engineCtx)
	go t.pipelineLoop(engineCtx)
	go t.rePollLoopWithCtx(engineCtx)
	go t.convergenceLoopWithCtx(engineCtx)

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

// InBootstrapPhase reports whether the engine is still INITIAL-SYNCING (fetching
// + executing the chain from the network frontier). It is the gate the bootstrap
// accept authority (Runtime.AcceptBootstrapBlock) checks: a fetched-from-frontier
// block may finalize without an α-of-K cert ONLY while this is true. Once the node
// reaches the frontier (FinishBootstrap) it returns false and the bootstrap path is
// fail-closed — the live cert-gate is the only finalizer thereafter.
func (t *Transitive) InBootstrapPhase() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.bootstrapPhase
}

// FinishBootstrap ends the bootstrap phase: the node has executed up to the
// discovered frontier and is entering live consensus. After this call the bootstrap
// accept path is fail-closed (InBootstrapPhase == false) and finality flows ONLY
// through the cert-witnessed α-of-K road. The node MUST call this exactly when it
// signals the chain bootstrapped (so the two transitions — "accept without a cert"
// and "no longer accept without a cert" — happen together). Idempotent.
func (t *Transitive) FinishBootstrap() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.bootstrapPhase = false
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

	// Seed the durable decided-height floor DIRECTLY from the reconciled last-accepted
	// height (v1.35.5). SyncStateFromVM passes vm.LastAccepted here, so this keeps
	// t.decidedFloor at the VM head independently of the consensus ledger's hint — which
	// lives on a different lock and can be cleared by an empty/genesis reset. Monotonic,
	// sign-gate-only (never touches byHeight / ledger.Height() — PART-A intact).
	if height > 0 {
		t.slotMu.Lock()
		if height > t.decidedFloor {
			t.decidedFloor = height
		}
		t.slotMu.Unlock()
	}

	// Clear any pending blocks that are now stale (below the synced height)
	for blockID, pending := range t.pendingBlocks {
		if pending.ConsensusBlock != nil && pending.ConsensusBlock.height <= height {
			t.dropPendingBlockLocked(blockID)
			// Votes parked for a stale (now-synced-past) block will never be drained
			// — drop them so a sync cannot leave buffered-vote residue.
			delete(t.bufferedVotes, blockID)
			// Same for its catch-up throttle entry: synced past ⇒ never re-fetched.
			delete(t.catchupRequested, blockID)
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

// SetLogger sets the engine logger after construction. The integration layer
// builds the engine via NewWithParams (which carries no logger), so without
// this setter the engine keeps its log.Noop() default and EVERY internal
// decision — build-loop drops, verify failures, AddBlock rejections, vote-path
// faults — is silently discarded. A rebuild storm ran 4M iterations with zero
// engine log lines because of exactly that. Nil / zero loggers are ignored so
// callers can pass their config logger unconditionally.
func (t *Transitive) SetLogger(l log.Logger) {
	if l == nil || l.IsZero() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.log = l
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

// IsAccepted reports whether the block has been FINALIZED by this engine —
// committed through the SOLE cert-gated finalizer (AcceptWithCert), which is
// reachable only with a VerifiedQuorumCert. It is the engine's finality truth,
// NOT a vote count.
//
// CRITICAL: this no longer forwards consensus.IsAccepted. That is the raw α-of-K
// COUNT predicate — consensus.go sets block.accepted=true (and even populates its
// per-height finalized ledger) the instant acceptVotes>=alpha, with NO stake
// check. On a stake-weighted chain a low-stake/high-count coalition flips that
// count true WITHOUT a ⅔-stake supermajority, so reporting it would leak a
// finality claim the engine REFUSED to act on (no VM.Accept ran). Reading the
// engine's own finalizedByCert set — written only by AcceptWithCert — makes
// "accepted" mean exactly "finalized with a verified cert". A block stuck at
// count-α but lacking the cert is correctly NOT accepted here.
func (t *Transitive) IsAccepted(blockID ids.ID) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	_, ok := t.finalizedByCert[blockID]
	return ok
}

// HasEnoughResponsesForRetry reports the LIVENESS signal: α-of-K validators have
// responded for the block (consensus.IsAccepted's raw count). This is the old
// IsAccepted count predicate, renamed to make its role unmistakable — it returns
// a RETRY signal, NEVER a finality verdict. A trigger may use it to decide
// whether it is worth calling TryAccept; it must NEVER itself finalize, and it
// does not increment blocksAccepted or touch the VM. Finality is decided solely
// by AcceptWithCert holding a VerifiedQuorumCert.
func (t *Transitive) HasEnoughResponsesForRetry(blockID ids.ID) bool {
	return t.consensus.IsAccepted(blockID)
}

// Preference returns preferred block.
func (t *Transitive) Preference() ids.ID {
	return t.consensus.Preference()
}

// PreferredBuildTip returns the deterministic build target — the deepest verified
// block extending the finalized chain — so the VM builds on the convergent tip
// (one block per height) instead of a competing sibling. See
// ChainConsensus.PreferredBuildTip.
func (t *Transitive) PreferredBuildTip() ids.ID {
	return t.consensus.PreferredBuildTip()
}

// HeldBuildTip is PreferredBuildTip filtered by the SINGLE-STORE INVARIANT: only ever
// name a block the VM actually HOLDS, else [fallback].
//
// PreferredBuildTip is a consensus-DAG value — the deepest VERIFIED, not-yet-accepted
// block. A validator that fell behind has a DAG tip ABOVE its own frontier, so the tip
// can name an id this VM cannot serve builds on. Ava never has this problem: it steers
// at Consensus.Preference(), which by construction was Verified INTO the VM
// (ava snow/engine/snowman/voter.go SetPreference, engine.go SetPreference).
//
// Steering anyway is silently lossy, not loud: our proposervm keeps its prior preference
// and returns nil on an unheld id (node/vms/proposervm/vm.go SetPreference), so an
// unguarded steer drops BOTH the tip AND the caller's own target — the VM keeps building
// on its old head after a finalize, with no error to react to. Resolving the tip here
// gives every steer site one answer computed one way.
//
// Callers MUST NOT hold t.mu: this calls into the VM.
func (t *Transitive) HeldBuildTip(ctx context.Context, vm BlockBuilder, fallback ids.ID) ids.ID {
	if vm == nil {
		return fallback
	}
	tip := t.PreferredBuildTip()
	if tip == ids.Empty || tip == fallback {
		return fallback
	}
	if _, err := vm.GetBlock(ctx, tip); err != nil {
		return fallback
	}
	return tip
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

// Re-poll backoff/cap constants. The re-poll loop is a RARE liveness BACKSTOP,
// not a hot loop: a block stuck because peers are BEHIND its parent is never
// recovered by re-soliciting the same block (those peers cannot vote), so we
// re-poll on a geometric schedule and then STOP, leaving recovery to the
// catch-up path (which fetches the missing chain so peers can vote).
const (
	// maxRePollBackoff caps the per-block re-poll interval. Starting from the base
	// RoundTO and doubling, the interval climbs RoundTO, 2·, 4·, … but never
	// exceeds this — so even a long-lived pending block is re-polled at most once
	// every maxRePollBackoff, turning a 250ms storm into a trickle.
	maxRePollBackoff = 16 * time.Second

	// maxRePollAttempts is the hard cap on re-poll attempts for a NON-OWN (gossiped)
	// block. After this many re-solicitations such a block is abandoned for re-poll
	// (rePollAbandoned) — it is never re-polled again (but stays pending, recoverable
	// via cert or catch-up): re-soliciting a gossiped block whose voters are behind
	// its parent is spam that re-poll cannot fix. An UNDECIDED OWN proposal is NEVER
	// abandoned (see rePollAllPending): the proposer keeps re-soliciting it until it
	// finalizes, matching avalanchego's "re-poll a processing block until decided".
	// With doubling from a 250ms base, 8 attempts span
	// 0.25+0.5+1+2+4+8+16+16 ≈ 48s of backstop before giving up on a gossiped block.
	maxRePollAttempts = 8

	// catchupCooldown rate-limits ancestor fetches per missing parent: at most one
	// RequestAncestors per missing block ID per cooldown, so many children of one
	// missing parent (or repeated gossip of one orphan) cannot become a fetch
	// storm. The cooldown is generous because a fetch is a round-trip that, once it
	// lands, removes the orphan condition entirely.
	catchupCooldown = 2 * time.Second

	// maxCatchupRequested HARD-bounds the catchupRequested throttle map so a
	// Byzantine validator streaming votes for forged random BlockIDs (which never
	// arrive, so the delete-on-track/decide reclaim never fires for them) cannot
	// grow it without limit. Layer-1 (delete-on-track/decide + the early-return
	// reclaims in claimCatchupLocked) keeps honest entries from accumulating; this
	// hard cap is layer-2 (defence in depth) for the all-forged flood where layer-1
	// never reclaims. At the cap we first TTL-sweep, then fail closed (refuse the
	// new claim) rather than grow — so the map can never exceed this size.
	maxCatchupRequested = 4096

	// catchupRequestTTL is the age past which a catchupRequested entry is reclaimed
	// by the at-cap sweep. It is far beyond catchupCooldown: an honest fetch either
	// lands (and is reclaimed by delete-on-track) or is abandoned well inside 30s,
	// so a still-young entry at the cap signals an active forged flood — which we
	// answer by refusing new claims, never by unbounded growth.
	catchupRequestTTL = 30 * time.Second

	// stuckRoundThreshold is the view-change round count past which a height that has
	// spun with NO POL and is NOT finalized is treated as the "fleet passed me"
	// liveness-stall signature — the one-lagging-validator freeze. A healthy split
	// re-converges in a handful of rounds (round-skip + POL), so ≥8 fruitless rounds
	// means the OTHER validators finalized this height and will not re-vote it: no POL
	// can ever form here. At that point the node stops waiting for a quorum that will
	// never come and pulls the accepted block + its α-of-K cert directly
	// (Runtime.requestCertCatchup → AcceptCatchupBlock). Same value as the historical
	// STUCK log threshold, now shared by the log and the self-heal trigger.
	stuckRoundThreshold = uint32(8)

	// maxBufferedVotesPerBlock caps how many votes we park for ONE missing block.
	// One vote per validator per block is the natural ceiling, so this must be ≥ the
	// largest supported validator set or the buffered fast-path silently drops
	// genuine votes 257..N and a net with α > cap cannot finalize from buffered votes
	// alone (recoverable only via re-poll). node/config/tokenomics.go defines a
	// 500-validator tier (and an unlimited tier); on a K=N / α=⌈⅔N⌉ chain with
	// N=500, α≈334. 512 covers the 500-tier with margin. A flood beyond the cap for
	// a single block ID is dropped (fail-closed) — the genuine α-of-K voters fit.
	// The bound stays small: 512 × maxBufferedVoteBlocks × ~64B ≈ 33MB worst case.
	maxBufferedVotesPerBlock = 512

	// maxBufferedVoteBlocks caps how many DISTINCT missing block IDs we will park
	// votes for at once. A spam stream of votes for never-delivered random block
	// IDs cannot grow the buffer past this many keys: once full, votes for a NEW
	// block ID are dropped (we do NOT evict an existing key — the simplest sound
	// bound). Happy-path keys are removed on drain (the block arrived) or on decide,
	// so this ceiling is only ever approached under adversarial junk.
	maxBufferedVoteBlocks = 1024
)

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
// (topology.go) already promises but that was never wired.
//
// It is a pure liveness retry: it re-solicits votes and re-attempts cert
// assembly, and changes NOTHING about the finality predicate. A block still
// finalizes only on a verified α-of-K cert (multi-validator) or the 1-of-1 force
// (single-validator); a genuine minority still cannot and does not finalize.
//
// The ticker wakes on the base RoundTO, but each block is gated by its OWN
// EXPONENTIAL BACKOFF (PendingBlock.rePollBackoff, doubling per attempt, capped)
// and a HARD ATTEMPT CAP (maxRePollAttempts): a stuck block is re-driven on a
// geometric schedule and then ABANDONED for re-poll — turning the former
// fixed-cadence 250ms storm (the devnet self-DoS) into a bounded trickle. A
// behind follower is NOT recovered by re-polling (it cannot vote without the
// parent); it is recovered by the catch-up path that fetches the missing chain.
func (t *Transitive) rePollLoopWithCtx(ctx context.Context) {
	defer t.wg.Done()

	// Wake on the base round budget; per-block backoff decides whether a given
	// block is actually due. Fall back to a conservative 250ms if unset.
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

// convergenceSettleWindow is how long a fork slot must have been OBSERVED (since this
// node first tracked any block at that height) before this node casts its one accept
// vote for the converged winner. It gives near-simultaneous sibling proposals time to
// gossip in, so every honest node has the SAME sibling set and selects the SAME
// lowest-canonical winner — the difference between a converging quorum and a permanent
// vote split. Derived from the round budget but clamped to a small band: long enough to
// cover intra-cluster gossip, short enough to add negligible per-block latency.
func (t *Transitive) convergenceSettleWindow() time.Duration {
	// An operator-set window wins outright — DECOUPLED from RoundTO so a high-latency (WAN)
	// validator set can lengthen the settle for its p99 gossip WITHOUT slowing the round
	// cadence (the M-liveness / N4 gate: prod RoundTO 250-400ms yields a 150ms auto window,
	// too tight for a 100-300ms-p99 WAN under a storm). A floor is still applied so a
	// misconfigured near-zero value cannot disable the settle entirely.
	if t.params.ConvergenceSettleWindow > 0 {
		w := t.params.ConvergenceSettleWindow
		if w < 150*time.Millisecond {
			w = 150 * time.Millisecond
		}
		return w
	}
	// AUTO: half the round budget — collect competing proposals for half a round, then vote
	// the lowest-canonical winner. This must comfortably exceed the sibling-gossip latency
	// so every honest node has the SAME sibling set before it binds its one signature — a
	// settle shorter than gossip lets a node settle on its OWN block before peers' arrive,
	// which splits the vote and, under one-signature-per-height, is unrecoverable. Clamped
	// so a tiny test round still gives a workable window and a huge production round does
	// not stall block production waiting to vote.
	w := t.params.RoundTO / 2
	if w < 150*time.Millisecond {
		w = 150 * time.Millisecond
	}
	if w > 2*time.Second {
		w = 2 * time.Second
	}
	return w
}

// convergenceLoopWithCtx drives the per-height vote CONVERGENCE. On a fast tick it asks
// the wired ConvergenceVoter to sweep every undecided, still-unsigned fork slot whose
// settle window has elapsed and cast this node's one accept vote for the deterministic
// winner (lowest-canonical live sibling). This is what makes a fresh-net storm — where
// many validators build competing siblings at one height — converge: instead of each
// node binding its signature to the block it built or first-saw (a 5-way split that
// never reaches α), all honest nodes independently pick the SAME winner and one α-of-K
// cert assembles. A no-op when no convergence voter is wired (single-engine tests, which
// inject votes directly). K is re-checked EVERY tick, NOT captured once (N2): a net that
// grows from K==1 (its own accept is the quorum, no convergence needed) to K>1 without an
// engine restart must begin converging the moment the set expands — gating the whole
// goroutine on the K seen at start would leave such a chain permanently unable to converge.
func (t *Transitive) convergenceLoopWithCtx(ctx context.Context) {
	defer t.wg.Done()
	if t.convergenceVoter == nil {
		return
	}
	tick := t.convergenceSettleWindow() / 3
	if tick < 20*time.Millisecond {
		tick = 20 * time.Millisecond
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if ctx.Err() != nil {
				return
			}
			if t.consensus.K() <= 1 {
				continue // single-validator right now: its own accept is the quorum
			}
			t.convergenceVoter.RunSettlePass(ctx)
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
//
// base is the base re-poll interval (RoundTO). Each block is gated by its OWN
// backoff window (rePollBackoff, doubling per attempt from base, capped at
// maxRePollBackoff) and abandoned once rePollAttempts reaches maxRePollAttempts.
func (t *Transitive) rePollAllPending(ctx context.Context, base time.Duration) {
	// K==1: no peer votes are ever needed; nothing to re-solicit.
	if t.consensus.K() <= 1 {
		return
	}

	now := time.Now()

	// Snapshot the blocks due for a re-poll under the lock, then act without it
	// (RequestVotes / tryFinalizeBlock take their own locks). A block is due once
	// it has been undecided for its CURRENT backoff window since the later of
	// ProposedAt and its last re-poll — so the FIRST re-poll waits one base
	// interval after proposal (giving the normal fast path time to finalize), then
	// the window DOUBLES each attempt, and after the cap the block is abandoned.
	type due struct {
		blockID   ids.ID
		blockData []byte
		ownProp   bool
	}
	var dueBlocks []due
	t.mu.Lock()
	for blockID, pending := range t.pendingBlocks {
		if pending.Decided || pending.rePollAbandoned {
			continue
		}
		// The window for THIS attempt: base for the first, doubling thereafter,
		// capped. rePollBackoff carries the PREVIOUS window (0 before the first).
		window := pending.rePollBackoff
		if window <= 0 {
			window = base
		}
		last := pending.lastRePoll
		if last.IsZero() {
			last = pending.ProposedAt
		}
		if now.Sub(last) < window {
			continue
		}

		// This block is due. Record the attempt and advance the backoff (double,
		// cap) so re-solicitation is a bounded trickle (≤ maxRePollBackoff), never a
		// storm.
		pending.lastRePoll = now
		pending.rePollAttempts++
		next := window * 2
		if next > maxRePollBackoff {
			next = maxRePollBackoff
		}
		pending.rePollBackoff = next
		// LIVENESS (the down/wedged/forked-proposer halt): an UNDECIDED OWN proposal
		// is NEVER abandoned. This node BUILT it on the finalized tip (its voters
		// therefore HAVE the parent and CAN vote), and as the proposer it owns driving
		// it to an α-of-K cert — so it must keep re-soliciting until the block decides
		// (then it leaves pendingBlocks and the re-poll quiesces). This is exactly
		// avalanchego's contract: re-poll a PROCESSING block until it is decided, and
		// only quiesce at NumProcessing()==0 (the upstream voter.go +
		// Engine.Gossip's 100ms repoll). Abandoning an own proposal after a fixed
		// attempt cap was the Lux-only divergence that froze mainnet C-Chain: the
		// substitute's canonical block stopped being re-solicited and the chain halted
		// even though the honest majority was ready to vote (zero-margin 4-of-5 once a
		// 5th validator forks/wedges). The bounded backoff above keeps this storm-safe.
		//
		// A NON-own (gossiped) block keeps the attempt cap: re-soliciting a block whose
		// voters are BEHIND its parent (the gossip-from-an-ahead-peer case) is pure spam
		// and never recovers it — that block recovers via cert-gossip or the catch-up
		// fetch, not by re-poll. So the cap still bounds the follower path.
		if !pending.IsOwnProposal && pending.rePollAttempts >= maxRePollAttempts {
			pending.rePollAbandoned = true
			t.log.Warn("re-poll: gossiped block abandoned after attempt cap — not re-soliciting further (recoverable via cert/catch-up)",
				"blockID", blockID, "attempts", pending.rePollAttempts)
		}

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
		// quorum recovers via the cert-gossip path that step (1) re-runs, or via
		// catch-up if it is behind the block's parent. The backoff above bounds how
		// often this fires; the cap stops it entirely for a terminally stuck block.
		if d.ownProp && proposer != nil {
			_ = proposer.RequestVotes(ctx, VoteRequest{
				BlockID:   d.blockID,
				BlockData: d.blockData,
			})
		}
	}
}

// claimCatchupLocked is the engine's idempotency + rate-limit GATE for "fetch
// this missing ancestor". It is the SINGLE decision point for whether a catch-up
// fetch should fire — the Runtime owns the actual network round-trip (it carries
// the networkID + transport). Caller holds t.mu.
//
// Returns true iff the caller should now issue exactly one RequestAncestors for
// missingID. It returns false (suppressing the fetch) when:
//   - no catchup transport is wired (legacy: a behind follower stays stranded),
//   - missingID is Empty (genesis/no parent — nothing to fetch),
//   - the block is already tracked or known to consensus (not actually missing),
//   - a fetch for this missing ID fired within catchupCooldown (throttle — so
//     many children of one missing parent, or repeated gossip of one orphan,
//     issue ONE fetch per cooldown, never a fetch storm).
//
// On a true return it records the throttle stamp, so the gate is self-arming:
// the caller does not need to remember anything. This is the catch-up analogue
// of the re-poll backoff — one mechanism, one place.
//
// The catchupRequested map is bounded TWO ways, both fail-closed:
//   - Layer 1 (reclaim-on-known): when missingID turns out to be already tracked
//     or known to consensus, its throttle entry can never be needed again, so we
//     delete it at those early returns (the dual of delete-on-track/decide at the
//     accept/reject/sync sites). An honest fetch that LANDS is reclaimed this way.
//   - Layer 2 (hard cap + TTL): a FORGED id that never arrives is never reclaimed
//     by layer 1, so before inserting a new entry at the cap we sweep entries
//     older than catchupRequestTTL; if the map is still at the cap (all young — an
//     active flood) we REFUSE the claim (no insert, no fetch). The map can never
//     exceed maxCatchupRequested.
func (t *Transitive) claimCatchupLocked(missingID ids.ID) bool {
	if t.catchup == nil || missingID == ids.Empty {
		return false
	}
	// Already have it tracked (or it just finalized) ⇒ not missing. A now-arrived
	// block can never be re-needed, so reclaim any throttle entry for it (layer 1).
	if _, tracked := t.pendingBlocks[missingID]; tracked {
		delete(t.catchupRequested, missingID)
		return false
	}
	if _, ok := t.consensus.GetBlock(missingID); ok {
		delete(t.catchupRequested, missingID)
		return false
	}
	now := time.Now()
	if last, ok := t.catchupRequested[missingID]; ok && now.Sub(last) < catchupCooldown {
		return false // throttled — one fetch per missing parent per cooldown
	}
	// HARD bound (layer 2): only when at the cap and inserting a NEW key. Sweep
	// entries past the TTL (an honest fetch resolves far inside catchupRequestTTL);
	// if still full afterward, every entry is young — an active forged-ID flood —
	// so fail closed: refuse the new claim rather than grow past the cap. The sweep
	// is O(size), bounded, and runs only at the cap (free in steady state).
	if _, existing := t.catchupRequested[missingID]; !existing && len(t.catchupRequested) >= maxCatchupRequested {
		for id, stamp := range t.catchupRequested {
			if now.Sub(stamp) >= catchupRequestTTL {
				delete(t.catchupRequested, id)
			}
		}
		if len(t.catchupRequested) >= maxCatchupRequested {
			return false // map saturated with active (young) entries — fail closed
		}
	}
	t.catchupRequested[missingID] = now
	return true
}

// claimCertCatchupLocked is the HEIGHT-keyed idempotency + rate-limit gate for the
// stuck-round cert-catch-up (Runtime.requestCertCatchup). It is the height-domain
// twin of claimCatchupLocked: that gate keys on a MISSING block ID and suppresses a
// fetch once the block is tracked; this gate deliberately does NOT consult tracking,
// because the stuck-round case fetches the CERT for a block we already hold. Caller
// holds t.mu. Returns true iff the caller should issue exactly one cert-fetch for
// `height` now; false when the catchup seam is unwired, the height is already
// finalized (reaped), throttled within catchupCooldown, or the map is saturated with
// active entries (fail-closed, same hard cap as catchupRequested).
func (t *Transitive) claimCertCatchupLocked(height uint64) bool {
	if t.catchup == nil {
		return false
	}
	// Already decided ⇒ never fetch again; reclaim any throttle entry (layer 1).
	if fh, set := t.consensus.GetFinalizedHeight(); set && height <= fh {
		delete(t.certCatchupRequested, height)
		return false
	}
	now := time.Now()
	if last, ok := t.certCatchupRequested[height]; ok && now.Sub(last) < catchupCooldown {
		return false // throttled — one cert-fetch per stuck height per cooldown
	}
	// HARD bound (layer 2): only when at the cap and inserting a NEW key. Reap entries
	// at/below the finalized floor (a decided height is never fetched again), then, if
	// still full, fail closed. The map can never exceed maxCatchupRequested.
	if _, existing := t.certCatchupRequested[height]; !existing && len(t.certCatchupRequested) >= maxCatchupRequested {
		fh, _ := t.consensus.GetFinalizedHeight()
		for h := range t.certCatchupRequested {
			if h <= fh {
				delete(t.certCatchupRequested, h)
			}
		}
		if len(t.certCatchupRequested) >= maxCatchupRequested {
			return false
		}
	}
	t.certCatchupRequested[height] = now
	return true
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

	// Phase 2: classify each candidate WITHOUT holding t.mu (avoids nested lock).
	//
	// consensus.IsAccepted / IsRejected here are the LIVENESS TRIGGER, not the
	// finality authority: a true IsAccepted means "α-of-K voters responded — it is
	// worth ATTEMPTING to finalize", nothing more. The actual accept decision is
	// made by TryAccept in phase 3, which finalizes ONLY with a VerifiedQuorumCert
	// (the strict >⅔-of-stake gate). So a low-stake/high-count coalition that flips
	// IsAccepted here cannot finalize: TryAccept refuses it (ErrNoVerifiedQC) and
	// the block stays pending. Rejections carry no stake-safety concern (a block is
	// dropped, not finalized) and are committed inline.
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

	// Phase 3: accepts go through the SOLE cert-gated path (TryAccept); rejects
	// are committed here. TryAccept is idempotent and takes its own lock — it
	// finalizes iff a verified cert exists, otherwise returns ErrNoVerifiedQC and
	// changes nothing (the block waits for the next tick). It also subsumes the
	// VM.Accept + SetPreference + pipeline-signal that the old phase 4 did inline,
	// so accepts no longer touch VM state from this loop at all.
	rejected := make([]blockAction, 0, len(actions))
	for _, action := range actions {
		if action.accept {
			_ = t.TryAccept(context.Background(), action.blockID)
			continue
		}
		rejected = append(rejected, action)
	}

	if len(rejected) == 0 {
		return
	}

	// Phase 4: commit the rejections (no cert required — a reject finalizes
	// nothing). Mirror the previous found/double-decide guard so a block already
	// decided by another trigger is not Reject'd twice.
	t.mu.Lock()
	ctx := t.ctx
	found := make([]bool, len(rejected))
	for i, action := range rejected {
		pending, exists := t.pendingBlocks[action.blockID]
		if !exists || pending.Decided {
			continue
		}
		found[i] = true
		pending.Decided = true
		t.blocksRejected++
		t.dropPendingBlockLocked(action.blockID)
		// Drop any votes parked for a now-rejected block (it will never be tracked
		// to drain them) so the buffer cannot leak.
		delete(t.bufferedVotes, action.blockID)
		// A rejected (decided) block is no longer "missing" — reclaim its catch-up
		// throttle entry so catchupRequested stays bounded.
		delete(t.catchupRequested, action.blockID)
	}
	t.mu.Unlock()

	for i, action := range rejected {
		if !found[i] {
			continue // already decided by another trigger
		}
		if action.vmBlock != nil {
			action.vmBlock.Reject(ctx)
		}
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
		// A vote for an ALREADY-FINALIZED (Nova-accepted) block needs nothing for ACCEPTANCE:
		// the block is decided (removed from pendingBlocks, recorded in finalizedByCert). Do NOT
		// buffer or fetch it — that would re-park a late/duplicate vote AFTER acceptWithCertCore
		// cleared the buffer. But it CAN still complete the EXPORT (Quasar) cert: the ⅔-th stake
		// vote necessarily TRAILS the bare-majority Nova accept, so a trailing accept vote is
		// routed to the attestor (verified against the exact accepted position — attestFinalizedVote
		// re-checks the signature, so this never bypasses the gate).
		if _, finalized := t.finalizedByCert[vote.BlockID]; finalized {
			verifier := t.voteVerifier
			t.mu.Unlock()
			t.attestFinalizedVote(vote, verifier)
			return
		}
		// VOTE FOR A BLOCK WE DO NOT YET TRACK — the gossip race that wedged the
		// write path: a peer's vote outran the block bytes. The old code DROPPED it
		// here, and since votes are solicited only once, the drop was permanent —
		// the missing-bytes follower could never reach α-of-K and the block never
		// Accepted. Instead BUFFER the vote (bounded, no signature work yet) and ask
		// the catch-up seam to FETCH the missing block, exactly as the missing-parent
		// path does. When the block lands at a tracking site, drainBufferedVotes
		// replays each parked vote through the normal channel path, where it is
		// signature-verified like any live vote (buffering NEVER bypasses the gate;
		// a forged/unsigned parked vote costs one map slot and is dropped on replay).
		accepted := t.bufferVoteLocked(vote)
		// GATE the fetch on buffer acceptance. If the buffer REFUSED the vote (a cap
		// was hit), do NOT fetch — a fetch for a vote we did not park is pure
		// amplification: there is nothing buffered for the fetched block to drain
		// into. Fetching ONLY for parked votes gives a bounded aggregate fetch rate:
		// at most min(maxBufferedVoteBlocks, maxCatchupRequested) distinct fetches in
		// flight (one per parked-and-claimed missing ID), each re-fireable at most
		// once per catchupCooldown. That is the global fetch ceiling — it falls out
		// of bounding BOTH bufferedVotes and catchupRequested.
		var fetch func(missingID ids.ID, from ids.NodeID)
		if accepted {
			fetch = t.requestMissing
		}
		t.mu.Unlock()
		// Fire the fetch WITHOUT the lock: requestMissing routes to
		// Runtime.requestCatchup, which claims its OWN lock for the idempotency gate
		// (claimCatchupLocked) and then does the RequestAncestors round-trip. Calling
		// it while still holding t.mu would re-enter the lock (non-reentrant) and
		// deadlock. nil ⇒ no runtime wired (bare engine) OR the buffer rejected the
		// vote (no payoff in fetching); either way the vote waits / is dropped.
		if fetch != nil {
			fetch(vote.BlockID, vote.NodeID)
		}
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

// bufferVoteLocked parks a vote for a not-yet-tracked block, enforcing both
// bounds (fail-closed) AND one-vote-per-(block, validator) dedup. Caller holds
// t.mu. It does NO signature work — a parked vote is verified only when
// drainBufferedVotes replays it through handleVote.
//
// Dedup invariant: at most ONE buffered vote per (BlockID, NodeID). If a vote for
// this {BlockID, NodeID} pair is already parked, it is REPLACED in place rather
// than appended, so the per-block slice is bounded by DISTINCT NodeIDs, not raw
// arrival count. This is the dual of the certVotes NodeID-keyed dedup that
// recordCertVoteLocked applies to live votes — same keying, mirrored onto the
// slice. It closes the single-Byzantine-ID crowd-out: one NodeID can occupy at
// most ONE slot per block, so it cannot flood maxBufferedVotesPerBlock junk
// entries and crowd genuine validators' votes out of the buffer fast-path.
//
// Returns accepted=true iff a vote for this block is now parked/represented; the
// caller uses this to GATE the catch-up fetch. A same-(block, node) REPLACEMENT
// still returns true: there is a parked vote for that block to drain into, so the
// fetch is still warranted. accepted=false only when a cap dropped the vote — and
// a vote we refused to even buffer must NOT trigger a fetch (firing one for a
// dropped vote is pure amplification with no payoff — nothing is parked for the
// fetched block to drain into).
//   - Per-block cap: if maxBufferedVotesPerBlock DISTINCT NodeIDs are already
//     parked for this block ID, a vote from a NEW NodeID is dropped (the real
//     α-of-K voters fit well within; a replacement of an existing NodeID never
//     hits the cap — it does not grow the slice).
//   - Total-keys cap: if this is a NEW block ID and maxBufferedVoteBlocks distinct
//     IDs are already parked, the new key is dropped (we never evict an existing
//     key — the simplest sound bound; existing keys drain on track or delete on
//     decide).
func (t *Transitive) bufferVoteLocked(vote Vote) (accepted bool) {
	existing, seen := t.bufferedVotes[vote.BlockID]
	if !seen && len(t.bufferedVotes) >= maxBufferedVoteBlocks {
		return false // total distinct-block ceiling reached — fail closed
	}
	// Dedup by NodeID (dual of certVotes): if this validator already has a vote
	// parked for this block, replace it in place — never append a second. This is
	// what bounds the slice by distinct NodeIDs and defeats single-ID crowd-out.
	for i := range existing {
		if existing[i].NodeID == vote.NodeID {
			existing[i] = vote
			return true // a vote for this block is parked — fetch still warranted
		}
	}
	if len(existing) >= maxBufferedVotesPerBlock {
		return false // per-block ceiling reached — fail closed
	}
	t.bufferedVotes[vote.BlockID] = append(existing, vote)
	return true
}

// drainBufferedVotes replays every vote parked for blockID now that the block is
// tracked. It removes the slice under t.mu, deletes the key (so the buffer cannot
// leak for a block that did arrive), and re-enqueues each vote via ReceiveVote —
// the SAME channel path a live vote takes. Re-enqueueing (rather than calling
// handleVote inline) avoids re-entrant locking and keeps ONE code path: each
// replayed vote re-enters handleVote with the block now tracked, so it is
// signature-verified exactly as a live vote (a forged/unsigned parked vote is
// dropped at the gate; it never counts). Called at every block-tracking site. If
// the vote channel is full, ReceiveVote returns false and the vote is dropped —
// acceptable: the periodic re-poll / re-gossip will re-deliver it.
func (t *Transitive) drainBufferedVotes(blockID ids.ID) {
	t.mu.Lock()
	parked := t.bufferedVotes[blockID]
	delete(t.bufferedVotes, blockID)
	t.mu.Unlock()

	for _, vote := range parked {
		t.ReceiveVote(vote)
	}
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
	// NON-EQUIVOCATION (fork guard): refuse to sign a conflicting sibling at a HEIGHT
	// this node has already committed to — and DURABLY record the binding before
	// signing so a crash cannot forget it (HIGH-1). Idempotent for the same canonical
	// block. One signature per consensus height is the invariant that keeps two
	// α-of-K certs at one height impossible.
	if !t.reserveSlotForSign(pos.Height, slotCanonical(pos)) {
		t.log.Warn("vote-once: refusing to sign a conflicting sibling at an already-committed slot",
			"height", pos.Height, "blockID", blockID)
		return
	}
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

// reserveSlotForSign enforces the per-HEIGHT non-equivocation rule before this node
// casts an accept signature, and DURABLY records the binding first so it survives a
// crash (HIGH-1). `canonical` is the block's inner execution commitment (the id
// finality certifies). The slot is epoch-BLIND: one signature per consensus height,
// full stop, so no proposer-chosen epoch can fragment it. Returns true if signing is
// permitted: the slot is unbound (binds it now, AFTER the durable write commits) or
// already bound to THIS canonical (idempotent — a legitimate re-solicit of the same
// block, already durable). Returns FALSE if the slot is bound to a DIFFERENT
// canonical — a conflicting sibling this node must NEVER sign (the cross-node fork
// Red proved) — OR if the durable write FAILS (fail-closed: no memory of the
// binding ⇒ no signature). Self-locking (slotMu) so both signing sites —
// recordOwnVoteLocked (t.mu held) and the follower path in followVerifiedBlock
// (t.mu released) — share the one guard without deadlock; the durable write also
// runs under slotMu, so the fixed-name temp file is never contended.
func (t *Transitive) reserveSlotForSign(height uint64, canonical ids.ID) bool {
	// DECIDED-HEIGHT GATE (avalanchego's lastAcceptedHeight frontier, lifted to the sign
	// path). A height at or below the decided frontier is DECIDED: exactly one block was
	// certified there, and — as in topological.go's acceptPreferredChild + rejectTransitively
	// — its every sibling is permanently unsignable and any vote for it is dropped. We
	// enforce that same invariant on the cert layer here: never sign at a decided height.
	//
	// This closes the prune-then-resign fork. The per-height committedSlot below refuses a
	// conflicting sibling only WHILE the slot lives; but the slot is pruned as the window
	// advances, and a losing sibling with a DIFFERENT outer parent (a bare/pre-fork envelope
	// vs a wrapped one) is NOT in losingSubtrees, so it stays tracked and undecided after the
	// winner finalizes. Without this gate, once the winner's height slot is pruned, that
	// late/differently-enveloped sibling would pass the (now empty) slot check and get this
	// node's SECOND signature — the path that assembled two α-of-K certs at one height on the
	// fresh devnet (artifacts-A: the conflicting cert's canonical == its envelope, the bare
	// sibling).
	//
	// The floor is the max of two DURABLE, restart-surviving sources — this is what makes the
	// refusal span a crash (the certified ledger.Height() alone does NOT: on boot it is a
	// (0,false) non-authoritative hint until the first post-restart cert — incident-1082814
	// PART-A):
	//   • t.decidedFloor — seeded on boot from the vote-guard file's fsync'd finalizedThrough
	//     (WithVoteGuard) and advanced at each finalize. This is authoritative: the vote-guard
	//     is written by the consensus engine at finalize time, so it NEVER lags the decision
	//     (unlike vm.LastAccepted, which a fire-and-forget Accept can leave frozen).
	//   • consensus.GetDecidedFloor() — max(certified height, vm.LastAccepted hint); a
	//     complementary lower bound, and the sole floor for a signer whose vote-guard is fresh.
	// SIGN-GATE ONLY: neither source enters the finality ledger / byHeight / the equivocation
	// index, so PART-A is preserved (Height() stays (0,false)-until-cert). Read the consensus
	// floor BEFORE slotMu so this method never holds slotMu while touching the consensus lock.
	consensusFloor := uint64(0)
	if t.consensus != nil {
		consensusFloor = t.consensus.GetDecidedFloor()
	}
	key := SlotKey{Height: height}
	t.slotMu.Lock()
	defer t.slotMu.Unlock()
	floor := t.decidedFloor
	if consensusFloor > floor {
		floor = consensusFloor
	}
	if height <= floor {
		return false // decided height — permanently unsignable (durable across restart)
	}
	if bound, ok := t.committedSlot[key]; ok {
		if bound == canonical {
			return true // same block ⇒ idempotent (already durable)
		}
		// NON-EQUIVOCATION: a conflicting canonical at an already-signed height ⇒ REFUSE.
		// One signature per consensus height makes two conflicting finality attestations at
		// one height impossible (Nova's single-accept property; this durable committedSlot is
		// its cross-restart WITNESS — a value/log, never a decision lock).
		return false
	}
	// First binding at this slot. PERSIST it durably BEFORE permitting the signature —
	// a crash after signing but before finalizing must not forget it. Mutate the map,
	// snapshot it to stable storage, and ROLL BACK on failure so in-memory state stays
	// consistent with what is durable and we FAIL CLOSED (return false ⇒ no signature).
	t.committedSlot[key] = canonical
	if t.voteGuard != nil {
		if err := t.voteGuard.Persist(t.committedSlot, t.decidedFloor); err != nil {
			delete(t.committedSlot, key)
			t.log.Error("vote-once: durable equivocation-guard write FAILED — refusing to sign (fail-closed)",
				"height", height, "error", err)
			return false
		}
	}
	return true
}

// hasSignedHeight reports whether this node has already bound its ONE accept
// signature at the given consensus height (any canonical). The convergence voter
// checks it to emit at most one vote per height and never re-broadcast. Self-locking
// (slotMu).
func (t *Transitive) hasSignedHeight(height uint64) bool {
	t.slotMu.Lock()
	_, ok := t.committedSlot[SlotKey{Height: height}]
	t.slotMu.Unlock()
	return ok
}

// committedCanonical returns the DURABLE canonical this node bound at height (the
// vote-guard binding), if any. The clone-recovery re-sign fallback (emitConvergedVote)
// uses it to target the durable binding ONLY — never a freshly-computed winner — so a
// mass-restarted node re-contributes its vote for exactly the block it already committed
// to, and reserveSlotForSign's idempotent-true(bound==canonical) admits it while its
// conflicting-sibling refusal keeps it equivocation-safe. Self-locking (slotMu).
func (t *Transitive) committedCanonical(height uint64) (ids.ID, bool) {
	t.slotMu.Lock()
	c, ok := t.committedSlot[SlotKey{Height: height}]
	t.slotMu.Unlock()
	return c, ok
}

// pendingByCanonicalLocked resolves an INNER canonical execution id to the tracked,
// undecided OUTER pending block that carries it — the inner→outer lookup the durable
// vote-guard clone-recovery needs (FIX #3). pendingBlocks is keyed by the OUTER
// proposervm envelope id, but committedSlot / slotCanonical store the INNER canonical id;
// a proposervm-wrapped block has inner != outer, so a direct pendingBlocks[canon] MISSES
// (it hit only on bare blocks where inner==outer — the v4 fallback's silent no-op on real
// wrappers). Returns the lowest-outer-id wrapper of that canonical (deterministic across
// equal-canonical aliases) and its outer id, or (nil, Empty) when no live wrapper is
// tracked. Caller holds t.mu.
func (t *Transitive) pendingByCanonicalLocked(canon ids.ID) (*PendingBlock, ids.ID) {
	if canon == ids.Empty {
		return nil, ids.Empty
	}
	var best *PendingBlock
	var bestID ids.ID
	for id, pb := range t.pendingBlocks {
		if pb.ConsensusBlock == nil || pb.Decided {
			continue
		}
		if pb.ConsensusBlock.canonicalRep() != canon {
			continue
		}
		if best == nil || id.Compare(bestID) < 0 {
			best, bestID = pb, id
		}
	}
	return best, bestID
}

// selfVotedForCanonicalLocked reports whether this node's signed accept for the given
// canonical execution identity is already collected in ANY tracked wrapper of it — the
// canonical-aggregated view (FIX #3) of "have I voted at this height". A wrapped block's
// vote may live in a DIFFERENT outer wrapper than the one being inspected (votes bind the
// canonical id; the re-sign resolves inner→outer to one wrapper), so a per-wrapper
// certVotes check would busy-replay a height already voted under a sibling envelope.
// Caller holds t.mu.
func (t *Transitive) selfVotedForCanonicalLocked(canon ids.ID) bool {
	for _, pb := range t.pendingBlocks {
		cb := pb.ConsensusBlock
		if cb == nil || cb.canonicalRep() != canon {
			continue
		}
		if _, ok := pb.certVotes[t.nodeID]; ok {
			return true
		}
	}
	return false
}

// pruneCommittedSlotsBelow drops equivocation-guard entries STRICTLY BELOW a finalized
// height — retaining the just-finalized tip's slot, exactly as avalanchego keeps the last
// accepted block in ts.blocks. Heights strictly below the tip are guaranteed unsignable by
// the decided-height gate in reserveSlotForSign (height <= finalizedHeight ⇒ refuse), which
// is durable and monotonic, so their in-memory slot is dead weight and is dropped to keep
// committedSlot (and its snapshot) bounded to the live window. The tip's slot is KEPT as a
// second, in-memory belt: it refuses a conflicting sibling at the tip height even in the
// window before finalizedHeight is observed. It is itself pruned only once the NEXT height
// finalizes (k.Height < height at the higher finalize), by which point the decided-height
// gate covers it durably.
//
// CRITICAL — why STRICTLY below, not at-or-below: an inclusive prune (k.Height <= height)
// deleted the just-finalized height's slot, opening the prune-then-resign fork. A losing
// sibling with a DIFFERENT outer parent than the winner (bare vs wrapped envelope) is not in
// losingSubtrees, so it stays tracked+undecided after the winner finalizes; with the slot
// deleted and no decided-height gate, the convergence pass would then place this node's
// SECOND signature on it — two α-of-K certs at one height → the exit(1) equivocation fatal.
//
// Called from the sole finalizer acceptWithCertCore, so EVERY finality path — local
// vote-assembly AND incoming-cert — prunes (MEDIUM-1). The durable shrink is best-effort: a
// stale-LARGER durable set only ever REFUSES more (fail-safe direction), corrected on the
// next successful Persist.
func (t *Transitive) pruneCommittedSlotsBelow(height uint64) {
	t.slotMu.Lock()
	defer t.slotMu.Unlock()
	// Advance the DURABLE decided-through floor: this height is now finalized, so it (and
	// everything below) is permanently unsignable. Monotonic. Persisting it in the SAME
	// write as the pruned map keeps the floor and the slot removal atomic on disk — so a
	// restart recovers "these heights are decided" even though their slots are gone.
	floorAdvanced := false
	if height > t.decidedFloor {
		t.decidedFloor = height
		floorAdvanced = true
	}
	changed := false
	for k := range t.committedSlot {
		if k.Height < height {
			delete(t.committedSlot, k)
			changed = true
		}
	}
	if (changed || floorAdvanced) && t.voteGuard != nil {
		if err := t.voteGuard.Persist(t.committedSlot, t.decidedFloor); err != nil {
			t.log.Warn("vote-once: durable equivocation-guard shrink failed (non-fatal; floor re-persisted on next bind)",
				"belowHeight", height, "error", err)
		}
	}
}

// seedDecidedFloorFromVMLocked advances the durable decided-height floor from the VM's
// last-accepted height. It runs at boot (from Start, before the signing goroutines launch)
// so the sign gate has a real floor from the first instant — the fix for the mainnet v1→v2
// vote-guard upgrade window, where the file floor is 0 and the certified ledger is a
// (0,false) hint until the first post-upgrade finalize. vm.LastAccepted is DURABLE and a
// sound lower bound on the decided height (every accepted block was finalized), so seeding
// the floor from it can only ever REFUSE MORE signing (fail-safe). SIGN-GATE ONLY: it
// touches only t.decidedFloor (under slotMu), never the finality ledger / byHeight — PART-A
// intact. Best-effort: any VM error / empty head leaves the floor as-is (the vote-guard
// file floor and the SyncState seed remain). Caller holds t.mu.
func (t *Transitive) seedDecidedFloorFromVM(ctx context.Context) {
	if t.vm == nil {
		return
	}
	id, err := t.vm.LastAccepted(ctx)
	if err != nil || id == ids.Empty {
		return
	}
	blk, err := t.vm.GetBlock(ctx, id)
	if err != nil || blk == nil {
		return
	}
	h := blk.Height()
	t.slotMu.Lock()
	if h > t.decidedFloor {
		t.decidedFloor = h
	}
	t.slotMu.Unlock()
}

// slotCanonical is the effective canonical identity a VotePosition binds for the
// equivocation guard: the inner execution commitment (CanonicalID) for a
// proposervm-wrapped block, else the outer id (bare-block degrade — matches
// canonicalVoteMessageFor, so the guarded identity is exactly the SIGNED one).
func slotCanonical(pos VotePosition) ids.ID {
	if pos.CanonicalID != ids.Empty {
		return pos.CanonicalID
	}
	return pos.BlockID
}

// ConvergenceVoter casts this node's single per-height accept vote for the
// deterministically-converged winner at each fork slot. It decouples the (binding,
// one-per-height) signature from block build/receipt: a node NEVER binds its vote to
// the block it merely built or first-saw (that fragments the α-of-K vote across
// siblings and stalls a fresh-net storm). Instead RunSettlePass — driven by the
// convergence tick — sweeps every undecided, still-unsigned fork slot whose settle
// window has elapsed and casts the vote for the lowest-canonical live sibling, so every
// honest node signs the SAME block and exactly one cert forms per height. Wired by the
// Runtime (which owns the sign+gossip path); nil in single-engine tests.
type ConvergenceVoter interface {
	RunSettlePass(ctx context.Context)
}

// convergedWinnerAtHeightLocked returns the block THIS node must place its one
// per-height accept signature on at the (height, parentID) fork slot: the LOWEST
// slotCanonical among tracked, undecided, non-abandoned sibling blocks extending
// parentID at that height, plus the count of such siblings. The tie-break is the
// signed canonical id (the exact identity a cert binds), so every honest node with
// the same tracked set selects the IDENTICAL winner and their one-vote-per-height
// signatures converge onto it. Abandoned blocks (a dead proposer's sibling that
// stopped being re-solicited) are excluded, so the winner advances to the
// lowest-canonical LIVE sibling — the f=1 self-heal. Caller holds t.mu.
//
// GRINDABILITY (RED M-grind, a testnet/mainnet gate — NOT a safety or halt break):
// the tie-break is the block's content hash (CanonicalID), which the PROPOSER controls.
// A validator eligible at a CONTESTED height can grind ~2^k block variants in the settle
// window to obtain the lowest canonical and make every honest node converge on ITS block
// — a censorship/MEV lever. It is bounded to MULTI-PROPOSER CONTENTION only: at a
// height with a single proposervm-eligible proposer (the steady state) there are no
// siblings to win, so the grind buys nothing; it bites only during the fresh-net /
// down-designated-proposer transient. Progress and single-block finality are UNAFFECTED
// (one block per height still finalizes). The grind-RESISTANT replacement is a tie-break
// the proposer cannot bias — a VRF over height‖parentID keyed to the staking key, or the
// proposervm eligibility VRF already carried in the wrapped block — which requires
// plumbing the proposer's VRF output into the consensus Block (a node-layer change) and
// is the tracked follow-up before adversarial-validator mainnet promotion.
// includeAbandoned selects whether rePoll-abandoned siblings still count as convergence
// candidates. The VIEW-CHANGE path passes true: abandonment only stops rePoll's RequestVotes
// re-solicitation (spam control), it is a PER-NODE decision (each node abandons on its own
// attempt clock), so excluding abandoned siblings would make different nodes compute a
// DIFFERENT lowest-canonical winner from the SAME sibling set → prevotes never align → no POL
// → the distributed liveness stall. Counting every live (undecided) sibling keeps the winner
// globally identical, which is what lets α aligned prevotes form. The LEGACY path passes false
// (unchanged behaviour). Never manufactures a vote or bypasses the α-of-K cert → safety intact.
// canonicalRep is the block's own EXECUTION identity — its inner canonicalID, or its
// outer id for a bare (non-wrapped) block. Two proposervm wrappers of the SAME inner
// block share this: they are ALIASES, not forks. Fork-choice keys on this, never the
// outer id (a transport/DAG alias).
func (b *Block) canonicalRep() ids.ID {
	if b.canonicalID != ids.Empty {
		return b.canonicalID
	}
	return b.id
}

// parentCanonicalRep is the block's PARENT execution identity — the parent's inner
// canonicalID, or the outer parentID when not exposed. Children of canonical-equivalent
// parent wrappers share this, so they belong to the SAME convergence group even though
// their outer parentIDs differ. Grouping by the outer parentID instead splits a forked
// parent's children across passes and lets different nodes pick different winners →
// prevotes never align → the liveness stall (the block-1082879 storm).
func (b *Block) parentCanonicalRep() ids.ID {
	if b.parentCanonicalID != ids.Empty {
		return b.parentCanonicalID
	}
	return b.parentID
}

func (t *Transitive) convergedWinnerAtHeightLocked(height uint64, parentID ids.ID, includeAbandoned bool) (ids.ID, int, bool) {
	// Resolve the target parent's CANONICAL identity so canonical-equivalent parent
	// wrappers (same inner block, different outer envelope) collapse to one group. An
	// unaccepted (possibly forked) parent is tracked in pendingBlocks and resolves to its
	// canonical; the accepted tip (height floor+1's parent) is not tracked, is single/
	// un-forked, and its children are matched by the outer parentID branch below.
	parentCanon := parentID
	if ppb, ok := t.pendingBlocks[parentID]; ok && ppb.ConsensusBlock != nil {
		parentCanon = ppb.ConsensusBlock.canonicalRep()
	}
	var winner, winnerCanon ids.ID
	count := 0
	for id, pb := range t.pendingBlocks {
		cb := pb.ConsensusBlock
		if cb == nil || pb.Decided || (!includeAbandoned && pb.rePollAbandoned) {
			continue
		}
		// Group by (height, parent CANONICAL) — alias-collapsing. Match a child whose
		// parent is the target outer id (accepted tip, single wrapper) OR whose parent is
		// canonical-equivalent to it (a forked pending parent's other wrappers).
		if cb.height != height || (cb.parentID != parentID && cb.parentCanonicalRep() != parentCanon) {
			continue
		}
		canon := cb.canonicalRep()
		count++
		// Deterministic representative: lowest canonical, then lowest OUTER id as the
		// tie-break. EQUAL-canonical aliases MUST resolve to the SAME winner on every
		// node — Go map iteration is randomized, so without the outer-id tie-break the
		// winner among equal canonicals is nondeterministic and prevotes never align.
		if winner == ids.Empty || canon.Compare(winnerCanon) < 0 ||
			(canon == winnerCanon && id.Compare(winner) < 0) {
			winner, winnerCanon = id, canon
		}
	}
	if winner == ids.Empty {
		return ids.Empty, 0, false
	}
	return winner, count, true
}

// parentIsProvenLoserLocked reports whether parentID has DEFINITIVELY lost the
// convergence at its OWN height: a tracked, non-abandoned SIBLING of parentID (same
// height, same grandparent) carries a strictly-lower signed-canonical. Such a parent is
// on a branch every honest node is converging AWAY from, so a height-H block extending it
// can never finalize; binding this node's one height-H signature to it would waste the
// vote under the height-only vote-once rule and could STALL height H — the transient
// H-1-fork case (N1). Conservative on purpose: an UNTRACKED parent (the finalized tip, or
// a block this node is behind) returns false — it cannot be PROVEN a loser and must not be
// filtered, or the normal H = finalizedHeight+1 path would itself stall. Caller holds t.mu.
func (t *Transitive) parentIsProvenLoserLocked(parentID ids.ID) bool {
	pb, ok := t.pendingBlocks[parentID]
	if !ok || pb.ConsensusBlock == nil {
		return false
	}
	p := pb.ConsensusBlock
	pc := p.canonicalRep()
	gpCanon := p.parentCanonicalRep()
	for sibID, sib := range t.pendingBlocks {
		cb := sib.ConsensusBlock
		if cb == nil || sib.rePollAbandoned || sibID == parentID {
			continue
		}
		// Same-slot sibling test by CANONICAL grandparent (alias-collapsing), so a sibling
		// built on a different wrapper of the same grandparent still counts.
		if cb.height != p.height || (cb.parentID != p.parentID && cb.parentCanonicalRep() != gpCanon) {
			continue
		}
		sc := cb.canonicalRep()
		// An equal-canonical block is an ALIAS of the parent (sc == pc ⇒ not < 0), never a
		// competitor — only a strictly-lower canonical proves the parent lost.
		if sc.Compare(pc) < 0 {
			return true // a strictly-lower-canonical sibling of the parent exists ⇒ parent lost
		}
	}
	return false
}

// votableSlot identifies one (height, parentID) fork slot the settle pass may need to
// cast a converged vote for.
type votableSlot struct {
	height   uint64
	parentID ids.ID
}

// snapshotVotableSlotsLocked returns the DISTINCT (height, parentID) fork slots that
// (a) have an undecided, non-abandoned tracked block, (b) this node has NOT yet signed
// (committedSlot has no binding at that height), and (c) have SETTLED — the earliest
// local track time of any sibling at that slot is at least one settle window ago, so
// near-simultaneous sibling proposals have had time to gossip in and the winner is
// stable. The settle window is the whole point: it is what lets every honest node see
// the SAME sibling set before it binds its one signature, so they all pick the SAME
// lowest-canonical winner instead of racing to bind their own first-seen block. Caller
// holds t.mu (and takes slotMu internally to read committedSlot).
func (t *Transitive) snapshotVotableSlotsLocked() []votableSlot {
	// Read the set of already-signed heights once, under slotMu.
	// Read the consensus decided floor BEFORE slotMu so this method never holds slotMu
	// while touching the consensus lock.
	consensusFloor := uint64(0)
	if t.consensus != nil {
		consensusFloor = t.consensus.GetDecidedFloor()
	}
	t.slotMu.Lock()
	signed := make(map[uint64]struct{}, len(t.committedSlot))
	for k := range t.committedSlot {
		signed[k.Height] = struct{}{}
	}
	// DECIDED-HEIGHT FILTER (liveness): never OFFER a slot at or below the DURABLE decided
	// floor (the same floor the sign gate enforces: max of the vote-guard-seeded decidedFloor
	// and the consensus floor). Such a height is decided — reserveSlotForSign would refuse it
	// anyway, but because a refused sign leaves committedSlot empty, the slot would otherwise
	// resurface on every tick and the pass would busy-replay it. Mirrors avalanchego dropping
	// votes for a Decided() block.
	decidedFloor := t.decidedFloor
	if consensusFloor > decidedFloor {
		decidedFloor = consensusFloor
	}
	t.slotMu.Unlock()

	settle := t.convergenceSettleWindow()
	now := time.Now()
	// Track the LATEST local track time of any sibling at each slot. Settling from the
	// LAST sibling (not the first) means a slot becomes votable only once NO new sibling
	// has arrived for a full window — so while proposals are still dribbling in (slow or
	// contended gossip) the node keeps waiting, and it binds its one signature only after
	// the sibling set has gone quiet. That is what makes every honest node vote the SAME
	// lowest-canonical winner instead of racing to bind an incomplete set.
	latest := make(map[votableSlot]time.Time)
	for _, pb := range t.pendingBlocks {
		cb := pb.ConsensusBlock
		if cb == nil || pb.Decided || pb.rePollAbandoned {
			continue
		}
		if cb.height <= decidedFloor {
			continue // decided height — permanently unsignable, never offer it
		}
		if _, ok := signed[cb.height]; ok {
			// CLONE-RECOVERY re-offer (v4 vote-guard). Normally an already-bound height is
			// skipped (one vote per height). But after a mass-restart from a mid-vote snapshot,
			// committedSlot[H] is re-seeded while certVotes is EMPTY — our self-vote is MISSING,
			// so the height can never reach α-of-K unless we re-contribute it. Re-offer the slot
			// ONLY when our self-vote is absent here; emitConvergedVote then re-signs the DURABLE
			// committedSlot[H] canonical (never a fresh winner) and reserveSlotForSign refuses any
			// conflicting sibling, so this is equivocation-safe.
			// CANONICAL-aggregated self-vote check (FIX #3): our vote for this height may be
			// recorded in a DIFFERENT wrapper of the same inner block than `pb` (votes bind the
			// canonical id; the re-sign resolves inner→outer to one wrapper), so inspect ANY
			// wrapper of cb's canonical — a per-`pb` check would busy-replay a height already
			// voted under a sibling envelope.
			if t.selfVotedForCanonicalLocked(cb.canonicalRep()) {
				continue // our vote is already in the cert set — normal one-per-height suppression
			}
			// fall through: re-offer this bound-but-unvoted slot for the re-sign fallback
		}
		s := votableSlot{height: cb.height, parentID: cb.parentID}
		if t1, ok := latest[s]; !ok || pb.ProposedAt.After(t1) {
			latest[s] = pb.ProposedAt
		}
	}
	var out []votableSlot
	for s, t1 := range latest {
		if now.Sub(t1) < settle {
			continue // not settled — siblings may still be arriving
		}
		if t.parentIsProvenLoserLocked(s.parentID) {
			continue // N1: parent lost its own height's convergence — its children are a dead branch
		}
		out = append(out, s)
	}
	return out
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

// canonicalCommitter is the OPTIONAL block interface that exposes the inner
// EXECUTION commitment (the incident-1082814 canonical identity), plumbed up from
// the proposervm wrapper. A proposervm signed block that wraps an inner execution
// block implements it; a bare/in-process VM block does not. Defined locally so the
// engine depends only on the four methods it reads.
//
// THE CONTRACT: CanonicalID is the inner execution block id — the value finality is
// defined over. Two outer proposervm envelopes wrapping the SAME inner block return
// the SAME CanonicalID, which is exactly what collapses them to duplicates instead
// of forks. ParentCanonicalID / ExecutionStateRoot / PayloadRoot bind the canonical
// ancestry and the exact execution result into the signed cert.
type canonicalCommitter interface {
	CanonicalID() ids.ID
	ParentCanonicalID() ids.ID
	ExecutionStateRoot() ids.ID
	PayloadRoot() ids.ID
}

// canonicalIDOf returns the block's inner EXECUTION commitment, or — for a block
// that does not expose one (bare/in-process VM, no proposervm wrapping at the engine
// boundary) — the block's OWN outer id. The fallback makes the scheme degrade
// EXACTLY to the pre-fix outer-id behavior on a non-wrapped chain (canonical ==
// outer ⇒ no two envelopes can share a canonical id ⇒ the duplicate-alias path is
// simply inert), while a real proposervm delivers the distinct inner id that
// distinguishes a duplicate envelope from a genuine fork. This is the SOLE place the
// engine reads the canonical id off a VM block.
func canonicalIDOf(b block.Block) ids.ID {
	if c, ok := b.(canonicalCommitter); ok {
		if id := c.CanonicalID(); id != ids.Empty {
			return id
		}
	}
	return b.ID()
}

// setCanonicalFromVM stamps the canonical execution-commitment fields onto a
// consensus Block from its VM block — the ONE boundary where the inner commitment
// enters consensus state. For a non-wrapped block canonicalID == outer id and the
// roots are Empty (unbound), so the position/cert are byte-compatible with the
// pre-canonical behavior. Called at every Block construction site (DRY).
func setCanonicalFromVM(cb *Block, vmBlock block.Block) {
	cb.canonicalID = canonicalIDOf(vmBlock)
	if c, ok := vmBlock.(canonicalCommitter); ok {
		cb.parentCanonicalID = c.ParentCanonicalID()
		cb.execStateRoot = c.ExecutionStateRoot()
		cb.payloadRoot = c.PayloadRoot()
	}
}

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
	var canonicalID, parentCanonicalID, execStateRoot, payloadRoot ids.ID
	if pending.ConsensusBlock != nil {
		parentID = pending.ConsensusBlock.parentID
		height = pending.ConsensusBlock.height
		canonicalID = pending.ConsensusBlock.canonicalID
		parentCanonicalID = pending.ConsensusBlock.parentCanonicalID
		execStateRoot = pending.ConsensusBlock.execStateRoot
		payloadRoot = pending.ConsensusBlock.payloadRoot
	}
	// canonicalID/parentCanonicalID are left as the block's RAW canonical fields: the
	// real inner id for a proposervm-wrapped block, or ids.Empty for a bare block. The
	// non-wrapped degrade (Empty ⇒ bind the outer id) is resolved in ONE place,
	// canonicalVoteMessageFor, so the signed bytes are identical for every producer of
	// a position (engine or test), for the same block.
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
		ChainID:            t.chainID,
		Height:             height,
		Round:              pending.Round,
		BlockID:            blockID,
		ParentID:           parentID,
		CanonicalID:        canonicalID,
		ParentCanonicalID:  parentCanonicalID,
		ExecutionStateRoot: execStateRoot,
		PayloadRoot:        payloadRoot,
		ValidatorSetRoot:   setRoot,
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
	setCanonicalFromVM(cb, blk) // stamp the inner execution commitment
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
	// Replay any votes a peer parked for this block before we tracked it (a
	// follower could have seen a peer's vote for our own block before our build
	// signal). Drain after unlock — drainBufferedVotes takes t.mu.
	t.drainBufferedVotes(blk.ID())
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
	pos := t.blockPositionLocked(pending, blockID)
	message := CanonicalVoteMessage(pos)
	// The epoch height pins every per-voter pubkey resolution + the stake tally to
	// the SAME P-chain height the position's set-root commits to (MEDIUM-1).
	epochHeight := t.epochHeightLocked(pending)
	// NOVA ACCEPT THRESHOLD — a bare-majority NovaQuorum(n) of the LIVE committee, NOT the ⅔
	// Quasar floor. This is the SOLE gate on VM.Accept (local execution): it must ignite at 3
	// of 5 so production continues when up to ⌊(n−1)/2⌋ crash faults keep the ⅔-stake quorum
	// unreachable (the "survive 3/5" mandate; the ⅔ bftAlpha=4-of-5 froze the fleet the instant
	// a 2nd node dropped). `n` is the live committee sized by effectiveCommittee — clamped to the
	// resolved set AND floored at the minimal BFT committee, so a transient/degenerate low count
	// can never drop the majority below a lone-reachable value (the 1085013 self-finality guard
	// carries over: NovaQuorum(floored-n) ≥ 3, unreachable by a lone node). The ⅔-by-stake QUASAR
	// EXPORT cert is a SEPARATE, trailing artifact (the attestation sidecar promoteQuasarLocked),
	// NEVER gated here — accept (Nova) and export (Quasar) are decomplected tiers.
	n, _ := t.effectiveCommittee(epochHeight)
	novaThreshold := NovaQuorum(n)
	if novaThreshold <= 0 {
		return nil
	}

	// FIX #3 (cert termination — per-CANONICAL vote aggregation). A vote is signed over the
	// CANONICAL execution identity, NOT the outer proposervm envelope
	// (canonicalVoteMessageFor): every wrapper of one inner block produces byte-identical
	// vote messages. But pendingBlocks is OUTER-keyed, so α validators that executed the
	// SAME inner block under DIFFERENT wrappers deposit their signed votes into DIFFERENT
	// pending blocks — and collecting only THIS wrapper's certVotes splits the quorum, so no
	// cert assembles and the height stalls forever (the block-288 wrapper-split; the SIGNING
	// aliases correctly but the AGGREGATION did not). Collect the signed votes from ALL
	// tracked sibling wrappers that share this winner's canonical identity and verify each
	// against the one canonical `message`. De-dup by NodeID HERE (a node that voted on two
	// wrappers signed the SAME bytes; AssembleQuorumCert REJECTS a duplicate NodeID), and
	// prefer this wrapper's own vote (collected first). This only ever ADDS votes, so the
	// signature verify + the ⅔-by-stake VerifyWeighted below remain the sole finality
	// authority — a liveness completion, never a safety relaxation.
	verified := make([]SignedVote, 0, len(pending.certVotes))
	seen := make(map[ids.NodeID]struct{}, len(pending.certVotes))
	collectVerified := func(votes map[ids.NodeID]SignedVote) {
		for nodeID, sv := range votes {
			if _, dup := seen[nodeID]; dup {
				continue
			}
			if t.voteVerifier.VerifyVote(sv.NodeID, message, sv.Signature, epochHeight) {
				seen[nodeID] = struct{}{}
				verified = append(verified, sv)
			}
		}
	}
	collectVerified(pending.certVotes)
	if pending.ConsensusBlock != nil {
		canon := pending.ConsensusBlock.canonicalRep()
		for _, sib := range t.pendingBlocks {
			if sib == pending || sib.ConsensusBlock == nil {
				continue
			}
			if sib.ConsensusBlock.canonicalRep() == canon {
				collectVerified(sib.certVotes)
			}
		}
	}
	if uint32(len(verified)) < uint32(novaThreshold) {
		return nil
	}
	cert, err := AssembleQuorumCert(pos, Nova, uint32(novaThreshold), verified)
	if err != nil {
		return nil
	}
	// Defence in depth: the Nova cert we just built must verify under our own verifier
	// before we treat it as a finality witness (catches any assembly invariant drift).
	// Assemble already enforced distinctness + threshold. On a stake-weighted chain
	// VerifyWeighted selects the NOVA tier — signatures + a strict COUNT majority of the
	// live set, DELIBERATELY NOT ⅔-by-stake — so acceptance ignites at a bare majority even
	// when the absent minority holds the stake majority (that is the whole point of Nova;
	// the ⅔-stake gate lives ONLY on the trailing Quasar cert). On a chain with no stake
	// source (equal-stake/dev) the tier-agnostic count-only Verify enforces the same
	// NovaQuorum count via the cert's own threshold.
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

// TryAccept is the ONE entry every finality trigger calls — a vote arrived, a
// re-poll fired, the pending queue changed, a block was built/verified, a poll
// timeout ticked. It is the single funnel onto the cert-gated acceptance path:
//
//	cert, err := <obtain a VerifiedQuorumCert for blockID>
//	if err != nil { return err }   // ErrNoVerifiedQC: not final yet, retry later
//	return t.AcceptWithCert(ctx, blockID, cert)
//
// A raw α-of-K COUNT is NOT an acceptance authority here. It is a LIVENESS
// signal: it may BRING us to TryAccept (the poll loop / vote handler call this
// when consensus signals "enough responses"), but TryAccept finalizes ONLY if a
// VerifiedQuorumCert can be produced — i.e. the votes assemble into a cert that
// clears VerifyWeighted's strict >⅔-of-stake gate. If not, TryAccept returns
// ErrNoVerifiedQC and changes nothing: the block stays pending+undecided and the
// trigger retries on its next tick. No count, no callback, no "enough voters
// responded" can finalize without the cert. This is the structural HIGH-3 fix.
//
//   - K>1: the cert is assembled+verified from the collected SIGNED votes
//     (assembleVerifiedCertLocked → BuildVerifiedQuorumCert semantics). The
//     verified cert is gossiped so followers finalize on the same proof.
//   - K==1: there are no peers; the sole validator's own accept IS the 1-of-1
//     quorum. We wrap it as a 1-of-1 VerifiedQuorumCert so even this path finalizes
//     ONLY through AcceptWithCert → FinalizeBranch (whose per-height gate keeps a
//     K==1 node from finalizing two blocks at one height) — one finalizer.
func (t *Transitive) TryAccept(ctx context.Context, blockID ids.ID) error {
	t.mu.Lock()
	pending, exists := t.pendingBlocks[blockID]
	if !exists || pending.Decided {
		t.mu.Unlock()
		return nil // nothing to accept (gone or already finalized) — not an error
	}
	// Track the live validator set: re-clamp the committee UP before choosing the single-validator
	// vs multi-validator finality path, so a chain that launched single-validator does not keep
	// finalizing unilaterally after it decentralizes (RED's 1→N fork).
	t.reclampCommitteeLocked()
	// K()==1 is the single-validator finality path. A multi-validator chain can no longer
	// REACH K()==1 through the live-committee sizer: bftCommittee floors a presetK>1
	// committee at the minimal BFT size (K≥4/α≥3) even when the validator count transiently
	// reads 1 during a restart, and reclampCommitteeLocked only ever grows K. So K()==1 now
	// implies a genuinely single-validator chain (presetK≤1: --dev, SingleValidatorParams,
	// or a launch-single L1 whose live set really is one) — the ONLY case where self-finality
	// is sound (no peer to fork against). The transient-K=1 self-finalization that forked
	// luxd-0/luxd-1 at 1085013 is closed at the ROOT (the sizer), not here.
	singleValidator := t.consensus.K() == 1

	if singleValidator {
		// 1-of-1 quorum: the sole validator's own accept IS the α-of-K. Build the
		// 1-of-1 verified cert and finalize through the SOLE finalizer (AcceptWithCert
		// → FinalizeBranch), whose per-height gate (a) keeps a K==1 node from
		// finalizing two blocks at one height. No separate force path.
		cert := t.buildSingleValidatorCertLocked(pending, blockID)
		t.mu.Unlock()
		return t.AcceptWithCert(ctx, blockID, cert)
	}

	// K>1: a verified α-of-K cert is the ONLY authority. Build+verify it from the
	// collected signed votes. nil ⇒ the verified ⅔-stake quorum is not present
	// yet ⇒ ErrNoVerifiedQC ⇒ liveness retry (NOT a finalize).
	cert, ok := t.assembleVerifiedCertLocked(pending, blockID)
	if !ok {
		t.mu.Unlock()
		return ErrNoVerifiedQC
	}
	var certBytes []byte
	if b, err := cert.Cert().MarshalBinary(); err == nil {
		certBytes = b
	}
	chainID := t.chainID
	gossiper := t.certGossiper
	t.mu.Unlock()

	// Distribute the finality proof so followers finalize on the same verifiable
	// witness (not a fast-follow guess). Best effort — local finality already
	// holds via the verified cert about to be committed.
	if gossiper != nil && certBytes != nil {
		_ = gossiper.GossipCert(chainID, blockID, certBytes)
	}

	return t.AcceptWithCert(ctx, blockID, cert)
}

// tryFinalizeBlock is a thin compatibility shim onto TryAccept for the
// peer-quorum triggers (poll-due, vote-handler). It exists so those call sites
// read as "try to finalize"; all real logic — and the cert gate — is in
// TryAccept. A consensus COUNT reaching α is what brings us here, but TryAccept
// finalizes only with a VerifiedQuorumCert; ErrNoVerifiedQC is the normal
// "wait" answer and is intentionally swallowed (the trigger retries next tick).
func (t *Transitive) tryFinalizeBlock(ctx context.Context, blockID ids.ID) {
	_ = t.TryAccept(ctx, blockID)
}

// finalizeOwnProposal is the proposer-side trigger after building its own block.
//
// THE FREEZE THIS USED TO "FIX" — AND HOW IT IS NOW FIXED WITHOUT SELF-FINALITY:
// the old version FORCE-ACCEPTED the proposer's own block on its lone self-vote
// (self-finality — a value could finalize with NO α-of-K agreement, so an
// equivocating proposer could fork the chain). DELETED for K>1. The freeze is
// now solved STRUCTURALLY by the vote-distribution topology (integration.go):
// followers gossip their SIGNED accept votes to ALL validators, the proposer
// assembles + gossips the cert, and finality comes via the verified cert.
//
// This is now just another trigger: it routes to TryAccept like every other
// trigger. K==1 finalizes via the 1-of-1 cert; K>1 finalizes IFF a verified
// α-of-K cert exists. Never forces a K>1 block on the lone self-vote.
func (t *Transitive) finalizeOwnProposal(ctx context.Context, blockID ids.ID) {
	// The own block is now tracked (buildBlocksLocked inserted it before calling
	// here, with the lock released). Replay any votes a peer parked for it before
	// our build signal so they count toward this attempt. Lock-free (drain takes
	// t.mu); every caller invokes this without holding t.mu.
	t.drainBufferedVotes(blockID)
	t.mu.RLock()
	pending, exists := t.pendingBlocks[blockID]
	own := exists && pending.IsOwnProposal && !pending.Decided
	t.mu.RUnlock()
	if !own {
		return
	}
	_ = t.TryAccept(ctx, blockID)
}

// assembleVerifiedCertLocked builds the FINALITY AUTHORITY TOKEN for blockID from
// the collected signed accept votes, or reports that no verified quorum exists
// yet. Caller holds t.mu. It delegates the predicate to assembleCertLocked
// (assemble + signature-verify + VerifyWeighted's strict >⅔-of-stake gate — the
// SINGLE place the stake predicate lives) and wraps the verified result. ok=false
// (zero cert) ⇒ the verified ⅔-stake quorum is not present yet ⇒ the caller must
// NOT finalize (it returns ErrNoVerifiedQC and the trigger retries). There is no
// other in-engine producer of the token for the multi-validator path, so the
// count road has no way to manufacture finality.
func (t *Transitive) assembleVerifiedCertLocked(pending *PendingBlock, blockID ids.ID) (VerifiedQuorumCert, bool) {
	cert := t.assembleCertLocked(pending, blockID)
	if cert == nil {
		return VerifiedQuorumCert{}, false
	}
	// assembleCertLocked has already run VerifyWeighted/Verify before caching the
	// cert, so promotion is safe; wrapVerifiedCert refuses only a nil cert.
	return wrapVerifiedCert(cert)
}

// buildSingleValidatorCertLocked produces the 1-of-1 VerifiedQuorumCert for the
// K==1 path so the single-validator node finalizes through the SAME sole finalizer
// (AcceptWithCert → FinalizeBranch) as every other path — one finalization road. The
// FinalizeBranch inside that finalizer is what commits the decision and enforces the
// per-height equivocation gate; this function only builds the authorizing token.
// Caller holds t.mu. On a K==1 chain α==1 and the sole validator's own signed accept
// (recordOwnVoteLocked, captured at build time) is the entire quorum; assembleCertLocked
// verifies that single signature and the (trivially satisfied) stake gate. If a
// verifier/signer is not wired (a pure single-node dev chain with no vote crypto), there
// is no signature to certify — we authorize the commit with a degenerate non-zero token
// whose cert carries the position, and FinalizeBranch (the real single-node safety gate:
// one block per height, contiguous, reorg-on-conflict) does the commit. This degenerate
// token exists ONLY for K==1 and can never arise for K>1 (TryAccept's K>1 branch never
// calls here).
// reclampCommitteeLocked re-clamps the live committee (k, alpha) UP to track the CURRENT
// validator count, so a chain that launched single-validator does not keep k stuck at 1 after it
// adds validators (the 1→N decentralization fork: a stuck-k=1 validator would finalize
// unilaterally via a synthesized 1-of-1 cert OR a single-signer stake cert at alpha=1). It is
// UP-ONLY: it never shrinks the committee here, so a transient low read (a syncing node, an
// unresolved set) can never DROP the quorum and enable a unilateral finalize; a genuine shrink is
// an operator/governance action. No-op without a wired sampler or preset (tests / --dev), or when
// the set is unresolved (count<1). Called from the finalize decision points (buildBlocksLocked,
// TryAccept) under t.mu — the same nesting order (t.mu → c.mu) K() already uses, and k is written
// only here so the K() read + Reclamp cannot race.
func (t *Transitive) reclampCommitteeLocked() {
	if t.liveValidatorCount == nil || t.presetK <= 0 {
		return
	}
	// RESOLVED-SET GATE (do not trust the sampler until the chain is actually running). A FRESH
	// single-validator chain can transiently over-report its validator count before the set is
	// resolved (the genesis/staking set not yet pruned to the active validator); reclamping K→N on
	// that read would skip the K==1 inline finalize and wedge the chain at block 0. Requiring at
	// least one FINALIZED block (GetFinalizedHeight set) means the chain has already produced +
	// finalized a block under its construction committee, so the live set read is now authoritative.
	// A genuinely single-validator chain (count==1) never reclamps regardless; a chain that
	// decentralizes has long since finalized blocks, so this never blocks the real 1→N transition.
	if _, set := t.consensus.GetFinalizedHeight(); !set {
		return
	}
	count := t.liveValidatorCount()
	if count < 1 {
		return // set not resolved/loaded for this network — keep the construction committee
	}
	newK := t.presetK
	if count < newK {
		newK = count
	}
	if newK <= t.consensus.K() {
		return // UP-ONLY (never auto-shrink)
	}
	newAlpha := bftAlpha(newK) // the BFT ⅔ supermajority — ONE formula (config.TwoThirdsStakeFloor)
	t.consensus.Reclamp(newK, newAlpha)
	if t.log != nil {
		t.log.Warn("committee re-clamped UP to the live validator set (decentralization) — "+
			"single-validator finality is no longer permitted",
			"newK", newK, "newAlpha", newAlpha, "liveCount", count, "presetK", t.presetK)
	}
}

func (t *Transitive) buildSingleValidatorCertLocked(pending *PendingBlock, blockID ids.ID) VerifiedQuorumCert {
	// Prefer the VERIFIED 1-of-1 cert: when vote crypto is wired the proposer
	// recorded its own signed accept (recordOwnVoteLocked), so assembleCertLocked
	// verifies that single signature (and the trivially-met stake gate) and we
	// finalize on a real witness — even on a single-validator chain.
	if cert, ok := t.assembleVerifiedCertLocked(pending, blockID); ok {
		return cert
	}
	// DECENTRALIZATION FORK GUARD (belt over reclampCommitteeLocked). Re-read the LIVE validator
	// count and REFUSE to synthesize a 1-of-1 cert if the set has grown past ONE validator. K is
	// clamped at construction and re-clamped UP by reclampCommitteeLocked before the
	// single-validator decision, so K()==1 should already imply a one-validator set — but a lone
	// validator must NEVER finalize a synthesized 1-of-1 cert on a multi-validator chain: RED's
	// 1→N fork is that validators 2..N reject the unsigned cert (VerifyWeighted / real quorum)
	// while validator-1 serves a DIVERGENT finalized chain over RPC. Returning ZERO makes
	// acceptWithCertCore refuse, so finality waits for a real k-of-N cert (assembled once the
	// peers' votes arrive). count<1 (unwired sampler / --dev / a not-yet-resolved set) keeps the
	// genuine-single-validator + dev path (the n=1 fix) — a fresh chain that knows of no other
	// validator has no fork to create.
	// RESOLVED-SET GATE (mirrors reclampCommitteeLocked): only refuse to synthesize once the chain
	// has FINALIZED at least one block, so a fresh single-validator chain that transiently
	// over-reports its count cannot wedge its own first block by refusing the 1-of-1 cert. Once the
	// set is authoritative, a live count > 1 means this stuck-K==1 engine must NOT finalize
	// unilaterally (the 1→N fork) — return ZERO and let a real quorum finalize.
	if t.liveValidatorCount != nil {
		if _, resolved := t.consensus.GetFinalizedHeight(); resolved {
			if count := t.liveValidatorCount(); count > 1 {
				return VerifiedQuorumCert{}
			}
		}
	}
	// K==1 FALLBACK — synthesize the 1-of-1 finality witness from the position. Both callers
	// gate on K()==1, so this is a genuinely SINGLE-validator engine: the dynamic committee
	// clamp (bftCommittee) sets K to the LIVE validator count, so K()==1 ⟺ exactly one
	// validator (a K>1 chain never reaches here). The sole validator's own accept IS the 1-of-1
	// quorum, so this is NOT fabricating a multi-party agreement — there is no other validator
	// to protect against — and FinalizeBranch's per-height gate (one block per height,
	// contiguous, no branching) remains the real single-node safety.
	//
	// This fallback is taken EVEN WHEN a verifier is wired but the signed self-vote did not
	// assemble into a verified cert — the n=1 DECIDE stall. On a fresh single-validator
	// sovereign L1 (Zoo 200200, Hanzo 36963, Pars 494949) the preset K>1 wires a verifier, then
	// the clamp drops K to 1; the validator set is not yet resolvable at the block's P-chain
	// epoch, so the self-vote's signature cannot be verified against it, assembleVerifiedCert
	// returns false, and the OLD code returned a ZERO cert that acceptWithCertCore refused — so
	// the block re-built every poll and NEVER decided (EVM head frozen). A single validator
	// finalizing its OWN block on its OWN chain needs no external signature witness; requiring
	// one that can never verify wedges the chain. (A real signed 1-of-1 cert is still PREFERRED
	// above when it assembles; making the sovereign-L1 set resolvable so it always does is a
	// separate hardening.)
	pos := t.blockPositionLocked(pending, blockID)
	return VerifiedQuorumCert{qc: &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
		Tier:      Nova, // a K==1 accept authorizes LOCAL execution; Quasar (export) is a separate trailing artifact
		Position:  pos,
		Threshold: 1,
	}}
}

// AcceptWithCert is the SOLE function that can finalize a block. It is impossible
// to call without a VerifiedQuorumCert value, and a zero VerifiedQuorumCert
// (cert==nil) is refused — so the ONLY way to reach VM.Accept is to first hold a
// cert that cleared the finality predicate (BuildVerifiedQuorumCert /
// assembleVerifiedCertLocked / the verified incoming-cert path). The old
// finalizePendingLocked body lives here unchanged; the difference is that it can
// no longer be reached by any count-only road — the type system enforces it.
//
// Idempotent: subsequent calls find pending.Decided=true and no-op.
//
// It signals the pipeline to build the next block on success — the right thing
// for an OUT-OF-BAND finalize (a vote/cert arrived async, or the poll loop fired)
// where nothing else is driving production. The synchronous in-build-loop path
// (buildBlocksLocked) instead uses acceptWithCertCore(..., signalNext=false): it
// is already inside the build loop, so re-signaling would spawn a SECOND
// concurrent builder and race the VM's block counter (the K=1 burst regression).
func (t *Transitive) AcceptWithCert(ctx context.Context, blockID ids.ID, cert VerifiedQuorumCert) error {
	return t.acceptWithCertCore(ctx, blockID, cert, true)
}

// acceptWithCertCore is the one finalization body. signalNext controls only
// whether it wakes the pipeline afterward (see AcceptWithCert). Everything that
// makes finality safe — the zero-cert refusal, the Decided/idempotency guard,
// the VM Accept+SetPreference ordering — is identical on both call paths.
func (t *Transitive) acceptWithCertCore(ctx context.Context, blockID ids.ID, cert VerifiedQuorumCert, signalNext bool) error {
	if cert.IsZero() {
		// No verified witness ⇒ no finality. This is the structural guarantee:
		// even an internal caller cannot finalize by passing a zero cert.
		return ErrNoVerifiedQC
	}

	// Fast idempotent out: an already-decided (or untracked) block needs no
	// re-finalize. FinalizeBranch is itself idempotent, but skipping it avoids the
	// consensus lock on the hot re-delivery path.
	t.mu.RLock()
	pending, exists := t.pendingBlocks[blockID]
	decided := exists && pending.Decided
	// Capture the block's P-chain EPOCH now, while the pending block still exists — the Nova
	// accept below drops it, and the trailing Quasar promotion needs the same epoch the Nova
	// cert was verified under to resolve the ⅔-stake tally (MEDIUM-1 parity).
	var quasarEpoch uint64
	if exists && pending != nil {
		quasarEpoch = t.epochHeightLocked(pending)
	}
	t.mu.RUnlock()
	if !exists || decided {
		return nil
	}

	// COMMIT FINALITY to the consensus ledger — the single finalize. FinalizeBranch
	// walks the certified branch from the finalized tip up to blockID, advances
	// finalized history, and returns the REORG plan: the path to Accept (ascending
	// height) and the losing-sibling subtrees to prune. On a safety violation —
	// equivocation (ErrHeightAlreadyFinalized), a conflicting/losing branch
	// (ErrConflictsWithFinalizedBranch), or a not-yet-tracked ancestor
	// (ErrAncestorNotTracked, a behind-node DEFER) — NOTHING is applied and the error
	// propagates (HandleIncomingCert surfaces equivocation; a DEFER simply retries).
	// Called WITHOUT t.mu so the consensus lock is never nested under it.
	pos := cert.Cert().Position
	// LOW-1 (defense-in-depth): the cert α-attests the FULL position {BlockID, ParentID,
	// Height, CanonicalID}; build the finalize Cert ENTIRELY from it so the fold can
	// never be fed a Block from one source and Parent/Height/Canonical from another.
	// blockID is only the pending lookup key above — a verified cert's position must
	// name that same outer block, else the trio is inconsistent and we fail closed
	// rather than finalize the wrong block. (When a cert is resolved by CANONICAL id
	// across a differing envelope, HandleIncomingCert finalizes the LOCAL outer id and
	// the cert's canonical, so this equality holds on the resolved target.)
	if pos.BlockID != blockID {
		return fmt.Errorf("cert position block %s != finalize target %s (inconsistent cert trio)", pos.BlockID, blockID)
	}
	plan, err := t.consensus.ApplyCert(Cert{Block: pos.BlockID, Parent: pos.ParentID, ParentCanonical: pos.ParentCanonicalID, Height: pos.Height, Canonical: pos.CanonicalID})
	if err != nil {
		return err
	}

	// Apply the plan to the VM + engine bookkeeping: VM.Accept the finalized path (fail-closed
	// — stops at the first VM.Accept error), VM.Reject the pruned losers, record engine
	// finality, store the serving cert. highestAccepted is the highest height the VM ACTUALLY
	// applied; the durable floor advances only to it (never past the VM's state).
	highestAccepted, acceptErr := t.applyBranchFinalization(ctx, plan, blockID, cert)

	// MEDIUM-1: drop the equivocation guard + view machines STRICTLY at/below what the VM
	// accepted — NEVER to pos.Height when the VM applied less (the fail-closed invariant: the
	// decided-floor can never run ahead of the EVM's accepted head, which is exactly the
	// consensus-finalize→VM-accept divergence this fix kills). Only advance if we accepted
	// something.
	if highestAccepted > 0 {
		t.pruneCommittedSlotsBelow(highestAccepted)
	}
	// MED-6: bound the slashing detector's per-height maps to a sliding window (memory).
	t.pruneSlashingBelowWindow()
	// Bound the trailing Quasar machinery (attestor buckets/certs + epoch map) to the SAME
	// window below the accepted height — the export cert chain is persisted by its consumer.
	if highestAccepted > slashingRetentionHeights {
		t.pruneQuasarBelow(highestAccepted - slashingRetentionHeights)
	}

	if acceptErr != nil {
		// FAIL-CLOSED: the VM refused to apply a consensus-finalized block. The floor did NOT
		// advance past the VM's applied state, so consensus and the EVM stay in lock-step. The
		// block stays pending and the finalize retries; the chain HALTS at the un-appliable
		// block rather than DIVERGING (floor ahead of the EVM). Recovery is the VM-side fix
		// (apply the finalized block regardless of emptiness) — then the retry finalizes it.
		return acceptErr
	}

	// TRAILING EXPORT PROMOTION (Nova-sole-decider, step 2): the block is now Nova-accepted
	// (in the ledger). Feed its OWN verified accept votes to the Quasar attestor; if a
	// ⅔-by-stake supermajority has attested, it emits the export cert and advances the Quasar
	// frontier. This runs strictly AFTER accept and NEVER gates it — a block that only reached
	// a bare majority stays Nova-only (degraded), and a later fully-voted block advances the
	// export frontier past the gap. Best-effort; an equivocation breach halts export
	// fail-closed inside promoteQuasar (never a silent export of a forked block).
	t.promoteQuasar(cert.Cert(), quasarEpoch)

	// FULL SUCCESS: REORG production onto the certified branch: SetPreference to the new tip
	// keeps the VM building on the block consensus just finalized.
	t.mu.RLock()
	vm := t.vm
	t.mu.RUnlock()
	if vm != nil {
		// Steer at the LIVE build anchor, never the stale local blockID. t.mu is released
		// across every VM call-out above, so by the time we get here another finalize may
		// already have advanced BOTH the ledger and the VM past blockID; SetPreference(blockID)
		// is then a BACKWARDS steer that the EVM correctly refuses ("cannot orphan finalized
		// block at height H to common block at height H-1"). PreferredBuildTip descends from
		// ledger.BuildAnchor — the HIGHER of {certified tip, recovery hint}, which the accept
		// ordering (ApplyCert BEFORE VM.Accept) keeps at or above the VM's own accepted head —
		// so it can never point below what the VM has accepted. This is the SAME value the
		// build path already steers with (buildBlocksLocked): one build target, computed one
		// way, read in both places.
		//
		// HeldBuildTip applies the single-store invariant: if this node fell behind and the
		// DAG tip is not in its own store, fall back to blockID — which VM.Accept just applied,
		// so the VM provably holds it. Falling back is strictly better than steering at an
		// unheld id, which our proposervm answers with a warn + nil, leaving the VM building
		// on a head OLDER than the block we just finalized. If blockID is itself the backwards
		// steer described above, SetPreference errors and the reconcile path below adjudicates
		// it against our own ledger — the same path, unchanged.
		target := t.HeldBuildTip(ctx, vm, blockID)
		if err := vm.SetPreference(ctx, target); err != nil {
			// The VM refused to move its preferred/accepted head onto `target`. The refusal we
			// must handle is the EVM's "cannot orphan finalized block" guard: the VM's OWN
			// accepted head sits on a DIFFERENT branch at or above target's height (a
			// provisional Nova tip that ran ahead of, or diverged from, consensus finality —
			// the build-ahead / second-accept-authority desync).
			//
			// This is NOT, by itself, a consensus safety violation: the finality LEDGER already
			// recorded blockID correctly (ApplyCert, above) and it is the SOLE certified
			// canonical at its height (byHeight is one-canonical-per-height). Crashing the node
			// (log.Crit → os.Exit) neither undoes that finality nor repairs the VM; it only
			// removes a healthy validator. Instead reconcile the VM to `target` — but ONLY
			// after proving, against our own ledger, that the tip we drop is UNCERTIFIED.
			// reconcileVMToCertified returns false ONLY when the orphaned head is itself a
			// consensus-certified block that `target` — a block our ledger does NOT certify at
			// its height — would displace; the one case that MUST stay fail-closed.
			if !t.reconcileVMToCertified(ctx, vm, target, err) {
				// The tip we would orphan is ITSELF a consensus-certified block that
				// `target` displaces — the one state that MUST halt fail-closed. This is
				// the SOLE safety halt on this path, so it must terminate independently of
				// logger wiring: log.Crit is fatal only under a real (non-noop) logger, and
				// the engine defaults to log.Noop(), under which Crit is a silent no-op that
				// would fall through to ForcePreference. Log the detail, then os.Exit(1)
				// unconditionally so the halt never depends on WithLogger being set.
				t.log.Crit("SetPreference would orphan a CONSENSUS-CERTIFIED block — refusing (fail-closed)",
					"certified", target,
					"error", err)
				os.Exit(1)
			}
			t.consensus.ForcePreference(target)
		}
	}

	// Pipeline: block finalized → immediately build next (out-of-band callers
	// only; the in-build-loop caller continues its own loop and passes false).
	if signalNext {
		t.signalPipeline()
	}
	return nil
}

// reconcileVMToCertified resolves a VM SetPreference refusal ("cannot orphan finalized
// block") that fired while steering the VM onto the just-finalized `certified` block. It
// returns true when it SAFELY handled the divergence (the caller must NOT halt) and false
// ONLY when the tip the VM would orphan is itself a consensus-certified block that
// `certified` displaces — a double-finalization the caller MUST treat as fail-closed.
//
// The consensus finality ledger is the SOLE authority. `certified` was just committed to
// byHeight at its height, so it is the one certified canonical there. We classify the VM's
// diverged accepted head against that ledger:
//
//   - head above the finalized frontier, OR a losing sibling of a finalized block (its
//     canonical is NOT the ledger's finalized canonical at its height) ⇒ UNCERTIFIED ⇒
//     safe to drop. Reconcile the VM to `certified` (via the optional PreferenceReconciler)
//     or, if the VM cannot reconcile live, surface the divergence and defer to recovery.
//     Either way the node keeps its correct consensus finality and does not crash.
//
//   - head IS the ledger's finalized canonical at its height AND `certified` is NOT the
//     ledger's certified canonical at ITS OWN height ⇒ steering there drops a certified
//     block for one we do not certify ⇒ return false so the caller halts fail-closed.
//     When BOTH are certified at their own heights they lie on the ONE certified chain
//     (one canonical per height, contiguous ancestry — Finalize enforces both), so the head
//     merely DESCENDS from `certified`: nothing is orphaned and the refusal was only a
//     backwards steer. Asking solely "is the head finalized at its height?" — as this did
//     before — is trivially true for every healthy node whose head is certified, and killed
//     five live validators on a benign head = certified+1.
//
// SAFETY: a tip is dropped ONLY when its canonical is provably NOT in byHeight, so no
// certified block is ever orphaned. The VM's own ⅔-Quasar floor gate (PreferenceReconciler
// contract) is a second, independent guarantee. Called WITHOUT t.mu (all reads go through
// the VM and the consensus lock).
func (t *Transitive) reconcileVMToCertified(ctx context.Context, vm BlockBuilder, certified ids.ID, setPrefErr error) (handled bool) {
	headID, err := vm.LastAccepted(ctx)
	if err != nil {
		// Cannot read the VM's head ⇒ cannot classify. Do NOT crash over a transient VM read
		// error and do NOT touch the VM (so nothing is orphaned); the ledger's finality is
		// already correct. Surface loudly and let recovery align the VM.
		t.log.Error("SetPreference refused and VM last-accepted is unreadable — leaving VM head untouched (consensus finality is correct; deferring reconcile to recovery)",
			"certified", certified, "setPreferenceError", setPrefErr, "lastAcceptedError", err)
		return true
	}
	if headID == certified {
		// No real divergence (a transient refusal); the caller's ForcePreference reasserts the
		// correct build anchor. Nothing to reconcile.
		return true
	}

	headBlk, err := vm.GetBlock(ctx, headID)
	if err != nil {
		t.log.Error("SetPreference refused and the VM's diverged head is unreadable — leaving VM head untouched (deferring reconcile to recovery)",
			"certified", certified, "divergedHead", headID, "err", err)
		return true
	}
	headHeight := headBlk.Height()
	headCanonical := canonicalIDOf(headBlk)

	// Classify against the finality ledger (the sole authority).
	finCanonical, finalized := t.consensus.FinalizedBlockAtHeight(headHeight)
	if finalized && finCanonical == headCanonical {
		// The head IS the ledger's certified canonical at its height. That ALONE is NOT a
		// double-finalization — it is the NORMAL state whenever the head is a certified
		// DESCENDANT of the block we are steering to (a finalize that completed while this
		// one sat between its VM call-outs leaves head = certified+1). The ledger holds
		// exactly ONE canonical per height along ONE contiguous chain — Finalize refuses
		// both equivocation (ErrHeightAlreadyFinalized) and non-descendant branches
		// (ErrConflictsWithFinalizedBranch) — so when `certified` is ITSELF the ledger's
		// certified canonical at ITS OWN height, the two lie on that one chain, the VM
		// already CONTAINS `certified`, and the refusal was merely a BACKWARDS steer.
		// Nothing is orphaned: no action, no halt.
		if t.certifiedAtItsHeight(ctx, vm, certified) {
			t.log.Debug("SetPreference refused a backwards steer — the VM head is a CERTIFIED DESCENDANT of the finalized block (nothing to orphan)",
				"certified", certified, "head", headID, "headHeight", headHeight)
			return true
		}
		// The head is certified AND `certified` is not on the certified chain: steering
		// there would drop a consensus-certified block in favour of one our own ledger does
		// NOT certify at its height. THE safety violation — halt fail-closed.
		t.log.Error("VM accepted head is CONSENSUS-CERTIFIED and the finalized block is NOT on the certified chain — refusing to orphan it",
			"certified", certified, "orphanedHead", headID,
			"orphanedHeight", headHeight, "orphanedCanonical", headCanonical)
		return false
	}

	// The head is UNCERTIFIED (above the finalized frontier, or a losing sibling): dropping it
	// orphans no certified state. Reconcile the VM to the certified block if it can do so live.
	if r, ok := vm.(PreferenceReconciler); ok {
		if rerr := r.ReconcilePreference(ctx, certified); rerr == nil {
			t.log.Warn("reconciled VM to the certified block — dropped an uncertified provisional tip that had diverged from consensus finality",
				"certified", certified, "droppedHead", headID, "droppedHeight", headHeight)
			return true
		} else {
			t.log.Error("VM reconcile to the certified block failed — VM head left diverged (consensus finality is correct; deferring to recovery)",
				"certified", certified, "droppedHead", headID, "err", rerr)
			return true
		}
	}

	// The VM cannot reconcile live. Do not crash: the ledger's finality is correct and the
	// dropped tip is uncertified. Surface for the offline accepted-tip rewind (core.FinalizeRewind).
	t.log.Error("SetPreference refused by an uncertified provisional VM tip and the VM has no live reconcile — consensus finality is correct; deferring VM head reconcile to recovery (non-fatal)",
		"certified", certified, "divergedHead", headID, "divergedHeight", headHeight, "setPreferenceError", setPrefErr)
	return true
}

// certifiedAtItsHeight reports whether id is the finality ledger's certified canonical at
// id's OWN height — i.e. whether id lies on the ONE certified chain. The height is read off
// the VM rather than plumbed down from the cert so the answer is correct for ANY steer
// target (the just-finalized block, or the live build anchor). Unreadable ⇒ false: the sole
// caller uses this to authorise NOT dropping a certified head, so the unprovable case must
// fail closed.
func (t *Transitive) certifiedAtItsHeight(ctx context.Context, vm BlockBuilder, id ids.ID) bool {
	blk, err := vm.GetBlock(ctx, id)
	if err != nil {
		return false
	}
	fin, ok := t.consensus.FinalizedBlockAtHeight(blk.Height())
	return ok && fin == canonicalIDOf(blk)
}

// promoteQuasar is the trailing EXPORT-cert step — the ONE place a ⅔-by-stake Quasar
// certificate is produced, run strictly AFTER a Nova accept and NEVER gating it. novaCert is
// the just-accepted block's Nova cert; its Votes are valid attestations of the same canonical
// position (byte-identical CanonicalVoteMessage), so they feed the attestor directly. When a
// ⅔ stake supermajority has attested, the attestor emits the export cert and this advances the
// consensus Quasar frontier (the ONLY tip GetQuasarTip / export consumers read). Called
// WITHOUT t.mu.
//
//   - Below ⅔ stake (e.g. 3 of 5): no export cert emits — the block stays Nova-only (the
//     degraded mode: producing but not certifying). A later fully-voted block advances the
//     export frontier past the gap (a Quasar tip finalizes all its ancestors), so no
//     per-block retroactive promotion is needed for liveness.
//   - Equivocation breach (a ⅔ export cert names a canonical the Nova ledger did NOT accept at
//     this height): needs >⅓ stake double-signing, beyond f<⅓ — export HALTS fail-closed and
//     the breach is logged as slashable. NEVER a silent export of a forked block.
func (t *Transitive) promoteQuasar(novaCert *QuorumCert, epochHeight uint64) {
	if novaCert == nil {
		return
	}
	t.mu.RLock()
	attestor := t.quasarAttestor
	stake := t.stakeSource
	t.mu.RUnlock()
	if stake == nil {
		return // no stake model wired ⇒ no export surface, no responsive-stake signal
	}
	pos := novaCert.Position
	// Record responsive stake from the ACCEPTED block's votes ALWAYS — the degraded-mode RPC
	// signal must update even when ⅔ was NOT reached (3/5 ⇒ 60% ⇒ degraded=true).
	t.recordResponsiveStake(stake, novaCert.Votes, epochHeight)
	if attestor == nil {
		return
	}
	// Remember the accepted block's signed position so LATE votes can complete the export cert
	// AFTER the pending block is dropped. The Nova cert freezes at the BARE MAJORITY (the accept
	// threshold), so the ⅔-th stake vote by definition TRAILS the accept — it arrives when the
	// block is already finalized and would otherwise be dropped. rememberAcceptedPos + the
	// finalized-block attestation tap (handleVote) route those trailing votes to the attestor so
	// export can still form. Feed the majority votes now (seeds the bucket + emits if ⅔ already
	// present, e.g. a burst-delivered vote set).
	t.rememberAcceptedPos(pos, epochHeight)
	for i := range novaCert.Votes {
		t.ingestAttestation(pos, epochHeight, novaCert.Votes[i])
	}
}

// ingestAttestation feeds ONE verified accept vote — an attestation of the accepted block's
// canonical position — into the Quasar attestor and, if it completes a ⅔-by-stake export cert,
// advances the Quasar frontier. It is idempotent and safe for pre-accept, at-accept, and
// post-accept (late) votes: the attestor dedups by NodeID and PromoteQuasar only ever moves the
// frontier forward to a Nova-accepted canonical. An equivocation breach (a ⅔ cert naming a
// canonical the Nova ledger did not accept) HALTS export fail-closed. Called WITHOUT t.mu.
func (t *Transitive) ingestAttestation(pos VotePosition, epochHeight uint64, sv SignedVote) {
	t.mu.RLock()
	attestor := t.quasarAttestor
	t.mu.RUnlock()
	if attestor == nil {
		return
	}
	// The epoch is passed as a VALUE bound to this position — the SAME epoch the Nova cert was
	// verified under — so the attestor never crosses epochs on a same-height fork (values, not places).
	cert, emitted, _ := attestor.Ingest(pos, epochHeight, sv)
	if !emitted {
		return
	}
	advanced, err := t.consensus.PromoteQuasar(Cert{Block: pos.BlockID, Parent: pos.ParentID, Height: pos.Height, Canonical: pos.CanonicalID})
	if err != nil {
		if t.log != nil {
			t.log.Error("QUASAR EQUIVOCATION — a ⅔-stake export cert conflicts with the Nova-accepted "+
				"block at this height; export HALTED fail-closed (slashable — needs >⅓ stake double-signing)",
				"height", pos.Height, "canonical", pos.CanonicalID, "error", err)
		}
		return
	}
	if advanced {
		// The export cert reflects the ACTUAL responsive stake for this block (≥⅔, since the trailing
		// votes completed it) — refresh the degraded-mode signal so CertificateAvailable/Degraded
		// clear once export forms, not stay pinned to the bare-majority Nova cert that froze earlier.
		t.mu.RLock()
		stake := t.stakeSource
		observer := t.quasarObserver
		t.mu.RUnlock()
		t.recordResponsiveStake(stake, cert.Votes, epochHeight)
		canonical := pos.CanonicalID
		if canonical == ids.Empty {
			canonical = pos.BlockID
		}
		// PUSH the export frontier advance to the VM/consumers (bridges / EVM `finalized`/`safe` /
		// warp export gating), symmetric with how block.Accept pushes the Nova tip. The observer is
		// the ONE seam an export surface subscribes to; it fires strictly AFTER the block's Nova
		// accept, monotonically. Best-effort, WITHOUT t.mu.
		if observer != nil {
			observer(canonical, pos.Height)
		}
		if t.log != nil {
			t.log.Debug("quasar: export frontier advanced (⅔-by-stake certificate formed)",
				"height", pos.Height, "canonical", canonical)
		}
	}
}

// acceptedPos is the remembered signed position + epoch of a Nova-accepted block (see the
// acceptedPos field), enabling attestation of trailing votes after the pending block is dropped.
type acceptedPos struct {
	pos   VotePosition
	epoch uint64
}

// rememberAcceptedPos records the accepted block's signed position + epoch keyed by its outer
// id, so a LATE accept vote (the ⅔-th stake vote trails the bare-majority Nova accept) can be
// fed to the attestor after the pending block is dropped. Bounded: pruned to the attestor
// window.
func (t *Transitive) rememberAcceptedPos(pos VotePosition, epoch uint64) {
	t.acceptedPosMu.Lock()
	defer t.acceptedPosMu.Unlock()
	if t.acceptedPos == nil {
		t.acceptedPos = make(map[ids.ID]acceptedPos)
	}
	t.acceptedPos[pos.BlockID] = acceptedPos{pos: pos, epoch: epoch}
}

// lookupAcceptedPos returns the remembered signed position + epoch for an accepted (now
// finalized) outer block id, so a trailing vote can be attested against the exact bytes the
// accept votes signed. ok=false when the block is unknown or aged out of the window.
func (t *Transitive) lookupAcceptedPos(blockID ids.ID) (acceptedPos, bool) {
	t.acceptedPosMu.Lock()
	defer t.acceptedPosMu.Unlock()
	ap, ok := t.acceptedPos[blockID]
	return ap, ok
}

// attestFinalizedVote routes a LATE accept vote for an already Nova-accepted (finalized) block
// to the Quasar attestor, so the ⅔-th stake vote — which necessarily trails the bare-majority
// accept — can still complete the EXPORT cert. It verifies the signature against the EXACT
// accepted position (the same bytes the accept votes signed) BEFORE feeding, so a late
// forged/wrong-position vote is dropped, never attested. No-op when the block aged out of the
// remembered window or no attestor/verifier is wired. Called WITHOUT t.mu.
func (t *Transitive) attestFinalizedVote(vote Vote, verifier VoteVerifier) {
	if !vote.Accept || verifier == nil || len(vote.Signature) == 0 {
		return
	}
	ap, ok := t.lookupAcceptedPos(vote.BlockID)
	if !ok {
		return
	}
	msg := CanonicalVoteMessage(ap.pos)
	if !verifier.VerifyVote(vote.NodeID, msg, vote.Signature, ap.epoch) {
		return // forged / wrong-position trailing vote — dropped, never attested
	}
	t.ingestAttestation(ap.pos, ap.epoch, SignedVote{NodeID: vote.NodeID, Accept: true, Signature: vote.Signature})
}

// recordResponsiveStake stores the stake that voted on the latest accepted block (numerator)
// out of the epoch total (denominator) — the degraded-mode RPC signal read by FinalityStatus.
func (t *Transitive) recordResponsiveStake(stake StakeSource, votes []SignedVote, epochHeight uint64) {
	if stake == nil {
		return
	}
	total := stake.TotalStake(epochHeight)
	if total == 0 {
		return
	}
	var voted uint64
	for i := range votes {
		voted += stake.Weight(votes[i].NodeID, epochHeight)
	}
	t.mu.Lock()
	t.responsiveStakeNum = voted
	t.responsiveStakeDen = total
	t.mu.Unlock()
}

// attestorEpochOf maps a value-chain height to the P-chain epoch its weighted set is pinned to
// — the attestor's per-voter pubkey + ⅔-stake resolution epoch. Returns the recorded epoch, or
// pruneQuasarBelow bounds the trailing export machinery (attestor buckets/certs + the
// remembered accepted positions) to a window below the finalized height — the external cert
// chain is persisted by its consumer; the engine keeps only a live window. No-op when no
// attestor is wired.
func (t *Transitive) pruneQuasarBelow(height uint64) {
	t.mu.RLock()
	attestor := t.quasarAttestor
	t.mu.RUnlock()
	if attestor != nil {
		attestor.PruneBelow(height)
	}
	t.acceptedPosMu.Lock()
	for id, ap := range t.acceptedPos {
		if ap.pos.Height < height {
			delete(t.acceptedPos, id)
		}
	}
	t.acceptedPosMu.Unlock()
}

// FinalityStatus is the two-tier finality snapshot for RPC / degraded-mode visibility. Each
// tier is a DISTINCT field: a Nova (accept) height is NEVER reported as certified/quasar. A
// consumer that needs export finality reads QuasarHeight (or GetQuasarTip), never NovaHeight.
type FinalityStatus struct {
	NovaHeight    uint64 // highest LOCALLY ACCEPTED (bare-majority) height — drives VM state; NOT exportable
	QuasarHeight  uint64 // highest EXPORT-FINAL (⅔-by-stake) height — the ONLY certified/exportable height
	HorizonHeight uint64 // highest PQ-sealed height (0 until the Horizon seal path is wired)
	// ResponsiveStakePct is the stake that voted on the latest accepted block / total at its
	// epoch (0..1); -1 when unknown (startup / no stake model).
	ResponsiveStakePct float64
	// CertificateAvailable reports whether a ⅔-stake (Quasar) cert is currently reachable —
	// the responding stake STRICTLY exceeds the ⅔ floor.
	CertificateAvailable bool
	// Degraded reports that the chain is PRODUCING Nova but NOT certifying Quasar (responding
	// stake ≤ ⅔) — production continues, certification pauses. The two-tier liveness mode.
	Degraded bool
}

// FinalityStatus returns the current two-tier finality snapshot. novaHeight ≥ quasarHeight
// always; the gap plus the responsive-stake signal expose the degraded mode. A Nova height is
// never conflated with a certified/quasar height.
func (t *Transitive) FinalityStatus() FinalityStatus {
	var s FinalityStatus
	if nh, ok := t.consensus.GetFinalizedHeight(); ok {
		s.NovaHeight = nh
	}
	if qh, ok := t.consensus.QuasarHeight(); ok {
		s.QuasarHeight = qh
	}
	// HorizonHeight stays 0: the ladder MARKS Horizon (Finality.Horizon /
	// AuthorizesIrreversibleSettlement) but the PQ seal path is left as-is — no frontier
	// advances to Horizon yet.
	t.mu.RLock()
	num, den := t.responsiveStakeNum, t.responsiveStakeDen
	t.mu.RUnlock()
	if den == 0 {
		s.ResponsiveStakePct = -1 // unknown: startup or no stake model — do not false-alarm degraded
		return s
	}
	s.ResponsiveStakePct = float64(num) / float64(den)
	s.CertificateAvailable = num > config.TwoThirdsStakeFloor(den)
	s.Degraded = !s.CertificateAvailable
	return s
}

// QuasarTip returns the canonical execution commitment of the highest EXPORT-FINAL (Quasar)
// block — the export-gating tip for bridges / DEX settlement / cross-chain. ids.Empty until
// the first ⅔-stake cert. NEVER the Nova (accept) tip.
func (t *Transitive) QuasarTip() ids.ID { return t.consensus.GetQuasarTip() }

// NovaTip returns the canonical execution commitment of the highest LOCALLY ACCEPTED (Nova)
// block — the accepted/execution head. Local-execution authority ONLY; not exportable.
func (t *Transitive) NovaTip() ids.ID { return t.consensus.GetNovaTip() }

// QuasarHeight returns the highest export-final (Quasar) height and whether any ⅔-stake cert
// has formed. Complements GetFinalizedHeight (the Nova/accept height).
func (t *Transitive) QuasarHeight() (uint64, bool) { return t.consensus.QuasarHeight() }

// SyncQuasarFrontier conservatively (re)seeds the EXPORT (Quasar) frontier from a durable source
// — the node calls it on boot with the VM's persisted export-final block so GetQuasarTip /
// QuasarHeight do not regress on restart. Advance-only (never moves the frontier backward).
func (t *Transitive) SyncQuasarFrontier(canonical ids.ID, height uint64) {
	t.consensus.SyncQuasarFrontier(canonical, height)
}

// QuasarCertAt returns the EXPORT (Quasar, ⅔-by-stake) certificate for a height, if one has
// formed — the portable witness a bridge / DEX settlement (0x9999) / cross-chain consumer
// verifies. nil,false when no export cert exists at that height (Nova-only / degraded) or no
// attestor is wired. This is the ONLY cert an export consumer may act on; the Nova accept cert
// (gossiped for follower fast-finalize) is NOT export-grade.
func (t *Transitive) QuasarCertAt(height uint64) (*QuorumCert, bool) {
	t.mu.RLock()
	attestor := t.quasarAttestor
	t.mu.RUnlock()
	if attestor == nil {
		return nil, false
	}
	return attestor.CertAt(height)
}

// applyBranchFinalization applies a consensus FinalizeBranch plan to the VM and
// engine bookkeeping. It mirrors avalanchego topological.go's accept/reject split:
// VM.Accept the finalized path (child.Accept, ascending height) and VM.Reject the
// pruned losing-sibling subtrees (rejectTransitively). The certified tip carries the
// cert retained for serving catch-up peers. The engine maps (finalizedByCert,
// pendingBlocks, bufferedVotes, catchupRequested) are reconciled under t.mu; the VM
// Accept/Reject calls run OUTSIDE the lock, Accept-before-Reject (the avalanchego
// order: accept the preferred child, then reject the conflicting siblings).
//
// finalizedByCert is written ONLY here (via the sole finalizer acceptWithCertCore),
// so engine finality is exactly "FinalizeBranch committed this block", never the
// count-driven consensus liveness flag.
// It is FAIL-CLOSED (the 2026-07 fix): VM.Accept is called BEFORE the engine commits a
// block's finality, and its error is CHECKED, not swallowed. If VM.Accept returns an error
// (e.g. the EVM refuses to apply the block), that block and every higher one in the plan are
// NOT marked finalized and the returned highestAccepted stops at the last block the VM
// actually applied — so the caller advances the durable decided-floor ONLY to the VM's
// applied state and NEVER past it. This kills the divergence where a swallowed VM.Accept
// error let the consensus finalized floor run ahead of the EVM's accepted head (the
// consensus-finalize→VM-accept disconnect: floor at 427 while the EVM stuck at 415). Returns
// the highest height the VM accepted and the first VM.Accept error (nil ⇒ full success).
func (t *Transitive) applyBranchFinalization(ctx context.Context, plan Plan, certifiedTip ids.ID, cert VerifiedQuorumCert) (highestAccepted uint64, acceptErr error) {
	// Snapshot the finalized path (ascending) WITHOUT committing finality yet.
	type pathBlock struct {
		id     ids.ID
		vmb    block.Block
		height uint64
	}
	t.mu.RLock()
	path := make([]pathBlock, 0, len(plan.Accept))
	for _, id := range plan.Accept {
		pending, ok := t.pendingBlocks[id]
		if !ok || pending.Decided {
			// Untracked or already applied: treat as accepted for floor purposes (the ledger
			// is the truth) but nothing to Accept on the VM.
			var h uint64
			if ok && pending.ConsensusBlock != nil {
				h = pending.ConsensusBlock.height
			}
			path = append(path, pathBlock{id: id, vmb: nil, height: h})
			continue
		}
		var h uint64
		if pending.ConsensusBlock != nil {
			h = pending.ConsensusBlock.height
		}
		path = append(path, pathBlock{id: id, vmb: pending.VMBlock, height: h})
	}
	t.mu.RUnlock()

	// Accept ascending; commit each block's finality ONLY after the VM applied it. Stop (fail
	// closed) at the first VM.Accept error — the floor will advance only to the last success.
	for _, pb := range path {
		if pb.vmb != nil {
			if err := pb.vmb.Accept(ctx); err != nil {
				acceptErr = fmt.Errorf("VM.Accept(%s height=%d) failed — refusing to advance finality past the "+
					"VM's applied state (fail-closed): %w", pb.id, pb.height, err)
				break
			}
		}
		t.mu.Lock()
		if pending, ok := t.pendingBlocks[pb.id]; ok && !pending.Decided {
			t.finalizedByCert[pb.id] = struct{}{}
			pending.Decided = true
			t.blocksAccepted++
			t.dropPendingBlockLocked(pb.id)
			delete(t.bufferedVotes, pb.id)
			delete(t.catchupRequested, pb.id)
		} else {
			t.finalizedByCert[pb.id] = struct{}{}
		}
		t.mu.Unlock()
		if pb.height > highestAccepted {
			highestAccepted = pb.height
		}
	}

	// Reject the losing-sibling subtrees + retain the serving cert ONLY on a full, clean
	// accept. A partial accept (VM.Accept failed) leaves the reorg incomplete — do NOT prune
	// the losers (they may still be the live branch) and do NOT serve a cert for a tip the VM
	// never applied.
	if acceptErr != nil {
		return highestAccepted, acceptErr
	}
	var toReject []block.Block
	t.mu.Lock()
	for _, id := range plan.Reject {
		pending, ok := t.pendingBlocks[id]
		if !ok || pending.Decided {
			continue
		}
		pending.Decided = true
		t.blocksRejected++
		t.dropPendingBlockLocked(id)
		delete(t.bufferedVotes, id)
		delete(t.catchupRequested, id)
		if pending.VMBlock != nil {
			toReject = append(toReject, pending.VMBlock)
		}
	}
	if qc := cert.Cert(); qc != nil {
		if b, err := qc.MarshalBinary(); err == nil {
			t.storeServedCertLocked(certifiedTip, b)
		}
	}
	t.mu.Unlock()
	for _, vmb := range toReject {
		_ = vmb.Reject(ctx)
	}
	return highestAccepted, nil
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

// DrainAccepted attempts to finalize any pending block consensus has SIGNALLED
// as accepted. Called from the ForwardVMNotifications loop after each Notify.
//
// consensus.IsAccepted is the LIVENESS trigger only (α-of-K responded — worth
// attempting). The finality decision is made by TryAccept, which finalizes ONLY
// with a VerifiedQuorumCert (the >⅔-stake gate) and otherwise returns
// ErrNoVerifiedQC and changes nothing. This closes the previously count-ONLY
// finalize road here: a block drained from this loop now finalizes through the
// exact same cert-gated path (AcceptWithCert) as every other trigger — no count
// can VM.Accept without the cert.
func (t *Transitive) DrainAccepted(ctx context.Context) {
	t.mu.RLock()
	candidates := make([]ids.ID, 0, len(t.pendingBlocks))
	for id, pending := range t.pendingBlocks {
		if !pending.Decided && t.consensus.IsAccepted(id) {
			candidates = append(candidates, id)
		}
	}
	t.mu.RUnlock()

	for _, id := range candidates {
		_ = t.TryAccept(ctx, id)
	}
}

func (t *Transitive) buildBlocksLocked(ctx context.Context) error {
	if t.vm == nil {
		return nil
	}

	for {
		if t.pendingBuildBlocks <= 0 {
			break
		}

		// CONSUME the demand for THIS attempt BEFORE calling out (avalanchego's model:
		// snow/engine/snowman decrements pendingBuildBlocks before BuildBlock and treats a
		// failed build as consumed). The old code held the demand on error and returned, so
		// the very next Notify re-entered and re-called BuildBlock — and under single-proposer
		// scheduling a NON-leader's BuildBlock ALWAYS fails ("not our proposer slot") until its
		// window opens, so the held demand became an unbounded hot spin (a non-leader burned CPU
		// rebuilding a slot it can never win, 4 of 5 nodes, forever). Consuming it makes a failed
		// build cost exactly one attempt; the node re-attempts on a FRESH trigger — its slot
		// opening (proposervm WaitForEvent) or the elected leader's block advancing our tip.
		t.pendingBuildBlocks--

		// UNLOCK-BEFORE-CALL-OUT: BuildBlock is an external VM call — release t.mu around it, then
		// re-acquire to process (buildBlocksLocked is entered and returns with t.mu held). The loop
		// re-checks pendingBuildBlocks after re-lock, so a concurrent change is handled.
		t.mu.Unlock()
		vmBlock, err := t.vm.BuildBlock(ctx)
		t.mu.Lock()
		if err != nil {
			// Not a fault: most commonly "not this node's proposer slot" under
			// single-proposer scheduling — the normal state for every non-leader at
			// each height. This attempt is already consumed; CLEAR any remaining
			// queued demand too, because it would fail identically on this same tick
			// (same preference, same slot). Then wait for a FRESH trigger — the
			// node's slot opening (proposervm WaitForEvent) or the leader's block
			// advancing our tip — rather than re-calling BuildBlock every notify (the
			// old hot spin). Debug (not Error) so a healthy fleet stays quiet.
			t.pendingBuildBlocks = 0
			t.log.Debug("BuildBlock produced no block (likely not our proposer slot) — waiting for a fresh trigger",
				"error", err)
			return nil
		}

		t.blocksBuilt++

		consensusBlock := &Block{
			id:           vmBlock.ID(),
			parentID:     vmBlock.ParentID(),
			height:       vmBlock.Height(),
			timestamp:    vmBlock.Timestamp().Unix(),
			data:         vmBlock.Bytes(),
			pChainHeight: pChainHeightOf(vmBlock), // epoch for the weighted set (MEDIUM-1)
		}
		setCanonicalFromVM(consensusBlock, vmBlock) // stamp the inner execution commitment

		// If we already hold an undecided proposal for this slot, the VM just
		// re-wrapped it: drop the sibling and re-solicit the one stable candidate
		// so peer votes accumulate on one ID to α instead of scattering. A new
		// parent/height is a new key and builds normally. Re-solicit is K>1 only.
		if existing := t.pendingOwnProposals[t.proposalKeyOf(consensusBlock)]; existing != nil && !existing.Decided {
			// The VM re-wrapped an undecided slot. This is expected ONCE in a while; a sustained
			// stream means the slot is never being decided and the VM is spinning (rebuild storm).
			t.log.Info("rebuilt an undecided slot — re-soliciting votes",
				"newBlkID", vmBlock.ID(),
				"existingBlkID", existing.ConsensusBlock.id,
				"keyParentID", consensusBlock.parentID,
				"keyHeight", consensusBlock.height,
				"existingHeight", existing.ConsensusBlock.height,
				"K", t.consensus.K())
			reSolicit := t.proposer != nil && t.consensus.K() > 1
			reqBlockID, reqBlockData := existing.ConsensusBlock.id, existing.ConsensusBlock.data
			t.mu.Unlock()
			if reSolicit {
				t.proposer.RequestVotes(ctx, VoteRequest{BlockID: reqBlockID, BlockData: reqBlockData})
			}
			t.mu.Lock()
			continue
		}

		// Verify BEFORE consensus — prevents accepting invalid blocks in K=1 mode
		// where self-vote causes immediate acceptance. If Verify fails, the block
		// is never added to consensus, so IsAccepted cannot return true for it.
		t.mu.Unlock()
		if err := vmBlock.Verify(ctx); err != nil {
			// A block we built ourselves that fails its OWN verification is a real fault
			// (VM state, proposer/epoch rule, timestamp window...). Dropping it silently left
			// the node in an INVISIBLE HOT LOOP: the VM keeps re-notifying while its txs sit in
			// the mempool, so the engine rebuilds → drops → rebuilds forever — no block ever
			// enters consensus and NOT ONE log line says why (observed: 4.07M consecutive
			// "built block" lines, chain stuck at height 1, zero errors logged). The drop itself
			// is correct (an unverifiable block must never reach consensus); the SILENCE is the
			// bug. Log the fault so it is diagnosable at the point of failure.
			t.log.Error("built block failed verification — dropping",
				"blkID", vmBlock.ID(),
				"height", vmBlock.Height(),
				"parentID", vmBlock.ParentID(),
				"error", err)
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
			// Same silent-drop class as the Verify failure above. consensus.AddBlock's only
			// rejection is "block already exists" — which a self-built block hits whenever the
			// VM re-mints an IDENTICAL envelope (the proposervm block timestamp is truncated to
			// a whole second, so every rebuild inside that second yields the SAME blkID). The
			// bare continue then skipped Propose/RequestVotes entirely: the block was never sent
			// to peers, never voted on, never decided — while the VM kept re-notifying because
			// its txs were still pending. Result: a silent rebuild storm with the chain pinned at
			// its height and no log line naming the cause. Log it at the point of the drop.
			t.log.Error("built block rejected by consensus — dropping",
				"blkID", vmBlock.ID(),
				"height", vmBlock.Height(),
				"parentID", vmBlock.ParentID(),
				"error", addErr)
			continue
		}

		// Track the block as a pending own proposal — ALWAYS, including the K=1
		// single-node case. We no longer VM.Accept inline here: finalization for
		// every block (K=1 or K>1) goes through the SOLE cert-gated finalizer via
		// TryAccept below, so there is exactly one acceptance road. In K=1 TryAccept
		// commits the 1-of-1 quorum (ForceAccept) and finalizes through
		// AcceptWithCert; the call is synchronous (see below) so it runs before the
		// next BuildBlock.
		//
		// Registered in BOTH indices via the ONE writer: pendingBlocks[outerID]
		// for transport lookup + pendingOwnProposals[ProposalKey] for consensus
		// identity. The reuse branch above already returned for a re-wrapped slot,
		// so this path is a genuinely new proposal.
		pb, _ := t.registerOrReuseOwnProposalLocked(consensusBlock, vmBlock)
		// VOTE EMISSION.
		//
		// K==1 (sole validator, no siblings ever): the proposer's own accept IS the
		// 1-of-1 quorum — record its signed self-vote now so the 1-of-1 cert assembles.
		//
		// K>1: DO NOT bind this node's one per-height signature to its own freshly-built
		// block. On a fresh-net storm many validators build competing siblings at one
		// height; if each self-votes its OWN block at build, all 5 lock to distinct
		// blocks, the α-of-K vote splits 5 ways, and NOTHING finalizes (the net-wide
		// stall). Instead the vote is emitted for the DETERMINISTICALLY CONVERGED winner
		// at this height (convergenceVoter) — which may be this node's block or a peer's
		// lower-canonical sibling — so every honest node signs the SAME block. The
		// confidence driver still counts the own proposal (ProcessVote above); only the
		// binding SIGNATURE is deferred to convergence.
		//
		// Track the live validator set FIRST: re-clamp the committee UP so a chain that launched
		// single-validator switches to the real k-of-N quorum path the moment it decentralizes,
		// rather than continuing to self-vote+synthesize a 1-of-1 cert (RED's 1→N fork).
		t.reclampCommitteeLocked()
		if t.consensus.K() == 1 {
			t.recordOwnVoteLocked(pb, vmBlock.ID())
		}

		// UNLOCK-BEFORE-CALL-OUT (the global invariant — see the Transitive.mu doc). Capture
		// everything the call-outs need UNDER t.mu, then RELEASE it and do EVERY call-out — the
		// network Propose/RequestVotes, VM.SetPreference, and the finalizer — with t.mu NOT held. A
		// call-out that reenters t.mu self-deadlocks (the removed selfVoter class); network sends
		// must also never sit under the engine lock. buildSingleValidatorCertLocked reads
		// pendingBlocks/position so its cert is built here (locked); its finalize runs unlocked.
		proposerWired := t.proposer != nil
		blockID := vmBlock.ID()
		singleValidator := t.consensus.K() == 1
		var proposal BlockProposal
		if proposerWired {
			proposal = BlockProposal{
				BlockID:   blockID,
				BlockData: vmBlock.Bytes(),
				Height:    vmBlock.Height(),
				ParentID:  vmBlock.ParentID(),
			}
		}
		var singleCert VerifiedQuorumCert
		if singleValidator {
			singleCert = t.buildSingleValidatorCertLocked(pb, blockID)
		}
		vm := t.vm
		t.mu.Unlock()

		// ---- ALL CALL-OUTS, t.mu RELEASED ----
		if proposerWired {
			// Gossip the block so non-validating FOLLOWERS receive it. Solicit peer votes ONLY on a
			// multi-validator chain — a K==1 chain has no peers and finalizes via the inline
			// finalizer below (the removed single-node self-vote shortcut was the t.mu-reentrant
			// deadlock).
			t.proposer.Propose(ctx, proposal)
			if !singleValidator {
				t.proposer.RequestVotes(ctx, VoteRequest{BlockID: blockID, BlockData: proposal.BlockData})
			}
		}
		if singleValidator {
			// The sole validator's accept IS the 1-of-1 quorum — finalize inline through the sole
			// cert-gated finalizer. signalNext=false: the build loop drives the next build itself
			// (re-signaling would spawn a concurrent builder and gap the VM block counter).
			_ = t.acceptWithCertCore(ctx, blockID, singleCert, false)
		} else if proposerWired {
			// K>1: STORM BOUND — steer the VM build target to the just-built tip so proposervm's
			// WaitForEvent advances to the NEXT height instead of re-returning "build THIS height"
			// (the mainnet 511-rebuild spin). A build hint only; finality is still the α-of-K cert.
			// This node does NOT self-vote its own block here (the convergence loop casts the single
			// vote for the settled winner); finalize only if a verified cert is already assemblable.
			// Same single-store invariant as the finalize steer: never name a tip this VM
			// does not hold. There is no better fallback here (blockID is the block we just
			// built, which IS the tip on the healthy path), so an unheld tip means skip —
			// the VM keeps its prior held preference and a later steer advances it.
			if tip := t.HeldBuildTip(ctx, vm, ids.Empty); tip != ids.Empty {
				_ = vm.SetPreference(ctx, tip)
			}
			t.finalizeOwnProposal(ctx, blockID)
		}
		t.mu.Lock()
	}
	return nil
}

// ProposalKey is a proposal's consensus position — the (parent, height) fork
// slot, not the proposervm envelope ID (which the VM re-mints per rebuild).
// Every rebuild of "my block at (P,H)" shares one key, so identity churn can't
// split votes.
type ProposalKey struct {
	ParentID ids.ID
	Height   uint64
}

func (t *Transitive) proposalKeyOf(b *Block) ProposalKey {
	return ProposalKey{ParentID: b.parentID, Height: b.height}
}

// registerOrReuseOwnProposalLocked returns the existing undecided proposal for
// this slot (reused=true), else registers b in both indices (reused=false).
// The sole own-proposal reuse decision. Holds t.mu.
func (t *Transitive) registerOrReuseOwnProposalLocked(consensusBlock *Block, vmBlock block.Block) (*PendingBlock, bool) {
	key := t.proposalKeyOf(consensusBlock)
	if existing := t.pendingOwnProposals[key]; existing != nil && !existing.Decided {
		return existing, true
	}
	pb := &PendingBlock{
		ConsensusBlock: consensusBlock,
		VMBlock:        vmBlock,
		ProposedAt:     time.Now(),
		VoteCount:      1,
		IsOwnProposal:  true,
	}
	t.pendingBlocks[consensusBlock.id] = pb
	t.pendingOwnProposals[key] = pb
	return pb, false
}

// dropPendingBlockLocked removes a block from both indices — the sole unwrite,
// so they never drift. The identity check keeps a same-slot sibling from
// evicting the live owner. Holds t.mu.
func (t *Transitive) dropPendingBlockLocked(id ids.ID) {
	pb, ok := t.pendingBlocks[id]
	if !ok {
		return
	}
	if pb.IsOwnProposal && pb.ConsensusBlock != nil {
		key := t.proposalKeyOf(pb.ConsensusBlock)
		if t.pendingOwnProposals[key] == pb {
			delete(t.pendingOwnProposals, key)
		}
	}
	delete(t.pendingBlocks, id)
}

// -----------------------------------------------------------------------------
// Transport (network layer interface)
// -----------------------------------------------------------------------------

// Transport handles message transport.
type Transport[ID comparable] interface {
	Send(ctx context.Context, to string, msg interface{}) error
	Receive(ctx context.Context) (interface{}, error)
}
