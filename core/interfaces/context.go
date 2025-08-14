package interfaces

import (
    "github.com/luxfi/ids"
    "github.com/luxfi/crypto/bls"
    "github.com/luxfi/log"
    "github.com/luxfi/metric"
)

// Context provides consensus engine configuration
type Context struct {
    NetworkID    uint32
    SubnetID     ids.ID
    ChainID      ids.ID
    NodeID       ids.NodeID
    PublicKey    *bls.PublicKey
    
    Log          log.Logger
    Metrics      metric.MultiGatherer
}
