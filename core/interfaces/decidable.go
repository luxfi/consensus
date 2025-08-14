package interfaces

import "github.com/luxfi/ids"

// Decidable represents an item that can be decided by consensus
type Decidable interface {
    ID() ids.ID
    Accept() error
    Reject() error
    Status() Status
}
