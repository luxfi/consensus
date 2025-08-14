package prism

import (
    "math/rand"
    "github.com/luxfi/ids"
)

// Splitter samples K validators from a set
type Splitter struct {
    K int
}

// Sample returns K random validators
func (s *Splitter) Sample(validators []ids.NodeID) []ids.NodeID {
    if len(validators) <= s.K {
        return validators
    }
    
    result := make([]ids.NodeID, s.K)
    perm := rand.Perm(len(validators))
    for i := 0; i < s.K; i++ {
        result[i] = validators[perm[i]]
    }
    return result
}
