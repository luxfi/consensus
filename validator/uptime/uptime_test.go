// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package uptime

import (
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/stretchr/testify/require"
)

// TestNoOpCalculatorCalculateUptime tests NoOpCalculator.CalculateUptime
func TestNoOpCalculatorCalculateUptime(t *testing.T) {
	require := require.New(t)

	calc := NoOpCalculator{}
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	uptime, total, err := calc.CalculateUptime(nodeID, subnetID)
	require.NoError(err)
	require.Equal(time.Duration(0), uptime)
	require.Equal(time.Duration(0), total)
}

// TestNoOpCalculatorCalculateUptimePercent tests NoOpCalculator.CalculateUptimePercent
func TestNoOpCalculatorCalculateUptimePercent(t *testing.T) {
	require := require.New(t)

	calc := NoOpCalculator{}
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	percent, err := calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(1.0, percent) // 100% uptime
}

// TestNoOpCalculatorCalculateUptimePercentFrom tests NoOpCalculator.CalculateUptimePercentFrom
func TestNoOpCalculatorCalculateUptimePercentFrom(t *testing.T) {
	require := require.New(t)

	calc := NoOpCalculator{}
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()
	from := time.Now().Add(-time.Hour)

	percent, err := calc.CalculateUptimePercentFrom(nodeID, subnetID, from)
	require.NoError(err)
	require.Equal(1.0, percent) // 100% uptime
}

// TestNoOpCalculatorSetCalculator tests NoOpCalculator.SetCalculator
func TestNoOpCalculatorSetCalculator(t *testing.T) {
	require := require.New(t)

	calc := NoOpCalculator{}
	subnetID := ids.GenerateTestID()

	// Should be a no-op but not error
	err := calc.SetCalculator(subnetID, NoOpCalculator{})
	require.NoError(err)

	err = calc.SetCalculator(subnetID, nil)
	require.NoError(err)
}

// TestZeroUptimeCalculatorCalculateUptime tests ZeroUptimeCalculator.CalculateUptime
func TestZeroUptimeCalculatorCalculateUptime(t *testing.T) {
	require := require.New(t)

	calc := ZeroUptimeCalculator{}
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	uptime, total, err := calc.CalculateUptime(nodeID, subnetID)
	require.NoError(err)
	require.Equal(time.Duration(0), uptime)
	require.Equal(time.Duration(1), total) // 0 out of 1
}

// TestZeroUptimeCalculatorCalculateUptimePercent tests ZeroUptimeCalculator.CalculateUptimePercent
func TestZeroUptimeCalculatorCalculateUptimePercent(t *testing.T) {
	require := require.New(t)

	calc := ZeroUptimeCalculator{}
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	percent, err := calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(0.0, percent) // 0% uptime
}

// TestZeroUptimeCalculatorCalculateUptimePercentFrom tests ZeroUptimeCalculator.CalculateUptimePercentFrom
func TestZeroUptimeCalculatorCalculateUptimePercentFrom(t *testing.T) {
	require := require.New(t)

	calc := ZeroUptimeCalculator{}
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()
	from := time.Now().Add(-time.Hour)

	percent, err := calc.CalculateUptimePercentFrom(nodeID, subnetID, from)
	require.NoError(err)
	require.Equal(0.0, percent) // 0% uptime
}

// TestZeroUptimeCalculatorSetCalculator tests ZeroUptimeCalculator.SetCalculator
func TestZeroUptimeCalculatorSetCalculator(t *testing.T) {
	require := require.New(t)

	calc := ZeroUptimeCalculator{}
	subnetID := ids.GenerateTestID()

	err := calc.SetCalculator(subnetID, NoOpCalculator{})
	require.NoError(err)
}

// TestNewLockedCalculator tests NewLockedCalculator
func TestNewLockedCalculator(t *testing.T) {
	require := require.New(t)

	calc := NewLockedCalculator()
	require.NotNil(calc)

	// Should have NoOp fallback behavior
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	percent, err := calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(1.0, percent) // NoOp returns 100%
}

// TestNewLockedCalculatorWithFallback tests NewLockedCalculatorWithFallback
func TestNewLockedCalculatorWithFallback(t *testing.T) {
	require := require.New(t)

	// With ZeroUptime fallback
	calc := NewLockedCalculatorWithFallback(ZeroUptimeCalculator{})
	require.NotNil(calc)

	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	percent, err := calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(0.0, percent) // ZeroUptime returns 0%

	// With nil fallback (should use NoOp)
	calc = NewLockedCalculatorWithFallback(nil)
	require.NotNil(calc)

	percent, err = calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(1.0, percent) // NoOp returns 100%
}

// TestLockedCalculatorCalculateUptime tests LockedCalculator.CalculateUptime
func TestLockedCalculatorCalculateUptime(t *testing.T) {
	require := require.New(t)

	calc := NewLockedCalculator()
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	// Default fallback
	uptime, total, err := calc.CalculateUptime(nodeID, subnetID)
	require.NoError(err)
	require.Equal(time.Duration(0), uptime)
	require.Equal(time.Duration(0), total)

	// Set specific calculator for subnet
	err = calc.SetCalculator(subnetID, ZeroUptimeCalculator{})
	require.NoError(err)

	uptime, total, err = calc.CalculateUptime(nodeID, subnetID)
	require.NoError(err)
	require.Equal(time.Duration(0), uptime)
	require.Equal(time.Duration(1), total)

	// Other subnet still uses fallback
	otherSubnetID := ids.GenerateTestID()
	uptime, total, err = calc.CalculateUptime(nodeID, otherSubnetID)
	require.NoError(err)
	require.Equal(time.Duration(0), uptime)
	require.Equal(time.Duration(0), total)
}

