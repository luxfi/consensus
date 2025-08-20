// Package validators is DEPRECATED.
// This package has been moved to the node repository where it belongs.
// Validator management is an application concern, not a consensus algorithm concern.
//
// Migration:
//
//	OLD: import "github.com/luxfi/consensus/validators"
//	NEW: import "github.com/luxfi/node/validators"
//
// This stub exists only for backward compatibility during migration.
package validators

import "errors"

var ErrDeprecated = errors.New("validators package has been moved to github.com/luxfi/node/validators")

// Deprecated: Use github.com/luxfi/node/validators
type Manager interface {
	// This is a stub interface for migration purposes
	Deprecated()
}

// Deprecated: Use github.com/luxfi/node/validators
type State interface {
	// This is a stub interface for migration purposes
	Deprecated()
}

// Deprecated: Use github.com/luxfi/node/validators
type Set interface {
	// This is a stub interface for migration purposes
	Deprecated()
}
