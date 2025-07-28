// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testing

import (
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// MockNode is a simple mock node for testing
type MockNode struct {
	mu sync.RWMutex
	
	id         ids.NodeID
	preference ids.ID
	confidence int
	finalized  bool
	
	// Behavior configuration
	shouldFinalize   bool
	roundsToFinalize int
	currentRound     int
}

// NewMockNode creates a new mock node
func NewMockNode(id ids.NodeID) *MockNode {
	return &MockNode{
		id:               id,
		preference:       ids.Empty,
		shouldFinalize:   true,
		roundsToFinalize: 10,
	}
}

// ID returns the node ID
func (n *MockNode) ID() ids.NodeID {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.id
}

// Preference returns the current preference
func (n *MockNode) Preference() ids.ID {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.preference
}

// SetPreference sets the preference
func (n *MockNode) SetPreference(pref ids.ID) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.preference = pref
}

// Confidence returns the confidence level
func (n *MockNode) Confidence() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.confidence
}

// IncrementConfidence increments confidence
func (n *MockNode) IncrementConfidence() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.confidence++
	n.currentRound++
	
	if n.shouldFinalize && n.currentRound >= n.roundsToFinalize {
		n.finalized = true
	}
}

// ResetConfidence resets confidence
func (n *MockNode) ResetConfidence() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.confidence = 0
}

// Finalized returns whether the node has finalized
func (n *MockNode) Finalized() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.finalized
}

// SetBehavior configures the mock node's behavior
func (n *MockNode) SetBehavior(shouldFinalize bool, roundsToFinalize int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.shouldFinalize = shouldFinalize
	n.roundsToFinalize = roundsToFinalize
}

// MockValidator represents a mock validator for testing
type MockValidator struct {
	nodeID ids.NodeID
	weight uint64
}

// NewMockValidator creates a new mock validator
func NewMockValidator(nodeID ids.NodeID, weight uint64) *MockValidator {
	return &MockValidator{
		nodeID: nodeID,
		weight: weight,
	}
}

// NodeID returns the validator's node ID
func (v *MockValidator) NodeID() ids.NodeID {
	return v.nodeID
}

// Weight returns the validator's weight
func (v *MockValidator) Weight() uint64 {
	return v.weight
}

// MockValidatorSet represents a set of validators
type MockValidatorSet struct {
	mu         sync.RWMutex
	validators map[ids.NodeID]*MockValidator
}

// NewMockValidatorSet creates a new validator set
func NewMockValidatorSet() *MockValidatorSet {
	return &MockValidatorSet{
		validators: make(map[ids.NodeID]*MockValidator),
	}
}

// Add adds a validator to the set
func (s *MockValidatorSet) Add(v *MockValidator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.validators[v.NodeID()] = v
}

// Remove removes a validator from the set
func (s *MockValidatorSet) Remove(nodeID ids.NodeID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.validators, nodeID)
}

// Get returns a validator by node ID
func (s *MockValidatorSet) Get(nodeID ids.NodeID) (*MockValidator, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.validators[nodeID]
	return v, ok
}

// Sample returns a random sample of k validators
func (s *MockValidatorSet) Sample(k int) []*MockValidator {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	if k > len(s.validators) {
		k = len(s.validators)
	}
	
	// Convert to slice for sampling
	all := make([]*MockValidator, 0, len(s.validators))
	for _, v := range s.validators {
		all = append(all, v)
	}
	
	// Simple random sampling
	sampled := make([]*MockValidator, k)
	for i := 0; i < k; i++ {
		sampled[i] = all[i]
	}
	
	return sampled
}

// Len returns the number of validators
func (s *MockValidatorSet) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.validators)
}

// MockTimer provides a controllable timer for testing
type MockTimer struct {
	mu       sync.Mutex
	current  time.Time
	timers   []*mockTimerEntry
}

type mockTimerEntry struct {
	deadline time.Time
	ch       chan time.Time
}

// NewMockTimer creates a new mock timer
func NewMockTimer() *MockTimer {
	return &MockTimer{
		current: time.Now(),
		timers:  make([]*mockTimerEntry, 0),
	}
}

// Now returns the current mock time
func (t *MockTimer) Now() time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.current
}

// Advance advances the mock time
func (t *MockTimer) Advance(d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	t.current = t.current.Add(d)
	
	// Trigger any timers that have expired
	remaining := make([]*mockTimerEntry, 0)
	for _, timer := range t.timers {
		if t.current.After(timer.deadline) || t.current.Equal(timer.deadline) {
			select {
			case timer.ch <- t.current:
			default:
			}
			close(timer.ch)
		} else {
			remaining = append(remaining, timer)
		}
	}
	t.timers = remaining
}

// After creates a timer that fires after duration d
func (t *MockTimer) After(d time.Duration) <-chan time.Time {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	ch := make(chan time.Time, 1)
	entry := &mockTimerEntry{
		deadline: t.current.Add(d),
		ch:       ch,
	}
	t.timers = append(t.timers, entry)
	
	return ch
}