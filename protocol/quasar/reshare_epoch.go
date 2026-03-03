// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

// Quasar epoch resharing — LSS-Pulsar wiring.
//
// This file exposes the human-readable rotation entry points:
//
//   - ReshareEpoch    set rotation; t' may differ from t.
//   - RefreshEpoch    same set; same threshold; only share bytes rotate.
//
// Both call into RotateEpoch, whose body now performs Bootstrap on
// first invocation, dynamic resharing through the LSS-Pulsar adapter
// on subsequent invocations, and the activation circuit-breaker
// before committing the new state. See `epoch.go: reshareEpochKeys`
// for the lattice math integration; see `pulsar/DESIGN.md` for the
// architectural contract.

// ActivationMessagePersonalization is the canonical signing prefix
// for post-reshare activation certificates. Mirrors
// `ActivationMessage.SignableBytes()` in
// `github.com/luxfi/pulsar/reshare/activation.go`. Re-exported here
// so the consensus layer can name the prefix at the call site without
// a transitive Pulsar import.
const ActivationMessagePersonalization = "QUASAR-PULSAR-ACTIVATE-v1"

// TranscriptVariantReshare and TranscriptVariantRefresh are the
// canonical Variant strings expected by pulsar/reshare's
// TranscriptInputs. Bound into the transcript so an activation cert
// produced for one variant cannot be replayed for the other.
const (
	TranscriptVariantReshare = "reshare"
	TranscriptVariantRefresh = "refresh"
)

// ReshareTranscriptInputs is a pure-data shape mirroring pulsar/
// reshare.TranscriptInputs. Field semantics match the Pulsar source
// of truth exactly. The canonical 32-byte transcript hash is BLAKE3
// (legacy) or SHA3 (production) over the personalized concatenation
// of these fields with length prefixes; see pulsar/reshare/transcript
// for the wire format.
type ReshareTranscriptInputs struct {
	ChainID            []byte
	GroupID            []byte
	OldEpochID         uint64
	NewEpochID         uint64
	OldSetHash         [32]byte
	NewSetHash         [32]byte
	ThresholdOld       uint32
	ThresholdNew       uint32
	GroupPublicKeyHash [32]byte
	Variant            string
}

// ReshareEpoch is the explicit name for a validator-set rotation
// under the Pulsar lifecycle. It performs a SHARE rotation under the
// unchanged GroupKey via lss.DynamicResharePulsar and gates the
// transition on the activation circuit-breaker.
//
// Naming rationale: "Rotate" reads as "swap to a different value",
// but with the Pulsar lifecycle the master secret s and the group
// public key DO NOT change — only the share distribution does.
// ReshareEpoch makes the new semantics explicit at the call site.
//
// The body delegates to RotateEpoch, which is the authoritative
// driver — the rate-limit guard, the change-detection guard, the
// LSS-Pulsar adapter call, and the activation cert verification all
// live there. Callers in core.go and adversarial_test.go SHOULD
// switch to ReshareEpoch for new code paths; the existing
// RotateEpoch signature is kept stable for compatibility.
func (em *EpochManager) ReshareEpoch(validators []string, force bool) (*EpochKeys, error) {
	return em.RotateEpoch(validators, force)
}

// RefreshEpoch is the same-committee proactive update — the HJKY
// zero-polynomial primitive at pulsar/reshare.Refresh. It refreshes
// the share bytes within an unchanged validator set.
//
// Today's wiring delegates to RotateEpoch with force=true on the
// current validator set. Under that path lss.DynamicResharePulsar is
// called with newPartyIDs == oldPartyIDs and threshold unchanged,
// which yields a refresh-shaped rotation — a new share distribution
// for the same committee at the same threshold under the same
// GroupKey. A future commit can switch to a dedicated Refresh path
// in pulsar/reshare that uses the HJKY zero-polynomial primitive
// directly (smaller transcript, stronger forward security against
// mobile adversaries within an epoch).
//
// Use case: a chain runs RefreshEpoch on a periodic schedule (e.g.
// every Nth block within a stable validator set) to defeat mobile
// adversaries that compromise < t parties per period.
func (em *EpochManager) RefreshEpoch() (*EpochKeys, error) {
	em.mu.RLock()
	validators := append([]string{}, em.currentValidators...)
	em.mu.RUnlock()
	return em.RotateEpoch(validators, true)
}
