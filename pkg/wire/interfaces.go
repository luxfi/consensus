// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wire

import (
	"context"
)

// =============================================================================
// THE 4-LAYER SEQUENCER STACK
// =============================================================================
//
// 1) Execution Payload - what gets executed/committed
// 2) Ordering         - who proposes, how conflicts resolve
// 3) Data Availability - where candidate bytes live
// 4) Finality         - what proof is required
//
// By freezing these interfaces, we support everything from K=1 to millions
// by simply selecting implementations.
// =============================================================================

// =============================================================================
// LAYER 1: MEMBERSHIP - Who can attest
// =============================================================================

// Validator represents a participant in consensus
type Validator struct {
	// ID is the voter identifier
	ID VoterID `json:"id"`

	// Weight is the stake/voting power (1 for unweighted)
	Weight uint64 `json:"weight"`

	// PublicKey for signature verification (scheme-tagged)
	PublicKey []byte `json:"public_key,omitempty"`

	// TransportAddr for network communication
	TransportAddr string `json:"transport_addr,omitempty"`
}

// ValidatorSet is the set of validators for an epoch
type ValidatorSet struct {
	// Epoch identifier
	Epoch uint64 `json:"epoch"`

	// Validators in this epoch
	Validators []Validator `json:"validators"`

	// TotalWeight is sum of all weights
	TotalWeight uint64 `json:"total_weight"`
}

// Membership provides validator set information
type Membership interface {
	// ValidatorSet returns validators for an epoch
	// For K=1: returns singleton
	// For millions: returns stake-weighted / committee-sampled set
	ValidatorSet(ctx context.Context, epoch uint64) (*ValidatorSet, error)

	// IsValidator checks if a voter is in the current set
	IsValidator(ctx context.Context, voterID VoterID) (bool, error)

	// SampleCommittee samples k validators (for large N)
	// Returns full set if len(validators) <= k
	SampleCommittee(ctx context.Context, epoch uint64, k int, seed []byte) ([]Validator, error)
}

// =============================================================================
// LAYER 2: ORDERING - Who proposes next candidate
// =============================================================================

// Proposer handles candidate creation and conflict resolution
type Proposer interface {
	// Propose creates the next candidate
	// For blocks: BuildBlock
	// For AI: aggregate agent decisions
	Propose(ctx context.Context) (*Candidate, error)

	// Observe notifies proposer about a candidate from elsewhere
	// Used for conflict resolution when multiple proposers exist
	Observe(ctx context.Context, candidateID CandidateID, daRef string) error

	// IsLeader checks if this node should propose for given round
	// For K=1: always true
	// For K=N: round-robin, VRF, or stake-weighted election
	IsLeader(ctx context.Context, round uint64) (bool, error)
}

// ProposerElection determines who proposes
type ProposerElection interface {
	// Leader returns the leader for a given round
	Leader(ctx context.Context, round uint64, validators *ValidatorSet) (VoterID, error)
}

// =============================================================================
// LAYER 3: DATA AVAILABILITY - Where candidate bytes live
// =============================================================================

// DARef is a reference to where candidate data is stored
type DARef struct {
	// Type identifies the DA layer (local, ipfs, blob, warp, etc.)
	Type string `json:"type"`

	// Ref is the type-specific reference (path, CID, hash, etc.)
	Ref string `json:"ref"`

	// Size is the payload size in bytes
	Size uint64 `json:"size,omitempty"`
}

// DA layer types
const (
	DATypeLocal = "local" // Local disk
	DATypeIPFS  = "ipfs"  // IPFS CID
	DATypeBlob  = "blob"  // EIP-4844 blob
	DATypeWarp  = "warp"  // Lux Warp message
	DATypeP2P   = "p2p"   // P2P gossip
	DATypeMCP   = "mcp"   // MCP mesh storage
)

// DataAvailability handles candidate data storage and retrieval
type DataAvailability interface {
	// Store saves candidate payload and returns reference
	Store(ctx context.Context, candidate *Candidate) (*DARef, error)

	// Retrieve fetches candidate payload from reference
	Retrieve(ctx context.Context, ref *DARef) ([]byte, error)

	// Verify checks that data at ref matches expected hash
	Verify(ctx context.Context, ref *DARef, expectedHash CandidateID) (bool, error)
}

// =============================================================================
// LAYER 4: FINALITY - What proof is required
// =============================================================================

// FinalityPolicy defines when a candidate is considered final
type FinalityPolicy interface {
	// PolicyID returns the policy identifier
	PolicyID() PolicyID

	// OnCandidate handles a new candidate observation
	OnCandidate(ctx context.Context, candidate *Candidate) error

	// OnVote handles a vote for a candidate
	OnVote(ctx context.Context, vote *Vote) error

	// MaybeFinalize checks if candidate can be finalized
	// Returns certificate if finalized, nil otherwise
	MaybeFinalize(ctx context.Context, candidateID CandidateID) (*Certificate, error)

	// Verify checks if a certificate is valid
	Verify(ctx context.Context, cert *Certificate) (bool, error)
}

// =============================================================================
// TRANSPORT - How votes move
// =============================================================================

