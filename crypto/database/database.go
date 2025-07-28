// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package database

// Batch is a write batch
type Batch interface {
	// Put adds a key-value pair to the batch
	Put(key, value []byte) error
	
	// Delete removes a key from the batch
	Delete(key []byte) error
	
	// Size returns the number of operations in the batch
	Size() int
	
	// Write commits the batch
	Write() error
	
	// Reset clears the batch
	Reset()
	
	// Replay replays the batch to another writer
	Replay(w Writer) error
}

// Database is a key-value database
type Database interface {
	Reader
	Writer
	
	// NewBatch creates a new batch
	NewBatch() Batch
	
	// Close closes the database
	Close() error
}

// Reader reads from a database
type Reader interface {
	// Has returns true if the key exists
	Has(key []byte) (bool, error)
	
	// Get returns the value for the key
	Get(key []byte) ([]byte, error)
}

// Writer writes to a database
type Writer interface {
	// Put sets the value for the key
	Put(key, value []byte) error
	
	// Delete removes the key
	Delete(key []byte) error
}