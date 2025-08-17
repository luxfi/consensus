// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package benchlist

import (
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// Manager manages benchlisted nodes
type Manager interface {
	IsBenched(nodeID ids.NodeID) bool
	Bench(nodeID ids.NodeID)
	RegisterResponse(nodeID ids.NodeID)
	RegisterFailure(nodeID ids.NodeID)
}

type manager struct {
	lock       sync.RWMutex
	benchlist  map[ids.NodeID]time.Time
	config     Config
	failures   map[ids.NodeID]int
	failedTime map[ids.NodeID]time.Time
}

// NewManager creates a new benchlist manager
func NewManager(config *Config) Manager {
	return &manager{
		benchlist:  make(map[ids.NodeID]time.Time),
		config:     *config,
		failures:   make(map[ids.NodeID]int),
		failedTime: make(map[ids.NodeID]time.Time),
	}
}

func (m *manager) IsBenched(nodeID ids.NodeID) bool {
	m.lock.RLock()
	defer m.lock.RUnlock()

	benchedUntil, exists := m.benchlist[nodeID]
	if !exists {
		return false
	}

	if time.Now().After(benchedUntil) {
		delete(m.benchlist, nodeID)
		return false
	}
	return true
}

func (m *manager) Bench(nodeID ids.NodeID) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.benchlist[nodeID] = time.Now().Add(m.config.Duration)
	delete(m.failures, nodeID)
	delete(m.failedTime, nodeID)
}

func (m *manager) RegisterResponse(nodeID ids.NodeID) {
	m.lock.Lock()
	defer m.lock.Unlock()

	delete(m.failures, nodeID)
	delete(m.failedTime, nodeID)
}

func (m *manager) RegisterFailure(nodeID ids.NodeID) {
	m.lock.Lock()
	defer m.lock.Unlock()

	// Check if we're already benched
	if _, benched := m.benchlist[nodeID]; benched {
		return
	}

	// Track first failure time
	if _, exists := m.failedTime[nodeID]; !exists {
		m.failedTime[nodeID] = time.Now()
	}

	// Increment failure count
	m.failures[nodeID]++

	// Check if we should benchlist
	if m.failures[nodeID] >= m.config.Threshold {
		failingDuration := time.Since(m.failedTime[nodeID])
		if failingDuration >= m.config.MinimumFailingDuration {
			m.benchlist[nodeID] = time.Now().Add(m.config.Duration)
			delete(m.failures, nodeID)
			delete(m.failedTime, nodeID)
		}
	}
}