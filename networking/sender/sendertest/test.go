// Package sendertest provides test utilities for message sending
package sendertest

import (
	"context"
	"github.com/luxfi/consensus/types"
)

// TestSender provides a test implementation for message senders
type TestSender struct {
	sentMessages []Message
}

// Message represents a sent message
type Message struct {
	To      types.NodeID
	Content []byte
	Type    string
}

// NewTestSender creates a new test sender
func NewTestSender() *TestSender {
	return &TestSender{
		sentMessages: make([]Message, 0),
	}
}

// SendMessage sends a message to a node
func (t *TestSender) SendMessage(ctx context.Context, to types.NodeID, content []byte, msgType string) error {
	t.sentMessages = append(t.sentMessages, Message{
		To:      to,
		Content: content,
		Type:    msgType,
	})
	return nil
}

// GetSentMessages returns all sent messages
func (t *TestSender) GetSentMessages() []Message {
	return t.sentMessages
}

// ClearMessages clears all sent messages
func (t *TestSender) ClearMessages() {
	t.sentMessages = make([]Message, 0)
}
