// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"sync/atomic"
	"time"
)

// Metrics tracks performance counters for the Prism DAG protocol.
type Metrics struct {
	// Vertex operations
	VerticesCreated  atomic.Uint64
	VerticesFinalized atomic.Uint64
	VerticesRejected atomic.Uint64

	// Cut operations
	CutsPerformed    atomic.Uint64
	CutLatencyNs     atomic.Int64
	LastCutTime      atomic.Int64

	// DAG statistics
	DAGWidth         atomic.Uint64 // Current frontier width
	DAGDepth         atomic.Uint64 // Maximum chain depth
	PendingVertices  atomic.Uint64

	// Throughput
	VerticesPerSecond atomic.Uint64
}

// NewMetrics creates a new Metrics instance.
func NewMetrics() *Metrics {
	return &Metrics{}
}

// RecordVertexCreated increments the vertex creation counter.
func (m *Metrics) RecordVertexCreated() {
	m.VerticesCreated.Add(1)
}

// RecordVertexFinalized increments the finalized vertex counter.
func (m *Metrics) RecordVertexFinalized() {
	m.VerticesFinalized.Add(1)
}

// RecordVertexRejected increments the rejected vertex counter.
func (m *Metrics) RecordVertexRejected() {
	m.VerticesRejected.Add(1)
}

// RecordCut records a DAG cut operation with its latency.
func (m *Metrics) RecordCut(latency time.Duration) {
	m.CutsPerformed.Add(1)
	m.CutLatencyNs.Store(latency.Nanoseconds())
	m.LastCutTime.Store(time.Now().UnixNano())
}

// UpdateDAGStats updates the current DAG width and depth.
func (m *Metrics) UpdateDAGStats(width, depth uint64) {
	m.DAGWidth.Store(width)
	m.DAGDepth.Store(depth)
}

// SetPendingVertices sets the current number of pending vertices.
func (m *Metrics) SetPendingVertices(count uint64) {
	m.PendingVertices.Store(count)
}

// Snapshot returns a copy of current metric values.
func (m *Metrics) Snapshot() MetricsSnapshot {
	return MetricsSnapshot{
		VerticesCreated:   m.VerticesCreated.Load(),
		VerticesFinalized: m.VerticesFinalized.Load(),
		VerticesRejected:  m.VerticesRejected.Load(),
		CutsPerformed:     m.CutsPerformed.Load(),
		CutLatencyNs:      m.CutLatencyNs.Load(),
		DAGWidth:          m.DAGWidth.Load(),
		DAGDepth:          m.DAGDepth.Load(),
		PendingVertices:   m.PendingVertices.Load(),
	}
}

// MetricsSnapshot is a point-in-time copy of metrics.
type MetricsSnapshot struct {
	VerticesCreated   uint64
	VerticesFinalized uint64
	VerticesRejected  uint64
	CutsPerformed     uint64
	CutLatencyNs      int64
	DAGWidth          uint64
	DAGDepth          uint64
	PendingVertices   uint64
}
