package version

// Version represents a software version
type Version struct {
	Major int
	Minor int
	Patch int
}

// Current returns the current consensus version
func Current() Version {
	return Version{
		Major: 1,
		Minor: 0,
		Patch: 0,
	}
}

// String returns the version as a string
func (v Version) String() string {
	return "v1.0.0"
}