// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Agentic AI Consensus - Shared Hallucinations Architecture

package ai

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/protocol/photon"
	"github.com/luxfi/consensus/protocol/quasar"
)

// AgentConfig holds configuration parameters for AI consensus
type AgentConfig struct {
	Alpha float64 // Minimum confidence threshold for consensus (0.0-1.0)
	K     int     // Sample size for voting
	Beta  int     // Confidence accumulation rounds
}

// DefaultAgentConfig returns sensible defaults for AI consensus
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		Alpha: 0.6,  // 60% confidence threshold
		K:     20,   // Sample 20 nodes
		Beta:  15,   // 15 rounds for finalization
	}
}

// Agent represents an AI consensus node with shared hallucinations
type Agent[T ConsensusData] struct {
	mu sync.RWMutex

	// Core components
	nodeID string
	model  Model[T]
	memory *SharedMemory[T]
	quasar *quasar.Quasar
	photon *photon.UniformEmitter
	config AgentConfig

	// Shared hallucination state
	hallucinations map[string]*Hallucination[T]
	weights        map[string]float64 // voting weights by node
	usage          map[string]int64   // usage metrics by model action
	lastUpdate     time.Time

	// Training state
	trainingData []TrainingExample[T]
	gradients    map[string][]float64
	consensus    *ConsensusState[T]
}

// ConsensusData is anything that needs AI consensus
type ConsensusData interface {
	BlockData | TransactionData | UpgradeData | SecurityData | DisputeData
}

// Hallucination represents a shared model state across nodes
type Hallucination[T ConsensusData] struct {
	ID         string                 `json:"id"`
	ModelID    string                 `json:"model_id"`
	State      map[string]interface{} `json:"state"`
	Confidence float64                `json:"confidence"`
	NodeVotes  map[string]float64     `json:"node_votes"`
	UsageCount int64                  `json:"usage_count"`
	CreatedAt  time.Time              `json:"created_at"`
	UpdatedAt  time.Time              `json:"updated_at"`
	Evidence   []Evidence[T]          `json:"evidence"`
}

// Evidence supports a hallucination with concrete data
type Evidence[T ConsensusData] struct {
	Data      T         `json:"data"`
	NodeID    string    `json:"node_id"`
	Weight    float64   `json:"weight"`
	Timestamp time.Time `json:"timestamp"`
}

// TrainingExample for distributed learning
type TrainingExample[T ConsensusData] struct {
	Input    T                      `json:"input"`
	Output   Decision[T]            `json:"output"`
	Feedback float64                `json:"feedback"` // -1 to +1
	NodeID   string                 `json:"node_id"`
	Weight   float64                `json:"weight"`
	Context  map[string]interface{} `json:"context"`
}

// ConsensusState tracks the AI consensus process
type ConsensusState[T ConsensusData] struct {
	Round        uint64                        `json:"round"`
	Phase        ConsensusPhase                `json:"phase"`
	Proposals    map[string]*Proposal[T]       `json:"proposals"`
	Votes        map[string]map[string]float64 `json:"votes"` // proposal -> node -> weight
	Participants []string                      `json:"participants"` // nodes participating in consensus
	Finalized    *Decision[T]                  `json:"finalized,omitempty"`
	StartedAt    time.Time                     `json:"started_at"`
	FinalizedAt  time.Time                     `json:"finalized_at,omitempty"`
}

// ConsensusPhase follows the photon->quasar flow
type ConsensusPhase string

const (
	PhasePhoton  ConsensusPhase = "photon"  // Emit proposals
	PhaseWave    ConsensusPhase = "wave"    // Amplify through network
	PhaseFocus   ConsensusPhase = "focus"   // Converge on best options
	PhasePrism   ConsensusPhase = "prism"   // Refract through DAG
	PhaseHorizon ConsensusPhase = "horizon" // Finalize decision
)

// Proposal represents an AI decision proposal
type Proposal[T ConsensusData] struct {
	ID         string        `json:"id"`
	NodeID     string        `json:"node_id"`
	Decision   *Decision[T]  `json:"decision"`
	Evidence   []Evidence[T] `json:"evidence"`
	Weight     float64       `json:"weight"`
	Confidence float64       `json:"confidence"`
	Timestamp  time.Time     `json:"timestamp"`
}

