package prism

// Traverser kept for back-compat; prefer Refractor.

// Traverser is an alias of Refractor.
type Traverser = Refractor

// NewTraverser returns a new Refractor (alias).
func NewTraverser(cfg RefractConfig) *Refractor {
    return NewRefractor(cfg)
}