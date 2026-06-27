// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pulsar_key_era.go — the PulsarKeyEra registry and the deliberately BORING
// Pulsar verifier.
//
// THE COMPACT GROUP KEY. For a 1000+ validator committee the Pulsar lane is
// ONE FIPS-204 ML-DSA signature under ONE group public key — not 1000
// per-validator signatures. A PulsarKeyEra is the registry record that pins
// that single group key to one validator-set era: which chain, which signer
// set (P-Chain-pinned), which key-era id + generation, what parameter set, and
// the activation evidence. One ML-DSA public key verifies the whole
// committee's threshold signature.
//
// DELIBERATELY BORING VERIFIER. The chain does not care HOW the group
// signature was produced — TALUS MPC, an HSM ceremony, a TEE, or a single
// trusted signer. VerifyPulsar verifies a NORMAL ML-DSA signature under the
// era group key over the canonical message M. That is the whole point: the
// trust model of key PRODUCTION (talus-mpc / ceremony / tee / p3q-rollup
// fallback) is recorded in the era's KeygenMode for audit, but it is ORTHOGONAL
// to verification. The verifier stays in its lane: ML-DSA sig, group key, M.
//
// This routes the actual signature check through the SAME stateless FIPS-204
// path the envelope's verifyPulsarLeg uses (pulsarwire.VerifyBytes / the
// on-chain ML-DSA precompile), so the standalone and envelope Pulsar verifies
// agree byte-for-byte — one verify path.
package quasar

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
)

// PulsarKeyEra pins one compact ML-DSA group key to one validator-set era.
// Owner-spec value types are expressed in this package's native types
// (uint32 chain id, [48]byte committed signer-set root) so an era binds
// directly to a ConsensusCert header — values, not places.
type PulsarKeyEra struct {
	// ChainID is the chain this era belongs to (matches ConsensusCert.ChainID).
	ChainID uint32

	// SignerSetID is the committed validator-set identity the group key was
	// generated for — the P-Chain-pinned signer set. Matches a ConsensusCert's
	// ValidatorSetRoot. A signature under this era's key attests THIS signer set.
	SignerSetID [48]byte

	// KeyEraID + Generation identify the era. KeyEraID advances on a signer-set
	// rotation; Generation advances on a within-era key refresh/reshare. Both
	// are bound into M and re-checked structurally by VerifyPulsar.
	KeyEraID   uint64
	Generation uint64

	// PChainHeight is the P-Chain height the signer set was pinned at.
	PChainHeight uint64

	// MLDSAPubKey is the ordinary FIPS-204 ML-DSA GROUP public key, wire-framed
	// as the boring verifier accepts it (pulsarwire.VerifyBytes / the ML-DSA
	// precompile). ONE key for the whole committee.
	MLDSAPubKey []byte

	// Threshold is the minimum aggregate signer WEIGHT the era's quorum
	// represents (e.g. > 2/3 stake). Informational at the era level; the
	// envelope's policy.ThresholdWeight() is the enforced floor per cert.
	Threshold uint64

	// SchemeID is the wire-stable suite ID the era's signatures are produced
	// under (e.g. "Lux-Pulsar-TALUS-MLDSA65"). MUST resolve, via the suite
	// registry, to a Pulsar ML-DSA lane — VerifyPulsar enforces this so a
	// mislabelled era can never accept a non-Pulsar signature.
	SchemeID string

	// KeygenMode records how the group key was produced — for AUDIT only, never
	// for verification: "talus-mpc" | "ceremony" | "tee" | "p3q-rollup-fallback".
	KeygenMode string

	// ActivationCert is the evidence that installed this era (e.g. the previous
	// era's threshold signature over this era's key). Stored for audit; the
	// activation-chain verifier (separate concern) checks it.
	ActivationCert []byte
}

