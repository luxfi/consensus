package core

// Message types for engine communication
type Message int

const (
    PendingTxs Message = iota
    StateSyncDone
)

// String implements fmt.Stringer
func (m Message) String() string {
    switch m {
    case PendingTxs:
        return "PendingTxs"
    case StateSyncDone:
        return "StateSyncDone"
    default:
        return "Unknown"
    }
}

// AppError represents an application error
type AppError struct {
    Code    int32
    Message string
}

// Error implements the error interface
func (e *AppError) Error() string {
    return e.Message
}

// Fx represents a feature extension
type Fx struct{}

// VM interface (minimal)
type VM interface {
    Initialize() error
    Shutdown() error
}
