// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package router

import "time"

// HealthConfig defines health check configuration for the router
type HealthConfig struct {
	// MaxDropRate is the maximum percentage of messages that can be dropped
	// before the router is considered unhealthy
	MaxDropRate float64 `json:"maxDropRate"`
	// MaxOutstandingRequests is the maximum number of outstanding requests
	// allowed before the router is considered unhealthy
	MaxOutstandingRequests int `json:"maxOutstandingRequests"`
	// MaxOutstandingDuration is the maximum time a request can be outstanding
	// before the router is considered unhealthy
	MaxOutstandingDuration time.Duration `json:"maxOutstandingDuration"`
	// MaxRunTimeHealthy is the maximum time the router can run before needing
	// a health check
	MaxRunTimeHealthy time.Duration `json:"maxRunTimeHealthy"`
	// MaxRunTimeRequests is the maximum runtime for requests
	MaxRunTimeRequests time.Duration `json:"maxRunTimeRequests"`
	// MaxDropRateHalflife is the halflife for the drop rate calculation
	MaxDropRateHalflife time.Duration `json:"maxDropRateHalflife"`
}