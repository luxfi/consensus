// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wave

import (
    "github.com/prometheus/client_golang/prometheus"
)

// Metrics for wave protocol
type Metrics struct {
    prisms           prometheus.Counter
    preferences     prometheus.GaugeVec
    finalized       prometheus.Counter
    pollDuration    prometheus.Histogram
    activeChoices   prometheus.Gauge
}

// NewMetrics creates new metrics for wave protocol
func NewMetrics(reg prometheus.Registerer) (*Metrics, error) {
    m := &Metrics{
        prisms: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "wave_prisms_total",
            Help: "Total number of prisms in wave protocol",
        }),
        preferences: *prometheus.NewGaugeVec(prometheus.GaugeOpts{
            Name: "wave_preference_strength",
            Help: "Preference strength for each choice in wave protocol",
        }, []string{"choice_id"}),
        finalized: prometheus.NewCounter(prometheus.CounterOpts{
            Name: "wave_finalized_total",
            Help: "Total number of finalized decisions in wave protocol",
        }),
        pollDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
            Name: "wave_poll_duration_seconds",
            Help: "Duration of prisms in wave protocol",
        }),
        activeChoices: prometheus.NewGauge(prometheus.GaugeOpts{
            Name: "wave_active_choices",
            Help: "Number of active choices in wave protocol",
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
        if err := reg.Register(m.activeChoices); err != nil {
            return nil, err
        }
    }
    
    return m, nil
}