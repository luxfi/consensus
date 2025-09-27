// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// Node Integration - Connect AI to Lux Node

package ai

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// NodeIntegration connects AI agents to the Lux node
type NodeIntegration struct {
	mu sync.RWMutex

	// Node integration
	nodeID   string
	enabled  bool

	// AI agents for different data types
	blockAgent     *BlockAgent
	txAgent        *Agent[TransactionData]
	upgradeAgent   *UpgradeAgent
	securityAgent  *SecurityAgent
	disputeAgent   *DisputeAgent

	// Integration state
	decisions      map[string]*AnyDecision
	lastUpdate     time.Time
	healthStatus   string

	// Cross-chain compute marketplace
	marketplace *ComputeMarketplace
	bridge      XChainBridge

	// Logging
	logger Logger
}

// AnyDecision wraps decisions from different agent types
type AnyDecision struct {
	Type      string      `json:"type"`
	Decision  interface{} `json:"decision"`
	Timestamp time.Time   `json:"timestamp"`
}

// Config for node integration
type IntegrationConfig struct {
	NodeID         string                  `json:"node_id"`
	Enabled        bool                    `json:"enabled"`
	ModelPaths     map[string]string       `json:"model_paths"`
	SyncInterval   time.Duration           `json:"sync_interval"`
	LogLevel       string                  `json:"log_level"`

	// Cross-chain marketplace config
	EnableMarketplace bool                   `json:"enable_marketplace"`
	SupportedChains   []*ChainConfig         `json:"supported_chains"`
	PricePerUnit      int64                  `json:"price_per_unit"`
	MaxComputeUnits   int64                  `json:"max_compute_units"`
}

// SimpleLogger implements the Logger interface
type SimpleLogger struct{}

func (l *SimpleLogger) Info(msg string, fields ...interface{}) {
	log.Printf("[INFO] %s %v", msg, fields)
}

func (l *SimpleLogger) Warn(msg string, fields ...interface{}) {
	log.Printf("[WARN] %s %v", msg, fields)
}

func (l *SimpleLogger) Error(msg string, fields ...interface{}) {
	log.Printf("[ERROR] %s %v", msg, fields)
}

// NewNodeIntegration creates AI integration for a Lux node
func NewNodeIntegration(nodeID string, config *IntegrationConfig) (*NodeIntegration, error) {
	logger := &SimpleLogger{}

	integration := &NodeIntegration{
		nodeID:       nodeID,
		enabled:      config.Enabled,
		decisions:    make(map[string]*AnyDecision),
		lastUpdate:   time.Now(),
		healthStatus: "healthy",
		logger:       logger,
	}

	// Initialize cross-chain marketplace if enabled
	if config.EnableMarketplace {
		bridge := NewSimpleBridge(nodeID, logger)
		marketplace := NewComputeMarketplace(nodeID, bridge, logger)

		// Add supported chains
		for _, chainConfig := range config.SupportedChains {
			if err := marketplace.AddSupportedChain(chainConfig); err != nil {
				return nil, fmt.Errorf("failed to add chain %s: %w", chainConfig.ChainID, err)
			}

			// Add to bridge
			bridgeChain := &BridgeChain{
				ChainID:  chainConfig.ChainID,
				Name:     chainConfig.Name,
				Currency: chainConfig.NativeCurrency,
				Active:   chainConfig.Enabled,
			}
			if err := bridge.AddChain(bridgeChain); err != nil {
				return nil, fmt.Errorf("failed to add bridge chain %s: %w", chainConfig.ChainID, err)
			}
		}

		integration.marketplace = marketplace
		integration.bridge = bridge
		logger.Info("cross-chain marketplace initialized", "chains", len(config.SupportedChains))
	}

	if config.Enabled {
		if err := integration.initializeAgents(); err != nil {
			return nil, fmt.Errorf("failed to initialize AI agents: %w", err)
		}
	}

	return integration, nil
}

// === BLOCKCHAIN OPERATION HANDLERS ===

