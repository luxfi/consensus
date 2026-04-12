// Package validators re-exports github.com/luxfi/validators.
package validators

import (
	"github.com/luxfi/validators"
)

// NewManager creates a new validator manager
func NewManager() Manager {
	return validators.NewManager()
}
