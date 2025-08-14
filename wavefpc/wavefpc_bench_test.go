package wavefpc

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// Mock validator set for benchmarks
type mockValidatorSet struct {
	validators map[ids.NodeID]uint64
	self       ids.NodeID
}

func (m *mockValidatorSet) Self() ids.NodeID {
	return m.self
}

func (m *mockValidatorSet) GetWeight(nodeID ids.NodeID) uint64 {
	return m.validators[nodeID]
}

func (m *mockValidatorSet) TotalWeight() uint64 {
	total := uint64(0)
	for _, weight := range m.validators {
		total += weight
	}
	return total
}

// Benchmark block creation with FPC
func BenchmarkBlockCreationWithFPC(b *testing.B) {
	benchmarks := []struct {
		name       string
		validators int
		txPerBlock int
	}{
		{"10val-100tx", 10, 100},
		{"10val-1000tx", 10, 1000},
		{"21val-100tx", 21, 100},
		{"21val-1000tx", 21, 1000},
		{"21val-10000tx", 21, 10000},
		{"100val-1000tx", 100, 1000},
		{"100val-10000tx", 100, 10000},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Setup validator set
			validators := make(map[ids.NodeID]uint64)
			for i := 0; i < bm.validators; i++ {
				nodeID := ids.GenerateTestNodeID()
				validators[nodeID] = 1000 // Equal weight
			}
			
			selfID := ids.GenerateTestNodeID()
			validators[selfID] = 1000
			
			valSet := &mockValidatorSet{
				validators: validators,
				self:       selfID,
			}
			
			// Create FPC manager
			config := &Config{
				QuorumThreshold:  0.67,
				MaxVotesPerBlock: 100,
				MaxCertsPerBlock: 10,
				VoteExpiry:       30 * time.Second,
				Log:              log.NoLog{},
				ValidatorSet:     valSet,
			}
			
			manager := NewManager(config)
			ctx := context.Background()
			manager.Start(ctx)
			defer manager.Stop()
			
			b.ResetTimer()
			b.ReportAllocs()
			
			for i := 0; i < b.N; i++ {
				// Simulate block creation
				blockID := ids.GenerateTestID()
				parentID := ids.GenerateTestID()
				
				// Build block extension with FPC data
				ext, err := manager.OnBuildBlock(ctx, parentID)
				if err != nil {
					b.Fatal(err)
				}
				
				// Simulate adding votes
				for nodeID := range validators {
					vote := &Vote{
						BlockID:   blockID,
						Height:    uint64(i),
						Signature: []byte("sig"),
						Timestamp: time.Now(),
					}
					manager.AddVote(nodeID, vote)
				}
				
				// Check for certificate
				_ = manager.HasCertificate(blockID)
				
				// Simulate block acceptance
				manager.OnAcceptBlock(ctx, blockID)
				
				// Report custom metrics
				if i == b.N-1 {
					b.ReportMetric(float64(bm.txPerBlock), "tx/block")
					b.ReportMetric(float64(ext.VoteCount), "votes/block")
					b.ReportMetric(float64(ext.CertCount), "certs/block")
				}
			}
		})
	}
}

// Benchmark concurrent vote processing
func BenchmarkConcurrentVoteProcessing(b *testing.B) {
	benchmarks := []struct {
		name       string
		validators int
		goroutines int
	}{
		{"21val-1worker", 21, 1},
		{"21val-4workers", 21, 4},
		{"21val-8workers", 21, 8},
		{"100val-4workers", 100, 4},
		{"100val-16workers", 100, 16},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Setup
			validators := make(map[ids.NodeID]uint64)
			nodeIDs := make([]ids.NodeID, 0, bm.validators)
			
			for i := 0; i < bm.validators; i++ {
				nodeID := ids.GenerateTestNodeID()
				validators[nodeID] = 1000
				nodeIDs = append(nodeIDs, nodeID)
			}
			
			valSet := &mockValidatorSet{
				validators: validators,
				self:       nodeIDs[0],
			}
			
			config := &Config{
				QuorumThreshold:  0.67,
				MaxVotesPerBlock: 100,
				MaxCertsPerBlock: 10,
				VoteExpiry:       30 * time.Second,
				Log:              log.NoLog{},
				ValidatorSet:     valSet,
			}
			
			manager := NewManager(config)
			ctx := context.Background()
			manager.Start(ctx)
			defer manager.Stop()
			
			b.ResetTimer()
			b.ReportAllocs()
			
			var votesProcessed atomic.Int64
			
			// Run benchmark
			var wg sync.WaitGroup
			votesPerWorker := b.N / bm.goroutines
			
			start := time.Now()
			
			for w := 0; w < bm.goroutines; w++ {
				wg.Add(1)
				go func(workerID int) {
					defer wg.Done()
					
					for i := 0; i < votesPerWorker; i++ {
						blockID := ids.GenerateTestID()
						
						// Each worker processes votes from all validators
						for _, nodeID := range nodeIDs {
							vote := &Vote{
								BlockID:   blockID,
								Height:    uint64(i),
								Signature: []byte("sig"),
								Timestamp: time.Now(),
							}
							
							if err := manager.AddVote(nodeID, vote); err == nil {
								votesProcessed.Add(1)
							}
						}
					}
				}(w)
			}
			
			wg.Wait()
			elapsed := time.Since(start)
			
			// Report metrics
			totalVotes := votesProcessed.Load()
			votesPerSec := float64(totalVotes) / elapsed.Seconds()
			
			b.ReportMetric(votesPerSec, "votes/sec")
			b.ReportMetric(float64(totalVotes)/float64(b.N), "votes/op")
		})
	}
}

