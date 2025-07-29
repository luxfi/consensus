// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"fmt"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/beam"
	"github.com/luxfi/consensus/focus"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/wave"
	"github.com/luxfi/consensus/engine/quasar"
	"github.com/luxfi/crypto/ringtail"
)

// Runtime implements a PQ-secured linear chain consensus runtime
type Runtime struct {
	// Consensus stages
	photonStage photon.Dyadic
	waveStage   wave.Dyadic
	focusStage  focus.Dyadic
	beamStage   beam.Consensus

	// Post-quantum security
	ringtail ringtail.Engine

	// Runtime state
	params pq.Parameters
	state  *chainState
}

// New creates a new PQ-secured chain runtime
func New(params pq.Parameters) (*Runtime, error) {
	rt, err := ringtail.New(params.SecurityLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize ringtail: %w", err)
	}

	return &Runtime{
		ringtail: rt,
		params:   params,
		state:    newChainState(),
	}, nil
}

// Initialize sets up the consensus stages
func (r *Runtime) Initialize(ctx context.Context) error {
	// Initialize photon sampling stage
	r.photonStage = photon.NewDyadicPhoton(0) // Start with choice 0

	// Initialize wave thresholding stage
	r.waveStage = wave.NewDyadicWave(
		r.params.AlphaPreference,
		[]wave.TerminationCondition{{
			AlphaConfidence: r.params.AlphaConfidence,
			Beta:            r.params.Beta,
		}},
		0,
	)

	// Initialize focus confidence stage
	r.focusStage = focus.NewDyadicFocus(
		r.params.AlphaPreference,
		[]focus.TerminationCondition{{
			AlphaConfidence: r.params.AlphaConfidence,
			Beta:            r.params.Beta,
		}},
		0,
	)

	// Initialize beam linear finalizer
	beamParams := beam.Parameters{
		K:               r.params.K,
		AlphaPreference: r.params.AlphaPreference,
		AlphaConfidence: r.params.AlphaConfidence,
		Beta:            r.params.Beta,
	}
	r.beamStage = beam.New(beamParams)

	r.state.stage = pq.PhotonStage
	return nil
}

// ProcessBlock processes a block through the consensus stages
func (r *Runtime) ProcessBlock(ctx context.Context, block Block) error {
	// Verify PQ signature
	if err := r.verifyBlockPQ(block); err != nil {
		return fmt.Errorf("PQ verification failed: %w", err)
	}

	// Process through consensus stages
	switch r.state.stage {
	case pq.PhotonStage:
		r.processPhoton(block)
	case pq.WaveStage:
		r.processWave(block)
	case pq.FocusStage:
		r.processFocus(block)
	case pq.FinalizationStage:
		r.processBeam(block)
	}

	return nil
}

// State returns the current runtime state
func (r *Runtime) State() pq.State {
	return r.state
}

// verifyBlockPQ verifies block with post-quantum signature
func (r *Runtime) verifyBlockPQ(block Block) error {
	return r.ringtail.Verify(block.Bytes(), block.Signature())
}

// processPhoton handles sampling stage
func (r *Runtime) processPhoton(block Block) {
	// Sampling logic
	r.photonStage.RecordSuccessfulPoll(block.Choice())
	
	// Check if we should move to wave stage
	if r.shouldTransitionToWave() {
		r.state.stage = pq.WaveStage
	}
}

// processWave handles thresholding stage
func (r *Runtime) processWave(block Block) {
	count := r.countVotes(block)
	r.waveStage.RecordPoll(count, block.Choice())
	
	if r.waveStage.Finalized() {
		r.state.stage = pq.FocusStage
	}
}

// processFocus handles confidence stage
func (r *Runtime) processFocus(block Block) {
	count := r.countVotes(block)
	r.focusStage.RecordPoll(count, block.Choice())
	
	if r.focusStage.Finalized() {
		r.state.stage = pq.FinalizationStage
	}
}

// processBeam handles linear finalization
func (r *Runtime) processBeam(block Block) {
	r.beamStage.Add(block.ID())
	votes := r.collectVotes(block)
	
	if r.beamStage.RecordPoll(votes) && r.beamStage.Finalized() {
		r.state.stage = pq.CompletedStage
		r.state.finalized = true
	}
}

// Block interface for chain blocks
type Block interface {
	ID() ids.ID
	ParentID() ids.ID
	Height() uint64
	Bytes() []byte
	Choice() int
	Signature() ringtail.Signature
}

// chainState tracks runtime state
type chainState struct {
	stage      pq.Stage
	preference ids.ID
	finalized  bool
	confidence map[ids.ID]int
}

func newChainState() *chainState {
	return &chainState{
		stage:      pq.PhotonStage,
		confidence: make(map[ids.ID]int),
	}
}

func (s *chainState) Stage() pq.Stage {
	return s.stage
}

func (s *chainState) Preference() ids.ID {
	return s.preference
}

func (s *chainState) Finalized() bool {
	return s.finalized
}

func (s *chainState) Confidence() map[ids.ID]int {
	return s.confidence
}

// Helper methods
func (r *Runtime) shouldTransitionToWave() bool {
	// Transition logic based on sampling results
	return true // Simplified for example
}

func (r *Runtime) countVotes(block Block) int {
	// Vote counting logic
	return r.params.AlphaPreference // Simplified
}

func (r *Runtime) collectVotes(block Block) bag.Bag[ids.ID] {
	// Vote collection for beam
	votes := bag.New[ids.ID]()
	votes.Add(block.ID())
	return votes
}