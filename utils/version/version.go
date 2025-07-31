// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package version

import (
	"fmt"
	"time"
)

// Semantic represents a semantic version
type Semantic struct {
	Major int
	Minor int
	Patch int
}

// String returns the string representation of the version
func (s Semantic) String() string {
	return fmt.Sprintf("%d.%d.%d", s.Major, s.Minor, s.Patch)
}

// Compare returns:
// -1 if s < o
// 0 if s == o  
// 1 if s > o
func (s Semantic) Compare(o Semantic) int {
	if s.Major != o.Major {
		if s.Major < o.Major {
			return -1
		}
		return 1
	}
	if s.Minor != o.Minor {
		if s.Minor < o.Minor {
			return -1
		}
		return 1
	}
	if s.Patch != o.Patch {
		if s.Patch < o.Patch {
			return -1
		}
		return 1
	}
	return 0
}

// Application represents the application version info
type Application struct {
	Name      string
	Version   Semantic
	Commit    string
	BuildTime time.Time
}

// String returns the string representation of the application version
func (a Application) String() string {
	return fmt.Sprintf("%s/%s", a.Name, a.Version)
}

// Compatible returns true if the given version is compatible
func (a Application) Compatible(o Application) bool {
	// For now, just check major version compatibility
	return a.Version.Major == o.Version.Major
}

// Before returns true if this version is before the given version
func (a Application) Before(o Application) bool {
	return a.Version.Compare(o.Version) < 0
}