package types

import "golang.org/x/exp/constraints"

// Number represents numeric types for consensus operations
type Number interface {
	constraints.Integer | constraints.Float
}

// Round represents a consensus round number
type Round uint64

// Seed represents a random seed value
type Seed uint64

// Hash20 represents a 20-byte hash interface
type Hash20 interface{ ~[20]byte }

// Hash32 represents a 32-byte hash interface
type Hash32 interface{ ~[32]byte }
