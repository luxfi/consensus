// Package version provides version information.
// This package re-exports github.com/luxfi/version for backward compatibility.
// New code should use github.com/luxfi/version directly.
package version

import (
	"github.com/luxfi/version"
)

// Application is an alias for version.Application for backward compatibility.
// New code should use version.Application directly.
type Application = version.Application

// Current returns the current application version
func Current() *Application {
	return version.CurrentApp
}
