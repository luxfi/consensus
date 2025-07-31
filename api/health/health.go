// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package health

import (
	"context"
	"time"
)

// Checker is the interface for health checking
type Checker interface {
	// HealthCheck returns information about the health of the service
	HealthCheck(context.Context) (interface{}, error)
}

// Checkable is the interface for health reporting
type Checkable interface {
	// Health returns a health report
	Health(context.Context) (interface{}, error)
}

// Report is a health report
type Report struct {
	// Details is a map of detailed health information
	Details map[string]interface{} `json:"details,omitempty"`

	// Healthy is true if the service is healthy
	Healthy bool `json:"healthy"`

	// Checks is a list of health checks performed
	Checks []Check `json:"checks,omitempty"`

	// Duration is how long the health check took
	Duration time.Duration `json:"duration"`
}

// Check is an individual health check
type Check struct {
	// Name is the name of the check
	Name string `json:"name"`

	// Healthy is true if the check passed
	Healthy bool `json:"healthy"`

	// Error is the error message if the check failed
	Error string `json:"error,omitempty"`

	// Details contains additional information about the check
	Details map[string]interface{} `json:"details,omitempty"`

	// Duration is how long this specific check took
	Duration time.Duration `json:"duration"`
}

// Health represents the health status of a component
type Health struct {
	// Healthy indicates if the component is healthy
	Healthy bool `json:"healthy"`
	// Details contains additional health information
	Details interface{} `json:"details,omitempty"`
}