// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/luxfi/consensus"
	"github.com/luxfi/ids"
	"github.com/luxfi/node/version"
)

// Config contains the common configuration for bootstrappers
type Config struct {
	// Context is the consensus context for the chain
	Context context.Context

	// StartupTracker tracks chain startup progress
	StartupTracker Tracker

	// Sender is used to send bootstrap messages
	Sender Sender

	// AncestorsMaxContainersRequested is the maximum number of containers to request ancestors for
	AncestorsMaxContainersRequested int

	// Blocked tracks blocks that are blocked on their parent
	Blocked Blocked

	// VM provides the VM interface
	VM VM
}

// Bootstrapper defines the interface for bootstrapping consensus
type Bootstrapper interface {
	// Start begins the bootstrapping process
	Start(ctx context.Context, startReqID uint32) error

	// Connected is called when a peer connects
	Connected(ctx context.Context, nodeID ids.NodeID, nodeVersion *version.Application) error

	// Disconnected is called when a peer disconnects
	Disconnected(ctx context.Context, nodeID ids.NodeID) error

	// Timeout is called when a request times out
	Timeout(ctx context.Context) error

	// Ancestors handles ancestor responses
	Ancestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, containers [][]byte) error

	// Put handles put responses
	Put(ctx context.Context, nodeID ids.NodeID, requestID uint32, container []byte) error

	// GetAncestorsFailed handles failed ancestor requests
	GetAncestorsFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error

	// GetFailed handles failed get requests
	GetFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error

	// HealthCheck returns the health status
	HealthCheck(ctx context.Context) (interface{}, error)

	// Shutdown stops the bootstrapper
	Shutdown(ctx context.Context) error
}

// Poll represents a poll for bootstrapping
type Poll struct {
	alpha   int
	results map[ids.NodeID]ids.ID
}

// NewPoll creates a new poll
func NewPoll(alpha int) *Poll {
	return &Poll{
		alpha:   alpha,
		results: make(map[ids.NodeID]ids.ID),
	}
}

// Vote records a vote in the poll
func (p *Poll) Vote(nodeID ids.NodeID, containerID ids.ID) {
	p.results[nodeID] = containerID
}

// Finished returns true if the poll is complete
func (p *Poll) Finished() bool {
	return len(p.results) >= p.alpha
}

// Result returns the most popular result
func (p *Poll) Result() (ids.ID, bool) {
	if !p.Finished() {
		return ids.Empty, false
	}

	counts := make(map[ids.ID]int)
	for _, id := range p.results {
		counts[id]++
	}

	var maxID ids.ID
	maxCount := 0
	for id, count := range counts {
		if count > maxCount {
			maxID = id
			maxCount = count
		}
	}

	return maxID, maxCount >= p.alpha
}

// State tracks the state of bootstrapping
type State int

const (
	// StateStarting indicates bootstrapping is starting
	StateStarting State = iota
	// StateFetching indicates we're fetching blocks
	StateFetching
	// StateExecuting indicates we're executing blocks
	StateExecuting
	// StateFinished indicates bootstrapping is complete
	StateFinished
)

// Interval represents a contiguous range of blocks
type Interval struct {
	LowerBound uint64
	UpperBound uint64
}

// Contains returns true if height is within the interval
func (i *Interval) Contains(height uint64) bool {
	return height >= i.LowerBound && height <= i.UpperBound
}

// IntervalTree tracks intervals of blocks
type IntervalTree struct {
	intervals []Interval
}

// Add adds an interval to the tree
func (t *IntervalTree) Add(interval Interval) {
	t.intervals = append(t.intervals, interval)
	// TODO: Merge overlapping intervals
}

// MissingRanges returns the missing ranges up to maxHeight
func (t *IntervalTree) MissingRanges(maxHeight uint64) []Interval {
	// TODO: Implement properly
	if len(t.intervals) == 0 {
		return []Interval{{LowerBound: 0, UpperBound: maxHeight}}
	}
	return nil
}

// Clear removes all intervals
func (t *IntervalTree) Clear() {
	t.intervals = nil
}

// Fetcher handles fetching containers during bootstrap
type Fetcher interface {
	// Clear removes all pending requests
	Clear() error

	// Add adds a container ID to fetch
	Add(containerID ids.ID) error

	// Remove removes a container ID from pending
	Remove(containerID ids.ID) error

	// NumFetching returns the number of containers being fetched
	NumFetching() int

	// Outstanding returns the outstanding request IDs
	Outstanding(containerID ids.ID) ([]uint32, bool)
}

