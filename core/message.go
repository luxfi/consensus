package core

import (
	"context"
	"errors"
	"net/http"

	"github.com/luxfi/trace"
)

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

// Common errors
var (
	ErrTimeout = errors.New("timeout")
)

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
	CreateHandlers(ctx context.Context) (map[string]http.Handler, error)
}

// Engine is a consensus engine
type Engine interface {
	// Start the engine
	Start(ctx context.Context) error
	// Stop the engine
	Stop() error
}

// TraceEngine wraps an engine with tracing
func TraceEngine(engine Engine, tracer trace.Tracer) Engine {
	return engine
}

// Halter can halt operations
type Halter interface {
	Halt(context.Context)
}

// BootstrapableEngine is an engine that can be bootstrapped
type BootstrapableEngine interface {
	Engine
	// Bootstrap the engine
	Bootstrap(ctx context.Context) error
}

// TraceBootstrapableEngine wraps a bootstrappable engine with tracing
func TraceBootstrapableEngine(engine BootstrapableEngine, tracer trace.Tracer) BootstrapableEngine {
	return engine
}
