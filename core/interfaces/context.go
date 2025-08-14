package interfaces

import (
    "github.com/luxfi/ids"
    "github.com/luxfi/crypto/bls"
    "github.com/luxfi/log"
    "github.com/luxfi/metric"
)

// Registerer is the metrics registerer interface
type Registerer = metrics.Registerer

// Context provides consensus engine configuration
type Context struct {
    NetworkID    uint32
    SubnetID     ids.ID
    ChainID      ids.ID
    NodeID       ids.NodeID
    PublicKey    *bls.PublicKey
    
    Log          log.Logger
    Metrics      metrics.MultiGatherer
}
