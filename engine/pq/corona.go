// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/types"
	"github.com/luxfi/ids"
)

// CoronaPhase represents the phase of the Corona protocol.
type CoronaPhase int

const (
	PhaseIdle CoronaPhase = iota
	PhasePropose
	PhaseCommit
)

// CoronaProposal represents a Phase I proposal.
type CoronaProposal struct {
	ProposerID   ids.NodeID
	VertexID     ids.ID
	Height       uint64
	Timestamp    time.Time
	ProposalHash [32]byte
}

// CoronaCommit represents a Phase II commit.
type CoronaCommit struct {
	VertexID      ids.ID
	ProposalHash  [32]byte
	CommitHash    [32]byte
	Signatures    [][]byte // Post-quantum signatures
	CommitTime    time.Time
}

// Corona implements the 2-phase lattice PQ overlay.
type Corona struct {
	mu              sync.RWMutex
	params          config.Parameters
	nodeID          ids.NodeID
	phase           CoronaPhase
	currentRound    uint64
	
	// Phase I: Propose
	proposals       map[ids.NodeID]*CoronaProposal
	myProposal      *CoronaProposal
	proposalVotes   map[ids.ID]int // Vertex ID -> vote count
	
	// Phase II: Commit  
	commits         map[ids.NodeID]*CoronaCommit
	finalizedVertex ids.ID
	finalizedCommit *CoronaCommit
	
	// Thresholds
	alphaPropose    int // αₚ - proposal threshold
	alphaCommit     int // α𝚌 - commit threshold
}

// NewCorona creates a new Corona PQ instance.
func NewCorona(params config.Parameters, nodeID ids.NodeID) *Corona {
	return &Corona{
		params:       params,
		nodeID:       nodeID,
		phase:        PhaseIdle,
		proposals:    make(map[ids.NodeID]*CoronaProposal),
		proposalVotes: make(map[ids.ID]int),
		commits:      make(map[ids.NodeID]*CoronaCommit),
		alphaPropose: params.AlphaPreference, // Use αₚ for proposal filtering
		alphaCommit:  params.AlphaConfidence, // Use α𝚌 for commit threshold
	}
}

// StartRound begins a new Corona round with the given frontier vertex.
func (r *Corona) StartRound(ctx context.Context, vertex Vertex) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.phase != PhaseIdle {
		return types.ErrWrongPhase
	}

	r.currentRound++
	r.phase = PhasePropose
	
	// Clear previous round data
	r.proposals = make(map[ids.NodeID]*CoronaProposal)
	r.proposalVotes = make(map[ids.ID]int)
	r.commits = make(map[ids.NodeID]*CoronaCommit)
	
	// Create our proposal
	r.myProposal = &CoronaProposal{
		ProposerID: r.nodeID,
		VertexID:   vertex.ID(),
		Height:     vertex.Height(),
		Timestamp:  time.Now(),
	}
	r.myProposal.ProposalHash = r.hashProposal(r.myProposal)
	
	// Add our own proposal
	r.proposals[r.nodeID] = r.myProposal
	r.proposalVotes[vertex.ID()] = 1
	
	return nil
}

// RecordProposal records a Phase I proposal from a peer.
func (r *Corona) RecordProposal(ctx context.Context, proposal *CoronaProposal) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.phase != PhasePropose {
		return types.ErrWrongPhase
	}

	// Verify proposal hash
	expectedHash := r.hashProposal(proposal)
	if proposal.ProposalHash != expectedHash {
		return types.ErrInvalidProposal
	}

	// Record the proposal
	r.proposals[proposal.ProposerID] = proposal
	r.proposalVotes[proposal.VertexID]++

	// Check if we should transition to Phase II
	if r.shouldTransitionToCommit() {
		return r.transitionToCommit(ctx)
	}

	return nil
}

// RecordCommit records a Phase II commit from a peer.
func (r *Corona) RecordCommit(ctx context.Context, commit *CoronaCommit) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.phase != PhaseCommit {
		return types.ErrWrongPhase
	}

	// Verify commit references a valid proposal
	if _, ok := r.proposalVotes[commit.VertexID]; !ok {
		return types.ErrInvalidCommit
	}

	// TODO: Verify post-quantum signatures
	// This would use the luxfi/corona package based on lattigo

	// Record the commit
	r.commits[ids.NodeID{}] = commit // TODO: extract node ID from signature

	// Check if we have enough commits
	commitCount := 0
	for _, c := range r.commits {
		if c.VertexID == commit.VertexID {
			commitCount++
		}
	}

	if commitCount >= r.alphaCommit {
		// We have achieved finality!
		r.finalizedVertex = commit.VertexID
		r.finalizedCommit = commit
		r.phase = PhaseIdle
		return nil
	}

	return nil
}

// GetFinalized returns the finalized vertex ID and commit, if any.
func (r *Corona) GetFinalized() (ids.ID, *CoronaCommit, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.finalizedVertex != ids.Empty {
		return r.finalizedVertex, r.finalizedCommit, true
	}
	return ids.Empty, nil, false
}

// shouldTransitionToCommit checks if we should move to Phase II.
func (r *Corona) shouldTransitionToCommit() bool {
	// Find the highest voted vertex
	maxVotes := 0
	var bestVertex ids.ID
	
	for vtxID, votes := range r.proposalVotes {
		if votes > maxVotes {
			maxVotes = votes
			bestVertex = vtxID
		}
	}

	// Check if it meets the proposal threshold
	return maxVotes >= r.alphaPropose && bestVertex != ids.Empty
}

// transitionToCommit moves from Phase I to Phase II.
func (r *Corona) transitionToCommit(ctx context.Context) error {
	r.phase = PhaseCommit

	// Find the vertex with most votes
	maxVotes := 0
	var bestVertex ids.ID
	
	for vtxID, votes := range r.proposalVotes {
		if votes > maxVotes {
			maxVotes = votes
			bestVertex = vtxID
		}
	}

	// Create our commit
	commit := &CoronaCommit{
		VertexID:     bestVertex,
		ProposalHash: r.myProposal.ProposalHash,
		CommitTime:   time.Now(),
	}
	
	// TODO: Add post-quantum signature using luxfi/corona
	// commit.Signatures = append(commit.Signatures, r.signCommit(commit))
	
	commit.CommitHash = r.hashCommit(commit)
	r.commits[r.nodeID] = commit

	return nil
}

// hashProposal computes the hash of a proposal.
func (r *Corona) hashProposal(p *CoronaProposal) [32]byte {
	h := sha256.New()
	h.Write(p.ProposerID[:])
	h.Write(p.VertexID[:])
	binary.Write(h, binary.BigEndian, p.Height)
	binary.Write(h, binary.BigEndian, p.Timestamp.Unix())
	return sha256.Sum256(h.Sum(nil))
}

// hashCommit computes the hash of a commit.
func (r *Corona) hashCommit(c *CoronaCommit) [32]byte {
	h := sha256.New()
	h.Write(c.VertexID[:])
	h.Write(c.ProposalHash[:])
	binary.Write(h, binary.BigEndian, c.CommitTime.Unix())
	for _, sig := range c.Signatures {
		h.Write(sig)
	}
	return sha256.Sum256(h.Sum(nil))
}

// GetPhase returns the current Corona phase.
func (r *Corona) GetPhase() CoronaPhase {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.phase
}

// GetRound returns the current round number.
func (r *Corona) GetRound() uint64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentRound
}