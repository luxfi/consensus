// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bft

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	luxbft "github.com/luxfi/bft"
	"github.com/luxfi/bft/testutil"
	"github.com/stretchr/testify/require"
)

// Mock implementations following luxfi/bft test patterns

// testSigner implements luxbft.Signer
type testSigner struct{}

func (t *testSigner) Sign([]byte) ([]byte, error) {
	return []byte{1, 2, 3}, nil
}

// testVerifier implements luxbft.SignatureVerifier
type testVerifier struct{}

func (t *testVerifier) VerifyBlock(luxbft.VerifiedBlock) error {
	return nil
}

func (t *testVerifier) Verify(_ []byte, _ []byte, _ luxbft.NodeID) error {
	return nil
}

// testSignatureAggregator implements luxbft.SignatureAggregator
type testSignatureAggregator struct {
	err error
}

func (t *testSignatureAggregator) Aggregate(signatures []luxbft.Signature) (luxbft.QuorumCertificate, error) {
	return testQC(signatures), t.err
}

// testQC implements luxbft.QuorumCertificate
type testQC []luxbft.Signature

func (t testQC) Signers() []luxbft.NodeID {
	res := make([]luxbft.NodeID, 0, len(t))
	for _, sig := range t {
		res = append(res, sig.Signer)
	}
	return res
}

func (t testQC) Verify(msg []byte) error {
	return nil
}

func (t testQC) Bytes() []byte {
	b, err := asn1.Marshal(t)
	if err != nil {
		panic(err)
	}
	return b
}

// noopComm implements luxbft.Communication
type noopComm []luxbft.NodeID

func (n noopComm) Nodes() []luxbft.NodeID {
	return n
}

func (n noopComm) Send(*luxbft.Message, luxbft.NodeID) {}

func (n noopComm) Broadcast(msg *luxbft.Message) {}

// testBlockBuilder implements luxbft.BlockBuilder
type testBlockBuilder struct {
	out chan *testBlock
	in  chan *testBlock
}

func (t *testBlockBuilder) BuildBlock(_ context.Context, metadata luxbft.ProtocolMetadata) (luxbft.VerifiedBlock, bool) {
	if t.in != nil && len(t.in) > 0 {
		block := <-t.in
		return block, true
	}

	tb := newTestBlock(metadata)

	if t.out != nil {
		select {
		case t.out <- tb:
		default:
		}
	}

	return tb, true
}

func (t *testBlockBuilder) IncomingBlock(ctx context.Context) {
	<-ctx.Done()
}

// testBlock implements luxbft.Block and luxbft.VerifiedBlock
type testBlock struct {
	data     []byte
	metadata luxbft.ProtocolMetadata
	digest   [32]byte
}

func newTestBlock(metadata luxbft.ProtocolMetadata) *testBlock {
	tb := &testBlock{
		metadata: metadata,
		data:     make([]byte, 32),
	}

	_, err := rand.Read(tb.data)
	if err != nil {
		panic(err)
	}

	tb.computeDigest()
	return tb
}

func (tb *testBlock) computeDigest() {
	var bb bytes.Buffer
	tbBytes, err := tb.Bytes()
	if err != nil {
		panic(fmt.Sprintf("failed to serialize test block: %v", err))
	}

	bb.Write(tbBytes)
	tb.digest = sha256.Sum256(bb.Bytes())
}

func (tb *testBlock) BlockHeader() luxbft.BlockHeader {
	return luxbft.BlockHeader{
		ProtocolMetadata: tb.metadata,
		Digest:           tb.digest,
	}
}

func (tb *testBlock) Bytes() ([]byte, error) {
	bh := luxbft.BlockHeader{
		ProtocolMetadata: tb.metadata,
	}

	mdBuff := bh.Bytes()

	buff := make([]byte, len(tb.data)+len(mdBuff)+4)
	binary.BigEndian.PutUint32(buff, uint32(len(tb.data)))
	copy(buff[4:], tb.data)
	copy(buff[4+len(tb.data):], mdBuff)
	return buff, nil
}

func (tb *testBlock) Verify(context.Context) (luxbft.VerifiedBlock, error) {
	return tb, nil
}

// blockDeserializer implements luxbft.BlockDeserializer
type blockDeserializer struct{}

