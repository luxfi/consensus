// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package atomic

import (
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// SharedMemory provides the interface for shared memory operations between chains
type SharedMemory interface {
	// Get returns a database scoped to [peerChainID]
	Get(peerChainID ids.ID, keys [][]byte) (values [][]byte, err error)
	
	// Put writes to the database scoped to [peerChainID]
	Put(peerChainID ids.ID, elems map[string][]byte, batches ...database.Batch) error
	
	// Remove deletes from the database scoped to [peerChainID]
	Remove(peerChainID ids.ID, keys [][]byte, batches ...database.Batch) error
	
	// NewSharedMemory returns a new SharedMemory scoped to [prefix]
	NewSharedMemory(chainID ids.ID) SharedMemory
}

// Memory is a simple in-memory implementation of SharedMemory
type Memory struct {
	data map[ids.ID]map[string][]byte
}

// NewMemory returns a new in-memory SharedMemory
func NewMemory() SharedMemory {
	return &Memory{
		data: make(map[ids.ID]map[string][]byte),
	}
}

// Get returns values for the given keys
func (m *Memory) Get(peerChainID ids.ID, keys [][]byte) ([][]byte, error) {
	chainData, exists := m.data[peerChainID]
	if !exists {
		return make([][]byte, len(keys)), nil
	}
	
	values := make([][]byte, len(keys))
	for i, key := range keys {
		values[i] = chainData[string(key)]
	}
	
	return values, nil
}

// Put writes key-value pairs
func (m *Memory) Put(peerChainID ids.ID, elems map[string][]byte, batches ...database.Batch) error {
	if m.data[peerChainID] == nil {
		m.data[peerChainID] = make(map[string][]byte)
	}
	
	for k, v := range elems {
		m.data[peerChainID][k] = v
	}
	
	return nil
}

// Remove deletes keys
func (m *Memory) Remove(peerChainID ids.ID, keys [][]byte, batches ...database.Batch) error {
	chainData, exists := m.data[peerChainID]
	if !exists {
		return nil
	}
	
	for _, key := range keys {
		delete(chainData, string(key))
	}
	
	return nil
}

// NewSharedMemory returns a new SharedMemory instance
func (m *Memory) NewSharedMemory(chainID ids.ID) SharedMemory {
	return m
}