// Benchmark certificate creation
func BenchmarkCertificateCreation(b *testing.B) {
	benchmarks := []struct {
		name       string
		validators int
	}{
		{"5validators", 5},
		{"11validators", 11},
		{"21validators", 21},
		{"51validators", 51},
		{"101validators", 101},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Setup
			validators := make(map[ids.NodeID]uint64)
			nodeIDs := make([]ids.NodeID, 0, bm.validators)
			
			for i := 0; i < bm.validators; i++ {
				nodeID := ids.GenerateTestNodeID()
				validators[nodeID] = 1000
				nodeIDs = append(nodeIDs, nodeID)
			}
			
			valSet := &mockValidatorSet{
				validators: validators,
				self:       nodeIDs[0],
			}
			
			config := &Config{
				QuorumThreshold:  0.67,
				MaxVotesPerBlock: 100,
				MaxCertsPerBlock: 10,
				VoteExpiry:       30 * time.Second,
				Log:              log.NoLog{},
				ValidatorSet:     valSet,
			}
			
			manager := NewManager(config)
			ctx := context.Background()
			manager.Start(ctx)
			defer manager.Stop()
			
			b.ResetTimer()
			b.ReportAllocs()
			
			certsCreated := 0
			
			for i := 0; i < b.N; i++ {
				blockID := ids.GenerateTestID()
				
				// Add votes until we get a certificate
				quorumSize := int(float64(bm.validators) * 0.67)
				for j := 0; j < quorumSize+1; j++ {
					vote := &Vote{
						BlockID:   blockID,
						Height:    uint64(i),
						Signature: []byte("sig"),
						Timestamp: time.Now(),
					}
					manager.AddVote(nodeIDs[j], vote)
				}
				
				if manager.HasCertificate(blockID) {
					certsCreated++
				}
			}
			
			b.ReportMetric(float64(certsCreated)/float64(b.N)*100, "cert_success_%")
		})
	}
}