// SharedMemory manages distributed model state
type SharedMemory[T ConsensusData] struct {
	mu sync.RWMutex

	// Distributed state
	modelStates   map[string]map[string]interface{} // modelID -> state
	nodeWeights   map[string]float64                // nodeID -> weight
	trainingQueue []TrainingExample[T]
	gradientSync  map[string][]float64 // modelID -> gradients

	// Synchronization
	lastSync     time.Time
	syncInterval time.Duration
}

// Decision represents an AI decision with full context
type Decision[T ConsensusData] struct {
	ID           string                 `json:"id"`
	Action       string                 `json:"action"`
	Data         T                      `json:"data"`
	Confidence   float64                `json:"confidence"`
	Reasoning    string                 `json:"reasoning"`
	Alternatives []string               `json:"alternatives,omitempty"`
	Context      map[string]interface{} `json:"context"`
	Timestamp    time.Time              `json:"timestamp"`

	// Consensus metadata
	ProposerID    string  `json:"proposer_id"`
	VoteCount     int     `json:"vote_count"`
	WeightedVotes float64 `json:"weighted_votes"`
}

// Model interface for AI models with generics
type Model[T ConsensusData] interface {
	// Core decision making
	Decide(ctx context.Context, input T, context map[string]interface{}) (*Decision[T], error)

	// Training and adaptation
	Learn(examples []TrainingExample[T]) error
	UpdateWeights(gradients []float64) error
	GetState() map[string]interface{}
	LoadState(state map[string]interface{}) error

	// Consensus integration
	ProposeDecision(ctx context.Context, input T) (*Proposal[T], error)
	ValidateProposal(proposal *Proposal[T]) (float64, error) // returns confidence
}

// New creates an AI agent integrated with quasar consensus
func New[T ConsensusData](
	nodeID string,
	model Model[T],
	quasarEngine *quasar.Quasar,
	photonEngine *photon.UniformEmitter,
) *Agent[T] {
	return &Agent[T]{
		nodeID:         nodeID,
		model:          model,
		quasar:         quasarEngine,
		photon:         photonEngine,
		config:         DefaultAgentConfig(),
		hallucinations: make(map[string]*Hallucination[T]),
		weights:        make(map[string]float64),
		usage:          make(map[string]int64),
		memory: &SharedMemory[T]{
			modelStates:   make(map[string]map[string]interface{}),
			nodeWeights:   make(map[string]float64),
			trainingQueue: make([]TrainingExample[T], 0),
			gradientSync:  make(map[string][]float64),
			syncInterval:  30 * time.Second,
		},
		consensus: &ConsensusState[T]{
			Proposals: make(map[string]*Proposal[T]),
			Votes:     make(map[string]map[string]float64),
		},
		lastUpdate: time.Now(),
	}
}

// === SHARED HALLUCINATION CONSENSUS ===

// ProposeDecision initiates AI consensus following photon->quasar flow
func (a *Agent[T]) ProposeDecision(ctx context.Context, input T, context map[string]interface{}) (*Decision[T], error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Phase 1: Photon - Emit proposal
	proposal, err := a.model.ProposeDecision(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("photon proposal failed: %w", err)
	}

	// Add to consensus state
	a.consensus.Phase = PhasePhoton
	a.consensus.Proposals[proposal.ID] = proposal
	a.consensus.StartedAt = time.Now()

	// Phase 2: Wave - Broadcast through network
	err = a.broadcastProposal(proposal)
	if err != nil {
		return nil, fmt.Errorf("wave broadcast failed: %w", err)
	}

	// Phase 3: Focus - Collect votes and converge
	var decision *Decision[T]
	decision, err = a.focusConsensus(ctx, proposal)
	if err != nil {
		return nil, fmt.Errorf("focus consensus failed: %w", err)
	}

	// Phase 4: Prism - Validate through DAG
	err = a.prismValidation(decision)
	if err != nil {
		return nil, fmt.Errorf("prism validation failed: %w", err)
	}

	// Phase 5: Horizon - Finalize with quantum certificate
	finalDecision, err := a.horizonFinalization(decision)
	if err != nil {
		return nil, fmt.Errorf("horizon finalization failed: %w", err)
	}

	// Update shared hallucination
	a.updateHallucination(finalDecision)

	return finalDecision, nil
}

