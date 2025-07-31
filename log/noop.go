// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package log

import (
	"github.com/luxfi/log"
)

// NewNoOpLogger returns a logger that doesn't log anything
func NewNoOpLogger() log.Logger {
	return log.NewNoOpLogger()
}