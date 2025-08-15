package consensus

import (
    "context"
)

// ContextInitializable provides initialization with context
type ContextInitializable interface {
    InitializeWithContext(ctx context.Context) error
}

// Contextualizable is an interface for types that can have their context set
type Contextualizable interface {
    InitCtx(ctx context.Context)
}