// Package consensus provides the core consensus protocols
package consensus

import (
    "github.com/luxfi/crypto/bls"
    "github.com/luxfi/database"
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/core/interfaces"
)

// Export core types
type (
    Context = interfaces.Context
    State   = interfaces.State
    Status  = interfaces.Status
    StateHolder = interfaces.StateHolder
)

// Export constants
const (
    Bootstrapping = interfaces.Bootstrapping
    NormalOp      = interfaces.NormalOp
    
    Unknown    = interfaces.Unknown
    Processing = interfaces.Processing
    Rejected   = interfaces.Rejected
    Accepted   = interfaces.Accepted
)

// ExtendedContext provides full configuration for consensus engines
type ExtendedContext struct {
    interfaces.Context
    
    XChainID        ids.ID
    CChainID        ids.ID
    LUXAssetID      ids.ID
    
    ChainDataDir    string
    SharedMemory    database.Database
    BCLookup        AliasLookup
    ValidatorState  ValidatorState
    WarpSigner      WarpSigner
}

// AliasLookup provides chain alias lookups
type AliasLookup interface {
    PrimaryAlias(id ids.ID) (string, error)
}

// ValidatorState provides validator information
type ValidatorState interface {
    GetCurrentHeight() (uint64, error)
    GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
}

// WarpSigner provides BLS signing for warp messages
type WarpSigner interface {
    Sign(msg []byte) (*bls.Signature, error)
}

// Message types for consensus engine communication
type Message interface{}

type PendingTxs struct{}
