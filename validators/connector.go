package validators

import (
    "context"
    "github.com/luxfi/ids"
)

// Connector provides connection information for validators
type Connector interface {
    Connected(ctx context.Context, nodeID ids.NodeID, version interface{}) error
    Disconnected(ctx context.Context, nodeID ids.NodeID) error
}