// pulsarEraSuite returns the era's suite, asserting it is a Pulsar ML-DSA lane.
func (e PulsarKeyEra) pulsarEraSuite() (Suite, error) {
	return resolveSuiteForLane(e.SchemeID, EvidencePulsarThresholdMLDSA, 0)
}

// PulsarEvidence is the compact Pulsar lane evidence: ONE FIPS-204 ML-DSA
// threshold signature plus the era coordinates it was produced under. It
// carries NO per-validator signatures — its size is O(1) in committee size.
type PulsarEvidence struct {
	// SuiteID names the concrete scheme (must equal the era's SchemeID and
	// resolve to a Pulsar ML-DSA suite).
	SuiteID string

	// KeyEraID + Generation must match the era being verified against.
	KeyEraID   uint64
	Generation uint64

	// SignerSetID is the committed signer set the evidence claims; must match
	// the era's SignerSetID.
	SignerSetID [48]byte

	// Signature is the standard ML-DSA group signature bytes over M.
	Signature []byte
}

// Typed errors for the boring verifier. Each names exactly one rejected clause.
var (
	// ErrSuiteMismatch — the evidence's suite id is not the era's scheme id.
	ErrSuiteMismatch = errors.New("quasar: pulsar evidence suite id does not match the key-era scheme id")

	// ErrWrongEra — the evidence's era coordinates (key-era id / generation /
	// signer set) do not match the era being verified against.
	ErrWrongEra = errors.New("quasar: pulsar evidence era coordinates do not match the key era")

	// ErrBadSignature — the ML-DSA group signature failed verification under
	// the era group key over M.
	ErrBadSignature = errors.New("quasar: pulsar ML-DSA group signature failed verification")

	// ErrEraKeyEmpty — the era carries no group key (mis-provisioned era).
	ErrEraKeyEmpty = errors.New("quasar: pulsar key era has no group public key")
)

// VerifyPulsar is the deliberately boring Pulsar verifier: it verifies a
// NORMAL FIPS-204 ML-DSA signature under the era's compact group key over the
// canonical finality message M. It is agnostic to how the signature was
// produced (TALUS MPC / HSM / TEE / committee).
//
// Checks, in order (each a distinct typed reject):
//
//  1. The evidence suite resolves (via the suite registry) to a Pulsar ML-DSA
//     lane AND equals the era's scheme id — no suite string can route a
//     non-Pulsar signature here (ErrUnknownSuite / ErrSuiteLaneMismatch /
//     ErrSuiteMismatch).
//  2. The era's scheme id ALSO resolves to a Pulsar ML-DSA lane — a
//     mislabelled era cannot accept a foreign signature.
//  3. Era coordinates match: KeyEraID, Generation, and SignerSetID
//     (ErrWrongEra). These are also bound into M, so this is defence in depth.
//  4. The ML-DSA group signature verifies under era.MLDSAPubKey over M
//     (ErrBadSignature), via the same stateless FIPS-204 path the envelope uses.
func VerifyPulsar(ev PulsarEvidence, M []byte, era PulsarKeyEra) error {
	// (1) the evidence's own suite must be a Pulsar ML-DSA suite.
	evSuite, err := resolveSuiteForLane(ev.SuiteID, EvidencePulsarThresholdMLDSA, 0)
	if err != nil {
		return err
	}
	// the evidence suite must be the era's declared scheme.
	if ev.SuiteID != era.SchemeID {
		return fmt.Errorf("%w: evidence %q era %q", ErrSuiteMismatch, ev.SuiteID, era.SchemeID)
	}
	// (2) the era's scheme must ALSO be a Pulsar ML-DSA suite with the same
	// parameter set (a mislabelled era is rejected before any signature math).
	eraSuite, err := era.pulsarEraSuite()
	if err != nil {
		return err
	}
	if evSuite.ParamSet != eraSuite.ParamSet {
		return fmt.Errorf("%w: evidence param 0x%02x era param 0x%02x",
			ErrSuiteParamMismatch, evSuite.ParamSet, eraSuite.ParamSet)
	}

	// (3) era coordinates.
	if ev.KeyEraID != era.KeyEraID || ev.Generation != era.Generation {
		return fmt.Errorf("%w: evidence era (%d,%d) key era (%d,%d)",
			ErrWrongEra, ev.KeyEraID, ev.Generation, era.KeyEraID, era.Generation)
	}
	if ev.SignerSetID != era.SignerSetID {
		return fmt.Errorf("%w: signer-set id", ErrWrongEra)
	}

	// (4) the boring ML-DSA group-signature verify. Same path as the envelope's
	// verifyPulsarLeg → pulsarwire.VerifyBytes (FIPS-204, stateless).
	if len(era.MLDSAPubKey) == 0 {
		return ErrEraKeyEmpty
	}
	if !verifyPulsarLeg(M, era.MLDSAPubKey, ev.Signature) {
		return ErrBadSignature
	}
	return nil
}

