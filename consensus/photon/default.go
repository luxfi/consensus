// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/luxfi/log"
)

// DefaultFactory is the default poll factory
var DefaultFactory Factory

func init() {
	// Initialize with a basic factory
	DefaultFactory = NewFactory(
		log.NewNoOpLogger(),
		prometheus.NewRegistry(),
		1, // alphaPreference
		1, // alphaConfidence
	)
}