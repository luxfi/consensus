// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zmq4

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/luxfi/zmq/v4/networking"
)

// Transport wraps the shared networking transport for consensus-specific functionality
type Transport struct {
	*networking.Transport
	nodeID string
}

// MessageHandler processes incoming messages (re-export for compatibility)
type MessageHandler = networking.MessageHandler

// Message represents a consensus message (re-export for compatibility)
type Message = networking.Message

// NewTransport creates a new ZMQ4 transport for consensus
func NewTransport(ctx context.Context, nodeID string, basePort int) *Transport {
	config := networking.DefaultConfig(nodeID, basePort)
	return &Transport{
		Transport: networking.New(ctx, config),
		nodeID:    nodeID,
	}
}

// ConsensusMessage creates a consensus-specific message
func (t *Transport) ConsensusMessage(msgType string, height uint64, round uint32, data interface{}) (*Message, error) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal data: %w", err)
	}

	return &Message{
		Type:   msgType,
		From:   t.nodeID,
		Height: height,
		Round:  round,
		Data:   jsonData,
	}, nil
}

// BroadcastConsensus sends a consensus message to all peers
func (t *Transport) BroadcastConsensus(msgType string, height uint64, round uint32, data interface{}) error {
	msg, err := t.ConsensusMessage(msgType, height, round, data)
	if err != nil {
		return err
	}
	return t.Broadcast(msg)
}

// SendConsensus sends a consensus message to a specific peer
func (t *Transport) SendConsensus(peerID, msgType string, height uint64, round uint32, data interface{}) error {
	msg, err := t.ConsensusMessage(msgType, height, round, data)
	if err != nil {
		return err
	}
	return t.Send(peerID, msg)
}

// All other methods are inherited from the embedded networking.Transport
// This includes:
// - Start() error
// - Stop()
// - ConnectPeer(peerID string, port int) error
// - DisconnectPeer(peerID string)
// - Broadcast(msg *Message) error
// - Send(peerID string, msg *Message) error
// - RegisterHandler(msgType string, handler MessageHandler)
// - GetPeers() []string
// - GetNodeID() string
// - GetMetrics() (sent, received, dropped uint64)
