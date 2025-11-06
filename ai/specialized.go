// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Specialized AI Agents - Concrete implementations for practical use

package ai

import (
	"context"
	"fmt"
	"math"
	"time"
)

// UpgradeAgent specializes in upgrade decisions
type UpgradeAgent struct {
	*Agent[UpgradeData]
}

// NewUpgradeAgent creates a new upgrade specialist agent
func NewUpgradeAgent(nodeID string, model *SimpleModel[UpgradeData]) *UpgradeAgent {
	agent := New[UpgradeData](nodeID, model, nil, nil)
	return &UpgradeAgent{Agent: agent}
}

// AutonomousUpgrade allows the blockchain to upgrade itself based on AI consensus
func (ua *UpgradeAgent) AutonomousUpgrade(ctx context.Context, currentVersion string, availableUpgrades []UpgradeData) (*Decision[UpgradeData], error) {
	ua.mu.Lock()
	defer ua.mu.Unlock()

	var bestUpgrade UpgradeData
	bestScore := 0.0

	for _, upgrade := range availableUpgrades {
		decision, err := ua.model.Decide(ctx, upgrade, map[string]interface{}{
			"current_version": currentVersion,
			"node_id":         ua.nodeID,
			"type":            "autonomous_upgrade",
		})
		if err != nil {
			continue
		}

		// Weight decision by confidence and network consensus
		score := decision.Confidence * ua.getNetworkConsensus(upgrade)
		if score > bestScore {
			bestScore = score
			bestUpgrade = upgrade
		}
	}

	if bestScore < 0.7 { // Require high confidence for autonomous upgrades
		return nil, fmt.Errorf("insufficient confidence for autonomous upgrade: %.2f", bestScore)
	}

	// Create upgrade decision
	decision := &Decision[UpgradeData]{
		ID:         generateID(),
		Action:     "approve_upgrade",
		Data:       bestUpgrade,
		Confidence: bestScore,
		Reasoning:  fmt.Sprintf("Autonomous upgrade to %s recommended based on AI consensus", bestUpgrade.Version),
		Context:    map[string]interface{}{"autonomous": true, "current_version": currentVersion},
		Timestamp:  time.Now(),
		ProposerID: ua.nodeID,
	}

	return decision, nil
}

func (ua *UpgradeAgent) getNetworkConsensus(upgrade UpgradeData) float64 {
	usageKey := fmt.Sprintf("upgrade_%s", upgrade.Version)
	usage := ua.usage[usageKey]
	return math.Min(1.0, float64(usage)/100.0)
}

// BlockAgent specializes in block and fork decisions
type BlockAgent struct {
	*Agent[BlockData]
}

// NewBlockAgent creates a new block specialist agent
func NewBlockAgent(nodeID string, model *SimpleModel[BlockData]) *BlockAgent {
	agent := New[BlockData](nodeID, model, nil, nil)
	return &BlockAgent{Agent: agent}
}

// ArbitrateFork resolves blockchain forks using AI consensus
func (ba *BlockAgent) ArbitrateFork(ctx context.Context, forkOptions []BlockData) (*Decision[BlockData], error) {
	ba.mu.Lock()
	defer ba.mu.Unlock()

	var bestFork BlockData
	bestScore := 0.0

	for _, fork := range forkOptions {
		decision, err := ba.model.Decide(ctx, fork, map[string]interface{}{
			"node_id": ba.nodeID,
			"type":    "fork_arbitration",
		})
		if err != nil {
			continue
		}

		// Consider network support and fork legitimacy
		networkSupport := ba.evaluateForkSupport(fork)
		score := decision.Confidence * networkSupport

		if score > bestScore {
			bestScore = score
			bestFork = fork
		}
	}

	if bestScore < 0.6 {
		return nil, fmt.Errorf("cannot determine fork winner with confidence: %.2f", bestScore)
	}

	decision := &Decision[BlockData]{
		ID:         generateID(),
		Action:     "choose_fork",
		Data:       bestFork,
		Confidence: bestScore,
		Reasoning:  fmt.Sprintf("Fork at height %d chosen based on AI arbitration", bestFork.Height),
		Context:    map[string]interface{}{"fork_arbitration": true},
		Timestamp:  time.Now(),
		ProposerID: ba.nodeID,
	}

	return decision, nil
}

func (ba *BlockAgent) evaluateForkSupport(fork BlockData) float64 {
	ageBonus := 1.0 - (time.Since(fork.Timestamp).Hours() / 24.0)  // Prefer recent forks
	txBonus := math.Min(1.0, float64(len(fork.Transactions))/10.0) // Prefer active forks
	return (ageBonus + txBonus) / 2.0
}

