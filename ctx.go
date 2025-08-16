// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// Context provides all the context needed for consensus operations
type Context struct {
	// Network configuration
	NetworkID       uint32
	SubnetID        ids.ID
	ChainID         ids.ID
	NodeID          ids.NodeID
	PublicKey       *bls.PublicKey
	
	// Chain-specific IDs
	XChainID        ids.ID
	CChainID        ids.ID
	DChainID        ids.ID
	XAssetID        ids.ID
	LUXAssetID      ids.ID
	
	// Logging
	Log             Logger
	
	// Blockchain configuration
	ChainDataDir    string
	
	// Services
	SharedMemory    interface{}
	BCLookup        interface{}
	ValidatorState  interface{}
	WarpSigner      interface{}
}

// Logger interface for consensus logging
type Logger interface {
	Debug(msg string, args ...interface{})
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// NoOpLogger is a logger that discards all messages
type NoOpLogger struct{}

func (NoOpLogger) Debug(string, ...interface{}) {}
func (NoOpLogger) Info(string, ...interface{})  {}
func (NoOpLogger) Warn(string, ...interface{})  {}
func (NoOpLogger) Error(string, ...interface{}) {}