// ----------------------------------------------------------------------------
// PulsarKeyEraRegistry — resolve the compact group key for a (chain, signer
// set, key-era, generation). The verifier loads the era from here, never from
// the cert (the cert only names the KeyEraID it claims; the registry supplies
// the trusted key material).
// ----------------------------------------------------------------------------

// pulsarEraKey is the registry index for one era.
type pulsarEraKey struct {
	chainID     uint32
	signerSetID [48]byte
	keyEraID    uint64
	generation  uint64
}

// PulsarKeyEraRegistry maps era coordinates to the trusted compact group key.
// Safe for concurrent reads after registration.
type PulsarKeyEraRegistry struct {
	mu   sync.RWMutex
	eras map[pulsarEraKey]PulsarKeyEra
}

// ErrEraNotFound — no era is registered for the requested coordinates.
var ErrEraNotFound = errors.New("quasar: no pulsar key era registered for these coordinates")

// ErrEraConflict — a DIFFERENT era is already registered for these exact
// coordinates. Era coordinates are immutable; re-registering a changed key
// under the same (chain, signer set, key-era, generation) is a hard reject.
var ErrEraConflict = errors.New("quasar: a different pulsar key era is already registered for these coordinates")

// NewPulsarKeyEraRegistry returns an empty registry.
func NewPulsarKeyEraRegistry() *PulsarKeyEraRegistry {
	return &PulsarKeyEraRegistry{eras: make(map[pulsarEraKey]PulsarKeyEra)}
}

// Register installs an era. The era's SchemeID must resolve to a Pulsar ML-DSA
// suite and it must carry a group key. Re-registering the SAME era (byte-equal
// key + scheme) is idempotent; registering a different key under the same
// coordinates is ErrEraConflict (eras are immutable).
func (r *PulsarKeyEraRegistry) Register(era PulsarKeyEra) error {
	if _, err := era.pulsarEraSuite(); err != nil {
		return err
	}
	if len(era.MLDSAPubKey) == 0 {
		return ErrEraKeyEmpty
	}
	k := pulsarEraKey{chainID: era.ChainID, signerSetID: era.SignerSetID, keyEraID: era.KeyEraID, generation: era.Generation}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.eras[k]; ok {
		if existing.SchemeID != era.SchemeID || !bytes.Equal(existing.MLDSAPubKey, era.MLDSAPubKey) {
			return ErrEraConflict
		}
		return nil // idempotent
	}
	r.eras[k] = era
	return nil
}

// Lookup resolves the era for the given coordinates.
func (r *PulsarKeyEraRegistry) Lookup(chainID uint32, signerSetID [48]byte, keyEraID, generation uint64) (PulsarKeyEra, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	era, ok := r.eras[pulsarEraKey{chainID: chainID, signerSetID: signerSetID, keyEraID: keyEraID, generation: generation}]
	if !ok {
		return PulsarKeyEra{}, ErrEraNotFound
	}
	return era, nil
}
