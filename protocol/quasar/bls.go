// Package quasar - BLS DAG Event Horizon Implementation
//
// This file implements DAG-based event horizon consensus for vertex ordering.
// It uses KMAC256 (NIST SP 800-185) keyed commitments for local attestations
// within the DAG, providing FIPS-aligned message authentication that slots
// into the SHA-3 hash suite the strict-PQ security profile pins.
//
// IMPORTANT: This is NOT the threshold signature implementation.
// For real cryptographic threshold signatures (BLS + Corona), see:
//   - quasar.go: Signer struct with github.com/luxfi/crypto/bls integration
//   - epoch.go: Pulsar threshold signing via github.com/luxfi/corona/threshold (Corona-equivalent kernel)
//
// The BLS struct here handles DAG vertex ordering and event horizon
// establishment, which operates independently of block finality signatures.
//
// Each MAC call site uses a distinct SP 800-185 customization string
// (see kmac256.go) so two MACs over the same (key, message) bytes
// remain independent oracles — domain separation is built into the
// kernel, not bolted on by call-site message prefixing.
package quasar

import (
	"context"
	"encoding/binary"
	"sort"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/dag"
	"github.com/luxfi/ids"
	"golang.org/x/crypto/sha3"
)

// uint64BufPool pools 8-byte buffers for binary.LittleEndian encoding in hot paths
var uint64BufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 8)
		return &buf
	},
}

// CertBundle contains both BLS and PQ certificates for a block.
//
// BLSAgg and PQCert are SP 800-185 KMAC256 outputs. The canonical
// production width is 48 bytes (SHA3-384). VerifyWithKeys computes a
// KMAC256 at exactly the stored byte length so legacy 32-byte bundles
// roundtrip on the classical-compat profile; the strict-PQ profile
// refuses anything narrower than 48 via VerifyWithKeysUnderProfile.
type CertBundle struct {
	BLSAgg  []byte // KMAC256(blsKey, Message, len, "QUASAR_EVENT_HORIZON_BLS_MAC_V1")
	PQCert  []byte // KMAC256(pqKey,  Message, len, "QUASAR_EVENT_HORIZON_PQ_MAC_V1")
	Message []byte // The message digest that was authenticated
}

// Verify is removed. Use VerifyWithKeys for cryptographic MAC verification.
// For full threshold BLS + Corona verification, see quasar.go
// signer.VerifyAggregatedSignature and epoch.go VerifySignatureForEpoch.
func (c *CertBundle) Verify(_ []string) bool {
	panic("CertBundle.Verify is removed: use VerifyWithKeys for cryptographic verification")
}

// VerifyWithKeys verifies both KMAC256 certificates against the provided
// keys. Returns true only if both the BLS and PQ MACs are valid for the
// stored message.
//
// The MAC width is read from the stored bytes — KMAC256 is variable-
// output by design, so a 32-byte legacy bundle and a 48-byte canonical
// bundle each verify against their own KMAC256(outLen) value. A
// strict-PQ caller that wants to refuse the legacy width must use
// VerifyWithKeysUnderProfile.
//
// For full threshold BLS + Corona signature verification, see:
//   - quasar.go: signer.VerifyAggregatedSignature (BLS threshold)
//   - epoch.go: EpochManager.VerifySignatureForEpoch (Corona threshold)
func (c *CertBundle) VerifyWithKeys(blsKey, pqKey []byte) bool {
	if c == nil || len(c.BLSAgg) == 0 || len(c.PQCert) == 0 || len(c.Message) == 0 {
		return false
	}

	wantBLS := kmac256(blsKey, c.Message, len(c.BLSAgg), customQuasarEventHorizonBLSMAC)
	if !macEqual(c.BLSAgg, wantBLS) {
		return false
	}

	wantPQ := kmac256(pqKey, c.Message, len(c.PQCert), customQuasarEventHorizonPQMAC)
	return macEqual(c.PQCert, wantPQ)
}

// VerifyWithKeysUnderProfile is VerifyWithKeys with an additional
// strict-PQ gate: when the profile pins HashSuiteSHA3NIST, the MAC
// bytes MUST be the canonical KMAC256 width (kmacMACOutLen). Legacy
// 32-byte bundles produced before this package switched from HMAC-
// SHA256 to KMAC256 are refused outright.
//
// Closes the F-class objection that a strict-PQ profile silently
// accepts a sub-width MAC.
func (c *CertBundle) VerifyWithKeysUnderProfile(blsKey, pqKey []byte, profile *config.ChainSecurityProfile) bool {
	if strictPQRejectsLegacyMAC(profile) {
		if len(c.BLSAgg) < kmacMACOutLen || len(c.PQCert) < kmacMACOutLen {
			return false
		}
	}
	return c.VerifyWithKeys(blsKey, pqKey)
}