func (b *blockDeserializer) DeserializeBlock(ctx context.Context, buff []byte) (luxbft.Block, error) {
	blockLen := binary.BigEndian.Uint32(buff[:4])
	bh := luxbft.BlockHeader{}
	if err := bh.FromBytes(buff[4+blockLen:]); err != nil {
		return nil, err
	}

	tb := &testBlock{
		data:     buff[4 : 4+blockLen],
		metadata: bh.ProtocolMetadata,
	}

	tb.computeDigest()

	return tb, nil
}

// InMemStorage implements luxbft.Storage
type InMemStorage struct {
	data map[uint64]struct {
		luxbft.VerifiedBlock
		luxbft.Finalization
	}

	lock   sync.Mutex
	signal sync.Cond
}

func newInMemStorage() *InMemStorage {
	s := &InMemStorage{
		data: make(map[uint64]struct {
			luxbft.VerifiedBlock
			luxbft.Finalization
		}),
	}

	s.signal = *sync.NewCond(&s.lock)

	return s
}

func (mem *InMemStorage) Height() uint64 {
	return uint64(len(mem.data))
}

func (mem *InMemStorage) Retrieve(seq uint64) (luxbft.VerifiedBlock, luxbft.Finalization, bool) {
	item, ok := mem.data[seq]
	if !ok {
		return nil, luxbft.Finalization{}, false
	}
	return item.VerifiedBlock, item.Finalization, true
}

func (mem *InMemStorage) Index(ctx context.Context, block luxbft.VerifiedBlock, certificate luxbft.Finalization) error {
	mem.lock.Lock()
	defer mem.lock.Unlock()

	seq := block.BlockHeader().Seq

	_, ok := mem.data[seq]
	if ok {
		panic(fmt.Sprintf("block with seq %d already indexed!", seq))
	}
	mem.data[seq] = struct {
		luxbft.VerifiedBlock
		luxbft.Finalization
	}{block, certificate}

	mem.signal.Signal()
	return nil
}

// testWAL implements luxbft.WriteAheadLog
type testWAL struct {
	entries [][]byte
	lock    sync.Mutex
}

func newTestWAL() *testWAL {
	return &testWAL{
		entries: make([][]byte, 0),
	}
}

func (w *testWAL) Append(data []byte) error {
	w.lock.Lock()
	defer w.lock.Unlock()
	w.entries = append(w.entries, data)
	return nil
}

func (w *testWAL) ReadAll() ([][]byte, error) {
	w.lock.Lock()
	defer w.lock.Unlock()
	return w.entries, nil
}

// Helper to create valid EpochConfig for testing
func newTestEpochConfig(t *testing.T) luxbft.EpochConfig {
	logger := testutil.MakeLogger(t, 1)
	nodes := []luxbft.NodeID{{1}, {2}, {3}, {4}}

	return luxbft.EpochConfig{
		MaxProposalWait:     5 * time.Second,
		Logger:              logger,
		ID:                  nodes[0],
		Signer:              &testSigner{},
		WAL:                 newTestWAL(),
		Verifier:            &testVerifier{},
		Storage:             newInMemStorage(),
		Comm:                noopComm(nodes),
		BlockBuilder:        &testBlockBuilder{out: make(chan *testBlock, 1)},
		SignatureAggregator: &testSignatureAggregator{},
		BlockDeserializer:   &blockDeserializer{},
		StartTime:           time.Now(),
	}
}

// TestNewEngine tests the New constructor function
func TestNewEngine(t *testing.T) {
	t.Run("successful creation", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node-1",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)
		require.NotNil(t, engine)
		require.NotNil(t, engine.simplex)
		require.Equal(t, cfg.NodeID, engine.config.NodeID)
		require.Equal(t, cfg.EpochLength, engine.config.EpochLength)
		require.Len(t, engine.config.Validators, 4)
	})

	t.Run("with custom epoch number", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		epochConfig.Epoch = 42

		cfg := Config{
			NodeID:      "test-node-2",
			Validators:  []string{"val1", "val2", "val3"},
			EpochLength: 200,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)
		require.NotNil(t, engine)
		require.Equal(t, uint64(42), engine.simplex.Epoch)
	})

	t.Run("with replication enabled", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		epochConfig.ReplicationEnabled = true

		cfg := Config{
			NodeID:      "test-node-3",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)
		require.NotNil(t, engine)
	})
}

// TestEngineStart tests the Start method
func TestEngineStart(t *testing.T) {
	t.Run("start returns nil", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		err = engine.Start(ctx, 0)
		require.NoError(t, err)
	})

	t.Run("start with different request IDs", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()

		// Test with various request IDs
		for _, reqID := range []uint32{0, 1, 100, 65535} {
			err = engine.Start(ctx, reqID)
			require.NoError(t, err)
		}
	})

	t.Run("start with cancelled context", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Start should still return nil since it doesn't use context
		err = engine.Start(ctx, 0)
		require.NoError(t, err)
	})
}

