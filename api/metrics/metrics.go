// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// Registerer is an interface for registering prometheus metrics
type Registerer interface {
	prometheus.Registerer
}

// Registry is an interface for prometheus registry
type Registry interface {
	prometheus.Registerer
	prometheus.Gatherer
}

// NewRegistry creates a new prometheus registry
func NewRegistry() Registry {
	return prometheus.NewRegistry()
}

// MultiGatherer is a prometheus gatherer that can gather metrics from multiple sources
type MultiGatherer interface {
	prometheus.Gatherer
	
	// Register adds a new gatherer to this multi-gatherer
	Register(string, prometheus.Gatherer) error
}

// multiGatherer implements MultiGatherer
type multiGatherer struct {
	gatherers map[string]prometheus.Gatherer
}

// NewMultiGatherer creates a new multi-gatherer
func NewMultiGatherer() MultiGatherer {
	return &multiGatherer{
		gatherers: make(map[string]prometheus.Gatherer),
	}
}

// Register adds a new gatherer
func (mg *multiGatherer) Register(name string, gatherer prometheus.Gatherer) error {
	mg.gatherers[name] = gatherer
	return nil
}

// Gather implements prometheus.Gatherer
func (mg *multiGatherer) Gather() ([]*dto.MetricFamily, error) {
	var result []*dto.MetricFamily
	for _, g := range mg.gatherers {
		metrics, err := g.Gather()
		if err != nil {
			return nil, err
		}
		result = append(result, metrics...)
	}
	return result, nil
}

// Metrics is the interface for consensus metrics
type Metrics interface {
	// Prisms tracks the number of prisms
	Prisms() prometheus.Counter
	
	// Successful tracks successful prisms
	Successful() prometheus.Counter
	
	// Failed tracks failed prisms
	Failed() prometheus.Counter
}

// NewMetrics creates a new metrics instance
func NewMetrics(namespace string, registerer prometheus.Registerer) (Metrics, error) {
	m := &metrics{
		prisms: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "prisms",
			Help:      "Number of prisms",
		}),
		successful: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "successful",
			Help:      "Number of successful prisms",
		}),
		failed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "failed",
			Help:      "Number of failed prisms",
		}),
	}
	
	if err := registerer.Register(m.prisms); err != nil {
		return nil, err
	}
	if err := registerer.Register(m.successful); err != nil {
		return nil, err
	}
	if err := registerer.Register(m.failed); err != nil {
		return nil, err
	}
	
	return m, nil
}

type metrics struct {
	prisms      prometheus.Counter
	successful prometheus.Counter
	failed     prometheus.Counter
}

func (m *metrics) Prisms() prometheus.Counter {
	return m.prisms
}

func (m *metrics) Successful() prometheus.Counter {
	return m.successful
}

func (m *metrics) Failed() prometheus.Counter {
	return m.failed
}