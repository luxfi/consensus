// Package core provides DEPRECATED compatibility shims.
// The old "core" package with engine interfaces has been restructured.
// Core consensus algorithms are now in core/wave, core/prism, etc.
//
// Migration:
//   OLD: import "github.com/luxfi/consensus/core"
//   NEW: import "github.com/luxfi/consensus" (for main API)
//        import "github.com/luxfi/consensus/core/wave" (for algorithms)
package core

// Deprecated: Use github.com/luxfi/consensus.Engine
type Engine interface {
	Deprecated()
}

// Deprecated: This is for compatibility only
func Deprecated() {}

// Context is deprecated but provided for compatibility
type Context struct {
	NodeID string
}

// AppHandler is deprecated - implement in node
type AppHandler interface {
	Deprecated()
}