// Request is a query to peers
type Request struct {
	// Type identifies the request (vote_request, candidate_request, etc.)
	Type string `json:"type"`

	// CandidateID for vote requests
	CandidateID CandidateID `json:"candidate_id,omitempty"`

	// Round for round-based protocols
	Round uint64 `json:"round,omitempty"`

	// Data is request-specific payload
	Data []byte `json:"data,omitempty"`
}

// Response from a peer
type Response struct {
	// From identifies the responder
	From VoterID `json:"from"`

	// Type matches request type
	Type string `json:"type"`

	// Vote if this is a vote response
	Vote *Vote `json:"vote,omitempty"`

	// Candidate if this is a candidate response
	Candidate *Candidate `json:"candidate,omitempty"`

	// Error if request failed
	Error string `json:"error,omitempty"`
}

// Transport handles peer communication
// Supports: local function call, TCP, QUIC, libp2p, MCP mesh, RPC
type Transport interface {
	// Query sends request to peers and collects responses
	Query(ctx context.Context, peers []VoterID, request *Request) <-chan *Response

	// Broadcast sends to all known peers
	Broadcast(ctx context.Context, request *Request) error

	// Send sends to a specific peer
	Send(ctx context.Context, peer VoterID, request *Request) (*Response, error)
}

// =============================================================================
// SEQUENCER - The unified pipeline
// =============================================================================

// Sequencer combines all layers into a single pipeline
type Sequencer interface {
	// Components
	Membership() Membership
	Proposer() Proposer
	DA() DataAvailability
	Finality() FinalityPolicy
	Transport() Transport

	// Lifecycle
	Start(ctx context.Context) error
	Stop(ctx context.Context) error

	// Operations
	Submit(ctx context.Context, payload []byte) (*Candidate, error)
	GetCandidate(ctx context.Context, id CandidateID) (*Candidate, error)
	GetCertificate(ctx context.Context, id CandidateID) (*Certificate, error)

	// State
	Head(ctx context.Context) (*Candidate, error)
	Height(ctx context.Context) (uint64, error)
	IsSoftFinalized(ctx context.Context, id CandidateID) (bool, error)
	IsHardFinalized(ctx context.Context, id CandidateID) (bool, error)
}

// =============================================================================
// CONFIGURATION
// =============================================================================

// SequencerConfig configures the sequencer pipeline
type SequencerConfig struct {
	// Domain identifier for this sequencer
	Domain []byte `json:"domain"`

	// K is the sample/committee size
	K int `json:"k"`

	// Consensus parameters
	Alpha float64 `json:"alpha"`  // Agreement threshold
	Beta1 float64 `json:"beta_1"` // Soft finality threshold
	Beta2 float64 `json:"beta_2"` // Hard finality threshold

	// Policy selection
	SoftPolicy PolicyID `json:"soft_policy"`
	HardPolicy PolicyID `json:"hard_policy"`

	// Timeouts
	RoundTimeoutMs    int64 `json:"round_timeout_ms"`
	FinalityTimeoutMs int64 `json:"finality_timeout_ms"`
}

// Preset configurations

// SingleNodeConfig returns config for K=1 self-sequencing
func SingleNodeConfig(domain []byte) SequencerConfig {
	return SequencerConfig{
		Domain:            domain,
		K:                 1,
		Alpha:             1.0,
		Beta1:             1.0,
		Beta2:             1.0,
		SoftPolicy:        PolicyNone,
		HardPolicy:        PolicyNone,
		RoundTimeoutMs:    100,
		FinalityTimeoutMs: 100,
	}
}

// AgentMeshConfig returns config for K=3/5 agent mesh
func AgentMeshConfig(domain []byte, k int) SequencerConfig {
	return SequencerConfig{
		Domain:            domain,
		K:                 k,
		Alpha:             0.6,
		Beta1:             0.5,
		Beta2:             0.8,
		SoftPolicy:        PolicyQuorum,
		HardPolicy:        PolicyQuorum,
		RoundTimeoutMs:    5000,
		FinalityTimeoutMs: 30000,
	}
}

// BlockchainConfig returns config for large permissionless network
func BlockchainConfig(domain []byte) SequencerConfig {
	return SequencerConfig{
		Domain:            domain,
		K:                 20,
		Alpha:             0.65,
		Beta1:             0.5,
		Beta2:             0.8,
		SoftPolicy:        PolicySampleConvergence,
		HardPolicy:        PolicyQuantum,
		RoundTimeoutMs:    1000,
		FinalityTimeoutMs: 60000,
	}
}

// RollupConfig returns config for OP Stack style rollup
func RollupConfig(domain []byte) SequencerConfig {
	return SequencerConfig{
		Domain:            domain,
		K:                 1,
		Alpha:             1.0,
		Beta1:             1.0,
		Beta2:             1.0,
		SoftPolicy:        PolicyNone,        // Sequencer head is soft
		HardPolicy:        PolicyL1Inclusion, // L1 is hard
		RoundTimeoutMs:    2000,
		FinalityTimeoutMs: 600000, // 10 minutes for L1 + challenge
	}
}
