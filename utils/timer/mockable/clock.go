package mockable

import (
	"time"
	
	nodetimer "github.com/luxfi/node/utils/timer/mockable"
)

// Clock wraps the node timer Clock
type Clock struct {
	*nodetimer.Clock
}

// NewClock creates a new Clock
func NewClock() *Clock {
	return &Clock{
		Clock: &nodetimer.Clock{},
	}
}

// Time returns current time
func (c *Clock) Time() time.Time {
	if c.Clock != nil {
		return c.Clock.Time()
	}
	return time.Now()
}

// UnixTime returns Unix timestamp
func (c *Clock) UnixTime() uint64 {
	if c.Clock != nil {
		return uint64(c.Clock.Time().Unix())
	}
	return uint64(time.Now().Unix())
}
