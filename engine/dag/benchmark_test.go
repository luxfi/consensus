// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package dag

import (
	"context"
	"crypto/rand"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/luxfi/consensus/core/choices"
	"github.com/luxfi/consensus/engine/dag/state"
	"github.com/luxfi/ids"
)

// mockVertex implements vertex.Vertex for benchmarking
type mockVertex struct {
	id      ids.ID
	parents []ids.ID
	height  uint64
	epoch   uint32
	txs     []ids.ID
	status  choices.Status
	bytes   []byte
}

func (v *mockVertex) ID() ids.ID                   { return v.id }
func (v *mockVertex) Bytes() []byte                { return v.bytes }
func (v *mockVertex) Height() uint64               { return v.height }
func (v *mockVertex) Epoch() uint32                { return v.epoch }
func (v *mockVertex) Parents() []ids.ID            { return v.parents }
func (v *mockVertex) Txs() []ids.ID                { return v.txs }
func (v *mockVertex) Status() choices.Status       { return v.status }
func (v *mockVertex) Accept(context.Context) error { return nil }
func (v *mockVertex) Reject(context.Context) error { return nil }
func (v *mockVertex) Verify(context.Context) error { return nil }

// mockStateVertex implements state.Vertex for benchmarking
type mockStateVertex struct {
	id      ids.ID
	parents []ids.ID
	height  uint64
	bytes   []byte
}

func (v *mockStateVertex) ID() ids.ID          { return v.id }
func (v *mockStateVertex) ParentIDs() []ids.ID { return v.parents }
func (v *mockStateVertex) Height() uint64      { return v.height }
func (v *mockStateVertex) Bytes() []byte       { return v.bytes }

// generateVertex creates a mock vertex with random data
func generateVertex(height uint64, numParents int, numTxs int) *mockVertex {
	vtx := &mockVertex{
		id:      ids.GenerateTestID(),
		height:  height,
		epoch:   uint32(height / 100),
		status:  choices.Processing,
		parents: make([]ids.ID, numParents),
		txs:     make([]ids.ID, numTxs),
		bytes:   make([]byte, 1024), // 1KB vertex size
	}

	// Generate random parent IDs
	for i := 0; i < numParents; i++ {
		vtx.parents[i] = ids.GenerateTestID()
	}

	// Generate random transaction IDs
	for i := 0; i < numTxs; i++ {
		vtx.txs[i] = ids.GenerateTestID()
	}

	// Fill bytes with random data
	rand.Read(vtx.bytes)

	return vtx
}

// generateStateVertex creates a mock state vertex
func generateStateVertex(height uint64, numParents int) *mockStateVertex {
	vtx := &mockStateVertex{
		id:      ids.GenerateTestID(),
		height:  height,
		parents: make([]ids.ID, numParents),
		bytes:   make([]byte, 1024),
	}

	for i := 0; i < numParents; i++ {
		vtx.parents[i] = ids.GenerateTestID()
	}

	rand.Read(vtx.bytes)
	return vtx
}

// BenchmarkVertexProcessingSingle benchmarks processing a single vertex
func BenchmarkVertexProcessingSingle(b *testing.B) {
	ctx := context.Background()
	vtx := generateVertex(100, 3, 10)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// Simulate vertex processing
		_ = vtx.Verify(ctx)
		_ = vtx.Accept(ctx)
		_ = vtx.Status()
		_ = vtx.Height()
		_ = vtx.Parents()
		_ = vtx.Txs()
	}
}

// BenchmarkVertexProcessingBatch benchmarks batch vertex processing
func BenchmarkVertexProcessingBatch(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("batch_%d", size), func(b *testing.B) {
			ctx := context.Background()
			vertices := make([]*mockVertex, size)
			for i := 0; i < size; i++ {
				vertices[i] = generateVertex(uint64(i), 3, 10)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				for _, vtx := range vertices {
					_ = vtx.Verify(ctx)
					_ = vtx.Accept(ctx)
				}
			}
		})
	}
}

