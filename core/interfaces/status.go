package interfaces

// Status represents the status of a decidable item
type Status uint8

const (
    Unknown Status = iota
    Processing
    Rejected
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
        return "Invalid"
    }
}
