// Package benchlist is DEPRECATED.
// Peer benchmarking belongs in the node's network layer.
//
// Migration:
//
//	OLD: import "github.com/luxfi/consensus/networking/benchlist"
//	NEW: import "github.com/luxfi/node/network/benchlist"
package benchlist

import "errors"

var ErrDeprecated = errors.New("benchlist package should be in github.com/luxfi/node/network/benchlist")

type Manager interface {
	Deprecated()
}

type Config struct {
	Deprecated bool
}
