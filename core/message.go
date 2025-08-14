package core

// Message types for engine communication
type Message int

const (
    PendingTxs Message = iota
    StateSyncDone
)

// AppError represents an application error
type AppError struct {
    Code    int32
    Message string
}

// Fx represents a feature extension
type Fx struct{}

// VM interface (minimal)
type VM interface {
    Initialize() error
    Shutdown() error
}
