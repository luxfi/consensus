// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/luxfi/consensus/config"
)

// NetworkCharacteristics represents the characteristics of a consensus network
type NetworkCharacteristics struct {
	TotalNodes          int
	ExpectedFailureRate float64
	NetworkLatencyMs    int
	TargetFinalityMs    int
	TargetThroughputTPS int
	IsProduction        bool
}

// SafetyLevel represents the safety level of consensus parameters
type SafetyLevel string

const (
	SafetyOptimal  SafetyLevel = "OPTIMAL"
	SafetyGood     SafetyLevel = "GOOD"
	SafetyWarning  SafetyLevel = "WARNING"
	SafetyCritical SafetyLevel = "CRITICAL"
	SafetyDanger   SafetyLevel = "DANGER"
)

// SafetyReport contains the safety analysis results
type SafetyReport struct {
	Level       SafetyLevel
	Issues      []string
	Warnings    []string
	Suggestions []string
	Explanation string
}

// ProbabilityAnalysis contains probability analysis results
type ProbabilityAnalysis struct {
	SafetyFailureProbability   float64
	LivenessFailureProbability float64
	ExpectedRoundsToFinality   float64
	ProbabilityOfDisagreement  float64
}

// CheckerReport contains comprehensive parameter analysis
type CheckerReport struct {
	SafetyCutoff        float64
	ThroughputAnalysis  ThroughputAnalysis
	ConsensusProperties ConsensusProperties
}

// ThroughputAnalysis contains throughput analysis results
type ThroughputAnalysis struct {
	MaxTransactionsPerSecond int
}

// ConsensusProperties contains consensus property analysis
type ConsensusProperties struct {
	// Add fields as needed
}

// ParameterGuide contains parameter guidance information
type ParameterGuide struct {
	Parameter   string
	Description string
	Formula     string
	MinValue    interface{}
	MaxValue    interface{}
	Typical     string
	Impact      string
	TradeOffs   string
}

// CalculateExpectedFinality calculates expected finality time
func CalculateExpectedFinality(p *config.Parameters, networkLatencyMs int) time.Duration {
	roundTime := time.Duration(networkLatencyMs) * time.Millisecond * 2 // Request + response
	totalTime := roundTime * time.Duration(p.Beta)
	return totalTime
}

// CalculateFaultTolerance calculates the fault tolerance of the parameters
func CalculateFaultTolerance(p *config.Parameters) (int, int) {
	// Preference fault tolerance
	prefTolerance := p.K - p.AlphaPreference
	// Confidence fault tolerance
	confTolerance := p.K - p.AlphaConfidence
	return prefTolerance, confTolerance
}

// CalculateOptimalParameters calculates optimal consensus parameters based on network characteristics
func CalculateOptimalParameters(nc NetworkCharacteristics) (*config.Parameters, string) {
	// Calculate K based on network size
	k := calculateK(nc.TotalNodes)

	// Calculate alpha values
	alphaPreference := k/2 + 1
	alphaConfidence := int(math.Ceil(float64(k) * 0.8))
	if alphaConfidence <= alphaPreference {
		alphaConfidence = alphaPreference + 1
	}

	// Calculate beta based on target finality
	beta := calculateBeta(nc.TargetFinalityMs, nc.NetworkLatencyMs)

	// Calculate processing parameters
	concurrentPolls := min(beta, 10)
	maxItemProcessingTime := time.Duration(nc.TargetFinalityMs) * time.Millisecond

	params := &config.Parameters{
		K:                     k,
		AlphaPreference:       alphaPreference,
		AlphaConfidence:       alphaConfidence,
		Beta:                  beta,
		ConcurrentPolls:       concurrentPolls,
		OptimalProcessing:     10,
		MaxOutstandingItems:   256,
		MaxItemProcessingTime: maxItemProcessingTime,
	}

	reasoning := fmt.Sprintf(
		"Calculated parameters for %d nodes with %.1f%% Byzantine tolerance:\n"+
			"- K=%d for adequate sampling\n"+
			"- AlphaPreference=%d (>K/2) for preference changes\n"+
			"- AlphaConfidence=%d for strong confidence\n"+
			"- Beta=%d rounds for %dms finality\n"+
			"- ConcurrentPolls=%d for parallel processing",
		nc.TotalNodes, nc.ExpectedFailureRate*100,
		k, alphaPreference, alphaConfidence, beta, nc.TargetFinalityMs, concurrentPolls)

	return params, reasoning
}

