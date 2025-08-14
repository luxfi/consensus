// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package testutils

import (
	"github.com/luxfi/metric"
)

// NewNoOpRegisterer returns a new no-op registerer for testing
func NewNoOpRegisterer() metrics.Registerer {
	return metrics.NewNoOpRegistry()
}