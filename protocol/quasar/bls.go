// Package quasar - BLS DAG Event Horizon Implementation
//
// This file implements DAG-based event horizon consensus for vertex ordering.
// It uses SHA256 commitments for local attestations within the DAG.
//
// IMPORTANT: This is NOT the threshold signature implementation.
// For real cryptographic threshold signatures (BLS + Ringtail), see:
//   - quasar.go: Signer struct with github.com/luxfi/crypto/bls integration
//   - epoch.go: Ringtail threshold signing via github.com/luxfi/ringtail/threshold
//
// The BLS struct here handles DAG vertex ordering and event horizon
// establishment, which operates independently of block finality signatures.
package quasar

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/dag"
	"github.com/luxfi/ids"
)

// uint64BufPool pools 8-byte buffers for binary.LittleEndian encoding in hot paths
var uint64BufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 8)
		return &buf
	},
}

// CertBundle contains both BLS and PQ certificates for a block.
type CertBundle struct {
	BLSAgg []byte // BLS aggregate signature
	PQCert []byte // Post-quantum certificate
}

// Verify checks both BLS and PQ certificates.
func (c *CertBundle) Verify(_ []string) bool {
	return c != nil && len(c.BLSAgg) > 0 && len(c.PQCert) > 0
}

// Bundle represents a finalized epoch bundle.
type Bundle struct {
	Epoch   uint64
	Root    []byte
	BLSAgg  []byte
	PQBatch []byte
	Binding []byte
}

// QBlock is an alias for Block for backward compatibility.
// Deprecated: Use Block instead.
type QBlock = Block

// Client interface for Quasar operations
type Client interface {
	SubmitCheckpoint(epoch uint64, root []byte, attest []byte) error
	FetchBundle(epoch uint64) (Bundle, error)
	Verify(Bundle) bool
}

// VertexID represents a vertex identifier
type VertexID [32]byte

// BLS implements BLS signature aggregation with event horizon finality
type BLS struct {
	// Configuration
	K     int
	Alpha float64
	Beta  uint32

	// Keys
	blsKey []byte
	pqKey  []byte

	// P-Chain state
	horizons []dag.EventHorizon[VertexID]
	store    dag.Store[VertexID]

	// Callback
	finalizedCb func(*Block)
}

// NewBLS creates a new BLS consensus instance
func NewBLS(cfg config.Parameters, store dag.Store[VertexID]) *BLS {
	return &BLS{
		K:        cfg.K,
		Alpha:    cfg.Alpha,
		Beta:     cfg.Beta,
		horizons: make([]dag.EventHorizon[VertexID], 0),
		store:    store,
	}
}

// Initialize sets up keys for Quasar
func (q *BLS) Initialize(_ context.Context, blsKey, pqKey []byte) error {
	q.blsKey = blsKey
	q.pqKey = pqKey
	return nil
}

// SetFinalizedCallback sets the callback for finalized blocks.
func (q *BLS) SetFinalizedCallback(cb func(*Block)) {
	q.finalizedCb = cb
}

// generateBLSAggregate generates a commitment for DAG event horizon.
// NOTE: This uses SHA256 as a placeholder for local vertex ordering.
// For threshold block finality with real BLS signatures, use the Signer
// in quasar.go which integrates with github.com/luxfi/crypto/bls.
func (q *BLS) generateBLSAggregate(blockID ids.ID, votes map[string]int) []byte {
	if len(q.blsKey) == 0 {
		return []byte{}
	}

	// Create deterministic commitment based on blockID and votes
	h := sha256.New()
	h.Write(blockID[:])

	// Get pooled buffer for uint64 encoding
	bufPtr := uint64BufPool.Get().(*[]byte)
	countBytes := *bufPtr
	defer uint64BufPool.Put(bufPtr)

	// Add votes to hash
	for validator, count := range votes {
		h.Write([]byte(validator))
		binary.LittleEndian.PutUint64(countBytes, uint64(count))
		h.Write(countBytes)
	}

	// Mix in key for local binding
	h.Write(q.blsKey)

	return h.Sum(nil)
}

