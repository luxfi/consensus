package choices

import "github.com/luxfi/consensus/core/interfaces"

// Re-export Status for compatibility
type Status = interfaces.Status

const (
	Unknown    = interfaces.Unknown
	Processing = interfaces.Processing
	Rejected   = interfaces.Rejected
	Accepted   = interfaces.Accepted
)
