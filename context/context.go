// Package context provides consensus context for VMs.
// This package re-exports runtime.Runtime for backward compatibility.
// New code should use github.com/luxfi/runtime directly.
package context

import (
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