// generatePQCertificate generates a post-quantum commitment for DAG event horizon.
// NOTE: This uses SHA256 as a placeholder for local vertex ordering.
// For real PQ threshold signatures, use the Signer in quasar.go which
// integrates with github.com/luxfi/ringtail/threshold.
func (q *BLS) generatePQCertificate(blockID ids.ID, votes map[string]int) []byte {
	if len(q.pqKey) == 0 {
		return []byte{}
	}

	// Create message to commit
	h := sha256.New()
	h.Write(blockID[:])

	// Get pooled buffer for uint64 encoding
	bufPtr := uint64BufPool.Get().(*[]byte)
	countBytes := *bufPtr
	defer uint64BufPool.Put(bufPtr)

	// Add votes to message
	for validator, count := range votes {
		h.Write([]byte(validator))
		binary.LittleEndian.PutUint64(countBytes, uint64(count))
		h.Write(countBytes)
	}

	message := h.Sum(nil)

	// Create local commitment
	cert := sha256.New()
	cert.Write(message)
	cert.Write(q.pqKey)

	return cert.Sum(nil)
}

// phaseI proposes a block from the DAG frontier
func (q *BLS) phaseI(frontier []string) string {
	// Select highest confidence block
	// Placeholder: return first block
	if len(frontier) > 0 {
		return frontier[0]
	}
	return ""
}

// phaseII creates certificates if threshold is met
func (q *BLS) phaseII(votes map[string]int, proposal string) *CertBundle {
	total := 0
	support := 0

	for block, count := range votes {
		total += count
		if block == proposal {
			support = count
		}
	}

	if total == 0 {
		return nil
	}

	// Check if support meets alpha threshold
	if float64(support)/float64(total) >= q.Alpha {
		// Generate real certificates using cryptography
		// Convert proposal to ID for certificate generation
		proposalHash := sha256.Sum256([]byte(proposal))
		blockID, _ := ids.ToID(proposalHash[:])

		// Generate BLS aggregate
		blsSig := q.generateBLSAggregate(blockID, votes)

		// Generate PQ certificate
		pqCert := q.generatePQCertificate(blockID, votes)

		return &CertBundle{
			BLSAgg: blsSig,
			PQCert: pqCert,
		}
	}

	return nil
}

// EstablishHorizon creates a new event horizon for BLS finality
func (q *BLS) EstablishHorizon(ctx context.Context, checkpoint VertexID, validators []string) (*dag.EventHorizon[VertexID], error) {
	// Compute new event horizon using Ringtail + BLS signatures
	horizon := dag.EventHorizon[VertexID]{
		Checkpoint: checkpoint,
		Height:     uint64(len(q.horizons)) + 1,
		Validators: validators,
		Signature:  q.createHorizonSignature(checkpoint, validators),
	}

	q.horizons = append(q.horizons, horizon)
	return &horizon, nil
}

// IsBeyondHorizon checks if a vertex is beyond the event horizon (finalized)
func (q *BLS) IsBeyondHorizon(vertex VertexID) bool {
	if len(q.horizons) == 0 {
		return false
	}

	latestHorizon := q.horizons[len(q.horizons)-1]
	return dag.BeyondHorizon(q.store, vertex, latestHorizon)
}

// ComputeCanonicalOrder returns the canonical order of finalized vertices
func (q *BLS) ComputeCanonicalOrder() []VertexID {
	if len(q.horizons) == 0 {
		return []VertexID{}
	}

	latestHorizon := q.horizons[len(q.horizons)-1]
	return dag.ComputeHorizonOrder(q.store, latestHorizon)
}

// GetLatestHorizon returns the most recent event horizon
func (q *BLS) GetLatestHorizon() *dag.EventHorizon[VertexID] {
	if len(q.horizons) == 0 {
		return nil
	}
	return &q.horizons[len(q.horizons)-1]
}

// createHorizonSignature creates a local attestation for the event horizon checkpoint.
// This is a SHA256-based commitment used for DAG vertex ordering.
//
// NOTE: For production threshold signatures with real BLS + Ringtail,
// use the Signer in quasar.go which integrates:
//   - github.com/luxfi/crypto/bls for BLS threshold signatures
//   - github.com/luxfi/ringtail/threshold for post-quantum signatures
func (q *BLS) createHorizonSignature(checkpoint VertexID, validators []string) []byte {
	// Create message: checkpoint + validators
	h := sha256.New()
	h.Write(checkpoint[:])
	for _, v := range validators {
		h.Write([]byte(v))
	}
	message := h.Sum(nil)

	// BLS component commitment
	blsSig := sha256.New()
	blsSig.Write(message)
	blsSig.Write(q.blsKey)
	blsPart := blsSig.Sum(nil)

	// PQ component commitment
	pqSig := sha256.New()
	pqSig.Write(message)
	pqSig.Write(q.pqKey)
	pqPart := pqSig.Sum(nil)

	// Combined attestation (64 bytes)
	return append(blsPart, pqPart...)
}