// BenchmarkParallelVoteProcessing benchmarks parallel vote processing
func BenchmarkParallelVoteProcessing(b *testing.B) {
	goroutines := []int{1, 2, 4, 8}

	for _, numGoroutines := range goroutines {
		b.Run(fmt.Sprintf("goroutines_%d", numGoroutines), func(b *testing.B) {
			ctx := context.Background()
			numVertices := 1000
			vertices := make([]*mockVertex, numVertices)
			for i := 0; i < numVertices; i++ {
				vertices[i] = generateVertex(uint64(i), 3, 10)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				ch := make(chan *mockVertex, numVertices)

				// Feed vertices into channel
				go func() {
					for _, vtx := range vertices {
						ch <- vtx
					}
					close(ch)
				}()

				// Process vertices in parallel
				wg.Add(numGoroutines)
				for g := 0; g < numGoroutines; g++ {
					go func() {
						defer wg.Done()
						for vtx := range ch {
							_ = vtx.Verify(ctx)
							// Simulate vote processing
							if vtx.Height()%2 == 0 {
								_ = vtx.Accept(ctx)
							} else {
								_ = vtx.Reject(ctx)
							}
						}
					}()
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkDAGFinalization benchmarks DAG finalization process
func BenchmarkDAGFinalization(b *testing.B) {
	depths := []int{10, 50, 100}

	for _, depth := range depths {
		b.Run(fmt.Sprintf("depth_%d", depth), func(b *testing.B) {
			ctx := context.Background()

			// Build a DAG with specified depth
			vertices := make([]*mockVertex, depth*3) // 3 vertices per layer
			for i := 0; i < len(vertices); i++ {
				height := uint64(i / 3)
				numParents := 0
				if height > 0 {
					numParents = 2 // Reference 2 vertices from previous layer
				}
				vertices[i] = generateVertex(height, numParents, 5)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Simulate finalization by processing vertices in topological order
				for _, vtx := range vertices {
					_ = vtx.Verify(ctx)
					if vtx.Height() < uint64(depth/2) {
						// Finalize older vertices
						_ = vtx.Accept(ctx)
					}
				}
			}
		})
	}
}

// BenchmarkConcurrentOperations benchmarks concurrent DAG operations
// NOTE: This benchmark is skipped by default because the current state
// implementation doesn't have proper synchronization for concurrent access.
// Enable when state package is made thread-safe.
func BenchmarkConcurrentOperations(b *testing.B) {
	b.Skip("Skipping concurrent operations benchmark - state package not thread-safe")

	operations := []struct {
		name        string
		readers     int
		writers     int
		numVertices int
	}{
		{"light", 4, 1, 100},
		{"balanced", 4, 2, 500},
		{"heavy", 8, 4, 1000},
	}

	for _, op := range operations {
		b.Run(op.name, func(b *testing.B) {
			s := state.New()

			// Pre-populate state
			for i := 0; i < op.numVertices; i++ {
				vtx := generateStateVertex(uint64(i), 2)
				_ = s.AddVertex(vtx)
			}

			vertices := make([]*mockStateVertex, op.numVertices)
			for i := 0; i < op.numVertices; i++ {
				vertices[i] = generateStateVertex(uint64(op.numVertices+i), 2)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup

				// Start readers
				wg.Add(op.readers)
				for r := 0; r < op.readers; r++ {
					go func(readerID int) {
						defer wg.Done()
						// Read random vertices
						for j := 0; j < 100; j++ {
							idx := (readerID*100 + j) % op.numVertices
							vtx := vertices[idx]
							_, _ = s.GetVertex(vtx.ID())
							_ = s.IsProcessing(vtx.ID())
						}
					}(r)
				}

				// Start writers
				wg.Add(op.writers)
				for w := 0; w < op.writers; w++ {
					go func(writerID int) {
						defer wg.Done()
						// Write new vertices
						for j := 0; j < 50; j++ {
							idx := (writerID*50 + j) % len(vertices)
							vtx := vertices[idx]
							_ = s.AddVertex(vtx)
							_ = s.VertexIssued(vtx)
						}
					}(w)
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkEngineOperations benchmarks DAG engine operations
func BenchmarkEngineOperations(b *testing.B) {
	benchmarks := []struct {
		name string
		fn   func(b *testing.B, e Engine)
	}{
		{"GetVtx", benchmarkEngineGetVtx},
		{"BuildVtx", benchmarkEngineBuildVtx},
		{"ParseVtx", benchmarkEngineParseVtx},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			e := New()
			ctx := context.Background()
			_ = e.Start(ctx, 1)
			defer e.Shutdown(ctx)

			bm.fn(b, e)
		})
	}
}

func benchmarkEngineGetVtx(b *testing.B, e Engine) {
	ctx := context.Background()
	id := ids.GenerateTestID()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = e.GetVtx(ctx, id)
	}
}

func benchmarkEngineBuildVtx(b *testing.B, e Engine) {
	ctx := context.Background()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = e.BuildVtx(ctx)
	}
}

func benchmarkEngineParseVtx(b *testing.B, e Engine) {
	ctx := context.Background()
	data := make([]byte, 1024)
	rand.Read(data)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		_, _ = e.ParseVtx(ctx, data)
	}
}

// BenchmarkDAGTraversal benchmarks DAG traversal operations
func BenchmarkDAGTraversal(b *testing.B) {
	sizes := []int{100, 1000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("size_%d", size), func(b *testing.B) {
			// Build a DAG structure in memory
			dag := make(map[ids.ID]*mockVertex, size)
			var root *mockVertex

			for i := 0; i < size; i++ {
				numParents := 0
				if i > 0 {
					numParents = min(i, 3) // Up to 3 parents
				}
				vtx := generateVertex(uint64(i), numParents, 5)
				dag[vtx.ID()] = vtx
				if i == 0 {
					root = vtx
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Traverse DAG using BFS
				visited := make(map[ids.ID]bool, size)
				queue := []ids.ID{root.ID()}

				for len(queue) > 0 {
					id := queue[0]
					queue = queue[1:]

					if visited[id] {
						continue
					}
					visited[id] = true

					if vtx, ok := dag[id]; ok {
						// Process vertex
						_ = vtx.Height()
						_ = vtx.Status()

						// Add children to queue (in real DAG, would follow edges)
						for _, parentID := range vtx.Parents() {
							if !visited[parentID] {
								queue = append(queue, parentID)
							}
						}
					}
				}
			}
		})
	}
}

// BenchmarkVotePropagation benchmarks vote propagation in DAG
func BenchmarkVotePropagation(b *testing.B) {
	layers := []int{5, 10, 20}
	width := 10 // vertices per layer

	for _, numLayers := range layers {
		b.Run(fmt.Sprintf("layers_%d", numLayers), func(b *testing.B) {
			ctx := context.Background()

			// Build layered DAG
			totalVertices := numLayers * width
			vertices := make([][]*mockVertex, numLayers)

			for layer := 0; layer < numLayers; layer++ {
				vertices[layer] = make([]*mockVertex, width)
				for i := 0; i < width; i++ {
					numParents := 0
					if layer > 0 {
						numParents = 3 // Reference 3 vertices from previous layer
					}
					vertices[layer][i] = generateVertex(uint64(layer), numParents, 5)
				}
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				votes := make(map[ids.ID]int, totalVertices)

				// Propagate votes through layers
				for layer := 0; layer < numLayers; layer++ {
					for _, vtx := range vertices[layer] {
						// Simulate vote collection
						votes[vtx.ID()]++

						// Propagate to children (next layer)
						if layer < numLayers-1 {
							for _, child := range vertices[layer+1] {
								// Check if child references this vertex
								for _, parentID := range child.Parents() {
									if parentID == vtx.ID() {
										votes[child.ID()]++
										break
									}
								}
							}
						}

						// Check if vertex has enough votes to accept
						if votes[vtx.ID()] > width/2 {
							_ = vtx.Accept(ctx)
						}
					}
				}
			}
		})
	}
}

// BenchmarkMemoryUsage benchmarks memory usage for large DAGs
func BenchmarkMemoryUsage(b *testing.B) {
	sizes := []int{1000, 5000, 10000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("vertices_%d", size), func(b *testing.B) {
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				s := state.New()

				// Add vertices to state
				for j := 0; j < size; j++ {
					vtx := generateStateVertex(uint64(j), min(j, 5))
					_ = s.AddVertex(vtx)
				}

				// Simulate some operations
				for j := 0; j < 100; j++ {
					id := ids.GenerateTestID()
					_, _ = s.GetVertex(id)
					_ = s.IsProcessing(id)
				}
			}
		})
	}
}

// BenchmarkConcurrentStateAccess benchmarks concurrent state access patterns
// NOTE: Skipped because state package is not thread-safe
func BenchmarkConcurrentStateAccess(b *testing.B) {
	b.Skip("Skipping concurrent state access benchmark - state package not thread-safe")

	s := state.New()
	numVertices := 10000

	// Pre-populate state
	vertices := make([]state.Vertex, numVertices)
	for i := 0; i < numVertices; i++ {
		vtx := generateStateVertex(uint64(i), 2)
		vertices[i] = vtx
		_ = s.AddVertex(vtx)
	}

	scenarios := []struct {
		name       string
		readRatio  float64 // percentage of reads vs writes
		goroutines int
	}{
		{"read_heavy", 0.9, 8},
		{"balanced", 0.5, 8},
		{"write_heavy", 0.1, 8},
	}

	for _, scenario := range scenarios {
		b.Run(scenario.name, func(b *testing.B) {
			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				wg.Add(scenario.goroutines)

				for g := 0; g < scenario.goroutines; g++ {
					go func(workerID int) {
						defer wg.Done()

						for op := 0; op < 100; op++ {
							idx := (workerID*100 + op) % numVertices
							vtx := vertices[idx]

							// Determine operation based on read ratio
							if float64(op%100)/100.0 < scenario.readRatio {
								// Read operation
								_, _ = s.GetVertex(vtx.ID())
								_ = s.VertexIssued(vtx)
								_ = s.IsProcessing(vtx.ID())
							} else {
								// Write operation
								newVtx := generateStateVertex(uint64(numVertices+op), 2)
								_ = s.AddVertex(newVtx)
							}
						}
					}(g)
				}

				wg.Wait()
			}
		})
	}
}

// BenchmarkTransactionOrdering benchmarks transaction ordering in vertices
func BenchmarkTransactionOrdering(b *testing.B) {
	txCounts := []int{10, 50, 100, 500}

	for _, numTxs := range txCounts {
		b.Run(fmt.Sprintf("txs_%d", numTxs), func(b *testing.B) {
			vertices := make([]*mockVertex, 100)
			for i := 0; i < 100; i++ {
				vertices[i] = generateVertex(uint64(i), 3, numTxs)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Simulate transaction ordering and deduplication
				txSet := make(map[ids.ID]bool, numTxs*100)
				orderedTxs := make([]ids.ID, 0, numTxs*100)

				for _, vtx := range vertices {
					for _, tx := range vtx.Txs() {
						if !txSet[tx] {
							txSet[tx] = true
							orderedTxs = append(orderedTxs, tx)
						}
					}
				}

				// Simulate processing ordered transactions
				for _, txID := range orderedTxs {
					// Process transaction
					_ = txID.String()
				}
			}
		})
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BenchmarkLatencySimulation simulates network latency effects on DAG consensus
func BenchmarkLatencySimulation(b *testing.B) {
	latencies := []time.Duration{
		0,                      // No latency
		10 * time.Microsecond,  // Low latency
		100 * time.Microsecond, // Medium latency
		1 * time.Millisecond,   // High latency
	}

	for _, latency := range latencies {
		b.Run(fmt.Sprintf("latency_%v", latency), func(b *testing.B) {
			ctx := context.Background()
			numVertices := 100
			vertices := make([]*mockVertex, numVertices)

			for i := 0; i < numVertices; i++ {
				vertices[i] = generateVertex(uint64(i), 3, 10)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var wg sync.WaitGroup
				wg.Add(numVertices)

				for _, vtx := range vertices {
					go func(v *mockVertex) {
						defer wg.Done()

						// Simulate network latency
						if latency > 0 {
							time.Sleep(latency)
						}

						// Process vertex
						_ = v.Verify(ctx)
						_ = v.Accept(ctx)
					}(vtx)
				}

				wg.Wait()
			}
		})
	}
}
