// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package mockable

import (
	"sync"
	"time"
)

// Clock provides an interface for retrieving time
type Clock struct {
	mu   sync.RWMutex
	time time.Time
	fake bool
}

// NewClock creates a new clock set to the current time
func NewClock() *Clock {
	return &Clock{
		time: time.Now(),
	}
}

// Time returns the current time
func (c *Clock) Time() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.fake {
		return c.time
	}
	return time.Now()
}

// Unix returns the current Unix timestamp
func (c *Clock) Unix() int64 {
	return c.Time().Unix()
}

// UnixTime returns the current time as a Unix timestamp
func (c *Clock) UnixTime() uint64 {
	return uint64(c.Time().Unix())
}

// Set sets the time to a specific value
func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.fake = true
	c.time = t
}

// Advance advances the time by the given duration
func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.fake = true
	c.time = c.time.Add(d)
}

// Sync resets the clock to use real time
func (c *Clock) Sync() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.fake = false
}
