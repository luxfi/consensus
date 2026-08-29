// Copyright (C) 2019-2025, Lux Partners Limited All rights reserved.
// See the file LICENSE for licensing terms.

package engine

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/types"
	"github.com/luxfi/consensus/protocol/focus"
	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/consensus/protocol/wave"
	"github.com/luxfi/consensus/protocol/wave/fpc"
	"github.com/luxfi/ids"
)

var ErrNoTransport = errors.New("no real transport configured: SimpleTransport cannot send vote requests over the network")

// Driver implements Lux's consensus protocol using Photon → Wave → Focus → Prism → Quasar
type Driver struct {
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

// NewLuxConsensus creates a new Lux consensus instance with stake-weighted sampling.
// The cut parameter provides the peer sampling strategy (use prism.NewStakeWeightedCut
// for production, or prism.NewUniformCut for testing).
// The transport parameter handles network vote requests.
func NewLuxConsensus(k int, alpha int, beta int, opts ...Option) *Driver {
	// These three bound how much evidence a decision needs, so each is clamped
	// into the range where it still demands some.
	//
	// k = 0 makes alphaRatio a division by zero, and a zero sample size puts the
	// vote threshold at zero.
	//
	// beta < 1 is the sharp one: it was clamped to 0 to make the uint32
	// conversion safe, which fixes the representation and breaks the meaning —
	// wave.Tick decides once Count >= Beta, so Beta = 0 finalises before a single
	// vote is counted. NewLuxConsensus(20, 15, -5) built exactly that.
	if k < 1 {
		k = config.DefaultParams().K
	}
	if beta < 1 {
		beta = 1
	}
	// #nosec G115 -- beta is guaranteed >= 1 above
	betaU32 := uint32(beta)

	alphaRatio := float64(alpha) / float64(k)
	if alphaRatio <= 0 || alphaRatio > 1 {
		alphaRatio = config.ConsensusSuperMajority
	}

	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	// Use provided cut or fall back to SimpleCut.
	var cut prism.Cut[ids.ID]
	if o.cut != nil {
		cut = o.cut
	} else {
		cut = &SimpleCut{k: k}
	}

	// Use provided transport or fall back to SimpleTransport.
	var transport wave.Transport[ids.ID]
	if o.transport != nil {
		transport = o.transport
	} else {
		transport = &SimpleTransport{}
	}

	// The FPC seed is derived, never drawn. A seed a node draws for itself is a
	// seed no other node can reproduce, so two honest validators sampling the
	// same committee demand different majorities of it: at k=20 the threshold
	// landed anywhere in 11..16 across instances. Deriving it from the epoch,
	// the chain and the last finalized parent gives every validator the same
	// threshold sequence, and gives no one it in advance — the parent hash is
	// only known once the previous epoch has finalized.
	fpcSeed := fpc.DeriveEpochSeed(o.epoch, o.chain, o.parent)

	// Create Wave configuration with FPC enabled for dynamic thresholds
	waveCfg := wave.Config{
		K:         k,
		Alpha:     alphaRatio,
		Beta:      betaU32,
		RoundTO:   1 * time.Second,
		EnableFPC: true, // Enable Fast Probabilistic Consensus
		ThetaMin:  0.5,  // FPC minimum threshold
		ThetaMax:  0.8,  // FPC maximum threshold
		FPCSeed:   fpcSeed,
	}

	// Create consensus components
	w, err := wave.New[ids.ID](waveCfg, cut, transport)
	if err != nil {
		panic("failed to create wave: " + err.Error())
	}
	f := focus.NewConfidence[ids.ID](beta, alphaRatio)

	return &Driver{
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

// Option configures Driver construction.
type Option func(*options)

type options struct {
	cut       prism.Cut[ids.ID]
	transport wave.Transport[ids.ID]

	// The epoch the FPC thresholds are drawn for. The zero value is the
	// genesis epoch: epoch 0, no chain, no parent. It is a real epoch and
	// every node computes the same seed from it, which is the property that
	// matters; a chain past genesis is expected to say which epoch it is in.
	epoch  uint64
	chain  []byte
	parent []byte
}

// WithCut sets the peer sampling strategy.
func WithCut(cut prism.Cut[ids.ID]) Option {
	return func(o *options) { o.cut = cut }
}

// WithTransport sets the network transport for vote requests.
func WithTransport(transport wave.Transport[ids.ID]) Option {
	return func(o *options) { o.transport = transport }
}

// WithEpoch names the epoch the FPC thresholds are drawn for: its number, the
// chain it belongs to, and the hash of the last block the previous epoch
// finalized. These three are the whole seed preimage — same three, same
// threshold sequence, on every validator.
func WithEpoch(number uint64, chain, parent []byte) Option {
	return func(o *options) {
		o.epoch, o.chain, o.parent = number, chain, parent
	}
}

// RecordVote records a vote for an item
func (lc *Driver) RecordVote(item ids.ID) {
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
func (lc *Driver) Poll(responses map[ids.ID]int) bool {
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
func (lc *Driver) Decided() bool {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	return len(lc.decided) > 0
}

// Preference returns the current preferred item
func (lc *Driver) Preference() ids.ID {
	lc.mu.RLock()
	defer lc.mu.RUnlock()

	return lc.preference
}

// Decision returns the decision for an item
func (lc *Driver) Decision(item ids.ID) (types.Decision, bool) {
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

func (t *SimpleTransport) RequestVotes(_ context.Context, _ []types.NodeID, _ ids.ID) <-chan wave.Photon[ids.ID] {
	// SimpleTransport has no real network connectivity.
	// Return a closed empty channel so callers see zero votes rather than
	// fabricated "Prefer: true" responses that bypass Sybil resistance.
	ch := make(chan wave.Photon[ids.ID])
	close(ch)
	return ch
}

// Err returns ErrNoTransport because SimpleTransport has no network connectivity.
func (t *SimpleTransport) Err() error {
	return ErrNoTransport
}

func (t *SimpleTransport) MakeLocalPhoton(item ids.ID, prefer bool) wave.Photon[ids.ID] {
	return wave.Photon[ids.ID]{
		Item:      item,
		Prefer:    prefer,
		Sender:    ids.GenerateTestNodeID(),
		Timestamp: time.Now(),
	}
}

// Threshold reports the vote count a round at this phase must see before it
// moves. Under FPC the number is drawn per phase from the epoch seed, so it is
// the number two validators must agree on to agree at all.
func (lc *Driver) Threshold(phase uint64) int {
	return lc.wave.Threshold(phase)
}

// Parameters returns the consensus parameters
func (lc *Driver) Parameters() config.Parameters {
	return config.Parameters{
		K:               lc.k,
		Alpha:           lc.alpha,
		Beta:            lc.beta,
		AlphaPreference: int(lc.alpha * float64(lc.k)),
		AlphaConfidence: int(lc.alpha * float64(lc.k)),
	}
}
