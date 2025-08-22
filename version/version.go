// Package version provides version information
package version

import "fmt"

// Application represents an application version
type Application struct {
	Major int
	Minor int
	Patch int
	
	// Additional metadata
	Name string
}

// String returns the version string
func (v *Application) String() string {
	return fmt.Sprintf("%s-%d.%d.%d", v.Name, v.Major, v.Minor, v.Patch)
}

// Compatible returns whether versions are compatible
func (v *Application) Compatible(other *Application) bool {
	// Major version must match
	return v.Major == other.Major
}

// Current returns the current version
func Current() *Application {
	return &Application{
		Major: 1,
		Minor: 0,
		Patch: 0,
		Name:  "lux",
	}
}