// Executor handles executing containers during bootstrap
type Executor interface {
	// Execute processes a container
	Execute(ctx context.Context, container []byte) error

	// Clear removes all pending containers
	Clear() error
}

// Stats tracks bootstrap statistics
type Stats struct {
	NumFetched  int
	NumAccepted int
	NumRejected int
}

// String returns a string representation of stats
func (s *Stats) String() string {
	return fmt.Sprintf("Fetched: %d, Accepted: %d, Rejected: %d",
		s.NumFetched, s.NumAccepted, s.NumRejected)
}

// Tracker tracks the progress of operations
type Tracker interface {
	// IsBootstrapped returns true if bootstrapping is complete
	IsBootstrapped() bool
	// Bootstrapped marks bootstrapping as complete
	Bootstrapped()
}

// Sender sends consensus messages
type Sender interface {
	// SendGetAncestors requests ancestors of a container
	SendGetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	// SendGet requests a container
	SendGet(ctx context.Context, nodeID ids.NodeID, requestID uint32, containerID ids.ID) error
	// SendPut sends a container
	SendPut(ctx context.Context, nodeID ids.NodeID, requestID uint32, container []byte) error
	// SendPushQuery sends a push query
	SendPushQuery(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, container []byte, requestedHeight uint64) error
	// SendPullQuery sends a pull query
	SendPullQuery(ctx context.Context, nodeIDs []ids.NodeID, requestID uint32, containerID ids.ID, requestedHeight uint64) error
}

// Blocked tracks containers blocked on their dependencies
type Blocked interface {
	// Add adds a blocked container
	Add(containerID ids.ID, parentID ids.ID)
	// Remove removes a blocked container
	Remove(containerID ids.ID)
	// Get returns containers blocked on the given ID
	Get(containerID ids.ID) []ids.ID
	// Len returns the number of blocked containers
	Len() int
}

// VM defines the virtual machine interface
type VM interface {
	// ParseBlock parses a block from bytes
	ParseBlock(ctx context.Context, blockBytes []byte) (consensus.Block, error)
	// GetBlock retrieves a block by ID
	GetBlock(ctx context.Context, blockID ids.ID) (consensus.Block, error)
	// SetPreference sets the preferred block
	SetPreference(ctx context.Context, blockID ids.ID) error
	// LastAccepted returns the last accepted block ID
	LastAccepted(ctx context.Context) (ids.ID, error)
}

// Manager manages the bootstrap process
type Manager struct {
	config   *Config
	state    State
	stats    Stats
	executor Executor
}

// NewManager creates a new bootstrap manager
func NewManager(config *Config) *Manager {
	return &Manager{
		config: config,
		state:  StateStarting,
	}
}

// Start begins the bootstrap process
func (m *Manager) Start(ctx context.Context) error {
	m.state = StateFetching
	return nil
}

// Connected handles a peer connection
func (m *Manager) Connected(ctx context.Context, nodeID ids.NodeID) error {
	return nil
}

// Disconnected handles a peer disconnection
func (m *Manager) Disconnected(ctx context.Context, nodeID ids.NodeID) error {
	return nil
}

// Timeout handles request timeouts
func (m *Manager) Timeout(ctx context.Context) error {
	return nil
}

// Put handles put responses
func (m *Manager) Put(ctx context.Context, nodeID ids.NodeID, requestID uint32, container []byte) error {
	m.stats.NumFetched++
	return m.executor.Execute(ctx, container)
}

// GetFailed handles failed get requests
func (m *Manager) GetFailed(ctx context.Context, nodeID ids.NodeID, requestID uint32) error {
	return nil
}

// HealthCheck returns the health status
func (m *Manager) HealthCheck(ctx context.Context) (interface{}, error) {
	return m.stats.String(), nil
}

// Metrics for bootstrapping
type Metrics struct {
	NumFetched  int64
	NumAccepted int64
	NumRejected int64
	FetchTime   time.Duration
}

// Utility functions

// ShouldFetch determines if a container should be fetched
func ShouldFetch(containerID ids.ID, lastAccepted ids.ID) bool {
	return containerID != lastAccepted
}

// IsBootstrapped returns true if the chain is bootstrapped
func IsBootstrapped() bool {
	// TODO: Implement properly
	return true
}

// GetMissingContainers returns containers that need to be fetched
func GetMissingContainers(have []ids.ID, want []ids.ID) []ids.ID {
	haveSet := make(map[ids.ID]bool)
	for _, id := range have {
		haveSet[id] = true
	}

	var missing []ids.ID
	for _, id := range want {
		if !haveSet[id] {
			missing = append(missing, id)
		}
	}
	return missing
}
