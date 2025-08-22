package core

import (
    "fmt"
    "github.com/luxfi/ids"
)

// BootstrapTracker tracks bootstrap status
type BootstrapTracker interface {
    // IsBootstrapped checks if a chain is bootstrapped
    IsBootstrapped(chainID ids.ID) bool
    
    // Bootstrapped marks a chain as bootstrapped
    Bootstrapped(chainID ids.ID)
    
    // RegisterBootstrapper registers a bootstrapper
    RegisterBootstrapper(chainID ids.ID)
    
    // UnregisterBootstrapper unregisters a bootstrapper
    UnregisterBootstrapper(chainID ids.ID)
}

// bootstrapTracker implementation
type bootstrapTracker struct {
    bootstrapped map[ids.ID]bool
}

// NewBootstrapTracker creates a new bootstrap tracker
func NewBootstrapTracker() BootstrapTracker {
    return &bootstrapTracker{
        bootstrapped: make(map[ids.ID]bool),
    }
}

// IsBootstrapped checks if a chain is bootstrapped
func (b *bootstrapTracker) IsBootstrapped(chainID ids.ID) bool {
    return b.bootstrapped[chainID]
}

// Bootstrapped marks a chain as bootstrapped
func (b *bootstrapTracker) Bootstrapped(chainID ids.ID) {
    b.bootstrapped[chainID] = true
}

// RegisterBootstrapper registers a bootstrapper
func (b *bootstrapTracker) RegisterBootstrapper(chainID ids.ID) {
    // Registration logic
}

// UnregisterBootstrapper unregisters a bootstrapper
func (b *bootstrapTracker) UnregisterBootstrapper(chainID ids.ID) {
    delete(b.bootstrapped, chainID)
}

// AppError represents an application error
type AppError struct {
    Code    int32
    Message string
}

// Error implements error interface
func (e *AppError) Error() string {
    return fmt.Sprintf("app error %d: %s", e.Code, e.Message)
}

// NewAppError creates a new app error
func NewAppError(code int32, message string) *AppError {
    return &AppError{
        Code:    code,
        Message: message,
    }
}