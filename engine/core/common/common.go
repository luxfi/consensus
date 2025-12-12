// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package common provides common types for consensus engines.
// This package aliases types from other packages for backwards compatibility.
package common

import (
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/consensus/engine/core"
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
