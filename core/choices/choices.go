package choices

// Status represents the status of a block
type Status uint8

const (
	Unknown Status = iota
	Processing
	Rejected
	Accepted
)

// String returns string representation
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
		return "Invalid"
	}
}

// Decidable represents a block that can be decided
type Decidable struct {
	Status Status
}
