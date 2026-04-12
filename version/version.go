// Package version re-exports github.com/luxfi/version.
package version

import (
	"github.com/luxfi/version"
)

// Application re-exports version.Application.
type Application = version.Application

// Current returns the current application version
func Current() *Application {
	return version.CurrentApp
}
