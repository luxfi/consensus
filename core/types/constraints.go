package types

// Number represents numeric types for consensus operations.
// Includes all integer and floating point types.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64
}

// Round represents a consensus round number
type Round uint64

// Seed represents a random seed value
type Seed uint64

// Hash20 represents a 20-byte hash interface
type Hash20 interface{ ~[20]byte }

// Hash32 represents a 32-byte hash interface
type Hash32 interface{ ~[32]byte }
