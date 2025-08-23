package types

import "github.com/luxfi/ids"

// NodeID represents a node identifier
type NodeID = ids.NodeID

// ID represents a generic identifier
type ID = ids.ID

// Hash represents a hash value
type Hash = ids.ID

// Signature represents a signature
type Signature []byte

// PublicKey represents a public key
type PublicKey []byte

// PrivateKey represents a private key
type PrivateKey []byte

// Decision represents a consensus decision outcome
type Decision int

const (
	DecideUndecided Decision = iota
	DecideAccept
	DecideReject
)