// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// key_era.go — the KeyEra registry and the deliberately BORING compact-threshold
// verifier. ONE KeyEra type plus ONE verify entry point (VerifyThresholdLeg)
// serve EVERY threshold lane: Pulsar (FIPS-204 ML-DSA group signature) and
// Corona (Ring-LWE Ringtail threshold signature).
//
// THE COMPACT GROUP KEY. For a 1000+ validator committee a threshold lane is
// ONE signature under ONE group public key — not 1000 per-validator signatures.
// A KeyEra is the registry record that pins that single group key to one
// validator-set era: which chain, which signer set (P-Chain-pinned), which
// key-era id + generation, what scheme/parameter set, and the activation
// evidence. ONE group key verifies the whole committee's threshold signature.
//
// VALUE, NOT PLACE. KeyEra is a generic value qualified by its SchemeID, not a
// pulsar-specific place: the SAME type serves the Pulsar and Corona lanes. The
// SchemeID says HOW to interpret the group-key bytes (ML-DSA vs Ringtail); the
// key material itself is a scheme-agnostic []byte (GroupPubKey). One type for
// all lanes — not a Pulsar-specific era type plus a separate Corona-specific
// one; the lane is a value (the scheme), not the type.
//
// DELIBERATELY BORING VERIFIER. The chain does not care HOW the group signature
// was produced — TALUS MPC, an HSM ceremony, a TEE, or a single trusted signer.
// VerifyThresholdLeg verifies a NORMAL group-key signature under the era group
// key over the canonical finality message M. That is the whole point: the trust
// model of key PRODUCTION (talus-mpc / ceremony / tee / p3q-rollup fallback) is
// recorded in the era's KeygenMode for audit, but it is ORTHOGONAL to
// verification. The verifier stays in its lane: group sig, group key, M.
//
// ONE VERIFY PATH PER LANE. VerifyThresholdLeg resolves the era's lane through
// the suite registry and routes the signature check through the SAME stateless
// path the envelope's leg verifier uses — pulsarwire.VerifyBytes (FIPS-204) for
// Pulsar, coronaThreshold.Verify (Ring-LWE) for Corona — so the standalone and
// envelope verifies agree byte-for-byte. The suite registry's lane safety
// (LookupSuite / resolveSuiteForLane) guarantees no scheme string can dispatch
// a signature into the wrong lane's verifier.
package quasar

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
)

// KeyEra pins one compact threshold GROUP key to one validator-set era. It is a
// generic value serving every threshold lane; the SchemeID names the suite (and
// thus the lane + parameter set) the group key is interpreted under. Owner-spec
// value types are expressed in this package's native types (uint32 chain id,
// [48]byte committed signer-set root) so an era binds directly to a
// ConsensusCert header — values, not places.
type KeyEra struct {
	// ChainID is the chain this era belongs to (matches ConsensusCert.ChainID).
	ChainID uint32

	// SignerSetID is the committed validator-set identity the group key was
	// generated for — the P-Chain-pinned signer set. Matches a ConsensusCert's
	// ValidatorSetRoot. A signature under this era's key attests THIS signer set.
	SignerSetID [48]byte

	// KeyEraID + Generation identify the era. KeyEraID advances on a signer-set
	// rotation; Generation advances on a within-era key refresh/reshare. Both
	// are bound into M and re-checked structurally by VerifyThresholdLeg.
	KeyEraID   uint64
	Generation uint64

	// PChainHeight is the P-Chain height the signer set was pinned at.
	PChainHeight uint64

	// GroupPubKey is the compact threshold GROUP public key, wire-framed as the
	// era's lane verifier accepts it (pulsarwire.VerifyBytes for a Pulsar ML-DSA
	// lane; coronaThreshold.GroupKey.UnmarshalBinary for a Corona Ringtail lane).
	// It is SCHEME-INTERPRETED: SchemeID says how to read these bytes. ONE key
	// for the whole committee.
	GroupPubKey []byte

	// Threshold is the minimum aggregate signer WEIGHT the era's quorum
	// represents (e.g. > 2/3 stake). Informational at the era level; the
	// envelope's policy.ThresholdWeight() is the enforced floor per cert.
	Threshold uint64

	// SchemeID is the wire-stable suite ID the era's signatures are produced
	// under (e.g. "Lux-Pulsar-TALUS-MLDSA65", "Lux-Corona-Ringtail-L3-v1"). MUST
	// resolve, via the suite registry, to a THRESHOLD group-key lane (Pulsar or
	// Corona) — VerifyThresholdLeg enforces this so a mislabelled era can never
	// accept a foreign-lane signature.
	SchemeID string

	// KeygenMode records how the group key was produced — for AUDIT only, never
	// for verification: "talus-mpc" | "ceremony" | "tee" | "p3q-rollup-fallback".
	KeygenMode string

	// ActivationCert is the evidence that installed this era (e.g. the previous
	// era's threshold signature over this era's key). Stored for audit; the
	// activation-chain verifier (separate concern) checks it.
	ActivationCert []byte
}

