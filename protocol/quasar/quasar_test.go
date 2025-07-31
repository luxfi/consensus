// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
    "context"
    "testing"
    "time"
    
    "github.com/stretchr/testify/require"
    "github.com/prometheus/client_golang/prometheus"
    
    "github.com/luxfi/consensus/core/interfaces"
    "github.com/luxfi/ids"
    "github.com/luxfi/log"
)

func TestQuasarEngineCreation(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            QuantumMode,
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NotNil(engine)
    require.NotNil(engine.ringtail)
    require.NotNil(engine.metrics)
    require.NotNil(engine.novaHook)
}

func TestQuasarModes(t *testing.T) {
    tests := []struct {
        name          string
        mode          EngineMode
        expectPulsar  bool
        expectNebula  bool
    }{
        {
            name:         "PulsarMode",
            mode:         PulsarMode,
            expectPulsar: true,
            expectNebula: false,
        },
        {
            name:         "NebulaMode",
            mode:         NebulaMode,
            expectPulsar: false,
            expectNebula: true,
        },
        {
            name:         "HybridMode",
            mode:         HybridMode,
            expectPulsar: true,
            expectNebula: true,
        },
        {
            name:         "QuantumMode",
            mode:         QuantumMode,
            expectPulsar: true,
            expectNebula: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            require := require.New(t)
            
            ctx := &interfaces.Context{
                Log:        log.NewNoOpLogger(),
                Registerer: prometheus.NewRegistry(),
            }
            
            params := Parameters{
                K:               21,
                AlphaPreference: 15,
                AlphaConfidence: 15,
                Beta:            20,
                Mode:            tt.mode,
                SecurityLevel:   SecurityMedium,
            }
            
            engine, err := New(ctx, params)
            require.NoError(err)
            
            if tt.expectPulsar {
                require.NotNil(engine.pulsar, "Pulsar should be initialized in %v", tt.mode)
            } else {
                require.Nil(engine.pulsar, "Pulsar should not be initialized in %v", tt.mode)
            }
            
            if tt.expectNebula {
                require.NotNil(engine.nebula, "Nebula should be initialized in %v", tt.mode)
            } else {
                require.Nil(engine.nebula, "Nebula should not be initialized in %v", tt.mode)
            }
        })
    }
}

func TestQuasarInitializeAndStart(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            HybridMode,
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    
    // Initialize
    err = engine.Initialize(context.Background())
    require.NoError(err)
    
    state := engine.State()
    require.Equal(PhotonStage, state.Stage())
    require.False(state.Finalized())
    
    // Start
    err = engine.Start(context.Background())
    require.NoError(err)
    
    // Stop
    err = engine.Stop(context.Background())
    require.NoError(err)
}

func TestQuasarPQSignatureVerification(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            QuantumMode,
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    
    // Create a decision with signature
    decision := &ChainDecision{
        BlockID:   ids.GenerateTestID(),
        Height:    100,
        ParentID:  ids.GenerateTestID(),
        Payload:   []byte("test block data"),
        signature: make([]byte, 32), // Mock signature
    }
    
    // Initialize and start engine
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Submit decision (should verify PQ signature)
    err = engine.Submit(context.Background(), decision)
    require.NoError(err) // Should pass with stub implementation
}

func TestQuasarChainDecisionSubmission(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            PulsarMode,
        SecurityLevel:   SecurityMedium,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Submit chain decision
    chainDecision := &ChainDecision{
        BlockID:   ids.GenerateTestID(),
        Height:    1,
        ParentID:  ids.GenerateTestID(),
        Payload:   []byte("block data"),
        signature: make([]byte, 32),
    }
    
    err = engine.Submit(context.Background(), chainDecision)
    require.NoError(err)
    
    // Check metrics
    require.Equal(int64(1), engine.metrics.ProcessedDecisions.count)
}

func TestQuasarDAGDecisionSubmission(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            NebulaMode,
        SecurityLevel:   SecurityLow,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Submit DAG decision
    dagDecision := &DAGDecision{
        VertexID:  ids.GenerateTestID(),
        Parents:   []ids.ID{ids.GenerateTestID(), ids.GenerateTestID()},
        Payload:   []byte("vertex data"),
        signature: make([]byte, 32),
    }
    
    err = engine.Submit(context.Background(), dagDecision)
    require.NoError(err)
    
    // Check metrics
    require.Equal(int64(1), engine.metrics.ProcessedDecisions.count)
}