// TestLockedCalculatorCalculateUptimePercent tests LockedCalculator.CalculateUptimePercent
func TestLockedCalculatorCalculateUptimePercent(t *testing.T) {
	require := require.New(t)

	calc := NewLockedCalculator()
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	// Default fallback
	percent, err := calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(1.0, percent)

	// Set specific calculator
	err = calc.SetCalculator(subnetID, ZeroUptimeCalculator{})
	require.NoError(err)

	percent, err = calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(0.0, percent)
}

// TestLockedCalculatorCalculateUptimePercentFrom tests LockedCalculator.CalculateUptimePercentFrom
func TestLockedCalculatorCalculateUptimePercentFrom(t *testing.T) {
	require := require.New(t)

	calc := NewLockedCalculator()
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()
	from := time.Now().Add(-time.Hour)

	// Default fallback
	percent, err := calc.CalculateUptimePercentFrom(nodeID, subnetID, from)
	require.NoError(err)
	require.Equal(1.0, percent)

	// Set specific calculator
	err = calc.SetCalculator(subnetID, ZeroUptimeCalculator{})
	require.NoError(err)

	percent, err = calc.CalculateUptimePercentFrom(nodeID, subnetID, from)
	require.NoError(err)
	require.Equal(0.0, percent)
}

// TestLockedCalculatorSetCalculator tests LockedCalculator.SetCalculator
func TestLockedCalculatorSetCalculator(t *testing.T) {
	require := require.New(t)

	calc := NewLockedCalculator()
	subnetID := ids.GenerateTestID()

	// Set calculator
	err := calc.SetCalculator(subnetID, ZeroUptimeCalculator{})
	require.NoError(err)

	// Setting nil should not modify (no error)
	err = calc.SetCalculator(subnetID, nil)
	require.NoError(err)

	// Original calculator should still be there
	nodeID := ids.GenerateTestNodeID()
	percent, err := calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(0.0, percent) // Still using ZeroUptimeCalculator
}

// TestLockedCalculatorConcurrentAccess tests thread safety
func TestLockedCalculatorConcurrentAccess(t *testing.T) {
	require := require.New(t)

	calc := NewLockedCalculator()
	nodeID := ids.GenerateTestNodeID()
	subnetID := ids.GenerateTestID()

	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				_ = calc.SetCalculator(subnetID, NoOpCalculator{})
			} else {
				_ = calc.SetCalculator(subnetID, ZeroUptimeCalculator{})
			}
		}
		done <- true
	}()

	// Reader goroutines
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_, _ = calc.CalculateUptimePercent(nodeID, subnetID)
				_, _, _ = calc.CalculateUptime(nodeID, subnetID)
				_, _ = calc.CalculateUptimePercentFrom(nodeID, subnetID, time.Now())
			}
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 6; i++ {
		<-done
	}

	// Should complete without race conditions
	require.True(true)
}

// TestCalculatorInterface tests that types implement Calculator interface
func TestCalculatorInterface(t *testing.T) {
	var _ Calculator = NoOpCalculator{}
	var _ Calculator = ZeroUptimeCalculator{}
	var _ Calculator = NewLockedCalculator()
	var _ LockedCalculator = NewLockedCalculator()
}

// mockCalculator for testing custom calculator behavior
type mockCalculator struct {
	uptime     time.Duration
	total      time.Duration
	percent    float64
	percentErr error
	uptimeErr  error
}

func (m *mockCalculator) CalculateUptime(ids.NodeID, ids.ID) (time.Duration, time.Duration, error) {
	return m.uptime, m.total, m.uptimeErr
}

func (m *mockCalculator) CalculateUptimePercent(ids.NodeID, ids.ID) (float64, error) {
	return m.percent, m.percentErr
}

func (m *mockCalculator) CalculateUptimePercentFrom(ids.NodeID, ids.ID, time.Time) (float64, error) {
	return m.percent, m.percentErr
}

func (m *mockCalculator) SetCalculator(ids.ID, Calculator) error {
	return nil
}

// TestLockedCalculatorWithCustomCalculator tests with a custom calculator
func TestLockedCalculatorWithCustomCalculator(t *testing.T) {
	require := require.New(t)

	customCalc := &mockCalculator{
		uptime:  time.Hour,
		total:   2 * time.Hour,
		percent: 0.5,
	}

	calc := NewLockedCalculator()
	subnetID := ids.GenerateTestID()
	nodeID := ids.GenerateTestNodeID()

	err := calc.SetCalculator(subnetID, customCalc)
	require.NoError(err)

	uptime, total, err := calc.CalculateUptime(nodeID, subnetID)
	require.NoError(err)
	require.Equal(time.Hour, uptime)
	require.Equal(2*time.Hour, total)

	percent, err := calc.CalculateUptimePercent(nodeID, subnetID)
	require.NoError(err)
	require.Equal(0.5, percent)
}
