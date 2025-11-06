// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// X-Chain Computation Funding - Pay for AI with any chain

package ai

import (
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// ComputeMarketplace handles cross-chain AI computation funding
type ComputeMarketplace struct {
	mu sync.RWMutex

	// Market state
	nodeID          string
	availableCompute *big.Int // Available compute units
	pricePerUnit    *big.Int // Price per compute unit in base currency

	// Cross-chain integration
	xchainBridge    XChainBridge
	supportedChains map[string]*ChainConfig

	// Resource allocation
	activeJobs      map[string]*ComputeJob
	reservedCompute map[string]*big.Int // nodeID -> reserved units

	// Billing and payments
	earnings        map[string]*big.Int // chainID -> total earnings
	lastSettlement  time.Time

	logger Logger
}

// XChainBridge interface for cross-chain communication
type XChainBridge interface {
	// Transfer funds from source chain to pay for compute
	TransferPayment(ctx context.Context, sourceChain, targetChain string, amount *big.Int, recipient string) error

	// Verify payment was received
	VerifyPayment(ctx context.Context, txHash string, expectedAmount *big.Int) (*PaymentProof, error)

	// Get exchange rate between chains
	GetExchangeRate(ctx context.Context, fromChain, toChain string) (*big.Int, error)

	// Submit computation result back to requesting chain
	SubmitResult(ctx context.Context, targetChain string, jobID string, result []byte) error
}

// ChainConfig defines supported chain parameters
type ChainConfig struct {
	ChainID         string   `json:"chain_id"`
	Name            string   `json:"name"`
	NativeCurrency  string   `json:"native_currency"`
	BridgeContract  string   `json:"bridge_contract"`
	MinPayment      *big.Int `json:"min_payment"`
	GasMultiplier   float64  `json:"gas_multiplier"`
	Enabled         bool     `json:"enabled"`
}

// ComputeJob represents a paid AI computation task
type ComputeJob struct {
	ID              string                 `json:"id"`
	SourceChain     string                 `json:"source_chain"`
	Requester       string                 `json:"requester"`
	JobType         string                 `json:"job_type"` // "inference", "training", "consensus"
	Data            map[string]interface{} `json:"data"`
	ComputeUnits    *big.Int              `json:"compute_units"`
	PaymentAmount   *big.Int              `json:"payment_amount"`
	PaymentTxHash   string                `json:"payment_tx_hash"`
	Status          JobStatus             `json:"status"`
	CreatedAt       time.Time             `json:"created_at"`
	StartedAt       time.Time             `json:"started_at,omitempty"`
	CompletedAt     time.Time             `json:"completed_at,omitempty"`
	Result          []byte                `json:"result,omitempty"`
	ErrorMessage    string                `json:"error_message,omitempty"`
}

// JobStatus represents the state of a compute job
type JobStatus string

const (
	JobPending    JobStatus = "pending"
	JobPaid       JobStatus = "paid"
	JobRunning    JobStatus = "running"
	JobCompleted  JobStatus = "completed"
	JobFailed     JobStatus = "failed"
	JobCancelled  JobStatus = "cancelled"
)

// PaymentProof contains verification of cross-chain payment
type PaymentProof struct {
	TxHash      string    `json:"tx_hash"`
	Amount      *big.Int  `json:"amount"`
	FromChain   string    `json:"from_chain"`
	ToChain     string    `json:"to_chain"`
	Sender      string    `json:"sender"`
	Recipient   string    `json:"recipient"`
	BlockHeight uint64    `json:"block_height"`
	Timestamp   time.Time `json:"timestamp"`
	Verified    bool      `json:"verified"`
}

// NewComputeMarketplace creates a new cross-chain compute marketplace
func NewComputeMarketplace(nodeID string, bridge XChainBridge, logger Logger) *ComputeMarketplace {
	return &ComputeMarketplace{
		nodeID:          nodeID,
		availableCompute: big.NewInt(1000000), // 1M compute units initially
		pricePerUnit:    big.NewInt(1000),     // Base price: 1000 units
		xchainBridge:    bridge,
		supportedChains: make(map[string]*ChainConfig),
		activeJobs:      make(map[string]*ComputeJob),
		reservedCompute: make(map[string]*big.Int),
		earnings:        make(map[string]*big.Int),
		lastSettlement:  time.Now(),
		logger:          logger,
	}
}

// AddSupportedChain adds a new chain for cross-chain payments
func (cm *ComputeMarketplace) AddSupportedChain(config *ChainConfig) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.supportedChains[config.ChainID] = config
	cm.earnings[config.ChainID] = big.NewInt(0)

	cm.logger.Info("added supported chain", "chain_id", config.ChainID, "name", config.Name)
	return nil
}

