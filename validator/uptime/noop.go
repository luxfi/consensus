// Package uptime re-exports github.com/luxfi/validators/uptime for backward compatibility.
package uptime

import (
	"github.com/luxfi/validators/uptime"
)

// Calculator is an alias for uptime.Calculator
type Calculator = uptime.Calculator

// NoOpCalculator is an alias for uptime.NoOpCalculator
type NoOpCalculator = uptime.NoOpCalculator

// ZeroUptimeCalculator is an alias for uptime.ZeroUptimeCalculator
type ZeroUptimeCalculator = uptime.ZeroUptimeCalculator
