package interfaces

import "github.com/luxfi/ids"

// Acceptor processes accepted items
type Acceptor interface {
    Accept(id ids.ID) error
}
