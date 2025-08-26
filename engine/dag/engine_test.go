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
}

func TestStart(t *testing.T) {
	engine := New()
	ctx := context.Background()

	err := engine.Start(ctx, 1)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
}

func TestShutdown(t *testing.T) {
	engine := New()
	ctx := context.Background()

	_ = engine.Start(ctx, 1)

	err := engine.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Shutdown failed: %v", err)
	}
}

func TestGetVtx(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// GetVtx should return nil (no-op for now)
	vertexID := ids.GenerateTestID()

	tx, err := engine.GetVtx(ctx, vertexID)
	if err != nil {
		t.Errorf("GetVtx should not error: %v", err)
	}
	if tx != nil {
		t.Error("GetVtx should return nil transaction")
	}
}

func TestBuildVtx(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// BuildVtx should return nil (no-op for now)
	tx, err := engine.BuildVtx(ctx)
	if err != nil {
		t.Errorf("BuildVtx should not error: %v", err)
	}
	if tx != nil {
		t.Error("BuildVtx should return nil transaction")
	}
}

func TestParseVtx(t *testing.T) {
	engine := New()
	ctx := context.Background()

	// ParseVtx should return nil (no-op for now)
	tx, err := engine.ParseVtx(ctx, []byte{})
	if err != nil {
		t.Errorf("ParseVtx should not error: %v", err)
	}
	if tx != nil {
		t.Error("ParseVtx should return nil transaction")
	}
}