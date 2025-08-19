// Package uptime is DEPRECATED.
// Uptime tracking is a node monitoring concern, not a consensus algorithm concern.
//
// Migration:
//   OLD: import "github.com/luxfi/consensus/uptime"
//   NEW: import "github.com/luxfi/node/uptime"
package uptime

import "errors"

var ErrDeprecated = errors.New("uptime package should be in github.com/luxfi/node/uptime")

// Deprecated: Use node's uptime tracking
type Calculator interface {
	Deprecated()
}

// Deprecated: Use node's uptime manager
type Manager interface {
	Deprecated()
}