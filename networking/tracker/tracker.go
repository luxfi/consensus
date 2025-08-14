package tracker

import "github.com/luxfi/ids"

// Tracker tracks network metrics
type Tracker interface {
    Connected(nodeID ids.NodeID)
    Disconnected(nodeID ids.NodeID)
}
