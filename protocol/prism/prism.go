package prism

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"

	"github.com/luxfi/ids"
)

var (
	// ErrNotInitialized is returned when the engine has not been initialized.
	ErrNotInitialized = errors.New("prism: engine not initialized")

	// ErrEmptyData is returned when proposing empty data.
	ErrEmptyData = errors.New("prism: proposal data must not be empty")

	// ErrProposalNotFound is returned when a proposal ID is not in the DAG.
	ErrProposalNotFound = errors.New("prism: proposal not found")

	// ErrAlreadyVoted is returned on duplicate vote.
	ErrAlreadyVoted = errors.New("prism: already voted on proposal")
)

// Prism consensus interface
type Prism interface {
	// Initialize prism consensus
	Initialize(context.Context, *Config) error

	// Propose proposes a value
	Propose(context.Context, []byte) error

	// Vote votes on a proposal
	Vote(context.Context, ids.ID, bool) error

	// GetProposal gets a proposal
	GetProposal(context.Context, ids.ID) (Proposal, error)
}

// NodeConfig defines prism node configuration
type NodeConfig struct {
	ProposerNodes    int
	VoterNodes       int
	TransactionNodes int
	BlockSize        int
}

// Proposal defines a proposal
type Proposal struct {
	ID       ids.ID
	Height   uint64
	Data     []byte
	Votes    int
	Accepted bool
}

// Engine defines prism consensus engine
type Engine struct {
	mu       sync.Mutex
	config   *Config
	metrics  *Metrics
	height   uint64
	vertices map[ids.ID]*Proposal // DAG vertex store
	frontier []ids.ID             // current frontier (tips)
}

// New creates a new prism engine
func New(config *Config) *Engine {
	return &Engine{
		config:   config,
		metrics:  NewMetrics(),
		vertices: make(map[ids.ID]*Proposal),
	}
}

// Initialize initializes the engine
func (e *Engine) Initialize(_ context.Context, config *Config) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.config = config
	e.metrics = NewMetrics()
	e.vertices = make(map[ids.ID]*Proposal)
	e.frontier = nil
	e.height = 0
	return nil
}

// Propose adds a vertex to the DAG frontier and records it.
func (e *Engine) Propose(_ context.Context, data []byte) error {
	if e.config == nil {
		return ErrNotInitialized
	}
	if len(data) == 0 {
		return ErrEmptyData
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Compute vertex ID from height + data
	e.height++
	id := vertexID(e.height, data)

	proposal := &Proposal{
		ID:     id,
		Height: e.height,
		Data:   data,
	}

	e.vertices[id] = proposal
	e.frontier = append(e.frontier, id)

	e.metrics.RecordVertexCreated()
	e.metrics.UpdateDAGStats(uint64(len(e.frontier)), e.height)
	e.metrics.SetPendingVertices(uint64(len(e.frontier)))

	return nil
}

// Vote processes a vote on a proposal. When the vote count reaches
// AlphaConfidence the proposal is finalized and removed from the frontier.
func (e *Engine) Vote(_ context.Context, proposalID ids.ID, accept bool) error {
	if e.config == nil {
		return ErrNotInitialized
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	proposal, ok := e.vertices[proposalID]
	if !ok {
		return ErrProposalNotFound
	}

	if proposal.Accepted {
		return ErrAlreadyVoted
	}

	if accept {
		proposal.Votes++
	}

	// Check finalization threshold
	if proposal.Votes >= e.config.AlphaConfidence {
		proposal.Accepted = true
		e.metrics.RecordVertexFinalized()

		// Remove from frontier
		e.frontier = removeFrontier(e.frontier, proposalID)
		e.metrics.SetPendingVertices(uint64(len(e.frontier)))
		e.metrics.UpdateDAGStats(uint64(len(e.frontier)), e.height)
	}

	return nil
}

// GetProposal gets a proposal
func (e *Engine) GetProposal(_ context.Context, proposalID ids.ID) (Proposal, error) {
	if e.config == nil {
		return Proposal{}, ErrNotInitialized
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	proposal, ok := e.vertices[proposalID]
	if !ok {
		return Proposal{}, ErrProposalNotFound
	}
	return *proposal, nil
}

// Frontier returns the current DAG frontier (tip vertices).
func (e *Engine) Frontier() []ids.ID {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]ids.ID, len(e.frontier))
	copy(out, e.frontier)
	return out
}

// vertexID deterministically derives an ID from height and data.
func vertexID(height uint64, data []byte) ids.ID {
	h := sha256.New()
	var buf [8]byte
	buf[0] = byte(height >> 56)
	buf[1] = byte(height >> 48)
	buf[2] = byte(height >> 40)
	buf[3] = byte(height >> 32)
	buf[4] = byte(height >> 24)
	buf[5] = byte(height >> 16)
	buf[6] = byte(height >> 8)
	buf[7] = byte(height)
	h.Write(buf[:])
	h.Write(data)
	hash := h.Sum(nil)
	var id ids.ID
	copy(id[:], hash)
	return id
}

// removeFrontier removes a single ID from the frontier slice.
func removeFrontier(frontier []ids.ID, id ids.ID) []ids.ID {
	for i, fid := range frontier {
		if fid == id {
			return append(frontier[:i], frontier[i+1:]...)
		}
	}
	return frontier
}
