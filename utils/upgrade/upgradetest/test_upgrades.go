// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package upgradetest

import (
	"time"

	"github.com/luxfi/consensus/utils/upgrade"
)

const (
	Latest = iota
)

// GetConfig returns a test upgrade configuration
func GetConfig(upgradeTime int) *upgrade.Config {
	now := time.Now()
	return &upgrade.Config{
		ApricotPhase1Time: now,
		ApricotPhase2Time: now,
		ApricotPhase3Time: now,
		ApricotPhase4Time: now,
		ApricotPhase5Time: now,
		BanffTime:         now,
		CortinaTime:       now,
		DurangoTime:       now,
	}
}