// Bundle represents a finalized epoch bundle.
type Bundle struct {
	Epoch   uint64
	Root    []byte
	BLSAgg  []byte
	PQBatch []byte
	Binding []byte
}

// Client interface for Quasar operations
type Client interface {
	SubmitCheckpoint(epoch uint64, root []byte, attest []byte) error
	FetchBundle(epoch uint64) (Bundle, error)
	Verify(Bundle) bool
}

// VertexID represents a vertex identifier
type VertexID [32]byte

// BLS implements BLS signature aggregation with event horizon finality.
//
// The MAC kernel is KMAC256 (SP 800-185 §4) keyed with blsKey / pqKey
// and per-call-site customization strings. See kmac256.go for the
// customization registry and bls.go ↓ for the call sites.
//
// Profile binding: SetProfile attaches a ChainSecurityProfile so that
// emission and verification can enforce profile-class refusals (e.g.
// strict-PQ refuses legacy-width MAC bytes). The profile pointer is
// optional; a nil profile means "no profile-class enforcement", which
// is the correct default for embedded test scenarios.
type BLS struct {
	// Configuration
	K     int
	Alpha float64
	Beta  uint32

	// Keys
	blsKey []byte
	pqKey  []byte

	// Optional profile binding for strict-PQ enforcement. Set via
	// SetProfile; nil means no profile-class enforcement.
	profile *config.ChainSecurityProfile

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

// SetProfile attaches a ChainSecurityProfile to this BLS engine. When
// set, the strict-PQ profile (HashSuiteID = HashSuiteSHA3NIST) forces
// MAC emission to the canonical KMAC256 width and refuses legacy-width
// MAC bytes on the verify path through VerifyWithKeysUnderProfile.
//
// A nil profile means "no profile-class enforcement", which is the
// correct default for embedded test scenarios that build CertBundles
// directly without going through the locked profile path.
func (q *BLS) SetProfile(p *config.ChainSecurityProfile) {
	q.profile = p
}

// generateBLSAggregate generates a KMAC256 commitment for DAG event
// horizon, keyed with the validator's BLS key material. Binds the
// block ID and votes to the key under the SP 800-185 customization
// "QUASAR_EVENT_HORIZON_BLS_MAC_V1".
//
// For full threshold BLS aggregate signatures, see quasar.go:
//   - signer.AggregateSignatures (threshold aggregation via github.com/luxfi/crypto/bls)
//   - signer.VerifyAggregatedSignature (threshold verification)
func (q *BLS) generateBLSAggregate(blockID ids.ID, votes map[string]int) []byte {
	if len(q.blsKey) == 0 {
		return []byte{}
	}
	return kmac256(q.blsKey, buildVoteDigest(blockID, votes), kmacMACOutLen, customQuasarEventHorizonBLSMAC)
}

// generatePQCertificate generates a KMAC256 commitment for DAG event
// horizon, keyed with the validator's PQ key material. Binds the
// block ID and votes to the key under the SP 800-185 customization
// "QUASAR_EVENT_HORIZON_PQ_MAC_V1".
//
// For full Corona threshold signatures, see:
//   - quasar.go: signer.CoronaRound1/Round2/Finalize (2-round protocol)
//   - epoch.go: EpochManager.VerifySignatureForEpoch (via github.com/luxfi/corona/threshold)
func (q *BLS) generatePQCertificate(blockID ids.ID, votes map[string]int) []byte {
	if len(q.pqKey) == 0 {
		return []byte{}
	}
	return kmac256(q.pqKey, buildVoteDigest(blockID, votes), kmacMACOutLen, customQuasarEventHorizonPQMAC)
}

// buildVoteDigest creates a deterministic message digest from a block ID
// and vote map. Used as the KMAC256 input for both BLS and PQ
// certificate generation.
//
// Kernel: cSHAKE256 with customization "QUASAR_VOTE_DIGEST_V1", reading
// 32 bytes. cSHAKE256 sits in the SP 800-185 family pinned by the
// strict-PQ profile, so the digest preimage is itself FIPS-aligned —
// no SHA-2 in the chain even though the MAC kernel above could absorb
// any byte string.
func buildVoteDigest(blockID ids.ID, votes map[string]int) []byte {
	bufPtr := uint64BufPool.Get().(*[]byte)
	countBytes := *bufPtr
	defer uint64BufPool.Put(bufPtr)

	// Sort validators for deterministic digest.
	validators := make([]string, 0, len(votes))
	for v := range votes {
		validators = append(validators, v)
	}
	sort.Strings(validators)

	h := sha3.NewCShake256(nil, []byte(customVoteDigest))
	_, _ = h.Write(blockID[:])
	for _, validator := range validators {
		count := votes[validator]
		_, _ = h.Write([]byte(validator))
		binary.LittleEndian.PutUint64(countBytes, uint64(count))
		_, _ = h.Write(countBytes)
	}
	out := make([]byte, 32)
	_, _ = h.Read(out)
	return out
}

// customVoteDigest is the cSHAKE256 customization for buildVoteDigest.
// Pinned at "_V1"; bumping invalidates every prior digest.
const customVoteDigest = "QUASAR_VOTE_DIGEST_V1"

// phaseI proposes a block from the DAG frontier using FIFO ordering.
// The frontier is maintained in insertion order, so the first element
// is the oldest pending block with highest priority.
func (q *BLS) phaseI(frontier []string) string {
	if len(frontier) > 0 {
		return frontier[0]
	}
	return ""
}

// phaseII creates certificates if the support fraction meets the alpha
// threshold. Both MACs in the returned CertBundle are KMAC256 outputs
// over the same canonical message digest, keyed independently by the
// BLS and PQ keys with distinct SP 800-185 customizations.
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

	if float64(support)/float64(total) >= q.Alpha {
		// Derive a stable blockID from the proposal string. Uses
		// cSHAKE256 with a pinned customization so the derivation
		// lives in the SP 800-185 family — no SHA-2 in the chain.
		blockID, _ := ids.ToID(deriveProposalID(proposal))

		// Build message digest once -- map iteration is non-deterministic,
		// so all three fields must use the same digest.
		msg := buildVoteDigest(blockID, votes)

		return &CertBundle{
			BLSAgg:  kmac256(q.blsKey, msg, kmacMACOutLen, customQuasarEventHorizonBLSMAC),
			PQCert:  kmac256(q.pqKey, msg, kmacMACOutLen, customQuasarEventHorizonPQMAC),
			Message: msg,
		}
	}

	return nil
}