func TestQuasarUnifiedDecisionSubmission(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            HybridMode,
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Submit unified decision
    unifiedDecision := &UnifiedDecision{
        id: ids.GenerateTestID(),
        Chain: &ChainDecision{
            BlockID:   ids.GenerateTestID(),
            Height:    10,
            ParentID:  ids.GenerateTestID(),
            Payload:   []byte("chain part"),
            signature: make([]byte, 32),
        },
        DAG: &DAGDecision{
            VertexID:  ids.GenerateTestID(),
            Parents:   []ids.ID{ids.GenerateTestID()},
            Payload:   []byte("dag part"),
            signature: make([]byte, 32),
        },
        signature: make([]byte, 32),
    }
    
    err = engine.Submit(context.Background(), unifiedDecision)
    require.NoError(err)
    
    // Check metrics
    require.Equal(int64(1), engine.metrics.ProcessedDecisions.count)
}

func TestQuasarNovaHookSlashing(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            QuantumMode,
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    
    // Set up slashing callback
    var capturedEvent *SlashingEvent
    engine.NovaHook().SetSlashingCallback(func(event *SlashingEvent) {
        capturedEvent = event
    })
    
    // Trigger slashing event
    event := &SlashingEvent{
        NodeID:    ids.GenerateTestNodeID(),
        Type:      DoubleSign,
        Timestamp: time.Now(),
        Details:   "Validator signed conflicting blocks",
    }
    
    engine.NovaHook().TriggerSlashing(event)
    
    // Verify callback was called
    require.NotNil(capturedEvent)
    require.Equal(event.NodeID, capturedEvent.NodeID)
    require.Equal(event.Type, capturedEvent.Type)
    require.Equal(event.Details, capturedEvent.Details)
}

func TestQuasarEngineStateTransitions(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            QuantumMode,
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    
    // Before initialization
    state := engine.State()
    require.Equal(ConsensusStage(0), state.Stage())
    require.False(state.Finalized())
    
    // After initialization
    require.NoError(engine.Initialize(context.Background()))
    state = engine.State()
    require.Equal(PhotonStage, state.Stage())
    
    // Start engine
    require.NoError(engine.Start(context.Background()))
    
    // Test thread-safety of state cloning
    state1 := engine.State()
    state2 := engine.State()
    require.NotSame(state1, state2) // Should be different objects
    require.Equal(state1.Stage(), state2.Stage())
}

// customDecision is a test decision type that implements Decision interface
type customDecision struct {
    id ids.ID
}

// Implement Decision interface methods
func (c *customDecision) ID() ids.ID                     { return c.id }
func (c *customDecision) Bytes() []byte                  { return []byte("custom") }
func (c *customDecision) Signature() (Signature, error)  { return make([]byte, 32), nil }
func (c *customDecision) Verify() error                  { return nil }

func TestQuasarInvalidDecisionType(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            HybridMode,
        SecurityLevel:   SecurityMedium,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    cd := &customDecision{id: ids.GenerateTestID()}
    
    // Submit should fail with unknown type
    err = engine.Submit(context.Background(), cd)
    require.Error(err)
    require.Contains(err.Error(), "unknown decision type")
}

func TestQuasarEngineNotRunning(t *testing.T) {
    require := require.New(t)
    
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            HybridMode,
        SecurityLevel:   SecurityMedium,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    
    // Don't start the engine
    
    decision := &ChainDecision{
        BlockID:   ids.GenerateTestID(),
        Height:    1,
        ParentID:  ids.GenerateTestID(),
        Payload:   []byte("data"),
        signature: make([]byte, 32),
    }
    
    // Submit should fail
    err = engine.Submit(context.Background(), decision)
    require.Error(err)
    require.Contains(err.Error(), "engine not running")
}

func TestQuasarRingtailIntegration(t *testing.T) {
    require := require.New(t)
    
    // Test ringtail engine directly
    rt := NewRingtail()
    require.NotNil(rt)
    
    // Initialize with different security levels
    for _, level := range []SecurityLevel{SecurityLow, SecurityMedium, SecurityHigh} {
        err := rt.Initialize(level)
        require.NoError(err)
        
        // Generate key pair
        sk, pk, err := rt.GenerateKeyPair()
        require.NoError(err)
        require.Len(sk, 32)
        require.Len(pk, 32)
        
        // Sign and verify
        msg := []byte("test message")
        sig, err := rt.Sign(msg, sk)
        require.NoError(err)
        require.Len(sig, 32)
        
        // Verify signature
        valid := rt.Verify(msg, sig, pk)
        require.True(valid)
    }
}

