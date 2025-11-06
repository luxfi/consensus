// Package routermock provides mock implementations for message routing
package routermock

import (
	"context"

	"github.com/luxfi/consensus/core/types"
)

// MockRouter provides a mock implementation for message routing
type MockRouter struct {
	routes map[types.NodeID][]Message
}

// Message represents a routed message
type Message struct {
	From    types.NodeID
	To      types.NodeID
	Content []byte
	Type    string
}

// NewMockRouter creates a new mock router
func NewMockRouter() *MockRouter {
	return &MockRouter{
		routes: make(map[types.NodeID][]Message),
	}
}

// RouteMessage routes a message between nodes
func (m *MockRouter) RouteMessage(ctx context.Context, from, to types.NodeID, content []byte, msgType string) error {
	if _, exists := m.routes[to]; !exists {
		m.routes[to] = make([]Message, 0)
	}

	m.routes[to] = append(m.routes[to], Message{
		From:    from,
		To:      to,
		Content: content,
		Type:    msgType,
	})

	return nil
}

// GetMessages returns all messages routed to a node
func (m *MockRouter) GetMessages(nodeID types.NodeID) []Message {
	return m.routes[nodeID]
}

// ClearMessages clears all routed messages
func (m *MockRouter) ClearMessages() {
	m.routes = make(map[types.NodeID][]Message)
}
