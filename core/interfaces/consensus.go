package interfaces

import (
    "context"
    "github.com/luxfi/ids"
)

// Consensus defines the core consensus interface
type Consensus interface {
    Parameters() Parameters
    IsVirtuous(ID ids.ID) bool
    Add(Decidable) error
    RecordPoll(votes []ids.ID) error
    Finalized() bool
}

// Parameters defines consensus parameters
type Parameters interface {
    K() int
    AlphaPreference() int
    AlphaConfidence() int
    Beta() int
}
