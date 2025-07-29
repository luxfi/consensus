// Copyright (C) 2019-2024, Lux Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package log

import (
	"github.com/luxfi/log"
)

// NewNoOpLogger returns a logger that doesn't log anything
func NewNoOpLogger() log.Logger {
	return log.NewNoOpLogger()
}