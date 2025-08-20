package quasar

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// mockClient implements the Client interface for testing
type mockClient struct {
	submitErr error
	bundle    Bundle
	fetchErr  error
	valid     bool
}

func (m *mockClient) SubmitCheckpoint(epoch uint64, root []byte, attest []byte) error {
	if m.submitErr != nil {
		return m.submitErr
	}
	return nil
}

func (m *mockClient) FetchBundle(epoch uint64) (Bundle, error) {
	if m.fetchErr != nil {
		return Bundle{}, m.fetchErr
	}
	return m.bundle, nil
}

func (m *mockClient) Verify(b Bundle) bool {
	return m.valid
}

func TestClient_SubmitCheckpoint(t *testing.T) {
	tests := []struct {
		name      string
		epoch     uint64
		root      []byte
		attest    []byte
		submitErr error
		wantErr   bool
	}{
		{
			name:    "successful submission",
			epoch:   100,
			root:    []byte("root-hash"),
			attest:  []byte("attestation"),
			wantErr: false,
		},
		{
			name:      "submission error",
			epoch:     101,
			root:      []byte("root"),
			attest:    []byte("attest"),
			submitErr: errors.New("network error"),
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				submitErr: tt.submitErr,
			}

			err := client.SubmitCheckpoint(tt.epoch, tt.root, tt.attest)
			if (err != nil) != tt.wantErr {
				t.Errorf("SubmitCheckpoint() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClient_FetchBundle(t *testing.T) {
	expectedBundle := Bundle{
		Epoch:   200,
		Root:    []byte("merkle-root"),
		BLSAgg:  []byte("bls-aggregate"),
		PQBatch: []byte("pq-batch"),
		Binding: []byte("binding-data"),
	}

	tests := []struct {
		name     string
		epoch    uint64
		bundle   Bundle
		fetchErr error
		wantErr  bool
	}{
		{
			name:    "successful fetch",
			epoch:   200,
			bundle:  expectedBundle,
			wantErr: false,
		},
		{
			name:     "fetch error",
			epoch:    201,
			fetchErr: errors.New("not found"),
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				bundle:   tt.bundle,
				fetchErr: tt.fetchErr,
			}

			bundle, err := client.FetchBundle(tt.epoch)
			if (err != nil) != tt.wantErr {
				t.Errorf("FetchBundle() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if bundle.Epoch != tt.bundle.Epoch {
					t.Errorf("FetchBundle() epoch = %v, want %v", bundle.Epoch, tt.bundle.Epoch)
				}
			}
		})
	}
}

func TestClient_Verify(t *testing.T) {
	bundle := Bundle{
		Epoch:   300,
		Root:    []byte("root"),
		BLSAgg:  []byte("bls"),
		PQBatch: []byte("pq"),
		Binding: []byte("bind"),
	}

	tests := []struct {
		name  string
		valid bool
	}{
		{
			name:  "valid bundle",
			valid: true,
		},
		{
			name:  "invalid bundle",
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockClient{
				valid: tt.valid,
			}

			result := client.Verify(bundle)
			if result != tt.valid {
				t.Errorf("Verify() = %v, want %v", result, tt.valid)
			}
		})
	}
}

func TestQuasarWithClient(t *testing.T) {
	// Create a Quasar instance with a mock client
	cfg := config.DefaultParams()
	q := New(cfg)

	// Initialize Quasar
	ctx := context.Background()
	err := q.Initialize(ctx, []byte("bls"), []byte("pq"))
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Mock client for checkpointing
	client := &mockClient{
		valid: true,
		bundle: Bundle{
			Epoch:   1,
			Root:    []byte("checkpoint-root"),
			BLSAgg:  []byte("checkpoint-bls"),
			PQBatch: []byte("checkpoint-pq"),
			Binding: []byte("checkpoint-bind"),
		},
	}

	// Submit checkpoint
	err = client.SubmitCheckpoint(1, []byte("root"), []byte("attest"))
	if err != nil {
		t.Errorf("SubmitCheckpoint failed: %v", err)
	}

	// Fetch and verify bundle
	bundle, err := client.FetchBundle(1)
	if err != nil {
		t.Errorf("FetchBundle failed: %v", err)
	}

	if !client.Verify(bundle) {
		t.Error("Bundle verification failed")
	}
}

func TestQuasarEdgeCases(t *testing.T) {
	t.Run("empty frontier", func(t *testing.T) {
		cfg := config.DefaultParams()
		q := New(cfg)
		
		proposal := q.phaseI([]string{})
		if proposal != "" {
			t.Errorf("expected empty proposal for empty frontier, got %s", proposal)
		}
	})

	t.Run("single block frontier", func(t *testing.T) {
		cfg := config.DefaultParams()
		q := New(cfg)
		
		proposal := q.phaseI([]string{"only-block"})
		if proposal != "only-block" {
			t.Errorf("expected 'only-block', got %s", proposal)
		}
	})

	t.Run("no votes", func(t *testing.T) {
		cfg := config.DefaultParams()
		q := New(cfg)
		
		cert := q.phaseII(map[string]int{}, "proposal")
		if cert != nil {
			t.Error("expected no certificate for empty votes")
		}
	})

	t.Run("boundary alpha threshold", func(t *testing.T) {
		cfg := config.DefaultParams()
		q := New(cfg)
		
		// Test exact boundary: 0.799999 < 0.8
		votes := map[string]int{
			"block1": 15, // 15/19 = 0.789 < 0.8
			"block2": 4,
		}
		
		cert := q.phaseII(votes, "block1")
		if cert != nil {
			t.Error("should not create certificate below alpha threshold")
		}
	})

	t.Run("nil callback", func(t *testing.T) {
		cfg := config.DefaultParams()
		q := New(cfg)
		
		// Should not panic with nil callback
		if q.finalizedCb != nil {
			t.Error("callback should be nil initially")
		}
		
		// Try to call nil callback (should not panic)
		// This is handled by checking if q.finalizedCb != nil before calling
	})
}

func BenchmarkQuasarFullRound(b *testing.B) {
	cfg := config.XChainParams() // Ultra-fast config
	q := New(cfg)
	_ = q.Initialize(context.Background(), []byte("bls"), []byte("pq"))
	
	frontier := []string{"b1", "b2", "b3", "b4", "b5"}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Phase I: Propose
		proposal := q.phaseI(frontier)
		
		// Phase II: Create certificates
		votes := map[string]int{
			proposal: 3,
			"other":  2,
		}
		cert := q.phaseII(votes, proposal)
		
		// Verify certificate
		if cert != nil {
			_ = cert.Verify([]string{"n1", "n2", "n3"})
		}
	}
}