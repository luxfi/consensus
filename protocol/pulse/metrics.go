// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pulse

import (
	"github.com/luxfi/consensus/core/interfaces"
	"github.com/luxfi/consensus/utils/metric"
)

// PulseMetrics is a no-op implementation
// Real metrics should be handled by the parent system
type PulseMetrics struct {
	prisms metric.Averager
	polls  metric.Averager
}

// NewPulseMetrics creates a new pulse metrics instance
func NewPulseMetrics(reg interfaces.Registerer) (*PulseMetrics, error) {
	return &PulseMetrics{
		prisms: metric.NewAverager(),
		polls:  metric.NewAverager(),
	}, nil
}

// Observe records metrics observations
func (m *PulseMetrics) Observe(prisms, polls int) {
	m.prisms.Observe(float64(prisms))
	m.polls.Observe(float64(polls))
}
