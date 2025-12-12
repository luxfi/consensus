// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package common provides common types for consensus engines.
// This package aliases types from other packages for backwards compatibility.
package common

import (
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/consensus/engine/core"
	"github.com/luxfi/warp"
)

// Message represents a message to the VM
type Message = block.Message

// Fx represents a feature extension
type Fx = block.Fx

// MessageType defines the type of a message (same as uint32 for compatibility)
type MessageType = uint32

// Message type constants (cast to uint32 for compatibility with Message.Type)
const (
	PendingTxs              MessageType = uint32(core.PendingTxs)
	PutBlock                MessageType = uint32(core.PutBlock)
	GetBlock                MessageType = uint32(core.GetBlock)
	GetAccepted             MessageType = uint32(core.GetAccepted)
	Accepted                MessageType = uint32(core.Accepted)
	GetAncestors            MessageType = uint32(core.GetAncestors)
	MultiPut                MessageType = uint32(core.MultiPut)
	GetFailed               MessageType = uint32(core.GetFailed)
	QueryFailed             MessageType = uint32(core.QueryFailed)
	Chits                   MessageType = uint32(core.Chits)
	ChitsV2                 MessageType = uint32(core.ChitsV2)
	GetAcceptedFrontier     MessageType = uint32(core.GetAcceptedFrontier)
	AcceptedFrontier        MessageType = uint32(core.AcceptedFrontier)
	GetAcceptedFrontierFailed MessageType = uint32(core.GetAcceptedFrontierFailed)
	WarpRequest             MessageType = uint32(core.WarpRequest)
	WarpResponse            MessageType = uint32(core.WarpResponse)
	WarpGossip              MessageType = uint32(core.WarpGossip)
)

// SendConfig is an alias for warp.SendConfig for backwards compatibility
type SendConfig = warp.SendConfig

// Sender is an alias for warp.Sender for backwards compatibility
type Sender = warp.Sender

// Handler is an alias for warp.Handler for backwards compatibility
type Handler = warp.Handler

// Error is an alias for warp.Error for backwards compatibility
type Error = warp.Error
