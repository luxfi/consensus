// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/types"
	"github.com/luxfi/consensus/protocol/focus"
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/ids"
)

// LuxConsensus implements Lux's consensus protocol using Photon → Wave → Focus → Prism → Quasar
type LuxConsensus struct {
	mu sync.RWMutex

	// Configuration
	k     int     // Sample size (K rays for Photon)
	alpha float64 // Threshold ratio
	beta  uint32  // Confidence threshold

	// Protocol components
	wave      *wave.Wave[ids.ID]
	focus     *focus.Confidence[ids.ID]
	prismCut  prism.Cut[ids.ID]
	transport wave.Transport[ids.ID]

	// State tracking
	preference ids.ID
	decided    map[ids.ID]bool
	decisions  map[ids.ID]types.Decision

	// Confidence tracking
	consecutiveSuccesses map[ids.ID]uint32
}

// NewLuxConsensus creates a new Lux consensus instance
func NewLuxConsensus(k int, alpha int, beta int) *LuxConsensus {
	alphaRatio := float64(alpha) / float64(k)

	// Ensure beta is non-negative for uint32 conversion
	if beta < 0 {
		beta = 0
	}
	betaU32 := uint32(beta)

	// Create a simple cut implementation for sampling
	cut := &SimpleCut{k: k}

	// Create a simple transport implementation
	transport := &SimpleTransport{}

	// Create Wave configuration
	waveCfg := wave.Config{
		K:       k,
		Alpha:   alphaRatio,
		Beta:    betaU32,
		RoundTO: 1 * time.Second,
	}

	// Create consensus components
	w := wave.New[ids.ID](waveCfg, cut, transport)
	f := focus.NewConfidence[ids.ID](beta, alphaRatio)

	return &LuxConsensus{
		k:                    k,
		alpha:                alphaRatio,
		beta:                 betaU32,
		wave:                 &w,
		focus:                f,
		prismCut:             cut,
		transport:            transport,
		decided:              make(map[ids.ID]bool),
		decisions:            make(map[ids.ID]types.Decision),
		consecutiveSuccesses: make(map[ids.ID]uint32),
	}
}

// RecordVote records a vote for an item
func (lc *LuxConsensus) RecordVote(item ids.ID) {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	// If already decided, ignore vote
	if lc.decided[item] {
		return
	}

	// Increment consecutive successes
	lc.consecutiveSuccesses[item]++
}

// Poll conducts a consensus poll using Lux protocols
func (lc *LuxConsensus) Poll(responses map[ids.ID]int) bool {
	lc.mu.Lock()
	defer lc.mu.Unlock()

	ctx := context.Background()

	for item, votes := range responses {
		// Skip if already decided
		if lc.decided[item] {
			continue
		}

		// Calculate vote ratio
		totalVotes := 0
		for _, v := range responses {
			totalVotes += v
		}

		if totalVotes == 0 {
			continue
		}

		ratio := float64(votes) / float64(totalVotes)

		// Update Focus confidence tracking
		lc.focus.Update(item, ratio)

		// Check if decision reached
		confidence, decided := lc.focus.State(item)

		if decided {
			lc.decided[item] = true
			if ratio >= lc.alpha {
				lc.decisions[item] = types.DecideAccept
				lc.preference = item
			} else {
				lc.decisions[item] = types.DecideReject
			}
			return false // Stop polling, decision made
		}

		// Use Wave protocol for threshold checking
		lc.wave.Tick(ctx, item)
		state, exists := lc.wave.State(item)
		if exists && state.Decided {
			lc.decided[item] = true
			lc.decisions[item] = state.Result
			if state.Result == types.DecideAccept {
				lc.preference = item
			}
			return false // Stop polling, decision made
		}

		// Update preference based on confidence
		if confidence > 0 && ratio >= lc.alpha {
			lc.preference = item
		}
	}

	// Continue polling if no decision
	return true
}

// Decided returns whether consensus has been reached
func (lc *LuxConsensus) Decided() bool {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	return len(lc.decided) > 0
}

// Preference returns the current preferred item
func (lc *LuxConsensus) Preference() ids.ID {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	return lc.preference
}

// Decision returns the decision for an item
func (lc *LuxConsensus) Decision(item ids.ID) (types.Decision, bool) {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	decision, exists := lc.decisions[item]
	return decision, exists
}

// SimpleCut implements a basic Cut for sampling
type SimpleCut struct {
	k int
}

func (c *SimpleCut) Sample(k int) []types.NodeID {
	// In a real implementation, this would sample from actual network nodes
	// For now, return mock node IDs
	nodes := make([]types.NodeID, k)
	for i := 0; i < k; i++ {
		// Create a proper NodeID - it's actually a ShortID (20-byte array)
		nodes[i] = ids.GenerateTestNodeID()
	}
	return nodes
}

// Luminance implements Cut interface
func (c *SimpleCut) Luminance() prism.Luminance {
	// Return basic luminance for testing
	return prism.Luminance{
		ActivePeers: c.k,
		TotalPeers:  c.k,
		Lx:          float64(c.k), // 1 lx per peer
	}
}

// SimpleTransport implements basic transport for voting
type SimpleTransport struct {
	mu    sync.RWMutex
	votes map[ids.ID]bool
}

func (t *SimpleTransport) RequestVotes(ctx context.Context, peers []types.NodeID, item ids.ID) <-chan wave.Photon[ids.ID] {
	ch := make(chan wave.Photon[ids.ID], len(peers))

	// Simulate vote collection
	go func() {
		defer close(ch)

		for _, peer := range peers {
			select {
			case <-ctx.Done():
				return
			case ch <- wave.Photon[ids.ID]{
				Item:      item,
				Prefer:    true, // Simulate preference
				Sender:    peer,
				Timestamp: time.Now(),
			}:
			}
		}
	}()

	return ch
}

func (t *SimpleTransport) MakeLocalPhoton(item ids.ID, prefer bool) wave.Photon[ids.ID] {
	return wave.Photon[ids.ID]{
		Item:      item,
		Prefer:    prefer,
		Sender:    ids.GenerateTestNodeID(),
		Timestamp: time.Now(),
	}
}

// Parameters returns the consensus parameters
func (lc *LuxConsensus) Parameters() config.Parameters {
	return config.Parameters{
		K:               lc.k,
		Alpha:           lc.alpha,
		Beta:            lc.beta,
		AlphaPreference: int(lc.alpha * float64(lc.k)),
		AlphaConfidence: int(lc.alpha * float64(lc.k)),
	}
}