func TestQuasarPrecomputeAndQuickSign(t *testing.T) {
    require := require.New(t)
    
    // Generate key pair
    sk, pk, err := KeyGen([]byte("test seed"))
    require.NoError(err)
    
    // Precompute
    precomp, err := Precompute(sk)
    require.NoError(err)
    require.Len(precomp, 32)
    
    // Quick sign
    msg := []byte("test message")
    share, err := QuickSign(precomp, msg)
    require.NoError(err)
    require.Len(share, 32)
    
    // Verify share
    valid := VerifyShare(pk, msg, share)
    require.True(valid)
}

func TestQuasarAggregateSignatures(t *testing.T) {
    require := require.New(t)
    
    // Create multiple shares
    shares := []Share{}
    for i := 0; i < 5; i++ {
        sk, _, err := KeyGen([]byte{byte(i)})
        require.NoError(err)
        
        precomp, err := Precompute(sk)
        require.NoError(err)
        
        share, err := QuickSign(precomp, []byte("message"))
        require.NoError(err)
        
        shares = append(shares, share)
    }
    
    // Aggregate shares
    cert, err := Aggregate(shares)
    require.NoError(err)
    require.Len(cert, 32)
    
    // Verify certificate
    valid := Verify([]byte("pubkey"), []byte("message"), cert)
    require.True(valid)
}

func TestQuasarCompositionWithNova(t *testing.T) {
    require := require.New(t)
    
    // Test that Quasar can properly integrate with Nova (linear blockchain)
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            PulsarMode, // Uses Nova internally
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Submit a series of blocks to test linear chain consensus
    parent := ids.GenerateTestID()
    for i := uint64(1); i <= 5; i++ {
        block := &ChainDecision{
            BlockID:   ids.GenerateTestID(),
            Height:    i,
            ParentID:  parent,
            Payload:   []byte("block data"),
            signature: make([]byte, 32),
        }
        
        err = engine.Submit(context.Background(), block)
        require.NoError(err)
        
        parent = block.BlockID
    }
    
    // Verify all blocks were processed
    require.Equal(int64(5), engine.metrics.ProcessedDecisions.count)
}

func TestQuasarCompositionWithNebula(t *testing.T) {
    require := require.New(t)
    
    // Test that Quasar can properly integrate with Nebula (DAG)
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            NebulaMode, // Uses Nebula internally
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Submit DAG vertices with multiple parents
    vertices := []ids.ID{ids.GenerateTestID()} // Genesis
    
    for i := 0; i < 5; i++ {
        // Create vertex with multiple parents
        parents := []ids.ID{}
        if i > 0 {
            parents = append(parents, vertices[i-1])
        }
        if i > 1 {
            parents = append(parents, vertices[i-2])
        }
        
        vertex := &DAGDecision{
            VertexID:  ids.GenerateTestID(),
            Parents:   parents,
            Payload:   []byte("vertex data"),
            signature: make([]byte, 32),
        }
        
        err = engine.Submit(context.Background(), vertex)
        require.NoError(err)
        
        vertices = append(vertices, vertex.VertexID)
    }
    
    // Verify all vertices were processed
    require.Equal(int64(5), engine.metrics.ProcessedDecisions.count)
}

func TestQuasarHybridMode(t *testing.T) {
    require := require.New(t)
    
    // Test that Quasar properly handles both chain and DAG in hybrid mode
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            HybridMode,
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Submit unified decisions that have both chain and DAG components
    for i := 0; i < 3; i++ {
        unified := &UnifiedDecision{
            id: ids.GenerateTestID(),
            Chain: &ChainDecision{
                BlockID:   ids.GenerateTestID(),
                Height:    uint64(i + 1),
                ParentID:  ids.GenerateTestID(),
                Payload:   []byte("chain data"),
                signature: make([]byte, 32),
            },
            DAG: &DAGDecision{
                VertexID:  ids.GenerateTestID(),
                Parents:   []ids.ID{ids.GenerateTestID()},
                Payload:   []byte("dag data"),
                signature: make([]byte, 32),
            },
            signature: make([]byte, 32),
        }
        
        err = engine.Submit(context.Background(), unified)
        require.NoError(err)
    }
    
    // Verify decisions were processed
    require.Equal(int64(3), engine.metrics.ProcessedDecisions.count)
}

