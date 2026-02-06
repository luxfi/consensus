// Package context provides consensus context for VMs.
// This package re-exports runtime.Runtime for backward compatibility.
// New code should use github.com/luxfi/runtime directly.
package context

import (
	"context"

	"github.com/luxfi/ids"
	"github.com/luxfi/runtime"
)

// Context is an alias for runtime.Runtime for backward compatibility.
// New code should use runtime.Runtime directly.
type Context = runtime.Runtime

// ValidatorState is an alias for runtime.ValidatorState
type ValidatorState = runtime.ValidatorState

// BCLookup is an alias for runtime.BCLookup
type BCLookup = runtime.BCLookup

// Keystore is an alias for runtime.Keystore
type Keystore = runtime.Keystore

// Metrics is an alias for runtime.Metrics
type Metrics = runtime.Metrics

// GetChainID extracts chain ID from context (backwards compatibility)
var GetChainID = runtime.GetChainID

// GetNetworkID extracts network ID from context (backwards compatibility)
var GetNetworkID = runtime.GetNetworkID

// GetValidatorState extracts validator state from context (backwards compatibility)
var GetValidatorState = runtime.GetValidatorState

// WithValidatorState adds a validator state to the context (backwards compatibility)
var WithValidatorState = runtime.WithValidatorState

// GetNetID is an alias for GetNetworkID for compatibility
func GetNetID(ctx context.Context) ids.ID {
	return ids.Empty // Stub for compatibility
}
