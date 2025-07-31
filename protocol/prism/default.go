// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package prism

import (
	"github.com/luxfi/consensus/testutils"
	"github.com/luxfi/log"
)

// DefaultFactory is the default prism factory
var DefaultFactory Factory

func init() {
	// Initialize with a basic factory
	DefaultFactory = NewFactory(
		log.NewNoOpLogger(),
		testutils.NewNoOpRegisterer(),
		1, // alphaPreference
		1, // alphaConfidence
	)
}