// eraSuite resolves the era's suite and asserts it is a THRESHOLD group-key lane
// — Pulsar ML-DSA or Corona Ringtail, the lanes a compact KeyEra can serve. Beam
// (classical aggregate) and P3Q (per-validator rollup) are NOT compact group-key
// threshold lanes and are refused here, so a KeyEra can never be registered or
// verified against a non-threshold scheme.
func (e KeyEra) eraSuite() (Suite, error) {
	s, ok := LookupSuite(e.SchemeID)
	if !ok {
		return Suite{}, fmt.Errorf("%w: %q", ErrUnknownSuite, e.SchemeID)
	}
	switch s.Kind {
	case EvidencePulsarThresholdMLDSA, EvidenceCoronaRingtail:
		return s, nil
	default:
		return Suite{}, fmt.Errorf("%w: suite %q is %q, not a threshold group-key lane",
			ErrSuiteLaneMismatch, e.SchemeID, s.Kind)
	}
}

// PulsarEvidence is the compact threshold-lane evidence: ONE threshold signature
// plus the era coordinates it was produced under. It carries NO per-validator
// signatures — its size is O(1) in committee size. The SAME shape carries any
// threshold lane's signature; the era's SchemeID (matched below) selects the
// lane that interprets it.
type PulsarEvidence struct {
	// SuiteID names the concrete scheme (must equal the era's SchemeID and
	// resolve to the era's threshold lane).
	SuiteID string

	// KeyEraID + Generation must match the era being verified against.
	KeyEraID   uint64
	Generation uint64

	// SignerSetID is the committed signer set the evidence claims; must match
	// the era's SignerSetID.
	SignerSetID [48]byte

	// Signature is the threshold GROUP signature bytes over M — a FIPS-204 ML-DSA
	// signature on the Pulsar lane, a Ringtail signature on the Corona lane.
	Signature []byte
}

// Typed errors for the boring verifier. Each names exactly one rejected clause.
var (
	// ErrSuiteMismatch — the evidence's suite id is not the era's scheme id.
	ErrSuiteMismatch = errors.New("quasar: threshold evidence suite id does not match the key-era scheme id")

	// ErrWrongEra — the evidence's era coordinates (key-era id / generation /
	// signer set) do not match the era being verified against.
	ErrWrongEra = errors.New("quasar: threshold evidence era coordinates do not match the key era")

	// ErrBadSignature — the threshold group signature failed verification under
	// the era group key over M.
	ErrBadSignature = errors.New("quasar: threshold group signature failed verification")

	// ErrEraKeyEmpty — the era carries no group key (mis-provisioned era).
	ErrEraKeyEmpty = errors.New("quasar: key era has no group public key")
)