// RequestCompute submits a computation request with cross-chain payment
func (cm *ComputeMarketplace) RequestCompute(ctx context.Context, req *ComputeRequest) (*ComputeJob, error) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	// Validate chain support
	chainConfig, exists := cm.supportedChains[req.SourceChain]
	if !exists {
		return nil, fmt.Errorf("unsupported chain: %s", req.SourceChain)
	}

	// Calculate required compute units and cost
	computeUnits := cm.estimateComputeUnits(req.JobType, req.Data)
	totalCost := new(big.Int).Mul(computeUnits, cm.pricePerUnit)

	// Apply chain-specific pricing
	if chainConfig.GasMultiplier != 1.0 {
		multiplier := big.NewInt(int64(chainConfig.GasMultiplier * 100))
		totalCost.Mul(totalCost, multiplier)
		totalCost.Div(totalCost, big.NewInt(100))
	}

	// Check minimum payment
	if totalCost.Cmp(chainConfig.MinPayment) < 0 {
		totalCost = new(big.Int).Set(chainConfig.MinPayment)
	}

	// Check available compute
	if cm.availableCompute.Cmp(computeUnits) < 0 {
		return nil, fmt.Errorf("insufficient compute available: need %s, have %s",
			computeUnits.String(), cm.availableCompute.String())
	}

	// Create job
	job := &ComputeJob{
		ID:            generateID(),
		SourceChain:   req.SourceChain,
		Requester:     req.Requester,
		JobType:       req.JobType,
		Data:          req.Data,
		ComputeUnits:  computeUnits,
		PaymentAmount: totalCost,
		Status:        JobPending,
		CreatedAt:     time.Now(),
	}

	cm.activeJobs[job.ID] = job

	cm.logger.Info("compute request created",
		"job_id", job.ID,
		"chain", req.SourceChain,
		"compute_units", computeUnits.String(),
		"cost", totalCost.String())

	return job, nil
}

// ProcessPayment verifies and processes cross-chain payment
func (cm *ComputeMarketplace) ProcessPayment(ctx context.Context, jobID, txHash string) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	job, exists := cm.activeJobs[jobID]
	if !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}

	if job.Status != JobPending {
		return fmt.Errorf("job not in pending state: %s", job.Status)
	}

	// Verify payment on source chain
	proof, err := cm.xchainBridge.VerifyPayment(ctx, txHash, job.PaymentAmount)
	if err != nil {
		return fmt.Errorf("payment verification failed: %w", err)
	}

	if !proof.Verified || proof.Amount.Cmp(job.PaymentAmount) < 0 {
		return fmt.Errorf("payment verification failed: insufficient amount")
	}

	// Update job status
	job.PaymentTxHash = txHash
	job.Status = JobPaid

	// Reserve compute resources
	cm.availableCompute.Sub(cm.availableCompute, job.ComputeUnits)
	cm.reservedCompute[jobID] = job.ComputeUnits

	// Record earnings
	cm.earnings[job.SourceChain].Add(cm.earnings[job.SourceChain], job.PaymentAmount)

	cm.logger.Info("payment processed",
		"job_id", jobID,
		"tx_hash", txHash,
		"amount", job.PaymentAmount.String())

	return nil
}

