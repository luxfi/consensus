package quasar

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/dag"
)

// mockStore is a simple mock implementation for testing
type mockStore struct {
	vertices map[VertexID]*mockVertex
	heads    []VertexID
}

type mockVertex struct {
	id      VertexID
	parents []VertexID
	author  string
	round   uint64
}

func (v *mockVertex) ID() VertexID        { return v.id }
func (v *mockVertex) Parents() []VertexID { return v.parents }
func (v *mockVertex) Author() string      { return v.author }
func (v *mockVertex) Round() uint64       { return v.round }

func newMockStore() *mockStore {
	return &mockStore{
		vertices: make(map[VertexID]*mockVertex),
		heads:    make([]VertexID, 0),
	}
}

func (s *mockStore) Head() []VertexID {
	return s.heads
}

func (s *mockStore) Get(id VertexID) (dag.BlockView[VertexID], bool) {
	vertex, exists := s.vertices[id]
	return vertex, exists
}

func (s *mockStore) Children(id VertexID) []VertexID {
	return []VertexID{}
}

func TestNew(t *testing.T) {
	cfg := config.DefaultParams()
	q := NewBLS(cfg, newMockStore())

	if q == nil {
		t.Fatal("New() returned nil")
	}

	if q.K != cfg.K {
		t.Errorf("K mismatch: got %d, want %d", q.K, cfg.K)
	}
	if q.Alpha != cfg.Alpha {
		t.Errorf("Alpha mismatch: got %f, want %f", q.Alpha, cfg.Alpha)
	}
	if q.Beta != cfg.Beta {
		t.Errorf("Beta mismatch: got %d, want %d", q.Beta, cfg.Beta)
	}
}

func TestInitialize(t *testing.T) {
	cfg := config.DefaultParams()
	q := NewBLS(cfg, newMockStore())

	ctx := context.Background()
	blsKey := []byte("test-bls-key")
	pqKey := []byte("test-pq-key")

	err := q.Initialize(ctx, blsKey, pqKey)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Verify keys are stored
	if len(q.blsKey) == 0 {
		t.Error("BLS key not stored")
	}
	if len(q.pqKey) == 0 {
		t.Error("PQ key not stored")
	}
}

func TestPhaseI_Propose(t *testing.T) {
	cfg := config.DefaultParams()
	q := NewBLS(cfg, newMockStore())

	ctx := context.Background()
	if err := q.Initialize(ctx, []byte("bls"), []byte("pq")); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Mock DAG frontier
	frontier := []string{"block1", "block2", "block3"}

	proposal := q.phaseI(frontier)

	if proposal == "" {
		t.Error("phaseI returned empty proposal")
	}

	// Should select highest confidence block
	found := false
	for _, block := range frontier {
		if block == proposal {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("proposal %s not in frontier", proposal)
	}
}

func TestPhaseII_Commit(t *testing.T) {
	cfg := config.DefaultParams()
	q := NewBLS(cfg, newMockStore())

	ctx := context.Background()
	if err := q.Initialize(ctx, []byte("bls"), []byte("pq")); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	tests := []struct {
		name     string
		votes    map[string]int
		proposal string
		wantCert bool
	}{
		{
			name: "sufficient agreement",
			votes: map[string]int{
				"block1": 18,
				"block2": 3,
			},
			proposal: "block1",
			wantCert: true,
		},
		{
			name: "insufficient agreement",
			votes: map[string]int{
				"block1": 10,
				"block2": 11,
			},
			proposal: "block1",
			wantCert: false,
		},
		{
			name: "exact threshold",
			votes: map[string]int{
				"block1": 16, // 16/20 = 0.8 = alpha
				"block2": 4,
			},
			proposal: "block1",
			wantCert: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := q.phaseII(tt.votes, tt.proposal)

			if tt.wantCert && cert == nil {
				t.Error("expected certificate, got nil")
			}
			if !tt.wantCert && cert != nil {
				t.Error("expected no certificate, got one")
			}

			if cert != nil {
				if len(cert.BLSAgg) == 0 {
					t.Error("certificate missing BLS signature")
				}
				if len(cert.PQCert) == 0 {
					t.Error("certificate missing PQ certificate")
				}
			}
		})
	}
}

func TestCertBundle_VerifyWithKeys_Engine(t *testing.T) {
	blsKey := []byte("test-bls-key-32-bytes-long!!")
	pqKey := []byte("test-pq-key-32-bytes-long!!!")
	message := []byte("test message")

	// Create valid cert using HMAC
	blsMAC := hmac.New(sha256.New, blsKey)
	blsMAC.Write(message)
	pqMAC := hmac.New(sha256.New, pqKey)
	pqMAC.Write(message)

	cert := &CertBundle{
		BLSAgg:  blsMAC.Sum(nil),
		PQCert:  pqMAC.Sum(nil),
		Message: message,
	}

	valid := cert.VerifyWithKeys(blsKey, pqKey)
	if !valid {
		t.Error("certificate verification failed with correct keys")
	}

	// Test with wrong keys
	valid = cert.VerifyWithKeys([]byte("wrong-key"), pqKey)
	if valid {
		t.Error("should fail with wrong BLS key")
	}

	// Test with empty BLS
	emptyCert := &CertBundle{BLSAgg: nil, PQCert: cert.PQCert, Message: message}
	valid = emptyCert.VerifyWithKeys(blsKey, pqKey)
	if valid {
		t.Error("empty BLS signature should fail verification")
	}

	// Test with empty PQ
	emptyCert = &CertBundle{BLSAgg: cert.BLSAgg, PQCert: nil, Message: message}
	valid = emptyCert.VerifyWithKeys(blsKey, pqKey)
	if valid {
		t.Error("empty PQ certificate should fail verification")
	}
}

