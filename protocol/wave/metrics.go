// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/utils/metric"
)

// waveMetrics is a no-op implementation
// Real metrics should be handled by the parent system
type waveMetrics struct {
	prisms metric.Averager
	polls  metric.Averager
}

func newWaveMetrics(reg interfaces.Registerer) (*waveMetrics, error) {
	return &waveMetrics{
		prisms: metric.NewAverager(),
		polls:  metric.NewAverager(),
	}, nil
}

func (m *waveMetrics) Observe(prisms, polls int) {
	m.prisms.Observe(float64(prisms))
	m.polls.Observe(float64(polls))
}