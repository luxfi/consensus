package wave

// State represents the consensus state for an item
type State[ID comparable] struct {
    // The item ID
    ID ID
    
    // Confidence score
    Confidence int
    
    // Whether the item is finalized
    Finalized bool
    
    // Parent IDs
    Parents []ID
    
    // Height in the consensus graph
    Height uint64
}

// NewState creates a new consensus state
func NewState[ID comparable](id ID) State[ID] {
    return State[ID]{
        ID:         id,
        Confidence: 0,
        Finalized:  false,
        Parents:    []ID{},
        Height:     0,
    }
}

// IsPreferred returns whether this state is preferred
func (s State[ID]) IsPreferred() bool {
    return s.Confidence > 0
}

// IncrementConfidence increments the confidence score
func (s *State[ID]) IncrementConfidence() {
    s.Confidence++
}

// Finalize marks the state as finalized
func (s *State[ID]) Finalize() {
    s.Finalized = true
}