// VerifyThresholdLeg is the ONE compact-threshold verify entry point. It serves
// EVERY threshold lane: it resolves the era's scheme to its lane via the suite
// registry and routes to that lane's deliberately boring verifier — Pulsar
// (FIPS-204 ML-DSA group signature) or Corona (Ring-LWE Ringtail threshold
// signature) — under the era's compact GROUP key over the canonical finality
// message M. It is agnostic to how the signature was produced (TALUS MPC / HSM /
// TEE / committee); KeygenMode records that for audit only.
//
// Checks, in order (each a distinct typed reject):
//
//  1. The era's scheme resolves (via the suite registry) to a THRESHOLD lane —
//     Pulsar or Corona; a non-threshold or unknown scheme is a hard reject
//     (ErrUnknownSuite / ErrSuiteLaneMismatch).
//  2. The evidence's own suite resolves to the SAME lane as the era — no suite
//     string can route a foreign-lane signature here (ErrUnknownSuite /
//     ErrSuiteLaneMismatch).
//  3. The evidence suite equals the era's declared scheme (ErrSuiteMismatch),
//     and their parameter sets match (ErrSuiteParamMismatch).
//  4. Era coordinates match: KeyEraID, Generation, and SignerSetID
//     (ErrWrongEra). These are also bound into M, so this is defence in depth.
//  5. The group signature verifies under era.GroupPubKey over M
//     (ErrBadSignature), via the same stateless path the matching envelope leg
//     uses — pulsarwire.VerifyBytes for Pulsar, coronaThreshold.Verify for
//     Corona.
func VerifyThresholdLeg(ev PulsarEvidence, M []byte, era KeyEra) error {
	// (1) the era's scheme must be a threshold group-key lane (Pulsar | Corona).
	eraSuite, err := era.eraSuite()
	if err != nil {
		return err
	}
	// (2) the evidence's own suite must resolve to the SAME lane — a different
	// lane (e.g. a Corona suite for a Pulsar era) is rejected before any
	// signature math, so no suite string reaches the wrong verifier.
	evSuite, err := resolveSuiteForLane(ev.SuiteID, eraSuite.Kind, 0)
	if err != nil {
		return err
	}
	// (3) the evidence suite must be the era's declared scheme with the same
	// parameter set (a mislabelled era is rejected here).
	if ev.SuiteID != era.SchemeID {
		return fmt.Errorf("%w: evidence %q era %q", ErrSuiteMismatch, ev.SuiteID, era.SchemeID)
	}
	if evSuite.ParamSet != eraSuite.ParamSet {
		return fmt.Errorf("%w: evidence param 0x%02x era param 0x%02x",
			ErrSuiteParamMismatch, evSuite.ParamSet, eraSuite.ParamSet)
	}

	// (4) era coordinates.
	if ev.KeyEraID != era.KeyEraID || ev.Generation != era.Generation {
		return fmt.Errorf("%w: evidence era (%d,%d) key era (%d,%d)",
			ErrWrongEra, ev.KeyEraID, ev.Generation, era.KeyEraID, era.Generation)
	}
	if ev.SignerSetID != era.SignerSetID {
		return fmt.Errorf("%w: signer-set id", ErrWrongEra)
	}

	// (5) the boring per-lane group-signature verify. Same stateless path the
	// envelope's leg verifier uses, dispatched by the resolved lane.
	if len(era.GroupPubKey) == 0 {
		return ErrEraKeyEmpty
	}
	switch eraSuite.Kind {
	case EvidencePulsarThresholdMLDSA:
		// pulsarwire.VerifyBytes (FIPS-204, stateless) — same as verifyPulsarLeg.
		if !verifyPulsarLeg(M, era.GroupPubKey, ev.Signature) {
			return ErrBadSignature
		}
	case EvidenceCoronaRingtail:
		// coronaThreshold.Verify (Ring-LWE) — same as verifyCoronaLeg, with the
		// group key decoded from the era's wire bytes.
		if !verifyCoronaGroupKeyLeg(M, era.GroupPubKey, ev.Signature) {
			return ErrBadSignature
		}
	default:
		// Unreachable: eraSuite() already constrained the lane to Pulsar|Corona.
		return fmt.Errorf("%w: suite %q is %q", ErrSuiteLaneMismatch, era.SchemeID, eraSuite.Kind)
	}
	return nil
}

