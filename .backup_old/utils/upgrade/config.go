// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package upgrade

import (
	"time"
)

// Config contains the network upgrade timeline
type Config struct {
	// Apricot upgrades
	ApricotPhase1Time time.Time
	ApricotPhase2Time time.Time
	ApricotPhase3Time time.Time
	ApricotPhase4Time time.Time
	ApricotPhase5Time time.Time

	// Banff upgrade
	BanffTime time.Time

	// Cortina upgrade
	CortinaTime time.Time

	// Durango upgrade
	DurangoTime time.Time

	// Etna upgrade
	EtnaTime time.Time
}

// IsApricotPhase1Activated returns true if the Apricot Phase 1 upgrade is active
func (c *Config) IsApricotPhase1Activated(timestamp time.Time) bool {
	return !timestamp.Before(c.ApricotPhase1Time)
}

// IsApricotPhase2Activated returns true if the Apricot Phase 2 upgrade is active
func (c *Config) IsApricotPhase2Activated(timestamp time.Time) bool {
	return !timestamp.Before(c.ApricotPhase2Time)
}

// IsApricotPhase3Activated returns true if the Apricot Phase 3 upgrade is active
func (c *Config) IsApricotPhase3Activated(timestamp time.Time) bool {
	return !timestamp.Before(c.ApricotPhase3Time)
}

// IsApricotPhase4Activated returns true if the Apricot Phase 4 upgrade is active
func (c *Config) IsApricotPhase4Activated(timestamp time.Time) bool {
	return !timestamp.Before(c.ApricotPhase4Time)
}

// IsApricotPhase5Activated returns true if the Apricot Phase 5 upgrade is active
func (c *Config) IsApricotPhase5Activated(timestamp time.Time) bool {
	return !timestamp.Before(c.ApricotPhase5Time)
}

// IsBanffActivated returns true if the Banff upgrade is active
func (c *Config) IsBanffActivated(timestamp time.Time) bool {
	return !timestamp.Before(c.BanffTime)
}

// IsCortinaActivated returns true if the Cortina upgrade is active
func (c *Config) IsCortinaActivated(timestamp time.Time) bool {
	return !timestamp.Before(c.CortinaTime)
}

// IsDurangoActivated returns true if the Durango upgrade is active
func (c *Config) IsDurangoActivated(timestamp time.Time) bool {
	return !timestamp.Before(c.DurangoTime)
}

// IsEtnaActivated returns true if the Etna upgrade is active
func (c *Config) IsEtnaActivated(timestamp time.Time) bool {
	return !timestamp.Before(c.EtnaTime)
}
