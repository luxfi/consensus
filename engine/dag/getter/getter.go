// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package getter

import (
	"context"
	"time"

	"github.com/luxfi/consensus/engine/dag/state"
	"github.com/luxfi/consensus/networking/sender"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// Getter gets vertices
type Getter interface {
	// Get gets a vertex
	Get(context.Context, ids.NodeID, uint32, ids.ID) error

	// GetAncestors gets ancestors
	GetAncestors(context.Context, ids.NodeID, uint32, ids.ID, int) error

	// Put puts a vertex
	Put(context.Context, ids.NodeID, uint32, []byte) error

	// PushQuery pushes a query
	PushQuery(context.Context, ids.NodeID, uint32, []byte) error

	// PullQuery pulls a query
	PullQuery(context.Context, ids.NodeID, uint32, ids.ID) error
}

// Handler handles get requests for DAG vertices
type Handler interface {
	Getter
}

// getter implementation
type getter struct {
	vtxManager               state.Manager
	sender                   sender.Sender
	log                      log.Logger
	maxTimeGetAncestors      time.Duration
	ancestorsMaxContainers   int
}

// Config for creating a Handler
type Config struct {
	VtxManager               state.Manager
	Sender                   sender.Sender
	Log                      log.Logger
	MaxTimeGetAncestors      time.Duration
	AncestorsMaxContainers   int
}

// New creates a new getter
func New() Getter {
	return &getter{}
}

// NewHandler creates a new handler with config
func NewHandler(
	vtxManager state.Manager,
	sender sender.Sender,
	log log.Logger,
	maxTimeGetAncestors time.Duration,
	ancestorsMaxContainers int,
) (Handler, error) {
	return &getter{
		vtxManager:             vtxManager,
		sender:                 sender,
		log:                    log,
		maxTimeGetAncestors:    maxTimeGetAncestors,
		ancestorsMaxContainers: ancestorsMaxContainers,
	}, nil
}

// Get gets a vertex
func (g *getter) Get(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertexID ids.ID) error {
	if g.vtxManager == nil {
		return nil
	}
	vtx, err := g.vtxManager.GetVertex(vertexID)
	if err != nil {
		return nil // Vertex not found is not an error
	}
	if vtx != nil && g.sender != nil {
		return g.sender.SendResponse(ctx, nodeID, requestID, vtx.Bytes())
	}
	return nil
}

// GetAncestors gets ancestors
func (g *getter) GetAncestors(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertexID ids.ID, maxVertices int) error {
	if g.vtxManager == nil {
		return nil
	}

	vtx, err := g.vtxManager.GetVertex(vertexID)
	if err != nil || vtx == nil {
		return nil
	}

	ancestors := [][]byte{vtx.Bytes()}

	// Get parent vertices up to maxVertices
	toProcess := vtx.ParentIDs()
	visited := make(map[ids.ID]bool)
	visited[vertexID] = true

	for len(toProcess) > 0 && len(ancestors) < maxVertices {
		parentID := toProcess[0]
		toProcess = toProcess[1:]

		if visited[parentID] {
			continue
		}
		visited[parentID] = true

		parent, err := g.vtxManager.GetVertex(parentID)
		if err != nil || parent == nil {
			continue
		}

		ancestors = append(ancestors, parent.Bytes())
		toProcess = append(toProcess, parent.ParentIDs()...)
	}

	if g.sender != nil {
		// Serialize ancestors to bytes (length-prefixed format)
		var ancestorBytes []byte
		for _, a := range ancestors {
			// Simple length-prefix encoding
			lenBytes := make([]byte, 4)
			lenBytes[0] = byte(len(a) >> 24)
			lenBytes[1] = byte(len(a) >> 16)
			lenBytes[2] = byte(len(a) >> 8)
			lenBytes[3] = byte(len(a))
			ancestorBytes = append(ancestorBytes, lenBytes...)
			ancestorBytes = append(ancestorBytes, a...)
		}
		return g.sender.SendResponse(ctx, nodeID, requestID, ancestorBytes)
	}
	return nil
}

// Put puts a vertex
func (g *getter) Put(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertex []byte) error {
	return nil
}

// PushQuery pushes a query
func (g *getter) PushQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertex []byte) error {
	return nil
}

// PullQuery pulls a query
func (g *getter) PullQuery(ctx context.Context, nodeID ids.NodeID, requestID uint32, vertexID ids.ID) error {
	return nil
}
