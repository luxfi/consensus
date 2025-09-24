package ids

import "github.com/luxfi/ids"

// Re-export IDs types
type (
	ID     = ids.ID
	NodeID = ids.NodeID
)

// Re-export IDs constants and functions
var (
	Empty          = ids.Empty
	EmptyNodeID    = ids.EmptyNodeID
	GenerateTestID = ids.GenerateTestID
)

// Version represents a version identifier
type Version uint32

// CurrentVersion is the current version
const CurrentVersion Version = 1
