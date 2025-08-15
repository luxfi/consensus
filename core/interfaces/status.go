package interfaces

// Status represents consensus status
type Status int

const (
	Unknown Status = iota
	Processing
	Rejected
	Accepted
)

func (s Status) Valid() error {
	if s < Unknown || s > Accepted {
		return ErrInvalidStatus
	}
	return nil
}

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

var ErrInvalidStatus = errStatus{}

type errStatus struct{}

func (errStatus) Error() string { return "invalid status" }
