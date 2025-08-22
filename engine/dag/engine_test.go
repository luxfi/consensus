package dag

import (
	"context"
	"testing"
	
	"github.com/luxfi/ids"
)

func TestNew(t *testing.T) {
	engine := New()
	if engine == nil {
		t.Fatal("New() returned nil")
	}
	
	if engine.IsBootstrapped() {
		t.Error("Engine should not be bootstrapped initially")
	}
}

func TestStart(t *testing.T) {
	engine := New()
	ctx := context.Background()
	
	err := engine.Start(ctx, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	
	if !engine.IsBootstrapped() {
		t.Error("Engine should be bootstrapped after Start")
	}
}

func TestStop(t *testing.T) {
	engine := New()
	ctx := context.Background()
	
	_ = engine.Start(ctx, 1)
	
	err := engine.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestHealthCheck(t *testing.T) {
	engine := New()
	ctx := context.Background()
	
	health, err := engine.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	
	if health == nil {
		t.Error("HealthCheck returned nil")
	}
	
	// Check it returns a map with healthy status
	if m, ok := health.(map[string]interface{}); ok {
		if v, exists := m["healthy"]; !exists || v != true {
			t.Error("Engine should report healthy")
		}
	}
}

func TestGetVertex(t *testing.T) {
	engine := New()
	ctx := context.Background()
	
	// GetVertex should return nil (no-op for now)
	nodeID := ids.EmptyNodeID
	vertexID := ids.Empty
	
	err := engine.GetVertex(ctx, nodeID, 1, vertexID)
	if err != nil {
		t.Errorf("GetVertex should not error: %v", err)
	}
}

func TestDAGWorkflow(t *testing.T) {
	engine := New()
	ctx := context.Background()
	
	// Start engine
	err := engine.Start(ctx, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	
	// Check bootstrapped
	if !engine.IsBootstrapped() {
		t.Error("Should be bootstrapped")
	}
	
	// Health check
	health, err := engine.HealthCheck(ctx)
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}
	if health == nil {
		t.Error("Health should not be nil")
	}
	
	// Stop engine
	err = engine.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}