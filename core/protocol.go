package core

import "context"

// ID interface for block/vertex IDs
type ID interface{ ~[32]byte }

// Protocol represents a consensus protocol that can be plugged into engines
type Protocol[I comparable] interface {
	// Initialize initializes the protocol
	Initialize(ctx context.Context) error
	
	// Step runs one poll/round of the protocol
	Step(ctx context.Context) error
	
	// Status returns the status of an item (e.g., {unknown, preferred, decided})
	Status(id I) (string, error)
}