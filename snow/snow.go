package snow

import (
	"context"
	"time"

	consensuscontext "github.com/luxfi/consensus/context"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// Consensus defines the Snow consensus interface
type Consensus interface {
	// Initialize consensus
	Initialize(context.Context, *Config) error

	// IsBootstrapped checks if consensus is bootstrapped
	IsBootstrapped() bool

	// Parameters returns consensus parameters
	Parameters() Parameters
}

// Config defines consensus configuration
type Config struct {
	Alpha             int
	BetaVirtuous      int
	BetaRogue         int
	Parents           int
	ConcurrentRepolls int
}

// Parameters defines consensus parameters
type Parameters struct {
	K                 int
	Alpha             int
	BetaVirtuous      int
	BetaRogue         int
	ConcurrentRepolls int
}

// Engine defines a consensus engine
type Engine interface {
	Consensus

	// Start starts the engine
	Start(context.Context, uint32) error

	// Stop stops the engine
	Stop(context.Context) error
}

// Context provides Snow consensus context
type Context struct {
	ConsensusContext
	Log        log.Logger
	Lock       interface{} // sync.RWMutex in actual use
	Registerer interface{} // prometheus.Registerer
	StartTime  time.Time

	// Network and chain IDs
	NetworkID uint32
	SubnetID  ids.ID
	ChainID   ids.ID
	NodeID    ids.NodeID
	PublicKey []byte

	// Chain IDs
	XChainID    ids.ID
	CChainID    ids.ID
	AVAXAssetID ids.ID

	// State management
	ValidatorState consensuscontext.ValidatorState
	Keystore       interface{}
	BCLookup       interface{}
	Metrics        interface{}
}

// ConsensusContext provides consensus-specific context
type ConsensusContext struct {
	// Fields used for consensus operations
	Alpha        int
	BetaVirtuous int
	BetaRogue    int
}

// State represents the state of a consensus engine
type State uint8

const (
	Initializing State = iota
	StateSyncing
	Bootstrapping
	NormalOp
)

// Voter votes on consensus decisions
type Voter interface {
	// Vote on a decision
	Vote(context.Context, ids.ID, ids.ID) error

	// GetVotes gets votes
	GetVotes(context.Context, ids.ID) ([]ids.ID, error)
}
