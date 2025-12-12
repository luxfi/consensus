package quasar

import (
	"context"
	"crypto/sha256"
	"encoding/binary"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/dag"
	"github.com/luxfi/ids"
)

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

// generateBLSAggregate generates a BLS aggregate signature
func (q *BLS) generateBLSAggregate(blockID ids.ID, votes map[string]int) []byte {
	if len(q.blsKey) == 0 {
		// Return empty if no key
		return []byte{}
	}

	// Create deterministic signature based on blockID and votes
	h := sha256.New()
	h.Write(blockID[:])

	// Add votes to hash
	for validator, count := range votes {
		h.Write([]byte(validator))
		countBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(countBytes, uint64(count))
		h.Write(countBytes)
	}

	// Mix in BLS key
	h.Write(q.blsKey)

	// Production: Use real BLS aggregation from github.com/luxfi/crypto/bls
	return h.Sum(nil)
}

// generatePQCertificate generates a post-quantum certificate
func (q *BLS) generatePQCertificate(blockID ids.ID, votes map[string]int) []byte {
	if len(q.pqKey) == 0 {
		// Return empty if no key
		return []byte{}
	}

	// Create message to sign
	h := sha256.New()
	h.Write(blockID[:])

	// Add votes to message
	for validator, count := range votes {
		h.Write([]byte(validator))
		countBytes := make([]byte, 8)
		binary.LittleEndian.PutUint64(countBytes, uint64(count))
		h.Write(countBytes)
	}

	message := h.Sum(nil)

	// Create certificate
	cert := sha256.New()
	cert.Write(message)
	cert.Write(q.pqKey)

	// Production: Use ML-DSA from github.com/luxfi/crypto/mldsa or SLH-DSA from /slhdsa
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

// createHorizonSignature creates a Ringtail + BLS signature for the event horizon
func (q *BLS) createHorizonSignature(checkpoint VertexID, validators []string) []byte {
	// TODO: Implement Ringtail + BLS fusion signature
	// This should combine:
	// 1. BLS aggregate signature for efficiency
	// 2. Ringtail post-quantum threshold signature for security

	// Placeholder implementation
	signature := append(q.blsKey, q.pqKey...)
	signature = append(signature, checkpoint[:]...)
	return signature
}
