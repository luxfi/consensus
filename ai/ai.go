// Copyright (C) 2025, Lux Partners Limited All rights reserved.
// AI for Blockchain - Rob Pike Style: Simple, Practical, No Bullshit

package ai

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SimpleAgent does one thing: makes decisions about blockchain operations
type SimpleAgent struct {
	mu    sync.RWMutex
	model BasicModel
	state *State
	log   Logger
}

// BasicModel is the AI that makes decisions
type BasicModel interface {
	// Decide takes context and returns a decision
	Decide(ctx context.Context, question string, data map[string]interface{}) (*SimpleDecision, error)
}

// SimpleDecision is what the AI decided
type SimpleDecision struct {
	Action     string                 `json:"action"`
	Confidence float64                `json:"confidence"`
	Reasoning  string                 `json:"reasoning"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
}

// State tracks what the AI knows
type State struct {
	mu         sync.RWMutex
	chains     map[string]*ChainState
	disputes   map[string]*Dispute
	upgrades   map[string]*Upgrade
	security   *SecurityState
	lastUpdate time.Time
}

// ChainState is what we know about a blockchain
type ChainState struct {
	Height      uint64                 `json:"height"`
	Hash        string                 `json:"hash"`
	Validators  []string               `json:"validators"`
	Performance *Performance           `json:"performance"`
	Issues      []string               `json:"issues"`
	LastSeen    time.Time              `json:"last_seen"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// Performance metrics for the chain
type Performance struct {
	TPS           float64 `json:"tps"`
	Latency       int64   `json:"latency_ms"`
	FaultRate     float64 `json:"fault_rate"`
	UpgradeNeeded bool    `json:"upgrade_needed"`
}

// Dispute represents a fork or conflict that needs resolution
type Dispute struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"` // "fork", "validator", "upgrade", "security"
	ChainID    string    `json:"chain_id"`
	Parties    []string  `json:"parties"`
	Evidence   []string  `json:"evidence"`
	Status     string    `json:"status"` // "open", "resolved", "escalated"
	Resolution string    `json:"resolution,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	ResolvedAt time.Time `json:"resolved_at,omitempty"`
}

// Upgrade represents a potential blockchain upgrade
type Upgrade struct {
	ID          string                 `json:"id"`
	Type        string                 `json:"type"` // "protocol", "vm", "consensus", "security"
	ChainID     string                 `json:"chain_id"`
	Version     string                 `json:"version"`
	Changes     []string               `json:"changes"`
	Risk        string                 `json:"risk"`   // "low", "medium", "high", "critical"
	Status      string                 `json:"status"` // "proposed", "testing", "approved", "deployed"
	TestResults map[string]interface{} `json:"test_results,omitempty"`
	CreatedAt   time.Time              `json:"created_at"`
	DeployedAt  time.Time              `json:"deployed_at,omitempty"`
}

// SecurityState tracks security status
type SecurityState struct {
	ThreatLevel   string            `json:"threat_level"` // "low", "medium", "high", "critical"
	ActiveThreats []string          `json:"active_threats"`
	Mitigations   map[string]string `json:"mitigations"`
	LastScan      time.Time         `json:"last_scan"`
}

// Logger for AI decisions (keep it simple)
type Logger interface {
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
}

// NewSimple creates a simple AI agent
func NewSimple(model BasicModel, logger Logger) *SimpleAgent {
	return &SimpleAgent{
		model: model,
		log:   logger,
		state: &State{
			chains:     make(map[string]*ChainState),
			disputes:   make(map[string]*Dispute),
			upgrades:   make(map[string]*Upgrade),
			security:   &SecurityState{ThreatLevel: "low"},
			lastUpdate: time.Now(),
		},
	}
}

// === PRACTICAL BLOCKCHAIN OPERATIONS ===

// ShouldUpgrade decides if a chain should upgrade
func (a *SimpleAgent) ShouldUpgrade(ctx context.Context, chainID string) (*SimpleDecision, error) {
	a.mu.RLock()
	chain := a.state.chains[chainID]
	a.mu.RUnlock()

	if chain == nil {
		return nil, fmt.Errorf("chain %s not found", chainID)
	}

	data := map[string]interface{}{
		"chain_id":    chainID,
		"height":      chain.Height,
		"performance": chain.Performance,
		"issues":      chain.Issues,
		"last_seen":   chain.LastSeen,
	}

	question := fmt.Sprintf("Should chain %s upgrade based on current performance and issues?", chainID)

	decision, err := a.model.Decide(ctx, question, data)
	if err != nil {
		return nil, fmt.Errorf("upgrade decision failed: %w", err)
	}

	a.log.Info("upgrade decision", "chain", chainID, "action", decision.Action, "confidence", decision.Confidence)
	return decision, nil
}

// ResolveDispute arbitrates conflicts
func (a *SimpleAgent) ResolveDispute(ctx context.Context, disputeID string) (*SimpleDecision, error) {
	a.mu.RLock()
	dispute := a.state.disputes[disputeID]
	a.mu.RUnlock()

	if dispute == nil {
		return nil, fmt.Errorf("dispute %s not found", disputeID)
	}

	data := map[string]interface{}{
		"dispute_id": disputeID,
		"type":       dispute.Type,
		"chain_id":   dispute.ChainID,
		"parties":    dispute.Parties,
		"evidence":   dispute.Evidence,
		"created_at": dispute.CreatedAt,
	}

	question := fmt.Sprintf("How should dispute %s be resolved based on evidence?", disputeID)

	decision, err := a.model.Decide(ctx, question, data)
	if err != nil {
		return nil, fmt.Errorf("dispute resolution failed: %w", err)
	}

	// Update dispute status
	a.mu.Lock()
	if dispute := a.state.disputes[disputeID]; dispute != nil {
		dispute.Status = "resolved"
		dispute.Resolution = decision.Reasoning
		dispute.ResolvedAt = time.Now()
	}
	a.mu.Unlock()

	a.log.Info("dispute resolved", "dispute", disputeID, "resolution", decision.Action)
	return decision, nil
}

// ResolveFork chooses the correct chain in a fork
func (a *SimpleAgent) ResolveFork(ctx context.Context, chainID string, forks []string) (*SimpleDecision, error) {
	data := map[string]interface{}{
		"chain_id":  chainID,
		"forks":     forks,
		"timestamp": time.Now(),
	}

	// Get chain states for each fork
	forkData := make(map[string]*ChainState)
	a.mu.RLock()
	for _, forkID := range forks {
		if state := a.state.chains[forkID]; state != nil {
			forkData[forkID] = state
		}
	}
	a.mu.RUnlock()

	data["fork_states"] = forkData

	question := fmt.Sprintf("Which fork should chain %s follow?", chainID)

	decision, err := a.model.Decide(ctx, question, data)
	if err != nil {
		return nil, fmt.Errorf("fork resolution failed: %w", err)
	}

	a.log.Info("fork resolved", "chain", chainID, "chosen_fork", decision.Action)
	return decision, nil
}

// CheckSecurity assesses security threats
func (a *SimpleAgent) CheckSecurity(ctx context.Context, chainID string) (*SimpleDecision, error) {
	a.mu.RLock()
	chain := a.state.chains[chainID]
	security := a.state.security
	a.mu.RUnlock()

	data := map[string]interface{}{
		"chain_id":       chainID,
		"chain_state":    chain,
		"threat_level":   security.ThreatLevel,
		"active_threats": security.ActiveThreats,
		"last_scan":      security.LastScan,
	}

	question := fmt.Sprintf("What security actions should be taken for chain %s?", chainID)

	decision, err := a.model.Decide(ctx, question, data)
	if err != nil {
		return nil, fmt.Errorf("security check failed: %w", err)
	}

	a.log.Info("security check", "chain", chainID, "action", decision.Action, "threat_level", security.ThreatLevel)
	return decision, nil
}

// === STATE MANAGEMENT ===

// UpdateChain updates what we know about a chain
func (a *SimpleAgent) UpdateChain(chainID string, height uint64, hash string, validators []string, perf *Performance) {
	a.mu.Lock()
	defer a.mu.Unlock()

	chain := a.state.chains[chainID]
	if chain == nil {
		chain = &ChainState{
			Metadata: make(map[string]interface{}),
		}
		a.state.chains[chainID] = chain
	}

	chain.Height = height
	chain.Hash = hash
	chain.Validators = validators
	chain.Performance = perf
	chain.LastSeen = time.Now()

	a.state.lastUpdate = time.Now()
}

// AddDispute registers a new dispute
func (a *SimpleAgent) AddDispute(id, disputeType, chainID string, parties, evidence []string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.disputes[id] = &Dispute{
		ID:        id,
		Type:      disputeType,
		ChainID:   chainID,
		Parties:   parties,
		Evidence:  evidence,
		Status:    "open",
		CreatedAt: time.Now(),
	}
}

// AddUpgrade registers a potential upgrade
func (a *SimpleAgent) AddUpgrade(id, upgradeType, chainID, version string, changes []string, risk string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.upgrades[id] = &Upgrade{
		ID:          id,
		Type:        upgradeType,
		ChainID:     chainID,
		Version:     version,
		Changes:     changes,
		Risk:        risk,
		Status:      "proposed",
		TestResults: make(map[string]interface{}),
		CreatedAt:   time.Now(),
	}
}

// UpdateSecurity updates security state
func (a *SimpleAgent) UpdateSecurity(threatLevel string, threats []string, mitigations map[string]string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.state.security.ThreatLevel = threatLevel
	a.state.security.ActiveThreats = threats
	a.state.security.Mitigations = mitigations
	a.state.security.LastScan = time.Now()
}

// GetState returns current state (read-only)
func (a *SimpleAgent) GetState() *State {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Return a copy to prevent external mutation
	stateCopy := &State{
		chains:     make(map[string]*ChainState),
		disputes:   make(map[string]*Dispute),
		upgrades:   make(map[string]*Upgrade),
		security:   a.state.security,
		lastUpdate: a.state.lastUpdate,
	}

	for k, v := range a.state.chains {
		stateCopy.chains[k] = v
	}
	for k, v := range a.state.disputes {
		stateCopy.disputes[k] = v
	}
	for k, v := range a.state.upgrades {
		stateCopy.upgrades[k] = v
	}

	return stateCopy
}
