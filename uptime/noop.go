// Package uptime provides uptime calculation functionality
package uptime

import (
	"time"
	
	"github.com/luxfi/ids"
)

// Calculator calculates uptime
type Calculator interface {
	CalculateUptime(nodeID ids.NodeID, subnetID ids.ID) (time.Duration, time.Duration, error)
	CalculateUptimePercent(nodeID ids.NodeID, subnetID ids.ID) (float64, error)
	CalculateUptimePercentFrom(nodeID ids.NodeID, subnetID ids.ID, from time.Time) (float64, error)
	SetCalculator(subnetID ids.ID, calc Calculator) error
}

// NoOpCalculator is a no-op implementation of Calculator
type NoOpCalculator struct{}

// CalculateUptime always returns 100% uptime
func (NoOpCalculator) CalculateUptime(ids.NodeID, ids.ID) (time.Duration, time.Duration, error) {
	return 0, 0, nil
}

// CalculateUptimePercent always returns 100% uptime
func (NoOpCalculator) CalculateUptimePercent(ids.NodeID, ids.ID) (float64, error) {
	return 1.0, nil
}

// CalculateUptimePercentFrom always returns 100% uptime
func (NoOpCalculator) CalculateUptimePercentFrom(ids.NodeID, ids.ID, time.Time) (float64, error) {
	return 1.0, nil
}

// SetCalculator is a no-op
func (NoOpCalculator) SetCalculator(ids.ID, Calculator) error {
	return nil
}