// TestEngineStop tests the Stop method
func TestEngineStop(t *testing.T) {
	t.Run("stop returns nil", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		err = engine.Stop(ctx)
		require.NoError(t, err)
	})

	t.Run("stop with cancelled context", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// Stop should still return nil
		err = engine.Stop(ctx)
		require.NoError(t, err)
	})

	t.Run("start then stop", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		err = engine.Start(ctx, 0)
		require.NoError(t, err)

		err = engine.Stop(ctx)
		require.NoError(t, err)
	})
}

// TestEngineIsBootstrapped tests the IsBootstrapped method
func TestEngineIsBootstrapped(t *testing.T) {
	t.Run("always returns true", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		// Should be true immediately after creation
		require.True(t, engine.IsBootstrapped())
	})

	t.Run("returns true before start", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		// Should be true even before Start is called
		require.True(t, engine.IsBootstrapped())
	})

	t.Run("returns true after start", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		err = engine.Start(ctx, 0)
		require.NoError(t, err)

		require.True(t, engine.IsBootstrapped())
	})

	t.Run("returns true after stop", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		_ = engine.Start(ctx, 0)
		_ = engine.Stop(ctx)

		require.True(t, engine.IsBootstrapped())
	})
}

// TestEngineHealthCheck tests the HealthCheck method
func TestEngineHealthCheck(t *testing.T) {
	t.Run("returns correct status structure", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		epochConfig.Epoch = 5

		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		health, err := engine.HealthCheck(ctx)
		require.NoError(t, err)
		require.NotNil(t, health)

		// Type assert to map
		healthMap, ok := health.(map[string]interface{})
		require.True(t, ok, "health should be a map[string]interface{}")

		// Verify expected fields
		require.Equal(t, "bft-simplex", healthMap["consensus"])
		require.Equal(t, "healthy", healthMap["status"])
		require.Equal(t, uint64(5), healthMap["epoch"])
	})

	t.Run("returns epoch zero by default", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		// Epoch defaults to 0

		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()
		health, err := engine.HealthCheck(ctx)
		require.NoError(t, err)

		healthMap := health.(map[string]interface{})
		require.Equal(t, uint64(0), healthMap["epoch"])
	})

	t.Run("with cancelled context returns no error", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		// HealthCheck should still work since it doesn't use context
		health, err := engine.HealthCheck(ctx)
		require.NoError(t, err)
		require.NotNil(t, health)
	})

	t.Run("health check is idempotent", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		epochConfig.Epoch = 10

		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		ctx := context.Background()

		// Call multiple times
		for i := 0; i < 5; i++ {
			health, err := engine.HealthCheck(ctx)
			require.NoError(t, err)
			healthMap := health.(map[string]interface{})
			require.Equal(t, "bft-simplex", healthMap["consensus"])
			require.Equal(t, "healthy", healthMap["status"])
			require.Equal(t, uint64(10), healthMap["epoch"])
		}
	})
}

// TestEngineGetSimplex tests the GetSimplex method
func TestEngineGetSimplex(t *testing.T) {
	t.Run("returns underlying epoch", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		simplex := engine.GetSimplex()
		require.NotNil(t, simplex)

		// Verify it's the same instance
		require.Same(t, engine.simplex, simplex)
	})

	t.Run("returns same instance on multiple calls", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		simplex1 := engine.GetSimplex()
		simplex2 := engine.GetSimplex()
		simplex3 := engine.GetSimplex()

		require.Same(t, simplex1, simplex2)
		require.Same(t, simplex2, simplex3)
	})

	t.Run("epoch properties match config", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		epochConfig.Epoch = 42

		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.NoError(t, err)

		simplex := engine.GetSimplex()
		require.Equal(t, uint64(42), simplex.Epoch)
	})
}

// TestConfigStruct tests the Config struct
func TestConfigStruct(t *testing.T) {
	t.Run("config fields are accessible", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "test-node-id",
			Validators:  []string{"v1", "v2", "v3"},
			EpochLength: 500,
			EpochConfig: epochConfig,
		}

		require.Equal(t, "test-node-id", cfg.NodeID)
		require.Len(t, cfg.Validators, 3)
		require.Equal(t, uint64(500), cfg.EpochLength)
		require.NotNil(t, cfg.EpochConfig.Logger)
	})

	t.Run("empty validators list", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		cfg := Config{
			NodeID:      "solo-node",
			Validators:  []string{},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		require.Empty(t, cfg.Validators)
	})
}

