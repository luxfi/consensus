// Package context re-exports runtime types for consensus VMs.
package context

import (
	"github.com/luxfi/runtime"
)

// Context re-exports runtime.Runtime.
type Context = runtime.Runtime

// ValidatorState re-exports runtime.ValidatorState.
type ValidatorState = runtime.ValidatorState

// BCLookup re-exports runtime.BCLookup.
type BCLookup = runtime.BCLookup

// Keystore re-exports runtime.Keystore.
type Keystore = runtime.Keystore

// Metrics re-exports runtime.Metrics.
type Metrics = runtime.Metrics

var (
	GetChainID       = runtime.GetChainID
	GetNetworkID     = runtime.GetNetworkID
	GetValidatorState = runtime.GetValidatorState
	WithValidatorState = runtime.WithValidatorState
)
