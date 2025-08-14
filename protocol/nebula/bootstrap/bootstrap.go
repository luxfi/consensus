// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bootstrap

import (
    "context"
    "time"
    
    "github.com/luxfi/ids"
)

// Bootstrapper handles DAG bootstrapping
type Bootstrapper struct {
    // Configuration
    maxOutstandingRequests int
    maxProcessingTime      time.Duration
    
    // State
    pendingVertices map[ids.ID]time.Time
}

// NewBootstrapper creates a new DAG bootstrapper
func NewBootstrapper(maxRequests int) *Bootstrapper {
    return &Bootstrapper{
        maxOutstandingRequests: maxRequests,
        maxProcessingTime:      30 * time.Second,
        pendingVertices:        make(map[ids.ID]time.Time),
    }
}

// Add adds a vertex to bootstrap
func (b *Bootstrapper) Add(vertexID ids.ID) {
    b.pendingVertices[vertexID] = time.Now()
}

// Process processes pending vertices
func (b *Bootstrapper) Process(ctx context.Context) error {
    // Process pending vertices
    for id, startTime := range b.pendingVertices {
        if time.Since(startTime) > b.maxProcessingTime {
            delete(b.pendingVertices, id)
        }
    }
    return nil
}
