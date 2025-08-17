package choices

// Status represents the status of a consensus item
type Status uint32

const (
	Unknown Status = iota
	Processing
	Accepted
	Rejected
)

func (s Status) String() string {
	switch s {
	case Unknown:
		return "Unknown"
	case Processing:
		return "Processing"
	case Accepted:
		return "Accepted"
	case Rejected:
		return "Rejected"
	default:
		return "Invalid"
	}
}

// Decidable represents an item that can be decided
type Decidable interface {
	ID() ids.ID
	Accept() error
	Reject() error
	Status() Status
}
