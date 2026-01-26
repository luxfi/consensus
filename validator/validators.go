// Package validators provides validator state management.
// This package re-exports github.com/luxfi/validators for backward compatibility.
// New code should use github.com/luxfi/validators directly.
package validators

import (
	"github.com/luxfi/validators"
)

// State is an alias for validators.State for backward compatibility.
type State = validators.State

// GetValidatorOutput is an alias for validators.GetValidatorOutput
type GetValidatorOutput = validators.GetValidatorOutput

// WarpValidator is an alias for validators.WarpValidator
type WarpValidator = validators.WarpValidator

// WarpSet is an alias for validators.WarpSet
type WarpSet = validators.WarpSet

// Set is an alias for validators.Set
type Set = validators.Set

// Validator is an alias for validators.Validator
type Validator = validators.Validator

// ValidatorImpl is an alias for validators.ValidatorImpl
type ValidatorImpl = validators.ValidatorImpl

// Manager is an alias for validators.Manager
type Manager = validators.Manager

// SetCallbackListener is an alias for validators.SetCallbackListener
type SetCallbackListener = validators.SetCallbackListener

// ManagerCallbackListener is an alias for validators.ManagerCallbackListener
type ManagerCallbackListener = validators.ManagerCallbackListener

// Connector is an alias for validators.Connector
type Connector = validators.Connector