// ProcessBlock runs AI consensus on block validation
func (ni *NodeIntegration) ProcessBlock(ctx context.Context, blockHeight uint64, blockHash string) (*Decision[BlockData], error) {
	if !ni.enabled || ni.blockAgent == nil {
		return nil, nil // AI disabled
	}

	// Convert block info to BlockData
	blockData := BlockData{
		Height:    blockHeight,
		Hash:      blockHash,
		Timestamp: time.Now(),
		// Add more fields as needed
	}

	// Get AI decision
	decision, err := ni.blockAgent.ProposeDecision(ctx, blockData, map[string]interface{}{
		"node_id": ni.nodeID,
		"type":    "block_validation",
	})

	if err != nil {
		ni.logger.Error("block AI decision failed", "error", err, "height", blockHeight)
		return nil, err
	}

	// Store decision
	ni.storeDecision("block", decision)

	ni.logger.Info("block AI decision", "height", blockHeight, "action", decision.Action, "confidence", decision.Confidence)
	return decision, nil
}

// ProcessTransaction runs AI consensus on transaction validation
func (ni *NodeIntegration) ProcessTransaction(ctx context.Context, tx interface{}) (*Decision[TransactionData], error) {
	if !ni.enabled || ni.txAgent == nil {
		return nil, nil
	}

	// Convert transaction to TransactionData
	txData := TransactionData{
		Hash:      fmt.Sprintf("%v", tx), // simplified
		Timestamp: time.Now(),
		// Add proper transaction parsing
	}

	decision, err := ni.txAgent.ProposeDecision(ctx, txData, map[string]interface{}{
		"node_id": ni.nodeID,
		"type":    "transaction_validation",
	})

	if err != nil {
		ni.logger.Error("transaction AI decision failed", "error", err)
		return nil, err
	}

	ni.storeDecision("transaction", decision)
	return decision, nil
}

// ProcessUpgrade runs AI consensus on upgrade proposals
func (ni *NodeIntegration) ProcessUpgrade(ctx context.Context, version string, changes []string, risk string) (*Decision[UpgradeData], error) {
	if !ni.enabled || ni.upgradeAgent == nil {
		return nil, nil
	}

	upgradeData := UpgradeData{
		Version:     version,
		Changes:     changes,
		Risk:        risk,
		TestResults: []string{}, // TODO: Add actual test results
		Timestamp:   time.Now(),
	}

	decision, err := ni.upgradeAgent.ProposeDecision(ctx, upgradeData, map[string]interface{}{
		"node_id": ni.nodeID,
		"type":    "upgrade_proposal",
	})

	if err != nil {
		ni.logger.Error("upgrade AI decision failed", "error", err, "version", version)
		return nil, err
	}

	ni.storeDecision("upgrade", decision)

	ni.logger.Info("upgrade AI decision", "version", version, "action", decision.Action, "confidence", decision.Confidence)
	return decision, nil
}

// ProcessSecurity runs AI consensus on security issues
func (ni *NodeIntegration) ProcessSecurity(ctx context.Context, threatLevel string, threats []string) (*Decision[SecurityData], error) {
	if !ni.enabled || ni.securityAgent == nil {
		return nil, nil
	}

	securityData := SecurityData{
		ThreatLevel: threatLevel,
		Threats:     threats,
		NodeID:      ni.nodeID,
		Evidence:    []string{}, // TODO: Add actual evidence
		Timestamp:   time.Now(),
	}

	decision, err := ni.securityAgent.ProposeDecision(ctx, securityData, map[string]interface{}{
		"node_id": ni.nodeID,
		"type":    "security_assessment",
	})

	if err != nil {
		ni.logger.Error("security AI decision failed", "error", err, "threat_level", threatLevel)
		return nil, err
	}

	ni.storeDecision("security", decision)

	ni.logger.Info("security AI decision", "threat_level", threatLevel, "action", decision.Action, "confidence", decision.Confidence)
	return decision, nil
}

