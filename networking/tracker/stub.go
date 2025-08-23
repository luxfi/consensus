// Package tracker is DEPRECATED.
// Resource tracking belongs in the node's network layer.
//
// Migration:
//
//	OLD: import "github.com/luxfi/consensus/networking/tracker"
//	NEW: import "github.com/luxfi/node/network/tracker"
package tracker

import "errors"

var ErrDeprecated = errors.New("tracker package should be in github.com/luxfi/node/network/tracker")

type ResourceTracker interface {
	Deprecated()
}

type CPUTracker interface {
	Deprecated()
}
