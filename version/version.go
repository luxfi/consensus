// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package version

import (
	"time"
)

// Application represents the version of a peer's application
type Application struct {
	Name  string
	Major int
	Minor int
	Patch int
}

// String returns the string representation of the version
func (a *Application) String() string {
	return a.Name
}

// Before returns true if this version is before the provided version
func (a *Application) Before(other *Application) bool {
	if a.Major != other.Major {
		return a.Major < other.Major
	}
	if a.Minor != other.Minor {
		return a.Minor < other.Minor
	}
	return a.Patch < other.Patch
}

// Compare returns:
// -1 if a < other
// 0 if a == other  
// 1 if a > other
func (a *Application) Compare(other *Application) int {
	if a.Major != other.Major {
		if a.Major < other.Major {
			return -1
		}
		return 1
	}
	if a.Minor != other.Minor {
		if a.Minor < other.Minor {
			return -1
		}
		return 1
	}
	if a.Patch != other.Patch {
		if a.Patch < other.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// Compatible returns true if the versions are compatible
func (a *Application) Compatible(other *Application) bool {
	return a.Major == other.Major
}

// Constants for version compatibility
type Compatibility struct {
	Version                      string
	VersionTime                  time.Time
	MinimumCompatibleVersion     string
	MinimumUnmaskedVersion       string
	PrevMinimumCompatibleVersion string
	MinimumMaskableVersion       string
}

// DefaultVersion returns the default application version
func DefaultVersion() *Application {
	return &Application{
		Name:  "lux",
		Major: 1,
		Minor: 0,
		Patch: 1,
	}
}
