// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package atomic

import (
	"sync"

	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// SharedMemory provides the interface for shared memory operations
// This is a standalone implementation for consensus module
type SharedMemory interface {
	Apply(map[ids.ID]*Requests, database.Batch) error
	Get(peerChainID ids.ID, keys [][]byte) ([][]byte, error)
	Remove(peerChainID ids.ID, keys [][]byte, batch database.Batch) error
	Put(peerChainID ids.ID, elems []*Element, batch database.Batch) error
}

// Memory is a simple in-memory implementation of SharedMemory
type Memory struct {
	mu   sync.RWMutex
	data map[ids.ID]map[string][]byte
}

// NewMemory creates a new in-memory shared memory
func NewMemory() SharedMemory {
	return &Memory{
		data: make(map[ids.ID]map[string][]byte),
	}
}

// Apply applies the given requests to the shared memory
func (m *Memory) Apply(requests map[ids.ID]*Requests, batch database.Batch) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for chainID, req := range requests {
		if _, ok := m.data[chainID]; !ok {
			m.data[chainID] = make(map[string][]byte)
		}

		// Process removes
		for _, key := range req.RemoveRequests {
			delete(m.data[chainID], string(key))
		}

		// Process puts
		for _, elem := range req.PutRequests {
			m.data[chainID][string(elem.Key)] = elem.Value
		}
	}

	return nil
}

// Get retrieves values for the given keys from the specified chain
func (m *Memory) Get(peerChainID ids.ID, keys [][]byte) ([][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	chainData, ok := m.data[peerChainID]
	if !ok {
		return make([][]byte, len(keys)), nil
	}

	values := make([][]byte, len(keys))
	for i, key := range keys {
		if val, exists := chainData[string(key)]; exists {
			values[i] = val
		}
	}

	return values, nil
}

// Remove removes the given keys from the specified chain
func (m *Memory) Remove(peerChainID ids.ID, keys [][]byte, batch database.Batch) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if chainData, ok := m.data[peerChainID]; ok {
		for _, key := range keys {
			delete(chainData, string(key))
		}
	}

	return nil
}

// Put stores the given elements in the specified chain
func (m *Memory) Put(peerChainID ids.ID, elems []*Element, batch database.Batch) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.data[peerChainID]; !ok {
		m.data[peerChainID] = make(map[string][]byte)
	}

	for _, elem := range elems {
		m.data[peerChainID][string(elem.Key)] = elem.Value
	}

	return nil
}

// Element represents a key-value pair
type Element struct {
	Key    []byte
	Value  []byte
	Traits [][]byte
}

// Requests contains put and remove requests
type Requests struct {
	PutRequests    []*Element
	RemoveRequests [][]byte
}