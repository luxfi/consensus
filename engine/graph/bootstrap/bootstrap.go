package bootstrap

import "context"

// Bootstrapper manages graph bootstrapping
type Bootstrapper struct{}

// New creates a new bootstrapper
func New() *Bootstrapper {
    return &Bootstrapper{}
}

// Bootstrap starts the bootstrap process
func (b *Bootstrapper) Bootstrap(ctx context.Context) error {
    return nil
}