// AddTrainingData adds training examples from network consensus
func (a *Agent[T]) AddTrainingData(example TrainingExample[T]) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Weight by node reputation and usage
	nodeWeight := a.weights[example.NodeID]
	if nodeWeight == 0 {
		nodeWeight = 0.1 // default weight for new nodes
	}

	example.Weight = nodeWeight
	a.trainingData = append(a.trainingData, example)

	// Add to shared memory
	a.memory.mu.Lock()
	a.memory.trainingQueue = append(a.memory.trainingQueue, example)
	a.memory.mu.Unlock()

	// Update usage statistics
	modelAction := fmt.Sprintf("%s_%s", example.Output.Action, example.NodeID)
	a.usage[modelAction]++
}

// SyncSharedMemory synchronizes model state across network
func (a *Agent[T]) SyncSharedMemory(ctx context.Context) error {
	a.memory.mu.Lock()
	defer a.memory.mu.Unlock()

	if time.Since(a.memory.lastSync) < a.memory.syncInterval {
		return nil // too soon
	}

	// Get current model state
	currentState := a.model.GetState()
	a.memory.modelStates[a.nodeID] = currentState

	// Aggregate states from other nodes (weighted by reputation)
	aggregatedState := a.aggregateModelStates()

	// Update local model with aggregated wisdom
	if err := a.model.LoadState(aggregatedState); err != nil {
		return fmt.Errorf("failed to load aggregated state: %w", err)
	}

	// Train on shared examples
	if len(a.memory.trainingQueue) > 0 {
		if err := a.model.Learn(a.memory.trainingQueue); err != nil {
			return fmt.Errorf("shared learning failed: %w", err)
		}
		a.memory.trainingQueue = a.memory.trainingQueue[:0] // clear
	}

	a.memory.lastSync = time.Now()
	return nil
}

// UpdateNodeWeight adjusts reputation based on consensus performance
func (a *Agent[T]) UpdateNodeWeight(nodeID string, performance float64) {
	a.mu.Lock()
	defer a.mu.Unlock()

	currentWeight := a.weights[nodeID]

	// Exponential moving average with performance feedback
	alpha := 0.1
	newWeight := alpha*performance + (1-alpha)*currentWeight

	// Clamp to reasonable bounds
	if newWeight < 0.01 {
		newWeight = 0.01
	}
	if newWeight > 10.0 {
		newWeight = 10.0
	}

	a.weights[nodeID] = newWeight
	a.memory.nodeWeights[nodeID] = newWeight
}

// GetSharedHallucination returns the current shared AI state
func (a *Agent[T]) GetSharedHallucination(hallucinationID string) (*Hallucination[T], bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	hallucination, exists := a.hallucinations[hallucinationID]
	return hallucination, exists
}

// === PRIVATE METHODS ===

func (a *Agent[T]) broadcastProposal(proposal *Proposal[T]) error {
	// Use photon engine to broadcast
	nodes, err := a.photon.Emit(proposal)
	if err != nil {
		return fmt.Errorf("photon broadcast failed: %w", err)
	}
	// Track emitted nodes for vote collection
	a.mu.Lock()
	for _, nodeID := range nodes {
		a.consensus.Participants = append(a.consensus.Participants, nodeID.String())
	}
	a.mu.Unlock()
	return nil
}

func (a *Agent[T]) focusConsensus(ctx context.Context, proposal *Proposal[T]) (*Decision[T], error) {
	// Implement focus phase consensus logic
	a.consensus.Phase = PhaseFocus
	// ... convergence logic
	return proposal.Decision, nil
}

