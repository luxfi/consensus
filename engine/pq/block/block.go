package block

import (
    "context"
    "time"
    "github.com/luxfi/ids"
)

// QuantumBlock is a post-quantum secured block
type QuantumBlock interface {
    ID() ids.ID
    ParentID() ids.ID
    Height() uint64
    Timestamp() time.Time
    
    // Post-quantum specific methods
    QuantumSignature() []byte
    QuantumProof() []byte
    Algorithm() string // ML-DSA-44, ML-DSA-65, ML-DSA-87
    
    // Standard block methods
    Bytes() []byte
    Verify(context.Context) error
    Accept(context.Context) error
    Reject(context.Context) error
}

// QuantumVM defines a post-quantum VM
type QuantumVM interface {
    // ParseBlock parses a block
    ParseBlock(context.Context, []byte) (QuantumBlock, error)
    
    // BuildBlock builds a block
    BuildBlock(context.Context) (QuantumBlock, error)
    
    // GetBlock gets a block
    GetBlock(context.Context, ids.ID) (QuantumBlock, error)
    
    // VerifyQuantumProof verifies a quantum proof
    VerifyQuantumProof([]byte, QuantumBlock) error
    
    // GenerateQuantumSignature generates a quantum signature
    GenerateQuantumSignature(QuantumBlock) ([]byte, error)
}