// Package verify provides verification utilities
package verify

// IsBlock is an empty struct that can be embedded to indicate a type is a block
type IsBlock struct{}

// Verifiable represents something that can be verified
type Verifiable interface {
	Verify() error
}
