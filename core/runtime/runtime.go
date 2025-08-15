// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package runtime

import (
	"context"
	"sync/atomic"
	"time"
	
	"github.com/luxfi/ids"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/log"
	metric "github.com/luxfi/metric"
)

// Runtime contains immutable runtime metadata for consensus operations
type Runtime struct {
	NetworkID    uint32
	SubnetID     ids.ID
	ChainID      ids.ID
	NodeID       ids.NodeID
	PublicKey    *bls.PublicKey
	XAssetID     ids.ID  // Native asset ID on X-Chain (settlement/custody layer)
	XChainID     ids.ID  // X-Chain ID for asset settlement and custody
	CChainID     ids.ID  // C-Chain ID for smart contract operations
	ChainDataDir string  // Directory for chain data storage
}

// Config contains consensus configuration parameters
type Config struct {
	// Consensus parameters
	K                     int
	AlphaPreference       int
	AlphaConfidence       int
	Beta                  int
	MaxItemProcessingTime time.Duration
	
	// Network parameters
	MaxMessageSize        int
	MaxPendingMessages    int
	NetworkTimeout        time.Duration
	
	// Chain parameters
	GossipBatchSize       int
	GossipFrequency       time.Duration
}

// Deps contains external dependencies for consensus operations
type Deps struct {
	Log             log.Logger
	Metrics         metric.MultiGatherer
	ValidatorState  ValidatorState
	BCLookup        BCLookup
	SharedMemory    SharedMemory
	Clock           Clock
	DB              Database
}

// ValidatorState provides validator state operations
type ValidatorState interface {
	GetCurrentHeight() (uint64, error)
	GetMinimumHeight(ctx context.Context) (uint64, error)
	GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error)
	GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
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

// Clock provides time operations
type Clock interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// Database provides storage operations
type Database interface {
	Get(key []byte) ([]byte, error)
	Put(key []byte, value []byte) error
	Delete(key []byte) error
	NewBatch() Batch
}

// Batch provides atomic database operations
type Batch interface {
	Put(key []byte, value []byte) error
	Delete(key []byte) error
	Write() error
	Reset()
}

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