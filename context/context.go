// Package context provides consensus context for VMs
package context

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// Context provides consensus context for VMs
type Context struct {
	// QuantumID is the root quantum network identifier
	QuantumID uint32 `json:"quantumID"`
	// NetworkID is an alias for QuantumID for backward compatibility
	NetworkID uint32 `json:"networkID"`
	// NetID identifies the specific network/subnet within the quantum network
	NetID    ids.ID `json:"netID"`
	SubnetID ids.ID `json:"subnetID"` // Alias for NetID
	// ChainID identifies the specific chain within the network
	ChainID      ids.ID     `json:"chainID"`
	NodeID       ids.NodeID `json:"nodeID"`
	PublicKey    []byte     `json:"publicKey"`
	XChainID     ids.ID     `json:"xChainID"`
	CChainID     ids.ID     `json:"cChainID"`
	XAssetID     ids.ID     `json:"xAssetID"`
	LUXAssetID   ids.ID     `json:"luxAssetID"`
	ChainDataDir string     `json:"chainDataDir"`

	// Timing
	StartTime time.Time `json:"startTime"`

	ValidatorState  interface{} // validators.State or ValidatorState interface
	Keystore        Keystore
	Metrics         interface{} // metrics.MultiGatherer or Metrics interface
	Log             interface{} // logging.Logger
	SharedMemory    interface{} // atomic.SharedMemory
	BCLookup        BCLookup    // Blockchain alias lookup
	WarpSigner      interface{} // warp.Signer
	NetworkUpgrades interface{} // upgrade.Config

	// Lock for thread-safe access to context
	Lock sync.RWMutex
}

// BCLookup provides blockchain alias lookup
type BCLookup interface {
	Lookup(alias string) (ids.ID, error)
	PrimaryAlias(id ids.ID) (string, error)
	Aliases(id ids.ID) ([]string, error)
}

// ValidatorState provides validator information
// This is kept as a minimal interface for compatibility with node package
type ValidatorState interface {
	GetChainID(ids.ID) (ids.ID, error)
	GetNetID(ids.ID) (ids.ID, error)
	GetSubnetID(chainID ids.ID) (ids.ID, error)
	GetValidatorSet(uint64, ids.ID) (map[ids.NodeID]uint64, error)
	GetCurrentHeight(context.Context) (uint64, error)
	GetMinimumHeight(context.Context) (uint64, error)
}

// GetValidatorOutput contains validator information
type GetValidatorOutput struct {
	NodeID    ids.NodeID
	PublicKey []byte
	Weight    uint64
}

// Keystore provides key management
type Keystore interface {
	GetDatabase(username, password string) (interface{}, error)
	NewAccount(username, password string) error
}

// BlockchainIDLookup is an alias for BCLookup for backward compatibility
type BlockchainIDLookup = BCLookup

// Metrics provides metrics tracking
type Metrics interface {
	Register(namespace string, registerer interface{}) error
}

// GetTimestamp returns the current timestamp
func GetTimestamp() int64 {
	return time.Now().Unix()
}

// GetChainID gets the chain ID from context
func GetChainID(ctx context.Context) ids.ID {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.ChainID
	}
	return ids.Empty
}

// GetNetID gets the network ID from context
func GetNetID(ctx context.Context) ids.ID {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.NetID
	}
	return ids.Empty
}

// Deprecated: GetSubnetID is deprecated, use GetNetID instead
func GetSubnetID(ctx context.Context) ids.ID {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.NetID
	}
	return ids.Empty
}

// GetNetworkID gets the network ID from context
func GetNetworkID(ctx context.Context) uint32 {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.QuantumID
	}
	return 0
}

// GetValidatorState gets the validator state from context
func GetValidatorState(ctx context.Context) interface{} {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.ValidatorState
	}
	return nil
}

// GetWarpSigner gets the warp signer from context
func GetWarpSigner(ctx context.Context) interface{} {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.WarpSigner
	}
	return nil
}

