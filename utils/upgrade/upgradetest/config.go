// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package upgradetest

import (
	"time"
	
	"github.com/luxfi/consensus/utils/upgrade"
)

// Config returns a test upgrade config with all upgrades activated
func Config() upgrade.Config {
	return upgrade.Config{
		ApricotPhase1Time: time.Time{}, // Always activated
		ApricotPhase2Time: time.Time{},
		ApricotPhase3Time: time.Time{},
		ApricotPhase4Time: time.Time{},
		ApricotPhase5Time: time.Time{},
		BanffTime:         time.Time{},
		CortinaTime:       time.Time{},
		DurangoTime:       time.Time{},
		EtnaTime:          time.Time{},
	}
}