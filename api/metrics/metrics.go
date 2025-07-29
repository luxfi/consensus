// Copyright (C) 2025, Lux Industries, Inc. All rights reserved.
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
	// Polls tracks the number of polls
	Polls() prometheus.Counter
	
	// Successful tracks successful polls
	Successful() prometheus.Counter
	
	// Failed tracks failed polls
	Failed() prometheus.Counter
}

// NewMetrics creates a new metrics instance
func NewMetrics(namespace string, registerer prometheus.Registerer) (Metrics, error) {
	m := &metrics{
		polls: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "polls",
			Help:      "Number of polls",
		}),
		successful: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "successful",
			Help:      "Number of successful polls",
		}),
		failed: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "failed",
			Help:      "Number of failed polls",
		}),
	}
	
	if err := registerer.Register(m.polls); err != nil {
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
	polls      prometheus.Counter
	successful prometheus.Counter
	failed     prometheus.Counter
}

func (m *metrics) Polls() prometheus.Counter {
	return m.polls
}

func (m *metrics) Successful() prometheus.Counter {
	return m.successful
}

func (m *metrics) Failed() prometheus.Counter {
	return m.failed
}