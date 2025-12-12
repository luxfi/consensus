// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package common

import "github.com/luxfi/ids"

// Message represents a network message
type Message struct {
	Type    MessageType
	NodeID  ids.NodeID
	Content []byte
}

// MessageType defines the type of a message
type MessageType uint32

// String returns the string representation of the message type
func (m MessageType) String() string {
	switch m {
	case PendingTxs:
		return "PendingTxs"
	case PutBlock:
		return "PutBlock"
	case GetBlock:
		return "GetBlock"
	case GetAccepted:
		return "GetAccepted"
	case Accepted:
		return "Accepted"
	case GetAncestors:
		return "GetAncestors"
	case MultiPut:
		return "MultiPut"
	case GetFailed:
		return "GetFailed"
	case QueryFailed:
		return "QueryFailed"
	case Chits:
		return "Chits"
	case ChitsV2:
		return "ChitsV2"
	case GetAcceptedFrontier:
		return "GetAcceptedFrontier"
	case AcceptedFrontier:
		return "AcceptedFrontier"
	case GetAcceptedFrontierFailed:
		return "GetAcceptedFrontierFailed"
	case WarpRequest:
		return "WarpRequest"
	case WarpResponse:
		return "WarpResponse"
	case WarpGossip:
		return "WarpGossip"
	default:
		return "Unknown"
	}
}

const (
	// PendingTxs indicates pending transactions
	PendingTxs MessageType = iota
	// PutBlock indicates a block to be added
	PutBlock
	// GetBlock indicates a request for a block
	GetBlock
	// GetAccepted indicates a request for accepted blocks
	GetAccepted
	// Accepted indicates an accepted block
	Accepted
	// GetAncestors indicates a request for ancestors
	GetAncestors
	// MultiPut indicates multiple blocks
	MultiPut
	// GetFailed indicates a failed get request
	GetFailed
	// QueryFailed indicates a failed query
	QueryFailed
	// Chits indicates chits message
	Chits
	// ChitsV2 indicates chits v2 message
	ChitsV2
	// GetAcceptedFrontier indicates a request for accepted frontier
	GetAcceptedFrontier
	// AcceptedFrontier indicates accepted frontier
	AcceptedFrontier
	// GetAcceptedFrontierFailed indicates a failed frontier request
	GetAcceptedFrontierFailed
	// WarpRequest indicates a warp request
	WarpRequest
	// WarpResponse indicates a warp response
	WarpResponse
	// WarpGossip indicates warp gossip
	WarpGossip
)
