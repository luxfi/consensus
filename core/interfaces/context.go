package interfaces

import (
    "context"
    "sync"
    "sync/atomic"
    "github.com/luxfi/ids"
    "github.com/luxfi/crypto/bls"
    "github.com/luxfi/log"
    metric "github.com/luxfi/metric"
)

// MultiGatherer is a metrics gatherer
type MultiGatherer = metric.MultiGatherer

// Registerer is the metrics registerer interface
type Registerer = metric.Registerer

// State represents chain operational state
type State uint8

const (
    // NormalOp is the normal operational state
    NormalOp State = iota
    // Bootstrapping indicates the node is syncing
    Bootstrapping
    // StateSyncing indicates state sync is active
    StateSyncing
)

// StateHolder manages atomic state updates
type StateHolder struct {
    value atomic.Value
}

// Get returns the current state
func (s *StateHolder) Get() State {
    if val := s.value.Load(); val != nil {
        return val.(State)
    }
    return NormalOp
}

// Set updates the current state
func (s *StateHolder) Set(state State) {
    s.value.Store(state)
}

// ValidatorState provides validator state operations
type ValidatorState interface {
    GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error)
    GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
    GetCurrentHeight(ctx context.Context) (uint64, error)
}

// ValidatorSet provides access to validator information for consensus
type ValidatorSet interface {
    // Self returns the node's own ID
    Self() ids.NodeID
    
    // GetWeight returns the weight of a validator
    GetWeight(nodeID ids.NodeID) uint64
    
    // TotalWeight returns the total weight of all validators
    TotalWeight() uint64
}

// BCLookup provides blockchain lookup operations
type BCLookup interface {
    PrimaryAlias(chainID ids.ID) (string, error)
    Lookup(alias string) (ids.ID, error)
}

// SharedMemory provides cross-chain atomic operations
type SharedMemory interface {
    Get(peerChainID ids.ID, keys [][]byte) ([][]byte, error)
    Apply(requests map[ids.ID]interface{}, batch ...interface{}) error
}

// Context provides consensus engine configuration
type Context struct {
    NetworkID    uint32
    SubnetID     ids.ID
    ChainID      ids.ID
    NodeID       ids.NodeID
    PublicKey    *bls.PublicKey
    
    Log          log.Logger
    Metrics      MultiGatherer
    
    // ValidatorState provides validator information
    ValidatorState ValidatorState
    
    // BCLookup provides blockchain alias lookup
    BCLookup BCLookup
    
    // SharedMemory for cross-chain operations
    SharedMemory SharedMemory
    
    // Lock for thread safety
    Lock sync.RWMutex
    
    // State represents the current chain state
    State StateHolder
}