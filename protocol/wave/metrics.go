// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/utils/metric"
)

// WaveMetrics tracks wave protocol metrics
// Real metrics should be handled by the parent system
//
//nolint:unused // TODO: Integrate with metrics system
type WaveMetrics struct {
	prisms metric.Averager
	polls  metric.Averager
}

// NewWaveMetrics creates a new wave metrics instance
//
//nolint:unused // TODO: Integrate with metrics system
func NewWaveMetrics(reg interfaces.Registerer) (*WaveMetrics, error) {
	return &WaveMetrics{
		prisms: metric.NewAverager(),
		polls:  metric.NewAverager(),
	}, nil
}

// Observe records metric observations
//
//nolint:unused // TODO: Integrate with metrics system
func (m *WaveMetrics) Observe(prisms, polls int) {
	m.prisms.Observe(float64(prisms))
	m.polls.Observe(float64(polls))
}