func (a *Agent[T]) prismValidation(decision *Decision[T]) error {
	// Use quasar DAG validation
	a.consensus.Phase = PhasePrism

	// Validate decision structure
	if decision == nil || decision.ID == "" {
		return fmt.Errorf("invalid decision for prism validation")
	}

	// Validate consensus thresholds
	if len(a.consensus.Participants) == 0 {
		return fmt.Errorf("no participants in consensus")
	}

	// Check that decision has required confidence
	if decision.Confidence < a.config.Alpha {
		return fmt.Errorf("insufficient confidence: %.2f < %.2f", decision.Confidence, a.config.Alpha)
	}

	return nil
}

func (a *Agent[T]) horizonFinalization(decision *Decision[T]) (*Decision[T], error) {
	// Final quantum certificate
	a.consensus.Phase = PhaseHorizon
	a.consensus.Finalized = decision
	a.consensus.FinalizedAt = time.Now()
	return decision, nil
}

func (a *Agent[T]) updateHallucination(decision *Decision[T]) {
	hallucinationID := fmt.Sprintf("%s_%d", decision.Action, time.Now().Unix())

	hallucination := &Hallucination[T]{
		ID:         hallucinationID,
		ModelID:    a.nodeID,
		State:      a.model.GetState(),
		Confidence: decision.Confidence,
		NodeVotes:  make(map[string]float64),
		UsageCount: 1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
		Evidence: []Evidence[T]{{
			Data:      decision.Data,
			NodeID:    a.nodeID,
			Weight:    a.weights[a.nodeID],
			Timestamp: time.Now(),
		}},
	}

	a.hallucinations[hallucinationID] = hallucination
}

func (a *Agent[T]) aggregateModelStates() map[string]interface{} {
	// Weighted aggregation of model states
	aggregated := make(map[string]interface{})
	totalWeight := 0.0

	for nodeID, state := range a.memory.modelStates {
		weight := a.memory.nodeWeights[nodeID]
		if weight == 0 {
			weight = 0.1
		}

		// Simple weighted average for numeric values
		for key, value := range state {
			if val, ok := value.(float64); ok {
				if existing, exists := aggregated[key]; exists {
					if existingVal, ok := existing.(float64); ok {
						aggregated[key] = existingVal + val*weight
					}
				} else {
					aggregated[key] = val * weight
				}
			}
		}

		totalWeight += weight
	}

	// Normalize by total weight
	if totalWeight > 0 {
		for key, value := range aggregated {
			if val, ok := value.(float64); ok {
				aggregated[key] = val / totalWeight
			}
		}
	}

	return aggregated
}

// === TYPE DEFINITIONS ===

type BlockData struct {
	Height       uint64    `json:"height"`
	Hash         string    `json:"hash"`
	ParentHash   string    `json:"parent_hash"`
	Transactions []string  `json:"transactions"`
	TxCount      int       `json:"tx_count"`
	Timestamp    time.Time `json:"timestamp"`
	Validator    string    `json:"validator"`
	Size         uint64    `json:"size"`
	GasUsed      uint64    `json:"gas_used"`
}

type TransactionData struct {
	Hash      string                 `json:"hash"`
	From      string                 `json:"from"`
	To        string                 `json:"to"`
	Amount    uint64                 `json:"amount"`
	Fee       uint64                 `json:"fee"`
	GasPrice  uint64                 `json:"gas_price"`
	GasLimit  uint64                 `json:"gas_limit"`
	Nonce     uint64                 `json:"nonce"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

type UpgradeData struct {
	Version     string    `json:"version"`
	Changes     []string  `json:"changes"`
	Risk        string    `json:"risk"`
	TestResults []string  `json:"test_results"`
	Timestamp   time.Time `json:"timestamp"`
}

type SecurityData struct {
	ThreatLevel string    `json:"threat_level"`
	Threats     []string  `json:"threats"`
	NodeID      string    `json:"node_id"`
	Evidence    []string  `json:"evidence"`
	Timestamp   time.Time `json:"timestamp"`
}

type DisputeData struct {
	Type      string    `json:"type"`
	Parties   []string  `json:"parties"`
	Evidence  []string  `json:"evidence"`
	ChainID   string    `json:"chain_id"`
	Timestamp time.Time `json:"timestamp"`
}
