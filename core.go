package consensus

// Fx represents a feature extension
type Fx interface {
	Initialize(interface{}) error
}

// State represents consensus state
type State interface {
	GetTimestamp() int64
	SetTimestamp(int64)
}

// AcceptorGroup manages a group of acceptors
type AcceptorGroup struct {
	// Add fields as needed
}

// NewAcceptorGroup creates a new acceptor group
func NewAcceptorGroup() *AcceptorGroup {
	return &AcceptorGroup{}
}
