// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// MultiGatherer is a collection of prometheus.Gatherers
type MultiGatherer interface {
	prometheus.Gatherer
	
	// Register registers a new gatherer
	Register(namespace string, gatherer prometheus.Gatherer) error
}

// multiGatherer implements MultiGatherer
type multiGatherer struct {
	gatherers map[string]prometheus.Gatherer
}

// NewMultiGatherer returns a new MultiGatherer
func NewMultiGatherer() MultiGatherer {
	return &multiGatherer{
		gatherers: make(map[string]prometheus.Gatherer),
	}
}

// Register registers a new gatherer
func (m *multiGatherer) Register(namespace string, gatherer prometheus.Gatherer) error {
	m.gatherers[namespace] = gatherer
	return nil
}

// Gather implements prometheus.Gatherer
func (m *multiGatherer) Gather() ([]*dto.MetricFamily, error) {
	var result []*dto.MetricFamily
	
	for _, gatherer := range m.gatherers {
		metrics, err := gatherer.Gather()
		if err != nil {
			return nil, err
		}
		result = append(result, metrics...)
	}
	
	return result, nil
}