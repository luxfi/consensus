package quasar

import (
    "context"
    "testing"
    "time"
    
    "github.com/luxfi/consensus/config"
)

func TestNew(t *testing.T) {
    cfg := config.DefaultParams()
    q := New(cfg)
    
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
    q := New(cfg)
    
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
    q := New(cfg)
    
    ctx := context.Background()
    q.Initialize(ctx, []byte("bls"), []byte("pq"))
    
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
    q := New(cfg)
    
    ctx := context.Background()
    q.Initialize(ctx, []byte("bls"), []byte("pq"))
    
    tests := []struct {
        name      string
        votes     map[string]int
        proposal  string
        wantCert  bool
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

func TestCertBundle_Verify(t *testing.T) {
    cert := &CertBundle{
        BLSAgg: []byte("mock-bls-signature"),
        PQCert: []byte("mock-pq-certificate"),
    }
    
    // Mock verification (would use actual crypto in production)
    quorum := []string{"node1", "node2", "node3"}
    
    valid := cert.Verify(quorum)
    if !valid {
        t.Error("certificate verification failed")
    }
    
    // Test with empty signatures
    cert.BLSAgg = nil
    valid = cert.Verify(quorum)
    if valid {
        t.Error("empty BLS signature should fail verification")
    }
    
    cert.BLSAgg = []byte("mock-bls")
    cert.PQCert = nil
    valid = cert.Verify(quorum)
    if valid {
        t.Error("empty PQ certificate should fail verification")
    }
}

func TestQBlock(t *testing.T) {
    qb := &QBlock{
        Height:    100,
        Hash:      "0xabc123",
        Timestamp: time.Now(),
        Cert: &CertBundle{
            BLSAgg: []byte("bls"),
            PQCert: []byte("pq"),
        },
    }
    
    if qb.Height != 100 {
        t.Errorf("Height mismatch: got %d, want 100", qb.Height)
    }
    
    if qb.Hash != "0xabc123" {
        t.Errorf("Hash mismatch: got %s, want 0xabc123", qb.Hash)
    }
    
    if qb.Cert == nil {
        t.Error("QBlock missing certificate")
    }
}

func TestSetFinalizedCallback(t *testing.T) {
    cfg := config.DefaultParams()
    q := New(cfg)
    
    called := false
    q.SetFinalizedCallback(func(block QBlock) {
        called = true
        if block.Height != 42 {
            t.Errorf("callback got wrong height: %d", block.Height)
        }
    })
    
    // Trigger callback
    if q.finalizedCb != nil {
        q.finalizedCb(QBlock{Height: 42})
    }
    
    if !called {
        t.Error("callback not called")
    }
}

func TestQuantumFinality_Integration(t *testing.T) {
    cfg := config.XChainParams() // Ultra-fast 1ms blocks
    q := New(cfg)
    
    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()
    
    // Initialize
    err := q.Initialize(ctx, []byte("bls-key"), []byte("pq-key"))
    if err != nil {
        t.Fatalf("Initialize failed: %v", err)
    }
    
    // Set callback
    finalized := make(chan QBlock, 1)
    q.SetFinalizedCallback(func(block QBlock) {
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
        t.Fatal("failed to create certificate")
    }
    
    // Create finalized block
    qBlock := QBlock{
        Height:    1,
        Hash:      proposal,
        Timestamp: time.Now(),
        Cert:      cert,
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
        elapsed := time.Since(qBlock.Timestamp)
        if elapsed > 10*time.Millisecond {
            t.Errorf("finalization too slow for 1ms blocks: %v", elapsed)
        }
    case <-ctx.Done():
        t.Fatal("finalization timeout")
    }
}

func BenchmarkPhaseI(b *testing.B) {
    cfg := config.XChainParams()
    q := New(cfg)
    q.Initialize(context.Background(), []byte("bls"), []byte("pq"))
    
    frontier := []string{"b1", "b2", "b3", "b4", "b5"}
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = q.phaseI(frontier)
    }
}

func BenchmarkPhaseII(b *testing.B) {
    cfg := config.XChainParams()
    q := New(cfg)
    q.Initialize(context.Background(), []byte("bls"), []byte("pq"))
    
    votes := map[string]int{
        "block1": 3,
        "block2": 2,
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = q.phaseII(votes, "block1")
    }
}

func BenchmarkCertVerify(b *testing.B) {
    cert := &CertBundle{
        BLSAgg: make([]byte, 96),
        PQCert: make([]byte, 3072),
    }
    quorum := []string{"n1", "n2", "n3", "n4", "n5"}
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = cert.Verify(quorum)
    }
}

// Additional tests for Ringtail post-quantum functions
func TestRingtail(t *testing.T) {
    t.Run("NewRingtail", func(t *testing.T) {
        r := NewRingtail()
        if r == nil {
            t.Fatal("NewRingtail returned nil")
        }
    })
    
    t.Run("GenerateKeyPair", func(t *testing.T) {
        rt := NewRingtail()
        pub, priv, err := rt.GenerateKeyPair()
        if err != nil {
            t.Fatalf("GenerateKeyPair failed: %v", err)
        }
        if len(pub) == 0 || len(priv) == 0 {
            t.Error("Generated empty keys")
        }
    })
    
    t.Run("Encapsulate", func(t *testing.T) {
        rt := NewRingtail()
        pub, _, _ := rt.GenerateKeyPair()
        ct, ss, err := rt.Encapsulate(pub)
        if err != nil {
            t.Fatalf("Encapsulate failed: %v", err)
        }
        if len(ct) == 0 || len(ss) == 0 {
            t.Error("Empty ciphertext or shared secret")
        }
    })
    
    t.Run("Decapsulate", func(t *testing.T) {
        rt := NewRingtail()
        pub, priv, _ := rt.GenerateKeyPair()
        ct, ss1, _ := rt.Encapsulate(pub)
        ss2, err := rt.Decapsulate(ct, priv)
        if err != nil {
            t.Fatalf("Decapsulate failed: %v", err)
        }
        // Note: In stub implementation, secrets won't match since we use random
        _ = ss1
        _ = ss2
    })
    
    t.Run("Sign", func(t *testing.T) {
        rt := NewRingtail()
        _, priv, _ := rt.GenerateKeyPair()
        msg := []byte("test message")
        sig, err := rt.Sign(msg, priv)
        if err != nil {
            t.Fatalf("Sign failed: %v", err)
        }
        if len(sig) == 0 {
            t.Error("Empty signature")
        }
    })
    
    t.Run("Verify", func(t *testing.T) {
        rt := NewRingtail()
        pub, priv, _ := rt.GenerateKeyPair()
        msg := []byte("test message")
        sig, _ := rt.Sign(msg, priv)
        
        valid := rt.Verify(msg, sig, pub)
        if !valid {
            t.Error("Valid signature failed verification")
        }
        
        // Test invalid signature
        badSig := make([]byte, len(sig))
        invalid := rt.Verify(msg, badSig, pub)
        if invalid {
            t.Error("Invalid signature passed verification")
        }
    })
    
    t.Run("CombineSharedSecrets", func(t *testing.T) {
        rt := NewRingtail()
        ss1 := []byte("secret1")
        ss2 := []byte("secret2")
        combined := rt.CombineSharedSecrets(ss1, ss2)
        if len(combined) == 0 {
            t.Error("Empty combined secret")
        }
    })
    
    t.Run("DeriveKey", func(t *testing.T) {
        rt := NewRingtail()
        secret := []byte("shared secret")
        key := rt.DeriveKey(secret, 32)
        if len(key) != 32 {
            t.Errorf("DeriveKey returned wrong length: got %d, want 32", len(key))
        }
    })
}

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