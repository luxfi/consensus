package field

import (
	"context"
	"testing"
	"time"
)

func TestNebulaService(t *testing.T) {
	// Test nebula service initialization
	service := New()

	if service == nil {
		t.Fatal("New() should not return nil")
	}
}

func TestNebulaServiceContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Test service creation
	service := New()
	_ = ctx // Service doesn't use context yet

	if service == nil {
		t.Error("Service should be created")
	}
}

func TestNebulaServiceCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel immediately
	cancel()

	// Test service with cancelled context
	service := New()
	_ = ctx // Service doesn't use context yet

	if service == nil {
		t.Error("Service should still be created")
	}
}

func BenchmarkNebulaService(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = New()
	}
}