// Benchmark transaction throughput with FPC
func BenchmarkTransactionThroughput(b *testing.B) {
	benchmarks := []struct {
		name          string
		validators    int
		txPerSec      int
		blockInterval time.Duration
	}{
		{"mainnet-1ktps", 21, 1000, 100 * time.Millisecond},
		{"mainnet-10ktps", 21, 10000, 100 * time.Millisecond},
		{"mainnet-50ktps", 21, 50000, 100 * time.Millisecond},
		{"testnet-1ktps", 11, 1000, 100 * time.Millisecond},
		{"testnet-10ktps", 11, 10000, 100 * time.Millisecond},
		{"local-100ktps", 5, 100000, 10 * time.Millisecond},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Setup
			validators := make(map[ids.NodeID]uint64)
			nodeIDs := make([]ids.NodeID, 0, bm.validators)
			
			for i := 0; i < bm.validators; i++ {
				nodeID := ids.GenerateTestNodeID()
				validators[nodeID] = 1000
				nodeIDs = append(nodeIDs, nodeID)
			}
			
			valSet := &mockValidatorSet{
				validators: validators,
				self:       nodeIDs[0],
			}
			
			config := &Config{
				QuorumThreshold:  0.67,
				MaxVotesPerBlock: 100,
				MaxCertsPerBlock: 10,
				VoteExpiry:       30 * time.Second,
				Log:              log.NoLog{},
				ValidatorSet:     valSet,
			}
			
			manager := NewManager(config)
			ctx := context.Background()
			manager.Start(ctx)
			defer manager.Stop()
			
			b.ResetTimer()
			
			// Track metrics
			var (
				totalTx        atomic.Int64
				totalBlocks    atomic.Int64
				totalCerts     atomic.Int64
				totalLatency   atomic.Int64
			)
			
			// Simulate continuous operation
			start := time.Now()
			blockTicker := time.NewTicker(bm.blockInterval)
			defer blockTicker.Stop()
			
			done := make(chan struct{})
			go func() {
				for {
					select {
					case <-blockTicker.C:
						blockStart := time.Now()
						
						// Create block
						blockID := ids.GenerateTestID()
						parentID := ids.GenerateTestID()
						
						// Build with FPC
						ext, _ := manager.OnBuildBlock(ctx, parentID)
						
						// Simulate votes
						for _, nodeID := range nodeIDs {
							vote := &Vote{
								BlockID:   blockID,
								Height:    uint64(totalBlocks.Load()),
								Signature: []byte("sig"),
								Timestamp: time.Now(),
							}
							manager.AddVote(nodeID, vote)
						}
						
						// Check certificate
						if manager.HasCertificate(blockID) {
							totalCerts.Add(1)
						}
						
						// Accept block
						manager.OnAcceptBlock(ctx, blockID)
						
						// Update metrics
						txInBlock := int64(bm.blockInterval.Seconds() * float64(bm.txPerSec))
						totalTx.Add(txInBlock)
						totalBlocks.Add(1)
						totalLatency.Add(int64(time.Since(blockStart)))
						
						// Stop after N operations
						if totalBlocks.Load() >= int64(b.N) {
							close(done)
							return
						}
						
						_ = ext // Use ext
						
					case <-done:
						return
					}
				}
			}()
			
			<-done
			elapsed := time.Since(start)
			
			// Calculate and report metrics
			blocks := totalBlocks.Load()
			txCount := totalTx.Load()
			certs := totalCerts.Load()
			avgLatency := time.Duration(totalLatency.Load() / blocks)
			
			actualTPS := float64(txCount) / elapsed.Seconds()
			certRate := float64(certs) / float64(blocks) * 100
			
			b.ReportMetric(actualTPS, "actual_tps")
			b.ReportMetric(float64(blocks), "total_blocks")
			b.ReportMetric(certRate, "cert_rate_%")
			b.ReportMetric(float64(avgLatency.Microseconds()), "avg_latency_us")
		})
	}
}

// Benchmark memory usage with different validator counts
func BenchmarkMemoryUsage(b *testing.B) {
	benchmarks := []struct {
		name         string
		validators   int
		activeBlocks int
	}{
		{"21val-10blocks", 21, 10},
		{"21val-100blocks", 21, 100},
		{"100val-10blocks", 100, 10},
		{"100val-100blocks", 100, 100},
		{"1000val-10blocks", 1000, 10},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			// Setup
			validators := make(map[ids.NodeID]uint64)
			nodeIDs := make([]ids.NodeID, 0, bm.validators)
			
			for i := 0; i < bm.validators; i++ {
				nodeID := ids.GenerateTestNodeID()
				validators[nodeID] = 1000
				nodeIDs = append(nodeIDs, nodeID)
			}
			
			valSet := &mockValidatorSet{
				validators: validators,
				self:       nodeIDs[0],
			}
			
			config := &Config{
				QuorumThreshold:  0.67,
				MaxVotesPerBlock: 100,
				MaxCertsPerBlock: 10,
				VoteExpiry:       30 * time.Second,
				Log:              log.NoLog{},
				ValidatorSet:     valSet,
			}
			
			manager := NewManager(config)
			ctx := context.Background()
			manager.Start(ctx)
			defer manager.Stop()
			
			b.ResetTimer()
			b.ReportAllocs()
			
			// Create votes for multiple blocks
			for i := 0; i < b.N; i++ {
				for block := 0; block < bm.activeBlocks; block++ {
					blockID := ids.ID{byte(block)}
					
					// Add votes from all validators
					for _, nodeID := range nodeIDs {
						vote := &Vote{
							BlockID:   blockID,
							Height:    uint64(i),
							Signature: make([]byte, 96), // BLS signature size
							Timestamp: time.Now(),
						}
						manager.AddVote(nodeID, vote)
					}
				}
				
				// Prune old blocks periodically
				if i%10 == 0 && i > 0 {
					manager.Prune(uint64(i - 10))
				}
			}
		})
	}
}