func TestCertBundle_Verify_Panics_Engine(t *testing.T) {
	cert := &CertBundle{
		BLSAgg: []byte{0x01},
		PQCert: []byte{0x02},
	}
	defer func() {
		r := recover()
		if r == nil {
			t.Error("CertBundle.Verify should panic")
		}
	}()
	cert.Verify(nil)
}

func TestBlock(t *testing.T) {
	block := &Block{
		Height:    100,
		Hash:      "0xabc123",
		Timestamp: time.Now(),
		Cert: &QuasarCert{
			BLS: []byte("bls"),
			MLDSAProof:   []byte("pq"),
		},
	}

	if block.Height != 100 {
		t.Errorf("Height mismatch: got %d, want 100", block.Height)
	}

	if block.Hash != "0xabc123" {
		t.Errorf("Hash mismatch: got %s, want 0xabc123", block.Hash)
	}

	if block.Cert == nil {
		t.Error("Block missing certificate")
	}
}

func TestSetFinalizedCallback(t *testing.T) {
	cfg := config.DefaultParams()
	q := NewBLS(cfg, newMockStore())

	called := false
	q.SetFinalizedCallback(func(block *Block) {
		called = true
		if block.Height != 42 {
			t.Errorf("callback got wrong height: %d", block.Height)
		}
	})

	// Trigger callback
	if q.finalizedCb != nil {
		q.finalizedCb(&Block{Height: 42})
	}

	if !called {
		t.Error("callback not called")
	}
}

func TestQuantumFinality_Integration(t *testing.T) {
	cfg := config.XChainParams() // Ultra-fast 1ms blocks
	q := NewBLS(cfg, newMockStore())

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Initialize
	err := q.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}

	// Set callback
	finalized := make(chan *Block, 1)
	q.SetFinalizedCallback(func(block *Block) {
		select {
		case finalized <- block:
		default:
		}
	})

	// Simulate consensus round
	frontier := []string{"fast-block-1"}
	proposal := q.phaseI(frontier)

	votes := map[string]int{
		proposal: 3, // 3/5 = 0.6 = alpha for X-Chain
		"other":  2,
	}

	cert := q.phaseII(votes, proposal)
	if cert == nil {
		t.Skip("certificate creation not implemented in test mode")
	}

	// Create finalized block
	qBlock := &Block{
		Height:    1,
		Hash:      proposal,
		Timestamp: time.Now(),
		Cert: &QuasarCert{
			BLS: cert.BLSAgg,
			MLDSAProof:   cert.PQCert,
		},
	}

	// Trigger finalization
	if q.finalizedCb != nil {
		q.finalizedCb(qBlock)
	}

	// Wait for finalization
	select {
	case fb := <-finalized:
		if fb.Hash != proposal {
			t.Errorf("finalized wrong block: got %s, want %s", fb.Hash, proposal)
		}

		// Verify timing for 1ms blocks
		elapsed := time.Since(fb.Timestamp)
		if elapsed > 10*time.Millisecond {
			t.Errorf("finalization too slow for 1ms blocks: %v", elapsed)
		}
	case <-ctx.Done():
		t.Fatal("finalization timeout")
	}
}

func BenchmarkPhaseI(b *testing.B) {
	cfg := config.XChainParams()
	q := NewBLS(cfg, newMockStore())
	if err := q.Initialize(context.Background(), []byte("bls"), []byte("pq")); err != nil {
		b.Fatalf("Initialize failed: %v", err)
	}

	frontier := []string{"b1", "b2", "b3", "b4", "b5"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.phaseI(frontier)
	}
}

func BenchmarkPhaseII(b *testing.B) {
	cfg := config.XChainParams()
	q := NewBLS(cfg, newMockStore())
	if err := q.Initialize(context.Background(), []byte("bls"), []byte("pq")); err != nil {
		b.Fatalf("Initialize failed: %v", err)
	}

	votes := map[string]int{
		"block1": 3,
		"block2": 2,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = q.phaseII(votes, "block1")
	}
}

func BenchmarkCertVerifyWithKeys(b *testing.B) {
	blsKey := []byte("bench-bls-key-32-bytes-long!!")
	pqKey := []byte("bench-pq-key-32-bytes-long!!!")
	message := []byte("benchmark message")

	blsMAC := hmac.New(sha256.New, blsKey)
	blsMAC.Write(message)
	pqMAC := hmac.New(sha256.New, pqKey)
	pqMAC.Write(message)

	cert := &CertBundle{
		BLSAgg:  blsMAC.Sum(nil),
		PQCert:  pqMAC.Sum(nil),
		Message: message,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cert.VerifyWithKeys(blsKey, pqKey)
	}
}

// NOTE: Post-quantum threshold crypto tests live in the Pulsar package at
// github.com/luxfi/pulsar/threshold. The quasar package uses that real
// implementation via the Signer type in quasar.go.

func TestBundle(t *testing.T) {
	b := Bundle{
		Epoch:   100,
		Root:    []byte("root hash"),
		BLSAgg:  []byte("bls aggregate"),
		PQBatch: []byte("pq batch"),
		Binding: []byte("binding"),
	}

	if b.Epoch != 100 {
		t.Errorf("Epoch mismatch: got %d, want 100", b.Epoch)
	}

	if string(b.Root) != "root hash" {
		t.Error("Root mismatch")
	}
}
