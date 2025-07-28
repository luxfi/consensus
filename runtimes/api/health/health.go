// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package health

import (
	"context"
	"time"
)

// Health represents the health status of a component
type Health struct {
	Healthy bool
	Details interface{}
}

// Checker can check the health of a component
type Checker interface {
	// Health returns the health status
	Health(ctx context.Context) (Health, error)
}

// Checkable can report its health status
type Checkable interface {
	// HealthCheck returns health status
	HealthCheck(ctx context.Context) (interface{}, error)
}

// NoOpChecker always returns healthy
type NoOpChecker struct{}

// Health always returns healthy
func (n *NoOpChecker) Health(ctx context.Context) (Health, error) {
	return Health{Healthy: true}, nil
}

// HealthCheck always returns healthy
func (n *NoOpChecker) HealthCheck(ctx context.Context) (interface{}, error) {
	return map[string]interface{}{
		"healthy": true,
		"timestamp": time.Now().Unix(),
	}, nil
}