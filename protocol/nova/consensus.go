package nova

import (
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/ids"
)

// Consensus implements the Nova linear chain consensus protocol
type Consensus struct {
	prism   *prism.Cut
	k       int // sample size
	refract *prism.Refractor

	preference           ids.ID
	// lastAccepted         ids.ID // TODO: Use for tracking accepted blocks
	consecutiveSuccesses int
}

// NewConsensus creates a new Nova consensus instance
func NewConsensus(k int, alphaPreference, alphaConfidence, beta int) *Consensus {
	return &Consensus{
		prism: &prism.Cut{
			AlphaPreference: alphaPreference,
			AlphaConfidence: alphaConfidence,
			Beta:            beta,
		},
		k:       k,
		refract: prism.NewRefractor(prism.RefractConfig{}),
	}
}

// RecordPoll records poll results
func (c *Consensus) RecordPoll(votes map[ids.ID]int) {
	threshold := c.prism.PreferenceThreshold(c.k)

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
