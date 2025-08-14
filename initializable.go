package consensus

import (
    "context"
)

// ContextInitializable provides initialization with context
type ContextInitializable interface {
    InitializeWithContext(ctx context.Context, chainCtx *Context) error
}