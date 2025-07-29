// Copyright (C) 2019-2024, Lux Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package uptimemock provides mock implementations for uptime testing
package uptimemock

import (
	"time"

	"github.com/luxfi/ids"
)

// Calculator is a mock uptime calculator
type Calculator struct {
	uptime float64
}

// NewCalculator creates a new mock calculator
func NewCalculator() *Calculator {
	return &Calculator{
		uptime: 1.0,
	}
}

// CalculateUptime returns the mock uptime
func (c *Calculator) CalculateUptime(nodeID ids.NodeID, subnetID ids.ID) (float64, error) {
	return c.uptime, nil
}

// SetUptime sets the mock uptime
func (c *Calculator) SetUptime(uptime float64) {
	c.uptime = uptime
}

// CalculateUptimePercent returns the mock uptime as percentage
func (c *Calculator) CalculateUptimePercent(nodeID ids.NodeID, subnetID ids.ID, startTime, endTime time.Time) (float64, error) {
	return c.uptime, nil
}

// CalculateUptimePercentFrom returns the mock uptime as percentage from a start time
func (c *Calculator) CalculateUptimePercentFrom(nodeID ids.NodeID, subnetID ids.ID, startTime time.Time) (float64, error) {
	return c.uptime, nil
}