package consensus

import (
    "context"
)

// ContextInitializable provides initialization with context
type ContextInitializable interface {
    InitializeWithContext(ctx context.Context, rt *Runtime) error
}

// Contextualizable is an interface for types that can have their runtime set
type Contextualizable interface {
    InitRuntime(rt *Runtime)
}