// Package vertexmock provides mock implementations for DAG vertices
package vertexmock

import (
	"time"

	"github.com/luxfi/ids"
)

// MockVertex provides a mock implementation for a DAG vertex
type MockVertex struct {
	id        ids.ID
	parentIDs []ids.ID
	height    uint64
	timestamp time.Time
	bytes     []byte
	status    Status
}

// Status represents vertex status
type Status uint8

const (
	StatusUnknown Status = iota
	StatusProcessing
	StatusAccepted
	StatusRejected
)

// NewMockVertex creates a new mock vertex
func NewMockVertex(id ids.ID, parentIDs []ids.ID, height uint64) *MockVertex {
	return &MockVertex{
		id:        id,
		parentIDs: parentIDs,
		height:    height,
		timestamp: time.Now(),
		status:    StatusProcessing,
	}
}

// ID returns the vertex ID
func (m *MockVertex) ID() ids.ID {
	return m.id
}

// ParentIDs returns the parent vertex IDs
func (m *MockVertex) ParentIDs() []ids.ID {
	return m.parentIDs
}

// Height returns the vertex height
func (m *MockVertex) Height() uint64 {
	return m.height
}

// Timestamp returns the vertex timestamp
func (m *MockVertex) Timestamp() time.Time {
	return m.timestamp
}

// Bytes returns the serialized vertex
func (m *MockVertex) Bytes() []byte {
	if m.bytes == nil {
		m.bytes = []byte{} // Empty bytes for mock
	}
	return m.bytes
}

// Status returns the vertex status
func (m *MockVertex) Status() Status {
	return m.status
}

// Accept marks the vertex as accepted
func (m *MockVertex) Accept() {
	m.status = StatusAccepted
}

// Reject marks the vertex as rejected
func (m *MockVertex) Reject() {
	m.status = StatusRejected
}