// WithContext adds consensus context to a context
func WithContext(ctx context.Context, cc *Context) context.Context {
	return context.WithValue(ctx, contextKey, cc)
}

// FromContext extracts consensus context from a context
func FromContext(ctx context.Context) *Context {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c
	}
	return nil
}

// GetNodeID gets the node ID from context
func GetNodeID(ctx context.Context) ids.NodeID {
	if c, ok := ctx.Value(contextKey).(*Context); ok {
		return c.NodeID
	}
	return ids.EmptyNodeID
}

// IDs holds the IDs for consensus context
type IDs struct {
	NetworkID    uint32
	QuantumID    uint32
	NetID        ids.ID
	ChainID      ids.ID
	NodeID       ids.NodeID
	PublicKey    []byte
	XAssetID     ids.ID
	LUXAssetID   ids.ID `json:"luxAssetID"`
	ChainDataDir string `json:"chainDataDir"`
}

// WithIDs adds IDs to the context
func WithIDs(ctx context.Context, ids IDs) context.Context {
	c := FromContext(ctx)
	if c == nil {
		c = &Context{}
	}
	c.QuantumID = ids.QuantumID
	c.NetID = ids.NetID
	c.ChainID = ids.ChainID
	c.NodeID = ids.NodeID
	c.PublicKey = ids.PublicKey
	return WithContext(ctx, c)
}

// WithValidatorState adds validator state to the context
func WithValidatorState(ctx context.Context, vs interface{}) context.Context {
	c := FromContext(ctx)
	if c == nil {
		c = &Context{}
	}
	c.ValidatorState = vs
	return WithContext(ctx, c)
}

type contextKeyType struct{}

var contextKey = contextKeyType{}

// Logger provides logging functionality
type Logger interface {
	Debug(msg string, fields ...interface{})
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Fatal(msg string, fields ...interface{})
}

// SharedMemory provides cross-chain shared memory
type SharedMemory interface {
	Get(peerChainID ids.ID, keys [][]byte) (values [][]byte, err error)
	Indexed(
		peerChainID ids.ID,
		traits [][]byte,
		startTrait, startKey []byte,
		limit int,
	) (values [][]byte, lastTrait, lastKey []byte, err error)
	// Apply applies atomic requests to shared memory
	Apply(requests map[ids.ID]*AtomicRequests, batch interface{}) error
}

// AtomicRequests contains atomic operations for a chain
type AtomicRequests struct {
	RemoveRequests [][]byte            // Keys to remove
	PutRequests    []*AtomicPutRequest // Key-value pairs to put
}

// AtomicPutRequest represents a put operation in shared memory
type AtomicPutRequest struct {
	Key    []byte   // The key to store
	Value  []byte   // The value to store
	Traits [][]byte // Traits for indexing
}

// WarpSigner provides BLS signing for Warp messages
type WarpSigner interface {
	// Sign signs the given message and returns the signature
	Sign(msg interface{}) ([]byte, error)
	// PublicKey returns the BLS public key bytes
	PublicKey() []byte
	// NodeID returns the node ID associated with this signer
	NodeID() ids.NodeID
}

// NetworkUpgrades contains network upgrade activation times
type NetworkUpgrades interface {
	// IsApricotPhase3Activated returns true if the Apricot Phase 3 upgrade is activated
	IsApricotPhase3Activated(timestamp time.Time) bool
	// IsApricotPhase5Activated returns true if the Apricot Phase 5 upgrade is activated
	IsApricotPhase5Activated(timestamp time.Time) bool
	// IsBanffActivated returns true if the Banff upgrade is activated
	IsBanffActivated(timestamp time.Time) bool
	// IsCortinaActivated returns true if the Cortina upgrade is activated
	IsCortinaActivated(timestamp time.Time) bool
	// IsDurangoActivated returns true if the Durango upgrade is activated
	IsDurangoActivated(timestamp time.Time) bool
	// IsEtnaActivated returns true if the Etna upgrade is activated
	IsEtnaActivated(timestamp time.Time) bool
}
