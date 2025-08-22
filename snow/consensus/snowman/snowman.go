package snowman

import (
    "context"
    "github.com/luxfi/ids"
)

// Consensus implements Snowman consensus
type Consensus interface {
    // Initialize initializes consensus
    Initialize(context.Context, *Config) error
    
    // Add adds a block to consensus
    Add(context.Context, Block) error
    
    // RecordPoll records poll results
    RecordPoll(context.Context, ids.Bag) error
    
    // Finalized checks if finalized
    Finalized() bool
    
    // Parameters returns consensus parameters
    Parameters() Parameters
    
    // Preference returns the preferred block
    Preference() ids.ID
    
    // HealthCheck performs health check
    HealthCheck(context.Context) (interface{}, error)
}

// Block represents a Snowman block
type Block interface {
    ID() ids.ID
    ParentID() ids.ID
    Height() uint64
    Verify(context.Context) error
    Accept(context.Context) error
    Reject(context.Context) error
    Bytes() []byte
}

// Config defines Snowman configuration
type Config struct {
    Beta            int
    ConcurrentRepolls int
    OptimalProcessing int
}

// Parameters defines consensus parameters
type Parameters struct {
    K                 int
    Alpha             int
    BetaVirtuous      int
    BetaRogue         int
    ConcurrentRepolls int
    OptimalProcessing int
}

// Topological provides topological Snowman consensus
type Topological struct {
    config     *Config
    preference ids.ID
    finalized  bool
}

// NewTopological creates a new topological Snowman instance
func NewTopological(config *Config) *Topological {
    return &Topological{
        config:    config,
        finalized: false,
    }
}

// Initialize initializes consensus
func (t *Topological) Initialize(ctx context.Context, config *Config) error {
    t.config = config
    return nil
}

// Add adds a block to consensus
func (t *Topological) Add(ctx context.Context, block Block) error {
    if t.preference == (ids.ID{}) {
        t.preference = block.ID()
    }
    return nil
}

// RecordPoll records poll results
func (t *Topological) RecordPoll(ctx context.Context, votes ids.Bag) error {
    return nil
}

// Finalized checks if finalized
func (t *Topological) Finalized() bool {
    return t.finalized
}

// Parameters returns consensus parameters
func (t *Topological) Parameters() Parameters {
    return Parameters{
        K:                 1,
        Alpha:             1,
        BetaVirtuous:      t.config.Beta,
        BetaRogue:         t.config.Beta,
        ConcurrentRepolls: t.config.ConcurrentRepolls,
        OptimalProcessing: t.config.OptimalProcessing,
    }
}

// Preference returns the preferred block
func (t *Topological) Preference() ids.ID {
    return t.preference
}

// HealthCheck performs health check
func (t *Topological) HealthCheck(ctx context.Context) (interface{}, error) {
    return map[string]interface{}{
        "finalized":  t.finalized,
        "preference": t.preference.String(),
    }, nil
}