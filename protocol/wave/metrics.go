// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/utils/metric"
)

// WaveMetrics tracks wave protocol metrics
// Real metrics should be handled by the parent system
type WaveMetrics struct {
	prisms metric.Averager
	polls  metric.Averager
}

// NewWaveMetrics creates a new wave metrics instance
func NewWaveMetrics(reg interfaces.Registerer) (*WaveMetrics, error) {
	return &WaveMetrics{
		prisms: metric.NewAverager(),
		polls:  metric.NewAverager(),
	}, nil
}

// Observe records metric observations
func (m *WaveMetrics) Observe(prisms, polls int) {
	m.prisms.Observe(float64(prisms))
	m.polls.Observe(float64(polls))
}