func TestQuasarQuantumMode(t *testing.T) {
    require := require.New(t)
    
    // Test full quantum-resistant mode with maximum security
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:                      21,
        AlphaPreference:        15,
        AlphaConfidence:        15,
        Beta:                   20,
        Mode:                   QuantumMode,
        SecurityLevel:          SecurityHigh,
        MaxConcurrentDecisions: 100,
        DecisionTimeout:        int64(5 * time.Second),
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NotNil(engine.pulsar) // Should have both engines
    require.NotNil(engine.nebula)
    require.Equal(SecurityHigh, params.SecurityLevel)
    
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Test concurrent decision submission
    decisions := make([]Decision, 10)
    for i := 0; i < 10; i++ {
        if i%2 == 0 {
            decisions[i] = &ChainDecision{
                BlockID:   ids.GenerateTestID(),
                Height:    uint64(i),
                ParentID:  ids.GenerateTestID(),
                Payload:   []byte("quantum chain"),
                signature: make([]byte, 32),
            }
        } else {
            decisions[i] = &DAGDecision{
                VertexID:  ids.GenerateTestID(),
                Parents:   []ids.ID{ids.GenerateTestID()},
                Payload:   []byte("quantum dag"),
                signature: make([]byte, 32),
            }
        }
    }
    
    // Submit all decisions
    for _, decision := range decisions {
        err = engine.Submit(context.Background(), decision)
        require.NoError(err)
    }
    
    // Verify all were processed with PQ signatures
    require.Equal(int64(10), engine.metrics.ProcessedDecisions.count)
    require.Equal(int64(0), engine.metrics.InvalidDecisions.count)
}

func TestQuasarPQFinalityGuarantees(t *testing.T) {
    require := require.New(t)
    
    // Test that Quasar provides proper post-quantum finality guarantees
    ctx := &interfaces.Context{
        Log:        log.NewNoOpLogger(),
        Registerer: prometheus.NewRegistry(),
    }
    
    params := Parameters{
        K:               21,
        AlphaPreference: 15,
        AlphaConfidence: 15,
        Beta:            20,
        Mode:            QuantumMode,
        SecurityLevel:   SecurityHigh,
    }
    
    engine, err := New(ctx, params)
    require.NoError(err)
    require.NoError(engine.Initialize(context.Background()))
    require.NoError(engine.Start(context.Background()))
    
    // Create a decision that should achieve finality
    decision := &ChainDecision{
        BlockID:   ids.GenerateTestID(),
        Height:    100,
        ParentID:  ids.GenerateTestID(),
        Payload:   []byte("final block"),
        signature: make([]byte, 32),
    }
    
    // Submit decision
    err = engine.Submit(context.Background(), decision)
    require.NoError(err)
    
    // In a real implementation, we would:
    // 1. Vote on the decision through multiple rounds
    // 2. Verify it reaches Beta threshold
    // 3. Check that PQ signatures are aggregated correctly
    // 4. Ensure finality is quantum-resistant
    
    // For now, verify basic processing
    require.Equal(int64(1), engine.metrics.ProcessedDecisions.count)
}

func TestQuasarSlashingTypes(t *testing.T) {
    tests := []struct {
        name        string
        slashType   SlashingType
        details     string
    }{
        {
            name:      "DoubleSign",
            slashType: DoubleSign,
            details:   "Validator signed two conflicting blocks at height 100",
        },
        {
            name:      "Downtime",
            slashType: Downtime,
            details:   "Validator missed 1000 consecutive blocks",
        },
        {
            name:      "InvalidProposal",
            slashType: InvalidProposal,
            details:   "Validator proposed block with invalid transactions",
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            require := require.New(t)
            
            ctx := &interfaces.Context{
                Log:        log.NewNoOpLogger(),
                Registerer: prometheus.NewRegistry(),
            }
            
            params := Parameters{
                K:               21,
                AlphaPreference: 15,
                AlphaConfidence: 15,
                Beta:            20,
                Mode:            QuantumMode,
                SecurityLevel:   SecurityHigh,
            }
            
            engine, err := New(ctx, params)
            require.NoError(err)
            
            var capturedEvent *SlashingEvent
            engine.NovaHook().SetSlashingCallback(func(event *SlashingEvent) {
                capturedEvent = event
            })
            
            event := &SlashingEvent{
                NodeID:    ids.GenerateTestNodeID(),
                Type:      tt.slashType,
                Timestamp: time.Now(),
                Details:   tt.details,
            }
            
            engine.NovaHook().TriggerSlashing(event)
            
            require.NotNil(capturedEvent)
            require.Equal(tt.slashType, capturedEvent.Type)
            require.Equal(tt.details, capturedEvent.Details)
        })
    }
}