// AnalyzeSafety analyzes the safety of consensus parameters
func AnalyzeSafety(params *config.Parameters, totalNodes int) SafetyReport {
	report := SafetyReport{
		Level:       SafetyGood,
		Issues:      []string{},
		Warnings:    []string{},
		Suggestions: []string{},
	}

	// Check K vs total nodes
	if params.K > totalNodes {
		report.Issues = append(report.Issues, fmt.Sprintf("K (%d) exceeds total nodes (%d)", params.K, totalNodes))
		report.Level = SafetyCritical
	} else if float64(params.K)/float64(totalNodes) > 0.5 {
		report.Warnings = append(report.Warnings, "K is more than 50% of total nodes, may impact performance")
	}

	// Check alpha parameters
	if params.AlphaPreference <= params.K/2 {
		report.Issues = append(report.Issues, "AlphaPreference must be > K/2")
		report.Level = SafetyCritical
	}

	if params.AlphaConfidence < params.AlphaPreference {
		report.Issues = append(report.Issues, "AlphaConfidence must be >= AlphaPreference")
		report.Level = SafetyCritical
	}

	// Check beta
	if params.Beta < 1 {
		report.Issues = append(report.Issues, "Beta must be >= 1")
		report.Level = SafetyCritical
	} else if params.Beta > 100 {
		report.Warnings = append(report.Warnings, "Very high Beta may lead to slow finality")
	}

	// Provide explanation
	if report.Level == SafetyGood || report.Level == SafetyOptimal {
		report.Explanation = "Parameters are well-configured for safe consensus operation"
	}

	return report
}

// AnalyzeProbabilities analyzes probability characteristics of the parameters
func AnalyzeProbabilities(params *config.Parameters, byzantineRatio float64) ProbabilityAnalysis {
	// Simplified probability calculations
	// In reality, these would use complex statistical models

	// Calculate safety failure probability (very rough approximation)
	safetyMargin := float64(params.AlphaConfidence-params.K/2) / float64(params.K)
	safetyFailureProb := math.Pow(byzantineRatio/safetyMargin, float64(params.Beta))

	// Calculate liveness failure probability
	livenessFailureProb := math.Pow(1-byzantineRatio, float64(params.K-params.AlphaPreference+1))

	// Expected rounds to finality
	expectedRounds := float64(params.Beta) * (1 + byzantineRatio)

	// Probability of disagreement
	disagreementProb := byzantineRatio * math.Pow(0.5, float64(params.Beta))

	return ProbabilityAnalysis{
		SafetyFailureProbability:   safetyFailureProb,
		LivenessFailureProbability: livenessFailureProb,
		ExpectedRoundsToFinality:   expectedRounds,
		ProbabilityOfDisagreement:  disagreementProb,
	}
}

// ValidateForProduction validates parameters for production use
func ValidateForProduction(params *config.Parameters, totalNodes int) error {
	if params.K < 5 {
		return fmt.Errorf("K must be at least 5 for production use")
	}

	if float64(params.K)/float64(totalNodes) > 0.7 {
		return fmt.Errorf("K should not exceed 70%% of total nodes in production")
	}

	if params.Beta < 5 {
		return fmt.Errorf("Beta should be at least 5 for production stability")
	}

	return nil
}

// RunChecker runs comprehensive parameter checking
func RunChecker(params *config.Parameters, totalNodes int, networkLatencyMs int) CheckerReport {
	// Calculate safety cutoff (simplified)
	byzantineNodes := params.K - params.AlphaConfidence
	safetyCutoff := float64(byzantineNodes) / float64(params.K) * 100

	// Calculate throughput (simplified)
	roundTime := float64(params.Beta) * float64(networkLatencyMs) / 1000.0 // seconds
	maxTPS := int(1000.0 / roundTime)

	return CheckerReport{
		SafetyCutoff: safetyCutoff,
		ThroughputAnalysis: ThroughputAnalysis{
			MaxTransactionsPerSecond: maxTPS,
		},
		ConsensusProperties: ConsensusProperties{},
	}
}

// FormatCheckerReport formats a checker report for display
func FormatCheckerReport(report CheckerReport, totalNodes int) string {
	return fmt.Sprintf(
		"Parameter Analysis Report\n"+
			"========================\n"+
			"Safety Cutoff: %.1f%% adversarial stake\n"+
			"Max Throughput: %d TPS\n",
		report.SafetyCutoff,
		report.ThroughputAnalysis.MaxTransactionsPerSecond)
}

