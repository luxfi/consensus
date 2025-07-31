// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package choices

// Status represents the current status of a block or decision
type Status uint32

const (
	// Unknown The status of this vertex is not yet known
	Unknown Status = iota

	// Processing The vertex is being processed
	Processing

	// Rejected The vertex was rejected
	Rejected

	// Accepted The vertex was accepted
	Accepted
)

func (s Status) String() string {
	switch s {
	case Unknown:
		return "Unknown"
	case Processing:
		return "Processing"
	case Rejected:
		return "Rejected"
	case Accepted:
		return "Accepted"
	default:
		return "Invalid status"
	}
}

// Valid returns true if the status is valid
func (s Status) Valid() bool {
	switch s {
	case Unknown, Processing, Rejected, Accepted:
		return true
	default:
		return false
	}
}

// Decided returns true if the status is Accepted or Rejected
func (s Status) Decided() bool {
	switch s {
	case Accepted, Rejected:
		return true
	default:
		return false
	}
}

// Fetched returns true if the status has been fetched
func (s Status) Fetched() bool {
	switch s {
	case Processing, Accepted, Rejected:
		return true
	default:
		return false
	}
}