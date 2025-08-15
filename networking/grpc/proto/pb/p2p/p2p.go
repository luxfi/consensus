// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package p2p

// EngineType represents the type of consensus engine
type EngineType int32

// Message types
const (
	EngineType_ENGINE_TYPE_UNSPECIFIED EngineType = 0
	EngineType_ENGINE_TYPE_CHAIN       EngineType = 1
	EngineType_ENGINE_TYPE_DAG         EngineType = 2
)

// Message represents a P2P message (stub)
type Message struct {
	// Stub implementation
}

// GetChainRequest represents a chain request
type GetChainRequest struct {
	ChainID     []byte
	RequestID   uint32
	Deadline    uint64
	ContainerID []byte
}

// GetChainResponse represents a chain response
type GetChainResponse struct {
	ChainID   []byte
	RequestID uint32
	Container []byte
}

// AppRequest represents an app request
type AppRequest struct {
	ChainID   []byte
	RequestID uint32
	Deadline  uint64
	AppBytes  []byte
}

// AppResponse represents an app response
type AppResponse struct {
	ChainID   []byte
	RequestID uint32
	AppBytes  []byte
}

// AppError represents an app error
type AppError struct {
	ChainID      []byte
	RequestID    uint32
	ErrorCode    int32
	ErrorMessage string
}
