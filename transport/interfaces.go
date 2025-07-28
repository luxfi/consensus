// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package transport

import (
	"github.com/luxfi/ids"
)

// MessageType represents the type of consensus message
type MessageType uint8

const (
	// VoteRequest requests votes from peers
	VoteRequest MessageType = iota
	// VoteResponse contains vote results
	VoteResponse
	// Proposal broadcasts a new proposal
	Proposal
	// Heartbeat for liveness checks
	Heartbeat
)

// Message represents a consensus message
type Message struct {
	Type    MessageType
	From    ids.NodeID
	To      ids.NodeID
	Payload []byte
}

// Handler processes incoming messages
type Handler func(from ids.NodeID, msg *Message)

// Transport defines the interface for consensus networking
type Transport interface {
	// NodeID returns the node's ID
	NodeID() ids.NodeID
	
	// Connect establishes connection to a peer
	Connect(peerID ids.NodeID, endpoint string) error
	
	// Broadcast sends a message to all connected peers
	Broadcast(msg *Message) error
	
	// Send sends a message to a specific peer
	Send(peerID ids.NodeID, msg *Message) error
	
	// RegisterHandler registers a message handler
	RegisterHandler(msgType MessageType, handler Handler)
	
	// Start begins listening for messages
	Start() error
	
	// Stop shuts down the transport
	Stop() error
}