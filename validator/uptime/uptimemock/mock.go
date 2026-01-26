// Package uptimemock re-exports github.com/luxfi/validators/uptime/uptimemock for backward compatibility.
package uptimemock

import (
	"github.com/luxfi/validators/uptime/uptimemock"
)

// MockUptimeTracker is an alias for uptimemock.MockUptimeTracker
type MockUptimeTracker = uptimemock.MockUptimeTracker

// NewMockUptimeTracker re-exports uptimemock.NewMockUptimeTracker
func NewMockUptimeTracker() *MockUptimeTracker {
	return uptimemock.NewMockUptimeTracker()
}
