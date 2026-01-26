// Package validators provides validator state management.
// This package re-exports github.com/luxfi/validators for backward compatibility.
// New code should use github.com/luxfi/validators directly.
package validators

import (
	"github.com/luxfi/validators"
)

// NewManager creates a new validator manager
func NewManager() Manager {
	return validators.NewManager()
}
