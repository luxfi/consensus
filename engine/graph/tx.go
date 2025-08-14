package graph

import (
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/choices"
)

// Tx represents a transaction in the DAG
type Tx interface {
    ID() ids.ID
    Bytes() []byte
    Status() choices.Status
    Accept() error
    Reject() error
    Verify() error
}