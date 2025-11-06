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

// Compare compares two versions
// Returns -1 if v < other, 0 if v == other, 1 if v > other
func (v *Application) Compare(other *Application) int {
	if v.Major < other.Major {
		return -1
	}
	if v.Major > other.Major {
		return 1
	}
	if v.Minor < other.Minor {
		return -1
	}
	if v.Minor > other.Minor {
		return 1
	}
	if v.Patch < other.Patch {
		return -1
	}
	if v.Patch > other.Patch {
		return 1
	}
	return 0
}

// Current returns the current version
func Current() *Application {
	return &Application{
		Major: 1,
		Minor: 21,
		Patch: 1,
		Name:  "lux",
	}
}
