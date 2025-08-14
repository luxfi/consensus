package nova

import (
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/protocol/prism"
)

// Consensus implements the Nova (Snowman) consensus protocol
type Consensus struct {
    prism     *prism.Cut
    splitter  *prism.Splitter
    refract   *prism.Refract
    
    preference ids.ID
    lastAccepted ids.ID
    consecutiveSuccesses int
}

// NewConsensus creates a new Nova consensus instance
func NewConsensus(k int, alphaPreference, alphaConfidence, beta int) *Consensus {
    return &Consensus{
        prism: &prism.Cut{
            AlphaPreference: alphaPreference,
            AlphaConfidence: alphaConfidence,
            Beta:           beta,
        },
        splitter: &prism.Splitter{K: k},
        refract:  &prism.Refract{},
    }
}

// RecordPoll records poll results
func (c *Consensus) RecordPoll(votes map[ids.ID]int) {
    threshold := c.prism.PreferenceThreshold(c.splitter.K)
    
    for id, count := range votes {
        if count >= threshold {
            if id == c.preference {
                c.consecutiveSuccesses++
            } else {
                c.preference = id
                c.consecutiveSuccesses = 1
            }
            break
        }
    }
}

// IsFinalized checks if the preference is finalized
func (c *Consensus) IsFinalized() bool {
    return c.prism.IsConfident(c.consecutiveSuccesses)
}
