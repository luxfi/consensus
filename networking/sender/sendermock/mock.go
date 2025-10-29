// Package sendermock provides mock implementations for message sending
package sendermock

import (
	"context"

	"github.com/luxfi/ids"
)

// MockSender provides a mock implementation for message sending
type MockSender struct {
	sentMessages []Message
}

// Message represents a sent message
type Message struct {
	NodeID  ids.NodeID
	Content []byte
	Type    string
}

// NewMockSender creates a new mock sender
func NewMockSender() *MockSender {
	return &MockSender{
		sentMessages: make([]Message, 0),
	}
}

// SendMessage sends a message to a node
func (m *MockSender) SendMessage(ctx context.Context, nodeID ids.NodeID, content []byte, msgType string) error {
	m.sentMessages = append(m.sentMessages, Message{
		NodeID:  nodeID,
		Content: content,
		Type:    msgType,
	})
	return nil
}

// GetSentMessages returns all sent messages
func (m *MockSender) GetSentMessages() []Message {
	return m.sentMessages
}

// ClearMessages clears all sent messages
func (m *MockSender) ClearMessages() {
	m.sentMessages = make([]Message, 0)
}
