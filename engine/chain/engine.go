package chain

import (
	"context"
	"errors"
	"fmt"
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
	mu sync.RWMutex

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

	// Block management
	pendingBlocks      map[ids.ID]*PendingBlock
	pendingBuildBlocks int

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
	voteGuard     VoteGuardStore

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
		consensus:        NewChainConsensus(cfg.Params.K, cfg.Params.AlphaPreference, int(cfg.Params.Beta)),
		params:           cfg.Params,
		vm:               cfg.VM,
		proposer:         cfg.Proposer,
		bootstrapPhase:   true, // a fresh engine is initial-syncing until it reaches the frontier
		pendingBlocks:    make(map[ids.ID]*PendingBlock),
		finalizedByCert:  make(map[ids.ID]struct{}),
		certBytesByBlock: make(map[ids.ID][]byte),
		committedSlot:    make(map[SlotKey]ids.ID),
		catchupRequested: make(map[ids.ID]time.Time),
		bufferedVotes:    make(map[ids.ID][]Vote),
		voteRequests:     make(chan VoteRequest, cfg.VoteRequestBuffer),
		votes:            make(chan Vote, cfg.VoteBuffer),
		pipelineSignal:   make(chan struct{}, 1),
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

	// A signing validator SHOULD have a durable equivocation guard so a crash between
	// signing and finalizing cannot forget a per-height binding and permit a fork
	// (HIGH-1). Memory-only is correct for verify-only nodes and tests; in production
	// the node wires WithVoteGuard. Warn (don't fail) so tests/dev keep working while
	// the gap is visible.
	if t.voteSigner != nil && t.voteGuard == nil {
		t.log.Warn("vote-once: signing WITHOUT a durable equivocation guard — a crash between " +
			"signing and finalizing may permit equivocation; wire WithVoteGuard in production")
	}

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

	// Clear any pending blocks that are now stale (below the synced height)
	for blockID, pending := range t.pendingBlocks {
		if pending.ConsensusBlock != nil && pending.ConsensusBlock.height <= height {
			delete(t.pendingBlocks, blockID)
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
		// only quiesce at NumProcessing()==0 (snow/engine/snowman voter.go +
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
		delete(t.pendingBlocks, action.blockID)
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
		// A vote for an ALREADY-FINALIZED block needs nothing: the block is decided
		// (removed from pendingBlocks, recorded in finalizedByCert). Do NOT buffer or
		// fetch it — that would re-park a late/duplicate vote AFTER acceptWithCertCore
		// cleared the buffer, leaking a slot for a block that will never re-track to
		// drain it. Drop it (the finality cert already exists).
		if _, finalized := t.finalizedByCert[vote.BlockID]; finalized {
			t.mu.Unlock()
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
	key := SlotKey{Height: height}
	t.slotMu.Lock()
	defer t.slotMu.Unlock()
	if bound, ok := t.committedSlot[key]; ok {
		return bound == canonical // same block ⇒ idempotent (already durable); sibling ⇒ refuse
	}
	// First binding at this slot. PERSIST it durably BEFORE permitting the signature —
	// a crash after signing but before finalizing must not forget it. Mutate the map,
	// snapshot it to stable storage, and ROLL BACK on failure so in-memory state stays
	// consistent with what is durable and we FAIL CLOSED (return false ⇒ no signature).
	t.committedSlot[key] = canonical
	if t.voteGuard != nil {
		if err := t.voteGuard.Persist(t.committedSlot); err != nil {
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

// pruneCommittedSlotsBelow drops equivocation-guard entries at or below a
// finalized height (across ALL epochs at those heights) — those heights are decided
// and can never legitimately be re-signed, so their guard is dead weight. Keeps
// committedSlot (and its durable snapshot) bounded to the live unfinalized window.
// Called from the sole finalizer acceptWithCertCore, so EVERY finality path — local
// vote-assembly AND incoming-cert — prunes (MEDIUM-1). The durable shrink is
// best-effort: a stale-LARGER durable set only ever REFUSES more (fail-safe
// direction), and it is corrected on the next successful Persist.
func (t *Transitive) pruneCommittedSlotsBelow(height uint64) {
	t.slotMu.Lock()
	defer t.slotMu.Unlock()
	changed := false
	for k := range t.committedSlot {
		if k.Height <= height {
			delete(t.committedSlot, k)
			changed = true
		}
	}
	if changed && t.voteGuard != nil {
		if err := t.voteGuard.Persist(t.committedSlot); err != nil {
			t.log.Warn("vote-once: durable equivocation-guard shrink failed (non-fatal; corrected on next bind)",
				"belowHeight", height, "error", err)
		}
	}
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
func (t *Transitive) convergedWinnerAtHeightLocked(height uint64, parentID ids.ID) (ids.ID, int, bool) {
	var winner, winnerCanon ids.ID
	count := 0
	for id, pb := range t.pendingBlocks {
		cb := pb.ConsensusBlock
		if cb == nil || pb.Decided || pb.rePollAbandoned {
			continue
		}
		if cb.height != height || cb.parentID != parentID {
			continue
		}
		canon := cb.canonicalID
		if canon == ids.Empty {
			canon = id
		}
		count++
		if winner == ids.Empty || canon.Compare(winnerCanon) < 0 {
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
	pc := p.canonicalID
	if pc == ids.Empty {
		pc = parentID
	}
	for sibID, sib := range t.pendingBlocks {
		cb := sib.ConsensusBlock
		if cb == nil || sib.rePollAbandoned || sibID == parentID {
			continue
		}
		if cb.height != p.height || cb.parentID != p.parentID {
			continue
		}
		sc := cb.canonicalID
		if sc == ids.Empty {
			sc = sibID
		}
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
	t.slotMu.Lock()
	signed := make(map[uint64]struct{}, len(t.committedSlot))
	for k := range t.committedSlot {
		signed[k.Height] = struct{}{}
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
		if _, ok := signed[cb.height]; ok {
			continue // already cast our one vote at this height
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
func (t *Transitive) buildSingleValidatorCertLocked(pending *PendingBlock, blockID ids.ID) VerifiedQuorumCert {
	// Prefer the VERIFIED 1-of-1 cert: when vote crypto is wired the proposer
	// recorded its own signed accept (recordOwnVoteLocked), so assembleCertLocked
	// verifies that single signature (and the trivially-met stake gate) and we
	// finalize on a real witness — even on a single-validator chain.
	if cert, ok := t.assembleVerifiedCertLocked(pending, blockID); ok {
		return cert
	}
	// Only when NO vote crypto is wired (a pure single-node dev chain, voteVerifier
	// nil) is there no signature to certify. Synthesize the 1-of-1 finality witness
	// from the position; FinalizeBranch (inside the finalizer this token authorizes)
	// is the real single-node safety gate — one block per height, contiguous, no
	// branching. This branch is K==1-only (both callers gate on K()==1) and
	// verifier-nil-only, so it can never substitute for a real α-of-K cert on any
	// multi-validator chain.
	if t.voteVerifier != nil {
		// Verifier wired but the self-vote did not assemble (should not happen on a
		// healthy K==1 node). Do NOT fabricate a witness when crypto is available —
		// return zero so AcceptWithCert refuses and the next trigger retries.
		return VerifiedQuorumCert{}
	}
	pos := t.blockPositionLocked(pending, blockID)
	return VerifiedQuorumCert{qc: &QuorumCert{
		Version:   QuorumCertVersion,
		Type:      QCFinality,
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
	plan, err := t.consensus.ApplyCert(Cert{Block: pos.BlockID, Parent: pos.ParentID, Height: pos.Height, Canonical: pos.CanonicalID})
	if err != nil {
		return err
	}

	// Apply the plan to the VM + engine bookkeeping: VM.Accept the finalized path,
	// VM.Reject the pruned losers, record engine finality, store the serving cert.
	t.applyBranchFinalization(ctx, plan, blockID, cert)

	// REORG production onto the certified branch: SetPreference to the new tip keeps
	// the VM building on the block consensus just finalized.
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

	// Pipeline: block finalized → immediately build next (out-of-band callers
	// only; the in-build-loop caller continues its own loop and passes false).
	if signalNext {
		t.signalPipeline()
	}

	// MED-6: bound the slashing detector's per-height vote/block maps to a
	// sliding window below the finalized height. Equivocation is only
	// actionable near the tip; retaining every height's records unboundedly is a
	// memory-exhaustion vector. Prune everything older than the window.
	t.pruneSlashingBelowWindow()
	// MEDIUM-1: drop the equivocation guard for the height just finalized. This is
	// the ONE funnel EVERY finality path passes through — local vote-assembly
	// (handleVote → tryFinalizeBlock) AND incoming-cert (HandleIncomingCert →
	// AcceptWithCert) — so committedSlot stays bounded to the live unfinalized window
	// on every validator (a locally-finalizing node never reaches HandleIncomingCert's
	// prune). pos.Height is the finalized tip; every slot at or below it is decided.
	t.pruneCommittedSlotsBelow(pos.Height)
	return nil
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
func (t *Transitive) applyBranchFinalization(ctx context.Context, plan Plan, certifiedTip ids.ID, cert VerifiedQuorumCert) {
	var toAccept, toReject []block.Block

	t.mu.Lock()
	// ACCEPT the finalized path (ascending height — plan.Accept is ordered).
	for _, id := range plan.Accept {
		t.finalizedByCert[id] = struct{}{}
		pending, ok := t.pendingBlocks[id]
		if !ok || pending.Decided {
			continue // not locally tracked, or already applied — the ledger is the truth
		}
		pending.Decided = true
		t.blocksAccepted++
		delete(t.pendingBlocks, id)
		delete(t.bufferedVotes, id)
		delete(t.catchupRequested, id)
		if pending.VMBlock != nil {
			toAccept = append(toAccept, pending.VMBlock)
		}
	}
	// PRUNE the losing-sibling subtrees: drop from tracking and reject. THIS is the
	// reorg the old engine never performed — without it, production ran away on a
	// losing branch and every cert for it was permanently refused.
	for _, id := range plan.Reject {
		pending, ok := t.pendingBlocks[id]
		if !ok || pending.Decided {
			continue
		}
		pending.Decided = true
		t.blocksRejected++
		delete(t.pendingBlocks, id)
		delete(t.bufferedVotes, id)
		delete(t.catchupRequested, id)
		if pending.VMBlock != nil {
			toReject = append(toReject, pending.VMBlock)
		}
	}
	// Retain the cert that authorized this finalize so a catching-up peer can fetch it.
	if qc := cert.Cert(); qc != nil {
		if b, err := qc.MarshalBinary(); err == nil {
			t.storeServedCertLocked(certifiedTip, b)
		}
	}
	t.mu.Unlock()

	// VM effects OUTSIDE the lock. Accept the path first (ascending), then reject the
	// losers — avalanchego's acceptPreferredChild order.
	for _, vmb := range toAccept {
		_ = vmb.Accept(ctx)
	}
	for _, vmb := range toReject {
		_ = vmb.Reject(ctx)
	}
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

		// A rebuild of an ALREADY-TRACKED block is our own UNDECIDED proposal that
		// the VM re-offered (mempool still non-empty, this height not yet finalized).
		// avalanchego never silently drops such a rebuild — its repoll keeps
		// re-querying the still-processing preferred block until it decides (snowman
		// engine.go repoll, quiescing only at Consensus.NumProcessing()==0). Mirror
		// that on the build path: RE-SOLICIT the block's votes instead of dropping the
		// signal, so a peer that missed the first PushQuery is re-asked IMMEDIATELY
		// (not only on the slower rePollAllPending backoff — the zero-margin 4-of-5
		// mainnet condition needs the prompt re-ask). This re-sends the SAME block and
		// the SAME position: it can never manufacture a vote or change WHICH block
		// finalizes (finality is still the α-of-K cert), so it is a pure liveness
		// retry. OWN proposals only — a gossiped (non-own) block keeps the rePoll
		// attempt cap, since re-soliciting a block whose voters are behind its parent
		// is spam that never recovers it (see rePollAllPending).
		if pb, exists := t.pendingBlocks[vmBlock.ID()]; exists {
			if pb.IsOwnProposal && !pb.Decided && t.proposer != nil {
				t.proposer.RequestVotes(ctx, VoteRequest{
					BlockID:   vmBlock.ID(),
					BlockData: vmBlock.Bytes(),
				})
			}
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
		setCanonicalFromVM(consensusBlock, vmBlock) // stamp the inner execution commitment

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

		// Track the block as a pending own proposal — ALWAYS, including the K=1
		// single-node case. We no longer VM.Accept inline here: finalization for
		// every block (K=1 or K>1) goes through the SOLE cert-gated finalizer via
		// TryAccept below, so there is exactly one acceptance road. In K=1 TryAccept
		// commits the 1-of-1 quorum (ForceAccept) and finalizes through
		// AcceptWithCert; the call is synchronous (see below) so it runs before the
		// next BuildBlock.
		pb := &PendingBlock{
			ConsensusBlock: consensusBlock,
			VMBlock:        vmBlock,
			ProposedAt:     time.Now(),
			VoteCount:      1,
			Decided:        false,
			IsOwnProposal:  true,
		}
		t.pendingBlocks[vmBlock.ID()] = pb
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
		if t.consensus.K() == 1 {
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

		// Self-finalize the just-built own block through the SOLE cert-gated
		// finalizer (AcceptWithCert). The block was verified locally above, so the
		// proposer has committed to its correctness; waiting only on peer Chits to
		// drive finality causes the lux-devnet stall when Chits arrive late.
		blockID := vmBlock.ID()
		singleValidator := t.consensus.K() == 1

		if singleValidator {
			// K==1: the sole validator's accept IS the 1-of-1 quorum. Build the 1-of-1
			// cert and finalize INLINE through the sole finalizer (FinalizeBranch).
			// signalNext=false: we are inside the build loop, which drives the next
			// build itself — re-signaling would spawn a concurrent builder and gap the
			// VM block counter (the K=1 burst-throughput stall).
			//
			// The old code REQUIRED this to run in lockstep with SetPreference because
			// the per-height ADMISSION gate refused any block whose parent != finalized
			// tip (ErrParentNotFinalizedTip) — so a build that outran SetPreference
			// stalled. That gate is GONE: FinalizeBranch reorgs instead of refusing, so
			// finalizing here is now purely the K==1 finalize trigger, not a lockstep
			// safety requirement.
			cert := t.buildSingleValidatorCertLocked(pb, blockID)
			t.mu.Unlock()
			_ = t.acceptWithCertCore(ctx, blockID, cert, false)
			t.mu.Lock()
		} else if proposerWired {
			// K>1 with a network proposer: attempt finality now via the verified
			// α-of-K cert. The cert may already be assemblable from collected votes;
			// if not, TryAccept no-ops (ErrNoVerifiedQC) and the poll loop / cert
			// gossip retry. NEVER forces a K>1 block on the lone self-vote.
			t.mu.Unlock()
			// STORM BOUND (avalanchego deliver()→SetPreference, the SAME steer
			// followVerifiedBlock applies on the receive side): advance the VM's build
			// target to the just-built tip so the proposervm's WaitForEvent moves to the
			// NEXT height instead of re-returning "build THIS height" every time the
			// mempool is non-empty and the block has not yet finalized. Without it the
			// node rebuilt one height hundreds of times while awaiting votes (the mainnet
			// 511-rebuild-in-4-min spin). This is a build hint only — it never changes
			// WHICH block finalizes or WHEN; finality is still the α-of-K cert.
			if t.vm != nil {
				if tip := t.PreferredBuildTip(); tip != ids.Empty {
					_ = t.vm.SetPreference(ctx, tip)
				}
			}
			// This node's accept vote for its OWN block is NOT cast here. On a fresh-net
			// storm many validators build competing siblings at one height; binding this
			// node's one signature to its own block at build time is exactly the 5-way
			// split that stalls the quorum. The convergence loop casts this node's single
			// vote for the settled, converged winner (which may be this block or a peer's
			// lower-canonical sibling). Finalization is still attempted below in case a
			// verified α-of-K cert is already assemblable from earlier heights.
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