// GetParameterGuides returns parameter guidance information
func GetParameterGuides() []ParameterGuide {
	return []ParameterGuide{
		{
			Parameter:   "K (Sample Size)",
			Description: "Number of nodes sampled in each consensus round",
			Formula:     "K = ceil(log(N) * 2) for N nodes",
			MinValue:    5,
			MaxValue:    100,
			Typical:     "11-21 for most networks",
			Impact:      "Higher K increases security but reduces performance",
			TradeOffs:   "Security vs. Latency",
		},
		{
			Parameter:   "AlphaPreference",
			Description: "Quorum threshold for preference change",
			Formula:     "AlphaPreference > K/2",
			MinValue:    "K/2 + 1",
			MaxValue:    "K",
			Typical:     "ceil(K * 0.6)",
			Impact:      "Higher values make preference changes harder",
			TradeOffs:   "Stability vs. Adaptability",
		},
		{
			Parameter:   "AlphaConfidence",
			Description: "Quorum threshold for confidence increase",
			Formula:     "AlphaConfidence >= AlphaPreference",
			MinValue:    "AlphaPreference",
			MaxValue:    "K",
			Typical:     "ceil(K * 0.8)",
			Impact:      "Higher values increase safety guarantees",
			TradeOffs:   "Safety vs. Liveness",
		},
		{
			Parameter:   "Beta",
			Description: "Consecutive successful rounds for finalization",
			Formula:     "Beta = ceil(log(1/ε) / log(2))",
			MinValue:    1,
			MaxValue:    100,
			Typical:     "10-20 for strong guarantees",
			Impact:      "Higher Beta increases finality confidence",
			TradeOffs:   "Finality Time vs. Certainty",
		},
	}
}

// Helper functions

func calculateK(totalNodes int) int {
	// Use logarithmic scaling
	k := int(math.Ceil(math.Log(float64(totalNodes)) * 2))

	// Apply bounds
	if k < 5 {
		k = 5
	} else if k > totalNodes {
		k = totalNodes
	} else if k > 100 {
		k = 100
	}

	return k
}

func calculateBeta(targetFinalityMs, networkLatencyMs int) int {
	// Estimate rounds needed
	roundTimeMs := networkLatencyMs * 2 // Request + response
	rounds := targetFinalityMs / roundTimeMs

	// Apply bounds
	if rounds < 5 {
		rounds = 5
	} else if rounds > 50 {
		rounds = 50
	}

	return rounds
}

// Builder-related functions for config package integration

// ConfigToParameters converts Config to Parameters
func ConfigToParameters(cfg *config.Config) *config.Parameters {
	return &config.Parameters{
		K:                     cfg.K,
		AlphaPreference:       cfg.AlphaPreference,
		AlphaConfidence:       cfg.AlphaConfidence,
		Beta:                  cfg.Beta,
		ConcurrentPolls:       cfg.ConcurrentPolls,
		OptimalProcessing:     cfg.OptimalProcessing,
		MaxOutstandingItems:   cfg.MaxOutstandingItems,
		MaxItemProcessingTime: cfg.MaxItemProcessingTime,
	}
}

// ParametersToConfig converts Parameters to Config
func ParametersToConfig(params *config.Parameters) *config.Config {
	return &config.Config{
		K:                     params.K,
		AlphaPreference:       params.AlphaPreference,
		AlphaConfidence:       params.AlphaConfidence,
		Beta:                  params.Beta,
		ConcurrentPolls:       params.ConcurrentPolls,
		OptimalProcessing:     params.OptimalProcessing,
		MaxOutstandingItems:   params.MaxOutstandingItems,
		MaxItemProcessingTime: params.MaxItemProcessingTime,
		MinRoundInterval:      100 * time.Millisecond, // Default
	}
}

// ParametersFromJSON parses parameters from JSON data
func ParametersFromJSON(data []byte) (*config.Parameters, error) {
	params := &config.Parameters{}
	if err := json.Unmarshal(data, params); err != nil {
		return nil, err
	}
	return params, nil
}

// ToJSON converts parameters to JSON
func ToJSON(p *config.Parameters) ([]byte, error) {
	return json.MarshalIndent(p, "", "  ")
}

// Summary returns a human-readable summary of the parameters
func Summary(p *config.Parameters) string {
	return fmt.Sprintf(
		"Consensus Parameters Summary:\n"+
			"  K (Sample Size): %d\n"+
			"  Alpha Preference: %d (%.1f%% of K)\n"+
			"  Alpha Confidence: %d (%.1f%% of K)\n"+
			"  Beta (Rounds): %d\n"+
			"  Concurrent Polls: %d\n"+
			"  Max Processing Time: %v",
		p.K,
		p.AlphaPreference, float64(p.AlphaPreference)/float64(p.K)*100,
		p.AlphaConfidence, float64(p.AlphaConfidence)/float64(p.K)*100,
		p.Beta,
		p.ConcurrentPolls,
		p.MaxItemProcessingTime)
}