// SecurityAgent specializes in security threat response
type SecurityAgent struct {
	*Agent[SecurityData]
}

// NewSecurityAgent creates a new security specialist agent
func NewSecurityAgent(nodeID string, model *SimpleModel[SecurityData]) *SecurityAgent {
	agent := New[SecurityData](nodeID, model, nil, nil)
	return &SecurityAgent{Agent: agent}
}

// AutomaticSecurityResponse handles security threats autonomously
func (sa *SecurityAgent) AutomaticSecurityResponse(ctx context.Context, threat SecurityData) (*Decision[SecurityData], error) {
	sa.mu.Lock()
	defer sa.mu.Unlock()

	decision, err := sa.model.Decide(ctx, threat, map[string]interface{}{
		"node_id": sa.nodeID,
		"type":    "security_response",
	})
	if err != nil {
		return nil, fmt.Errorf("security response failed: %w", err)
	}

	urgencyMultiplier := sa.getThreatUrgency(threat.ThreatLevel)
	responseConfidence := decision.Confidence * urgencyMultiplier

	// Auto-execute if threat is critical and confidence is high
	if threat.ThreatLevel == "critical" && responseConfidence > 0.9 {
		if err := sa.executeSecurityResponse(decision.Action, threat); err != nil {
			return nil, fmt.Errorf("auto-execution failed: %w", err)
		}
	}

	response := &Decision[SecurityData]{
		ID:         generateID(),
		Action:     decision.Action,
		Data:       threat,
		Confidence: responseConfidence,
		Reasoning:  fmt.Sprintf("Automatic security response to %s threat", threat.ThreatLevel),
		Context:    map[string]interface{}{"automatic": true, "threat_level": threat.ThreatLevel},
		Timestamp:  time.Now(),
		ProposerID: sa.nodeID,
	}

	return response, nil
}

func (sa *SecurityAgent) getThreatUrgency(threatLevel string) float64 {
	switch threatLevel {
	case "critical":
		return 1.5
	case "high":
		return 1.2
	case "medium":
		return 1.0
	case "low":
		return 0.8
	default:
		return 1.0
	}
}

func (sa *SecurityAgent) executeSecurityResponse(action string, threat SecurityData) error {
	// TODO: Implement actual security response execution
	switch action {
	case "block_node":
		return nil
	case "quarantine":
		return nil
	case "emergency_halt":
		return nil
	default:
		return fmt.Errorf("unknown security action: %s", action)
	}
}

// DisputeAgent specializes in dispute resolution
type DisputeAgent struct {
	*Agent[DisputeData]
}

// NewDisputeAgent creates a new dispute specialist agent
func NewDisputeAgent(nodeID string, model *SimpleModel[DisputeData]) *DisputeAgent {
	agent := New[DisputeData](nodeID, model, nil, nil)
	return &DisputeAgent{Agent: agent}
}

// ResolveDispute handles governance and protocol disputes
func (da *DisputeAgent) ResolveDispute(ctx context.Context, dispute DisputeData) (*Decision[DisputeData], error) {
	da.mu.Lock()
	defer da.mu.Unlock()

	decision, err := da.model.Decide(ctx, dispute, map[string]interface{}{
		"node_id": da.nodeID,
		"type":    "dispute_resolution",
	})
	if err != nil {
		return nil, fmt.Errorf("dispute resolution failed: %w", err)
	}

	if decision.Confidence < 0.8 {
		return nil, fmt.Errorf("insufficient confidence for dispute resolution: %.2f", decision.Confidence)
	}

	networkAgreement := da.validateDisputeResolution(dispute, decision.Action)
	finalConfidence := decision.Confidence * networkAgreement

	resolution := &Decision[DisputeData]{
		ID:         generateID(),
		Action:     decision.Action,
		Data:       dispute,
		Confidence: finalConfidence,
		Reasoning:  fmt.Sprintf("Dispute %s resolved: %s", dispute.Type, decision.Reasoning),
		Context:    map[string]interface{}{"dispute_resolution": true, "network_agreement": networkAgreement},
		Timestamp:  time.Now(),
		ProposerID: da.nodeID,
	}

	return resolution, nil
}

func (da *DisputeAgent) validateDisputeResolution(dispute DisputeData, resolution string) float64 {
	agreementKey := fmt.Sprintf("dispute_%s_%s", dispute.Type, resolution)
	agreement := da.usage[agreementKey]
	return math.Min(1.0, float64(agreement)/50.0)
}
