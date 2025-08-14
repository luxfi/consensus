package interfaces

import "github.com/luxfi/ids"

// Acceptor defines the interface for accepting decisions
type Acceptor interface {
    Accept() error
    Reject() error
    ID() ids.ID
}
