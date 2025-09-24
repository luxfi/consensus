// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package uptime

import (
	"time"

	"github.com/luxfi/ids"
)

// State tracks validator uptime
type State interface {
	// GetUptime returns the uptime for a validator
	GetUptime(nodeID ids.NodeID, netID ids.ID) (time.Duration, time.Duration, error)

	// SetUptime sets the uptime for a validator
	SetUptime(nodeID ids.NodeID, netID ids.ID, uptime time.Duration, lastUpdated time.Time) error

	// GetStartTime returns when the validator started
	GetStartTime(nodeID ids.NodeID, netID ids.ID) (time.Time, error)
}
