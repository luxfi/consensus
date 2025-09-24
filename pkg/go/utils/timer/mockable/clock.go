package mockable

import "time"

// Clock is a mockable clock
type Clock struct {
	time   time.Time
	mocked bool
}

// NewClock creates a new clock
func NewClock() *Clock {
	return &Clock{
		time: time.Now(),
	}
}

// Now returns current time
func (c *Clock) Now() time.Time {
	if c.mocked {
		return c.time
	}
	return time.Now()
}

// Set sets the clock time
func (c *Clock) Set(t time.Time) {
	c.time = t
	c.mocked = true
}

// Advance advances the clock
func (c *Clock) Advance(d time.Duration) {
	c.time = c.time.Add(d)
}

// Real returns to real time
func (c *Clock) Real() {
	c.mocked = false
}
