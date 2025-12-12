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

// MessageType defines the type of a message
type MessageType = core.MessageType

// Message type constants
const (
	PendingTxs              = core.PendingTxs
	PutBlock                = core.PutBlock
	GetBlock                = core.GetBlock
	GetAccepted             = core.GetAccepted
	Accepted                = core.Accepted
	GetAncestors            = core.GetAncestors
	MultiPut                = core.MultiPut
	GetFailed               = core.GetFailed
	QueryFailed             = core.QueryFailed
	Chits                   = core.Chits
	ChitsV2                 = core.ChitsV2
	GetAcceptedFrontier     = core.GetAcceptedFrontier
	AcceptedFrontier        = core.AcceptedFrontier
	GetAcceptedFrontierFailed = core.GetAcceptedFrontierFailed
	WarpRequest             = core.WarpRequest
	WarpResponse            = core.WarpResponse
	WarpGossip              = core.WarpGossip
)

// SendConfig is an alias for warp.SendConfig for backwards compatibility
type SendConfig = warp.SendConfig

// Sender is an alias for warp.Sender for backwards compatibility
type Sender = warp.Sender

// Handler is an alias for warp.Handler for backwards compatibility
type Handler = warp.Handler

// Error is an alias for warp.Error for backwards compatibility
type Error = warp.Error