// deriveProposalID returns a 32-byte deterministic ID for a proposal
// string under SP 800-185 cSHAKE256 with customization
// "QUASAR_PROPOSAL_ID_V1". This replaces the prior SHA-256 derivation
// so the entire phaseII path sits in the SHA-3 family pinned by the
// strict-PQ profile.
func deriveProposalID(proposal string) []byte {
	h := sha3.NewCShake256(nil, []byte(customProposalID))
	_, _ = h.Write([]byte(proposal))
	out := make([]byte, 32)
	_, _ = h.Read(out)
	return out
}

// customProposalID is the cSHAKE256 customization for deriveProposalID.
const customProposalID = "QUASAR_PROPOSAL_ID_V1"

// EstablishHorizon creates a new event horizon for BLS finality
func (q *BLS) EstablishHorizon(ctx context.Context, checkpoint VertexID, validators []string) (*dag.EventHorizon[VertexID], error) {
	// Compute new event horizon using Corona + BLS signatures
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

// createHorizonSignature creates a local attestation for the event
// horizon checkpoint. Uses KMAC256 (SP 800-185) keyed with blsKey and
// pqKey, with per-half customization strings — distinct customization
// makes the two halves independent oracles over the same (key, message)
// pair.
//
// Output layout: BLS half (kmacMACOutLen bytes) || PQ half (kmacMACOutLen
// bytes) — 96 bytes total at the canonical SHA3-384 width.
//
// For full threshold signatures with real BLS + Corona, see:
//   - quasar.go: signer.AggregateSignatures (BLS threshold via github.com/luxfi/crypto/bls)
//   - epoch.go: BundleSigner.SignBundle (Pulsar 2-round via github.com/luxfi/corona/threshold)
func (q *BLS) createHorizonSignature(checkpoint VertexID, validators []string) []byte {
	// Build message preimage under cSHAKE256 so the entire MAC chain
	// stays in the SP 800-185 family.
	h := sha3.NewCShake256(nil, []byte(customHorizonSigDigest))
	_, _ = h.Write(checkpoint[:])
	for _, v := range validators {
		_, _ = h.Write([]byte(v))
	}
	message := make([]byte, 32)
	_, _ = h.Read(message)

	blsPart := kmac256(q.blsKey, message, kmacMACOutLen, customQuasarHorizonSigBLSMAC)
	pqPart := kmac256(q.pqKey, message, kmacMACOutLen, customQuasarHorizonSigPQMAC)

	out := make([]byte, 0, len(blsPart)+len(pqPart))
	out = append(out, blsPart...)
	out = append(out, pqPart...)
	return out
}

// customHorizonSigDigest is the cSHAKE256 customization for the
// createHorizonSignature message preimage.
const customHorizonSigDigest = "QUASAR_HORIZON_SIG_DIGEST_V1"
