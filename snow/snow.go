package snow

import (
    "context"
    "github.com/luxfi/ids"
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
    Alpha         int
    BetaVirtuous  int
    BetaRogue     int
    Parents       int
    ConcurrentRepolls int
}

// Parameters defines consensus parameters
type Parameters struct {
    K                int
    Alpha            int
    BetaVirtuous     int
    BetaRogue        int
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

// Voter votes on consensus decisions
type Voter interface {
    // Vote on a decision
    Vote(context.Context, ids.ID, ids.ID) error
    
    // GetVotes gets votes
    GetVotes(context.Context, ids.ID) ([]ids.ID, error)
}