// ExecuteJob runs the paid computation
func (cm *ComputeMarketplace) ExecuteJob(ctx context.Context, jobID string, agent interface{}) error {
	cm.mu.Lock()
	job, exists := cm.activeJobs[jobID]
	if !exists {
		cm.mu.Unlock()
		return fmt.Errorf("job not found: %s", jobID)
	}

	if job.Status != JobPaid {
		cm.mu.Unlock()
		return fmt.Errorf("job not paid: %s", job.Status)
	}

	job.Status = JobRunning
	job.StartedAt = time.Now()
	cm.mu.Unlock()

	// Execute the computation based on job type
	var result []byte
	var err error

	switch job.JobType {
	case "inference":
		result, err = cm.executeInference(ctx, job, agent)
	case "training":
		result, err = cm.executeTraining(ctx, job, agent)
	case "consensus":
		result, err = cm.executeConsensus(ctx, job, agent)
	default:
		err = fmt.Errorf("unsupported job type: %s", job.JobType)
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	if err != nil {
		job.Status = JobFailed
		job.ErrorMessage = err.Error()
		job.CompletedAt = time.Now()

		// Release reserved compute
		cm.availableCompute.Add(cm.availableCompute, cm.reservedCompute[jobID])
		delete(cm.reservedCompute, jobID)

		return fmt.Errorf("job execution failed: %w", err)
	}

	// Job completed successfully
	job.Status = JobCompleted
	job.Result = result
	job.CompletedAt = time.Now()

	// Release reserved compute
	delete(cm.reservedCompute, jobID)

	// Submit result back to requesting chain
	if err := cm.xchainBridge.SubmitResult(ctx, job.SourceChain, jobID, result); err != nil {
		cm.logger.Warn("failed to submit result to chain", "error", err, "job_id", jobID)
	}

	cm.logger.Info("job completed",
		"job_id", jobID,
		"duration", time.Since(job.StartedAt).String())

	return nil
}

// ComputeRequest represents a cross-chain computation request
type ComputeRequest struct {
	SourceChain string                 `json:"source_chain"`
	Requester   string                 `json:"requester"`
	JobType     string                 `json:"job_type"`
	Data        map[string]interface{} `json:"data"`
	MaxPayment  *big.Int              `json:"max_payment"`
}

// estimateComputeUnits calculates required compute for a job
func (cm *ComputeMarketplace) estimateComputeUnits(jobType string, data map[string]interface{}) *big.Int {
	switch jobType {
	case "inference":
		// Base cost for inference
		return big.NewInt(100)
	case "training":
		// Higher cost for training
		return big.NewInt(1000)
	case "consensus":
		// Medium cost for consensus decisions
		return big.NewInt(500)
	default:
		return big.NewInt(100)
	}
}

// executeInference runs AI inference computation
func (cm *ComputeMarketplace) executeInference(ctx context.Context, job *ComputeJob, agent interface{}) ([]byte, error) {
	// TODO: Implement actual inference execution
	// This would integrate with the AI agents to run inference

	// Placeholder: serialize a simple result
	result := map[string]interface{}{
		"job_id": job.ID,
		"type":   "inference_result",
		"data":   "inference completed successfully",
		"confidence": 0.95,
	}

	// Simulate computation time
	time.Sleep(100 * time.Millisecond)

	return []byte(fmt.Sprintf(`{"result": %v}`, result)), nil
}

// executeTraining runs distributed training computation
func (cm *ComputeMarketplace) executeTraining(ctx context.Context, job *ComputeJob, agent interface{}) ([]byte, error) {
	// TODO: Implement actual training execution
	// This would integrate with the shared hallucinations training

	result := map[string]interface{}{
		"job_id": job.ID,
		"type":   "training_result",
		"data":   "training epoch completed",
		"loss":   0.05,
	}

	// Simulate longer computation time for training
	time.Sleep(500 * time.Millisecond)

	return []byte(fmt.Sprintf(`{"result": %v}`, result)), nil
}

// executeConsensus runs consensus decision computation
func (cm *ComputeMarketplace) executeConsensus(ctx context.Context, job *ComputeJob, agent interface{}) ([]byte, error) {
	// TODO: Implement actual consensus execution
	// This would integrate with the agentic consensus system

	result := map[string]interface{}{
		"job_id": job.ID,
		"type":   "consensus_result",
		"action": "approve",
		"confidence": 0.88,
	}

	// Simulate moderate computation time
	time.Sleep(200 * time.Millisecond)

	return []byte(fmt.Sprintf(`{"result": %v}`, result)), nil
}

// GetMarketStats returns marketplace statistics
func (cm *ComputeMarketplace) GetMarketStats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	totalEarnings := big.NewInt(0)
	for _, earnings := range cm.earnings {
		totalEarnings.Add(totalEarnings, earnings)
	}

	activeJobCount := 0
	for _, job := range cm.activeJobs {
		if job.Status == JobRunning || job.Status == JobPaid {
			activeJobCount++
		}
	}

	return map[string]interface{}{
		"node_id":           cm.nodeID,
		"available_compute": cm.availableCompute.String(),
		"price_per_unit":    cm.pricePerUnit.String(),
		"active_jobs":       activeJobCount,
		"total_jobs":        len(cm.activeJobs),
		"total_earnings":    totalEarnings.String(),
		"supported_chains":  len(cm.supportedChains),
		"last_settlement":   cm.lastSettlement,
	}
}

// SettleEarnings processes periodic settlements to chains
func (cm *ComputeMarketplace) SettleEarnings(ctx context.Context) error {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for chainID, earnings := range cm.earnings {
		if earnings.Cmp(big.NewInt(0)) > 0 {
			// TODO: Implement actual settlement to chain
			cm.logger.Info("settling earnings", "chain", chainID, "amount", earnings.String())

			// Reset earnings after settlement
			cm.earnings[chainID] = big.NewInt(0)
		}
	}

	cm.lastSettlement = time.Now()
	return nil
}