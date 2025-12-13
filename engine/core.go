// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package core re-exports core types from github.com/luxfi/consensus/core
// for backwards compatibility. New code should import core directly.
package engine

import "github.com/luxfi/consensus/core"

// Re-export types from core package
type (
	Fx          = core.Fx
	Message     = core.Message
	MessageType = core.MessageType
)

// Re-export message type constants
const (
	PendingTxs                = core.PendingTxs
	PutBlock                  = core.PutBlock
	GetBlock                  = core.GetBlock
	GetAccepted               = core.GetAccepted
	Accepted                  = core.Accepted
	GetAncestors              = core.GetAncestors
	MultiPut                  = core.MultiPut
	GetFailed                 = core.GetFailed
	QueryFailed               = core.QueryFailed
	Chits                     = core.Chits
	ChitsV2                   = core.ChitsV2
	GetAcceptedFrontier       = core.GetAcceptedFrontier
	AcceptedFrontier          = core.AcceptedFrontier
	GetAcceptedFrontierFailed = core.GetAcceptedFrontierFailed
	WarpRequest               = core.WarpRequest
	WarpResponse              = core.WarpResponse
	WarpGossip                = core.WarpGossip
	StateSyncDone             = core.StateSyncDone
)