// VerifyPulsar is the thin Pulsar-suite wrapper over the one VerifyThresholdLeg
// entry point, retained for callers that name the lane explicitly. It refuses
// any non-Pulsar era up-front (so a Corona era can never enter via this door),
// then defers to VerifyThresholdLeg, which performs the boring FIPS-204 ML-DSA
// group-signature verify under the era's compact group key over M.
func VerifyPulsar(ev PulsarEvidence, M []byte, era KeyEra) error {
	if _, err := resolveSuiteForLane(era.SchemeID, EvidencePulsarThresholdMLDSA, 0); err != nil {
		return err
	}
	return VerifyThresholdLeg(ev, M, era)
}

// ----------------------------------------------------------------------------
// KeyEraRegistry — resolve the compact group key for a (chain, signer set,
// key-era, generation). The verifier loads the era from here, never from the
// cert (the cert only names the KeyEraID it claims; the registry supplies the
// trusted key material). One registry holds eras for EVERY threshold lane.
// ----------------------------------------------------------------------------

// eraKey is the registry index for one era.
type eraKey struct {
	chainID     uint32
	signerSetID [48]byte
	keyEraID    uint64
	generation  uint64
}

// KeyEraRegistry maps era coordinates to the trusted compact group key. Safe for
// concurrent reads after registration. Lane-agnostic: it holds Pulsar and Corona
// eras alike, each self-describing via its SchemeID.
type KeyEraRegistry struct {
	mu   sync.RWMutex
	eras map[eraKey]KeyEra
}

// ErrEraNotFound — no era is registered for the requested coordinates.
var ErrEraNotFound = errors.New("quasar: no key era registered for these coordinates")

// ErrEraConflict — a DIFFERENT era is already registered for these exact
// coordinates. Era coordinates are immutable; re-registering a changed key
// under the same (chain, signer set, key-era, generation) is a hard reject.
var ErrEraConflict = errors.New("quasar: a different key era is already registered for these coordinates")

// NewKeyEraRegistry returns an empty registry.
func NewKeyEraRegistry() *KeyEraRegistry {
	return &KeyEraRegistry{eras: make(map[eraKey]KeyEra)}
}

// Register installs an era. The era's SchemeID must resolve to a THRESHOLD lane
// (Pulsar or Corona) and it must carry a group key. Re-registering the SAME era
// (byte-equal key + scheme) is idempotent; registering a different key under the
// same coordinates is ErrEraConflict (eras are immutable).
func (r *KeyEraRegistry) Register(era KeyEra) error {
	if _, err := era.eraSuite(); err != nil {
		return err
	}
	if len(era.GroupPubKey) == 0 {
		return ErrEraKeyEmpty
	}
	k := eraKey{chainID: era.ChainID, signerSetID: era.SignerSetID, keyEraID: era.KeyEraID, generation: era.Generation}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.eras[k]; ok {
		if existing.SchemeID != era.SchemeID || !bytes.Equal(existing.GroupPubKey, era.GroupPubKey) {
			return ErrEraConflict
		}
		return nil // idempotent
	}
	r.eras[k] = era
	return nil
}

// Lookup resolves the era for the given coordinates.
func (r *KeyEraRegistry) Lookup(chainID uint32, signerSetID [48]byte, keyEraID, generation uint64) (KeyEra, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	era, ok := r.eras[eraKey{chainID: chainID, signerSetID: signerSetID, keyEraID: keyEraID, generation: generation}]
	if !ok {
		return KeyEra{}, ErrEraNotFound
	}
	return era, nil
}
