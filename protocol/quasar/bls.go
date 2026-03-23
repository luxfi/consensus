// Package quasar - BLS DAG Event Horizon Implementation
//
// This file implements DAG-based event horizon consensus for vertex ordering.
// It uses HMAC-SHA256 keyed commitments for local attestations within the DAG,
// providing cryptographically sound message authentication for the event horizon.
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
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"sort"
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
	BLSAgg  []byte // HMAC-SHA256 keyed with blsKey
	PQCert  []byte // HMAC-SHA256 keyed with pqKey
	Message []byte // The message digest that was signed
}

// Verify performs structural checks only (non-empty fields).
//
// Deprecated: Use VerifyWithKeys for cryptographic HMAC verification.
// For full threshold BLS + Ringtail verification, see quasar.go
// signer.VerifyAggregatedSignature and epoch.go VerifySignatureForEpoch.
func (c *CertBundle) Verify(_ []string) bool {
	return c != nil && len(c.BLSAgg) > 0 && len(c.PQCert) > 0
}

// VerifyWithKeys verifies both HMAC-SHA256 certificates against the provided keys.
// Returns true only if both the BLS and PQ HMACs are valid for the stored message.
//
// For full threshold BLS + Ringtail signature verification, see:
//   - quasar.go: signer.VerifyAggregatedSignature (BLS threshold)
//   - epoch.go: EpochManager.VerifySignatureForEpoch (Ringtail threshold)
func (c *CertBundle) VerifyWithKeys(blsKey, pqKey []byte) bool {
	if c == nil || len(c.BLSAgg) == 0 || len(c.PQCert) == 0 || len(c.Message) == 0 {
		return false
	}

	// Verify BLS HMAC
	blsMAC := hmac.New(sha256.New, blsKey)
	blsMAC.Write(c.Message)
	if !hmac.Equal(c.BLSAgg, blsMAC.Sum(nil)) {
		return false
	}

	// Verify PQ HMAC
	pqMAC := hmac.New(sha256.New, pqKey)
	pqMAC.Write(c.Message)
	return hmac.Equal(c.PQCert, pqMAC.Sum(nil))
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

// generateBLSAggregate generates an HMAC-SHA256 commitment for DAG event horizon,
// keyed with the validator's BLS key material. This binds the block ID and votes
// to the key using a cryptographically sound MAC construction.
//
// For full threshold BLS aggregate signatures, see quasar.go:
//   - signer.AggregateSignatures (threshold aggregation via github.com/luxfi/crypto/bls)
//   - signer.VerifyAggregatedSignature (threshold verification)
func (q *BLS) generateBLSAggregate(blockID ids.ID, votes map[string]int) []byte {
	if len(q.blsKey) == 0 {
		return []byte{}
	}
	return computeHMAC(q.blsKey, buildVoteDigest(blockID, votes))
}

// generatePQCertificate generates an HMAC-SHA256 commitment for DAG event horizon,
// keyed with the validator's PQ key material. This binds the block ID and votes
// to the key using a cryptographically sound MAC construction.
//
// For full Ringtail threshold signatures, see:
//   - quasar.go: signer.RingtailRound1/Round2/Finalize (2-round protocol)
//   - epoch.go: EpochManager.VerifySignatureForEpoch (via github.com/luxfi/ringtail/threshold)
func (q *BLS) generatePQCertificate(blockID ids.ID, votes map[string]int) []byte {
	if len(q.pqKey) == 0 {
		return []byte{}
	}
	return computeHMAC(q.pqKey, buildVoteDigest(blockID, votes))
}

// buildVoteDigest creates a deterministic message digest from a block ID and vote map.
// Used as the HMAC input for both BLS and PQ certificate generation.
func buildVoteDigest(blockID ids.ID, votes map[string]int) []byte {
	h := sha256.New()
	h.Write(blockID[:])

	bufPtr := uint64BufPool.Get().(*[]byte)
	countBytes := *bufPtr
	defer uint64BufPool.Put(bufPtr)

	// Sort validators for deterministic digest
	validators := make([]string, 0, len(votes))
	for v := range votes {
		validators = append(validators, v)
	}
	sort.Strings(validators)

	for _, validator := range validators {
		count := votes[validator]
		h.Write([]byte(validator))
		binary.LittleEndian.PutUint64(countBytes, uint64(count))
		h.Write(countBytes)
	}

	return h.Sum(nil)
}

// computeHMAC returns HMAC-SHA256(key, message).
func computeHMAC(key, message []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(message)
	return mac.Sum(nil)
}

// phaseI proposes a block from the DAG frontier using FIFO ordering.
// The frontier is maintained in insertion order, so the first element
// is the oldest pending block with highest priority.
func (q *BLS) phaseI(frontier []string) string {
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
		// Convert proposal to ID for certificate generation
		proposalHash := sha256.Sum256([]byte(proposal))
		blockID, _ := ids.ToID(proposalHash[:])

		// Build message digest once -- map iteration is non-deterministic,
		// so all three fields must use the same digest.
		msg := buildVoteDigest(blockID, votes)

		return &CertBundle{
			BLSAgg:  computeHMAC(q.blsKey, msg),
			PQCert:  computeHMAC(q.pqKey, msg),
			Message: msg,
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
// Uses HMAC-SHA256 keyed with blsKey and pqKey for cryptographically sound binding.
//
// For full threshold signatures with real BLS + Ringtail, see:
//   - quasar.go: signer.AggregateSignatures (BLS threshold via github.com/luxfi/crypto/bls)
//   - epoch.go: BundleSigner.SignBundle (Ringtail 2-round via github.com/luxfi/ringtail/threshold)
func (q *BLS) createHorizonSignature(checkpoint VertexID, validators []string) []byte {
	// Create message digest: checkpoint + validators
	h := sha256.New()
	h.Write(checkpoint[:])
	for _, v := range validators {
		h.Write([]byte(v))
	}
	message := h.Sum(nil)

	// BLS component: HMAC-SHA256(blsKey, message)
	blsMAC := hmac.New(sha256.New, q.blsKey)
	blsMAC.Write(message)
	blsPart := blsMAC.Sum(nil)

	// PQ component: HMAC-SHA256(pqKey, message)
	pqMAC := hmac.New(sha256.New, q.pqKey)
	pqMAC.Write(message)
	pqPart := pqMAC.Sum(nil)

	// Combined attestation (64 bytes: 32 BLS + 32 PQ)
	return append(blsPart, pqPart...)
}
