package interfaces

import (
    "context"
    "github.com/luxfi/ids"
    "github.com/luxfi/crypto/bls"
    "github.com/luxfi/log"
    "github.com/luxfi/metric"
)

// Registerer is the metrics registerer interface
type Registerer = metrics.Registerer

// ValidatorState provides validator state operations
type ValidatorState interface {
    GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error)
    GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
}

// BCLookup provides blockchain lookup operations
type BCLookup interface {
    PrimaryAlias(chainID ids.ID) (string, error)
}

// Context provides consensus engine configuration
type Context struct {
    NetworkID    uint32
    SubnetID     ids.ID
    ChainID      ids.ID
    NodeID       ids.NodeID
    PublicKey    *bls.PublicKey
    
    Log          log.Logger
    Metrics      metrics.MultiGatherer
    
    // ValidatorState provides validator information
    ValidatorState ValidatorState
    
    // BCLookup provides blockchain alias lookup
    BCLookup BCLookup
}
