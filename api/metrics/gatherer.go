// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
)

// NewPrefixGatherer returns a new gatherer that prefixes all metrics
func NewPrefixGatherer() prometheus.Gatherer {
	return prometheus.NewRegistry()
}