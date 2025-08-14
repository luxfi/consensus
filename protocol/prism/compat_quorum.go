package prism

// Quorum kept for back-compat; prefer Cut.

// Quorum is an alias of Cut.
type Quorum = Cut

// NewQuorum constructs a Cut (alias).
func NewQuorum(alphaPreference, alphaConfidence, beta int) *Cut {
    return NewCut(alphaPreference, alphaConfidence, beta)
}