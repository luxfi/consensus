package interfaces

import "github.com/luxfi/ids"

// Decidable represents an item that can be decided
type Decidable interface {
    ID() ids.ID
    Status() Status
    Accept() error
    Reject() error
}
