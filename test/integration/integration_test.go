package consensus

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/ai"
	"github.com/luxfi/consensus/protocol/quasar"
)

func TestAIConsensusIntegration(t *testing.T) {
	t.Run("AI Agent with Quasar Consensus", func(t *testing.T) {
		// Create Quasar hybrid consensus
		consensus, err := quasar.NewQuasarHybridConsensus(2)
		if err != nil {
			t.Fatalf("Failed to create consensus: %v", err)
		}

		// Add validators
		err = consensus.AddValidator("validator1", 100)
		if err != nil {
			t.Fatalf("Failed to add validator: %v", err)
		}

		err = consensus.AddValidator("validator2", 100)
		if err != nil {
			t.Fatalf("Failed to add validator: %v", err)
		}

		// Create a simple model that implements BasicModel
		model := &testModel{}

		// Create a simple logger for testing
		logger := &testLogger{}

		// Create simple AI agent
		simpleAgent := ai.NewSimple(model, logger)

		// Test agent's various capabilities
		ctx := context.Background()

		// First update chain state so agent knows about the chain
		simpleAgent.UpdateChain("test-chain", 100, "0xabc", []string{"validator1", "validator2"}, &ai.Performance{
			TPS:           1000.0,
			Latency:       20,
			FaultRate:     0.01,
			UpgradeNeeded: false,
		})

		// Test upgrade decision
		decision, err := simpleAgent.ShouldUpgrade(ctx, "test-chain")
		if err != nil {
			t.Fatalf("Failed to make upgrade decision: %v", err)
		}
		if decision == nil {
			t.Fatal("Upgrade decision is nil")
		}

		// Test security check
		secDecision, err := simpleAgent.CheckSecurity(ctx, "test-chain")
		if err != nil {
			t.Fatalf("Failed to check security: %v", err)
		}
		if secDecision == nil {
			t.Fatal("Security decision is nil")
		}

		// Test fork resolution
		forkDecision, err := simpleAgent.ResolveFork(ctx, "test-chain", []string{"fork1", "fork2"})
		if err != nil {
			t.Fatalf("Failed to resolve fork: %v", err)
		}
		if forkDecision == nil {
			t.Fatal("Fork decision is nil")
		}

		// Update chain state again with new height
		simpleAgent.UpdateChain("test-chain", 101, "0xdef", []string{"validator1", "validator2"}, &ai.Performance{
			TPS:           1100.0,
			Latency:       18,
			FaultRate:     0.005,
			UpgradeNeeded: false,
		})

		// Get agent state
		state := simpleAgent.GetState()
		if state == nil {
			t.Fatal("Agent state is nil")
		}

		// Sign message with hybrid consensus
		message := []byte("block_data")
		sig, err := consensus.SignMessage("validator1", message)
		if err != nil {
			t.Fatalf("Failed to sign: %v", err)
		}

		// Verify signature
		if !consensus.VerifyHybridSignature(message, sig) {
			t.Fatal("Signature verification failed")
		}

		// Test threshold with multiple signatures
		sig2, err := consensus.SignMessage("validator2", message)
		if err != nil {
			t.Fatalf("Failed to sign with validator2: %v", err)
		}

		sigs := []*quasar.HybridSignature{sig, sig2}
		aggSig, err := consensus.AggregateSignatures(message, sigs)
		if err != nil {
			t.Fatalf("Failed to aggregate signatures: %v", err)
		}

		if !consensus.VerifyAggregatedSignature(message, aggSig) {
			t.Fatal("Aggregated signature verification failed")
		}
	})

	t.Run("Specialized AI Agents", func(t *testing.T) {
		// Test upgrade agent creation with properly initialized model
		upgradeAgent := ai.NewUpgradeAgent("test-node", nil)
		if upgradeAgent == nil {
			t.Fatal("Failed to create upgrade agent")
		}

		// Test block agent creation
		blockAgent := ai.NewBlockAgent("test-node", nil)
		if blockAgent == nil {
			t.Fatal("Failed to create block agent")
		}

		// Test security agent creation
		securityAgent := ai.NewSecurityAgent("test-node", nil)
		if securityAgent == nil {
			t.Fatal("Failed to create security agent")
		}

		// Test dispute agent creation
		disputeAgent := ai.NewDisputeAgent("test-node", nil)
		if disputeAgent == nil {
			t.Fatal("Failed to create dispute agent")
		}

		// Test cross-chain marketplace
		marketplace := ai.NewComputeMarketplace("test-node", nil, nil)
		if marketplace == nil {
			t.Fatal("Failed to create compute marketplace")
		}
	})

	t.Run("Node Integration", func(t *testing.T) {
		// Test node integration creation with correct config structure
		config := &ai.IntegrationConfig{
			NodeID:       "test-node",
			Enabled:      true,
			ModelPaths:   map[string]string{"default": "/tmp/test-model"},
			SyncInterval: 60 * time.Second,
			LogLevel:     "info",
		}

		integration, err := ai.NewNodeIntegration("test-node", config)
		if err != nil {
			t.Fatalf("Failed to create node integration: %v", err)
		}

		if integration == nil {
			t.Fatal("Integration is nil")
		}

		// Test integration exists (Start/Stop might not be exported)
		// Just verify creation was successful
	})

	t.Run("AI Engine and Builder", func(t *testing.T) {
		// Test engine creation
		engine := ai.NewEngine()
		if engine == nil {
			t.Fatal("Failed to create AI engine")
		}

		// Test builder creation
		builder := ai.NewBuilder()
		if builder == nil {
			t.Fatal("Failed to create AI builder")
		}

		// Build engine (Build returns engine and error)
		builtEngine, err := builder.Build()
		if err != nil {
			t.Fatalf("Failed to build engine: %v", err)
		}
		if builtEngine == nil {
			t.Fatal("Built engine is nil")
		}
	})

	t.Run("AI Modules", func(t *testing.T) {
		// Test inference module
		inferenceModule := ai.NewInferenceModule("inference-1", nil)
		if inferenceModule == nil {
			t.Fatal("Failed to create inference module")
		}

		// Test decision module
		decisionModule := ai.NewDecisionModule("decision-1", nil)
		if decisionModule == nil {
			t.Fatal("Failed to create decision module")
		}

		// Test learning module
		learningModule := ai.NewLearningModule("learning-1", nil)
		if learningModule == nil {
			t.Fatal("Failed to create learning module")
		}

		// Test coordination module
		coordinationModule := ai.NewCoordinationModule("coordination-1", nil)
		if coordinationModule == nil {
			t.Fatal("Failed to create coordination module")
		}
	})
}

// testModel is a simple implementation of BasicModel for testing
type testModel struct{}

func (m *testModel) Decide(ctx context.Context, question string, data map[string]interface{}) (*ai.SimpleDecision, error) {
	return &ai.SimpleDecision{
		Action:     "accept",
		Confidence: 0.9,
		Reasoning:  "test decision",
		Data:       data,
	}, nil
}

// testLogger is a simple implementation of Logger for testing
type testLogger struct{}

func (l *testLogger) Info(msg string, keysAndValues ...interface{})  {}
func (l *testLogger) Error(msg string, keysAndValues ...interface{}) {}
func (l *testLogger) Debug(msg string, keysAndValues ...interface{}) {}
func (l *testLogger) Warn(msg string, keysAndValues ...interface{})  {}