// TestEngineLifecycle tests complete lifecycle
func TestEngineLifecycle(t *testing.T) {
	t.Run("full lifecycle: new -> start -> healthcheck -> stop", func(t *testing.T) {
		epochConfig := newTestEpochConfig(t)
		epochConfig.Epoch = 1

		cfg := Config{
			NodeID:      "lifecycle-test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		// Create
		engine, err := New(cfg)
		require.NoError(t, err)
		require.NotNil(t, engine)

		ctx := context.Background()

		// Start
		err = engine.Start(ctx, 1)
		require.NoError(t, err)

		// Check bootstrapped
		require.True(t, engine.IsBootstrapped())

		// Health check
		health, err := engine.HealthCheck(ctx)
		require.NoError(t, err)
		healthMap := health.(map[string]interface{})
		require.Equal(t, "healthy", healthMap["status"])

		// Get simplex
		simplex := engine.GetSimplex()
		require.NotNil(t, simplex)
		require.Equal(t, uint64(1), simplex.Epoch)

		// Stop
		err = engine.Stop(ctx)
		require.NoError(t, err)

		// Should still be bootstrapped after stop
		require.True(t, engine.IsBootstrapped())
	})
}

// failingStorage implements luxbft.Storage and fails on Retrieve
type failingStorage struct {
	height uint64
}

func (f *failingStorage) Height() uint64 {
	return f.height
}

func (f *failingStorage) Retrieve(seq uint64) (luxbft.VerifiedBlock, luxbft.Finalization, bool) {
	return nil, luxbft.Finalization{}, false
}

func (f *failingStorage) Index(ctx context.Context, block luxbft.VerifiedBlock, certificate luxbft.Finalization) error {
	return nil
}

// TestNewEngineError tests error handling in New constructor
func TestNewEngineError(t *testing.T) {
	t.Run("fails when storage retrieve fails", func(t *testing.T) {
		logger := testutil.MakeLogger(t, 1)
		nodes := []luxbft.NodeID{{1}, {2}, {3}, {4}}

		// Create a storage that claims to have blocks but fails to retrieve them
		epochConfig := luxbft.EpochConfig{
			MaxProposalWait:     5 * time.Second,
			Logger:              logger,
			ID:                  nodes[0],
			Signer:              &testSigner{},
			WAL:                 newTestWAL(),
			Verifier:            &testVerifier{},
			Storage:             &failingStorage{height: 1}, // Claims height 1 but fails to retrieve
			Comm:                noopComm(nodes),
			BlockBuilder:        &testBlockBuilder{out: make(chan *testBlock, 1)},
			SignatureAggregator: &testSignatureAggregator{},
			BlockDeserializer:   &blockDeserializer{},
			StartTime:           time.Now(),
		}

		cfg := Config{
			NodeID:      "test-node",
			Validators:  []string{"val1", "val2", "val3", "val4"},
			EpochLength: 100,
			EpochConfig: epochConfig,
		}

		engine, err := New(cfg)
		require.Error(t, err)
		require.Nil(t, engine)
		require.Contains(t, err.Error(), "failed retrieving last block")
	})
}

// TestMultipleEngines tests creating multiple engines
func TestMultipleEngines(t *testing.T) {
	t.Run("create multiple independent engines", func(t *testing.T) {
		engines := make([]*Engine, 3)

		for i := 0; i < 3; i++ {
			epochConfig := newTestEpochConfig(t)
			epochConfig.Epoch = uint64(i)

			cfg := Config{
				NodeID:      fmt.Sprintf("node-%d", i),
				Validators:  []string{"val1", "val2", "val3", "val4"},
				EpochLength: uint64(100 + i*10),
				EpochConfig: epochConfig,
			}

			engine, err := New(cfg)
			require.NoError(t, err)
			engines[i] = engine
		}

		// Verify each engine is independent
		for i, engine := range engines {
			require.Equal(t, fmt.Sprintf("node-%d", i), engine.config.NodeID)
			require.Equal(t, uint64(i), engine.simplex.Epoch)
			require.True(t, engine.IsBootstrapped())
		}

		// Verify simplex instances are different
		require.NotSame(t, engines[0].GetSimplex(), engines[1].GetSimplex())
		require.NotSame(t, engines[1].GetSimplex(), engines[2].GetSimplex())
	})
}
