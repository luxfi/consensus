package core

// Status represents the consensus status of an item
type Status int

const (
	// StatusUnknown means the status is unknown
	StatusUnknown Status = iota
	
	// StatusPending means the item is pending decision
	StatusPending
	
	// StatusProcessing means the item is being processed
	StatusProcessing
	
	// StatusAccepted means the item has been accepted
	StatusAccepted
	
	// StatusRejected means the item has been rejected
	StatusRejected
)

// String returns the string representation of the status
func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusProcessing:
		return "processing"
	case StatusAccepted:
		return "accepted"
	case StatusRejected:
		return "rejected"
	default:
		return "unknown"
	}
}

// Decided returns true if the status represents a final decision
func (s Status) Decided() bool {
	return s == StatusAccepted || s == StatusRejected
}