// ProcessDispute runs AI consensus on dispute resolution
func (ni *NodeIntegration) ProcessDispute(ctx context.Context, disputeType string, parties []string, evidence []string) (*Decision[DisputeData], error) {
	if !ni.enabled || ni.disputeAgent == nil {
		return nil, nil
	}

	disputeData := DisputeData{
		Type:      disputeType,
		Parties:   parties,
		Evidence:  evidence,
		ChainID:   ni.nodeID, // simplified
		Timestamp: time.Now(),
	}

	decision, err := ni.disputeAgent.ProposeDecision(ctx, disputeData, map[string]interface{}{
		"node_id": ni.nodeID,
		"type":    "dispute_resolution",
	})

	if err != nil {
		ni.logger.Error("dispute AI decision failed", "error", err, "type", disputeType)
		return nil, err
	}

	ni.storeDecision("dispute", decision)

	ni.logger.Info("dispute AI decision", "type", disputeType, "action", decision.Action, "confidence", decision.Confidence)
	return decision, nil
}

// === NETWORK SYNCHRONIZATION ===

// SyncWithNetwork synchronizes AI models across the network
func (ni *NodeIntegration) SyncWithNetwork(ctx context.Context) error {
	if !ni.enabled {
		return nil
	}

	ni.mu.Lock()
	defer ni.mu.Unlock()

	var errors []error

	// Sync each agent type
	if ni.blockAgent != nil {
		if err := ni.blockAgent.SyncSharedMemory(ctx); err != nil {
			errors = append(errors, fmt.Errorf("block agent sync failed: %w", err))
		}
	}

	if ni.txAgent != nil {
		if err := ni.txAgent.SyncSharedMemory(ctx); err != nil {
			errors = append(errors, fmt.Errorf("tx agent sync failed: %w", err))
		}
	}

	if ni.upgradeAgent != nil {
		if err := ni.upgradeAgent.SyncSharedMemory(ctx); err != nil {
			errors = append(errors, fmt.Errorf("upgrade agent sync failed: %w", err))
		}
	}

	if ni.securityAgent != nil {
		if err := ni.securityAgent.SyncSharedMemory(ctx); err != nil {
			errors = append(errors, fmt.Errorf("security agent sync failed: %w", err))
		}
	}

	if ni.disputeAgent != nil {
		if err := ni.disputeAgent.SyncSharedMemory(ctx); err != nil {
			errors = append(errors, fmt.Errorf("dispute agent sync failed: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("sync errors: %v", errors)
	}

	ni.lastUpdate = time.Now()
	ni.logger.Info("AI network sync completed", "node_id", ni.nodeID)
	return nil
}

// AddTrainingFeedback adds feedback for model learning
func (ni *NodeIntegration) AddTrainingFeedback(decisionType string, decisionID string, feedback float64) error {
	if !ni.enabled {
		return nil
	}

	// TODO: Implement training feedback system
	ni.logger.Info("training feedback added", "type", decisionType, "id", decisionID, "feedback", feedback)
	return nil
}

// === STATUS AND CONTROL ===

// IsEnabled returns whether AI is currently enabled
func (ni *NodeIntegration) IsEnabled() bool {
	ni.mu.RLock()
	defer ni.mu.RUnlock()
	return ni.enabled
}

// SetEnabled enables or disables AI functionality
func (ni *NodeIntegration) SetEnabled(enabled bool) error {
	ni.mu.Lock()
	defer ni.mu.Unlock()

	if ni.enabled == enabled {
		return nil // no change
	}

	ni.enabled = enabled

	if enabled {
		if err := ni.initializeAgents(); err != nil {
			ni.enabled = false
			return fmt.Errorf("failed to enable AI: %w", err)
		}
		ni.logger.Info("AI enabled", "node_id", ni.nodeID)
	} else {
		ni.logger.Info("AI disabled", "node_id", ni.nodeID)
	}

	return nil
}

// GetHealthStatus returns the current health status
func (ni *NodeIntegration) GetHealthStatus() string {
	ni.mu.RLock()
	defer ni.mu.RUnlock()
	return ni.healthStatus
}

// GetDecisionHistory returns recent AI decisions
func (ni *NodeIntegration) GetDecisionHistory() map[string]*AnyDecision {
	ni.mu.RLock()
	defer ni.mu.RUnlock()

	// Return a copy
	history := make(map[string]*AnyDecision)
	for k, v := range ni.decisions {
		history[k] = v
	}

	return history
}

// === PRIVATE METHODS ===

func (ni *NodeIntegration) initializeAgents() error {
	// Initialize photon emitter (simplified for now)
	// TODO: Pass actual validator set from engine
	nodes := []interface{}{ni.nodeID} // Convert to proper types
	_ = nodes // Placeholder

	// Initialize specialized agents
	blockExtractor := &BlockFeatureExtractor{}
	blockModel := NewSimpleModel[BlockData](ni.nodeID, blockExtractor)
	ni.blockAgent = NewBlockAgent(ni.nodeID, blockModel)

	// Initialize transaction agent (keep generic for now)
	txExtractor := &TransactionFeatureExtractor{}
	txModel := NewSimpleModel[TransactionData](ni.nodeID, txExtractor)
	ni.txAgent = New[TransactionData](ni.nodeID, txModel, nil, nil)

	// Initialize specialized upgrade agent
	upgradeExtractor := &UpgradeFeatureExtractor{}
	upgradeModel := NewSimpleModel[UpgradeData](ni.nodeID, upgradeExtractor)
	ni.upgradeAgent = NewUpgradeAgent(ni.nodeID, upgradeModel)

	// Initialize specialized security agent
	securityExtractor := &SecurityFeatureExtractor{}
	securityModel := NewSimpleModel[SecurityData](ni.nodeID, securityExtractor)
	ni.securityAgent = NewSecurityAgent(ni.nodeID, securityModel)

	// Initialize specialized dispute agent
	disputeExtractor := &DisputeFeatureExtractor{}
	disputeModel := NewSimpleModel[DisputeData](ni.nodeID, disputeExtractor)
	ni.disputeAgent = NewDisputeAgent(ni.nodeID, disputeModel)

	return nil
}

func (ni *NodeIntegration) storeDecision(decisionType string, decision interface{}) {
	key := fmt.Sprintf("%s_%d", decisionType, time.Now().UnixNano())

	ni.mu.Lock()
	defer ni.mu.Unlock()

	ni.decisions[key] = &AnyDecision{
		Type:      decisionType,
		Decision:  decision,
		Timestamp: time.Now(),
	}

	// Keep only recent decisions
	if len(ni.decisions) > 1000 {
		// Remove oldest decisions
		oldest := time.Now().Add(-24 * time.Hour)
		for k, v := range ni.decisions {
			if v.Timestamp.Before(oldest) {
				delete(ni.decisions, k)
			}
		}
	}
}

// === ADDITIONAL FEATURE EXTRACTORS ===

type SecurityFeatureExtractor struct{}

func (e *SecurityFeatureExtractor) Extract(data SecurityData) map[string]float64 {
	threatScore := 0.0
	switch data.ThreatLevel {
	case "low":
		threatScore = 0.25
	case "medium":
		threatScore = 0.5
	case "high":
		threatScore = 0.75
	case "critical":
		threatScore = 1.0
	}

	return map[string]float64{
		"threat_score":    threatScore,
		"threat_count":    float64(len(data.Threats)),
		"evidence_count":  float64(len(data.Evidence)),
		"age_minutes":     time.Since(data.Timestamp).Minutes(),
		"node_entropy":    addressEntropy(data.NodeID),
	}
}

func (e *SecurityFeatureExtractor) Names() []string {
	return []string{"threat_score", "threat_count", "evidence_count", "age_minutes", "node_entropy"}
}

type DisputeFeatureExtractor struct{}

func (e *DisputeFeatureExtractor) Extract(data DisputeData) map[string]float64 {
	typeScore := 0.0
	switch data.Type {
	case "fork":
		typeScore = 0.8
	case "validator":
		typeScore = 0.6
	case "upgrade":
		typeScore = 0.4
	case "security":
		typeScore = 1.0
	}

	return map[string]float64{
		"type_score":      typeScore,
		"party_count":     float64(len(data.Parties)),
		"evidence_count":  float64(len(data.Evidence)),
		"age_hours":       time.Since(data.Timestamp).Hours(),
		"chain_entropy":   addressEntropy(data.ChainID),
	}
}

func (e *DisputeFeatureExtractor) Names() []string {
	return []string{"type_score", "party_count", "evidence_count", "age_hours", "chain_entropy"}
}

// === CROSS-CHAIN COMPUTE MARKETPLACE ===

// OfferCompute makes this node available for cross-chain AI computation
func (ni *NodeIntegration) OfferCompute(ctx context.Context, req *ComputeRequest) (*ComputeJob, error) {
	if ni.marketplace == nil {
		return nil, fmt.Errorf("marketplace not enabled")
	}

	job, err := ni.marketplace.RequestCompute(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("compute request failed: %w", err)
	}

	ni.logger.Info("compute job offered", "job_id", job.ID, "chain", req.SourceChain)
	return job, nil
}

// ProcessComputePayment verifies payment and starts computation
func (ni *NodeIntegration) ProcessComputePayment(ctx context.Context, jobID, txHash string) error {
	if ni.marketplace == nil {
		return fmt.Errorf("marketplace not enabled")
	}

	if err := ni.marketplace.ProcessPayment(ctx, jobID, txHash); err != nil {
		return fmt.Errorf("payment processing failed: %w", err)
	}

	// Start the computation job
	go func() {
		if err := ni.executeComputeJob(ctx, jobID); err != nil {
			ni.logger.Error("compute job execution failed", "job_id", jobID, "error", err)
		}
	}()

	return nil
}

// executeComputeJob runs the paid AI computation
func (ni *NodeIntegration) executeComputeJob(ctx context.Context, jobID string) error {
	if ni.marketplace == nil {
		return fmt.Errorf("marketplace not enabled")
	}

	// Choose appropriate agent based on job type
	var agent interface{}

	// For now, use the first available agent
	// TODO: Select agent based on job requirements
	if ni.blockAgent != nil {
		agent = ni.blockAgent
	} else if ni.upgradeAgent != nil {
		agent = ni.upgradeAgent
	} else if ni.securityAgent != nil {
		agent = ni.securityAgent
	} else if ni.disputeAgent != nil {
		agent = ni.disputeAgent
	} else {
		return fmt.Errorf("no suitable agent available")
	}

	return ni.marketplace.ExecuteJob(ctx, jobID, agent)
}

// GetMarketplaceStats returns marketplace statistics
func (ni *NodeIntegration) GetMarketplaceStats() map[string]interface{} {
	if ni.marketplace == nil {
		return map[string]interface{}{
			"enabled": false,
		}
	}

	stats := ni.marketplace.GetMarketStats()
	stats["enabled"] = true

	if ni.bridge != nil {
		bridgeStats := ni.bridge.(*SimpleBridge).GetBridgeStats()
		stats["bridge"] = bridgeStats
	}

	return stats
}

// SettleMarketplaceEarnings processes cross-chain earnings settlement
func (ni *NodeIntegration) SettleMarketplaceEarnings(ctx context.Context) error {
	if ni.marketplace == nil {
		return fmt.Errorf("marketplace not enabled")
	}

	return ni.marketplace.SettleEarnings(ctx)
}

// RequestCrossChainCompute requests AI computation from another chain
func (ni *NodeIntegration) RequestCrossChainCompute(ctx context.Context, targetChain string, req *ComputeRequest) (*ComputeJob, error) {
	if ni.bridge == nil {
		return nil, fmt.Errorf("cross-chain bridge not enabled")
	}

	// TODO: Implement client-side compute requests
	// This would involve:
	// 1. Finding available compute providers on target chain
	// 2. Negotiating price and terms
	// 3. Making payment through bridge
	// 4. Waiting for results

	ni.logger.Info("cross-chain compute request", "target_chain", targetChain, "type", req.JobType)

	// Placeholder implementation
	return &ComputeJob{
		ID:            generateID(),
		SourceChain:   req.SourceChain,
		Requester:     req.Requester,
		JobType:       req.JobType,
		Data:          req.Data,
		Status:        JobPending,
		CreatedAt:     time.Now(),
	}, nil
}