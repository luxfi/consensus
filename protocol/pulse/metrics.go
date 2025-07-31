// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package pulse

import (
    "github.com/prometheus/client_golang/prometheus"
)

// Metrics for pulse protocol
type Metrics struct {
    prisms           prometheus.Counter
    preferences     prometheus.Gauge
    finalized       prometheus.Counter
    pollDuration    prometheus.Histogram
}

// NewMetrics creates new metrics for pulse protocol
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
    m := &Metrics{
        prisms: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "pulse_prisms_total",
            Help: "Total number of prisms in pulse protocol",
        }),
        preferences: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "pulse_preference",
            Help: "Current preference in pulse protocol (0 or 1)",
        }),
        finalized: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "pulse_finalized_total",
            Help: "Total number of finalized decisions in pulse protocol",
        }),
        pollDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
            Name: "pulse_poll_duration_seconds",
            Help: "Duration of prisms in pulse protocol",
        }),
    }
    
    if reg != nil {
        if err := reg.Register(m.prisms); err != nil {
            return nil, err
        }
        if err := reg.Register(m.preferences); err != nil {
            return nil, err
        }
        if err := reg.Register(m.finalized); err != nil {
            return nil, err
        }
        if err := reg.Register(m.pollDuration); err != nil {
            return nil, err
        }
    }
    
    return m, nil
}
