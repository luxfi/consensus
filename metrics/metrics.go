// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metrics

import (
    "github.com/prometheus/client_golang/prometheus"
)

// Metrics provides consensus metrics
type Metrics struct {
    Registry prometheus.Registerer
}

// NewMetrics creates new metrics instance
func NewMetrics(reg prometheus.Registerer) *Metrics {
    return &Metrics{
        Registry: reg,
    }
}

// Register registers a prometheus collector
func (m *Metrics) Register(collector prometheus.Collector) error {
    return m.Registry.Register(collector)
}
