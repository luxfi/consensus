package common

// Common engine utilities
type Engine interface {
    Start() error
    Stop() error
}