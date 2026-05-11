// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/log"
)

// verifier_manifest.go — the pinned verifier registry.
//
// A VerifierManifest is the chain's record of "this is the verifier we
// trust under this VerifierID." Backends do not self-assert: the manifest
// holds the source-commit, build profile, program hash, and verifier-key
// hash, and VerifyZProofUnderProfile compares envelope-declared values
// against the manifest before dispatching the backend.
//
// The registry is populated at boot from either genesis (mainnet) or a
// static config (testnet / devnet). After population it is read-only;
// the only place new manifests are admissible is the boot path, never
// at runtime. Forbidden runtime mutation closes the "swap the verifier
// out from under finality" attack class.

// VerifierManifest is one row in the registry.
type VerifierManifest struct {
	// VerifierID is the wire byte that identifies this verifier. Unique
	// per registry; Register refuses a duplicate.
	VerifierID config.VerifierID

	// BackendID is the implementation family this verifier belongs to.
	// Bound here so the envelope's BackendID can be compared against
	// the manifest without trusting either field alone.
	BackendID config.ProofBackendID

	// Version is a human-readable semver string. Informational; the
	// authoritative pinning is via SourceCommit + VerifierKeyHash.
	Version string

	// SourceCommit is the 20-byte (160-bit) git SHA of the source the
	// verifier was built from. Audit pipelines compare this against the
	// signed-off commit list.
	SourceCommit [20]byte

	// BuildProfile is the build flag set the verifier was compiled
	// under (e.g. "production", "audit", "race-debug"). Bound here so
	// a "debug" build cannot accidentally serve production traffic.
	BuildProfile string

	// ProofFormatID is the byte layout this verifier accepts in
	// ProofBytes. A verifier registered for ProofFormatSP1BinaryV1
	// MUST NOT be served a ProofFormatRISC0BinaryV1 envelope — the
	// envelope-vs-manifest comparison catches the mismatch before
	// dispatch.
	ProofFormatID config.ProofFormatID

	// ProgramOrAirHash is the 48-byte content hash of the program /
	// AIR bytes this verifier was set up against. Bound; the envelope
	// declares the same value, and any mismatch is a config-drift
	// refusal.
	ProgramOrAirHash [48]byte

	// VerifierKeyHash is the 48-byte hash of the verifier key the
	// backend was set up against (e.g. a STARK Merkle commitment over
	// the prover's setup, a Plonky3 verifier-data hash, etc.). Same
	// drift-refusal semantics as ProgramOrAirHash.
	VerifierKeyHash [48]byte

	// SupportsPolicyIDs is the allow-list of ProofPolicyIDs this
	// verifier can serve. A verifier built for STARK_FRI_SHA3_PQ
	// cannot serve STARK_FRI_KECCAK_PQ envelopes; the policy field
	// makes that explicit.
	SupportsPolicyIDs []config.ProofPolicyID

	// SoundnessBitsReviewed is the soundness level an external auditor
	// signed off on for this verifier. The envelope declares its own
	// claimed soundness; the profile floors compare against the
	// envelope's claim. The reviewed number lives here so audit
	// tooling can detect a backend that claims more soundness than
	// it has been reviewed at.
	SoundnessBitsReviewed uint16

	// HashOutputBits is the canonical hash width the verifier consumes
	// (e.g. 256 for Keccak Merkle, 384 for cSHAKE256 / SHA3-384).
	HashOutputBits uint16

	// Capability flags — same shape as the envelope's flags, kept here
	// so the verifier identity ships its own truth alongside the
	// envelope's claim. Mismatch is NOT itself a verification failure
	// (some profiles tolerate it), but audit tooling flags the gap.
	UsesPairings              bool
	UsesKZG                   bool
	UsesTrustedSetup          bool
	UsesClassicalSNARKWrapper bool
}

// SupportsPolicy returns true iff pid is in m.SupportsPolicyIDs.
func (m *VerifierManifest) SupportsPolicy(pid config.ProofPolicyID) bool {
	for _, p := range m.SupportsPolicyIDs {
		if p == pid {
			return true
		}
	}
	return false
}

// Typed registry errors.
var (
	ErrVerifierManifestNil                 = errors.New("zchain: nil verifier manifest")
	ErrVerifierManifestDuplicate           = errors.New("zchain: verifier id already registered")
	ErrVerifierManifestInvalidID           = errors.New("zchain: VerifierNone may not be registered")
	ErrVerifierManifestMissingField        = errors.New("zchain: verifier manifest is missing required field")
	ErrVerifierManifestForbiddenBackend    = errors.New("zchain: verifier manifest BackendID is forbidden in PQ mode")
	ErrVerifierManifestForbiddenFormat     = errors.New("zchain: verifier manifest ProofFormatID is forbidden in PQ mode")
	ErrVerifierManifestForbiddenPolicy     = errors.New("zchain: verifier manifest SupportsPolicyIDs contains forbidden policy")
	ErrVerifierManifestBackendFormatPair   = errors.New("zchain: verifier manifest (BackendID, ProofFormatID) pair is not in the allowed table")
	ErrVerifierManifestUnknownBuildProfile = errors.New("zchain: verifier manifest BuildProfile is not a known value")
)

// BuildProfile is the typed string enum for verifier build kinds.
// Free-form strings here open a category-mismatch footgun (case-sensitivity,
// typos that silently bypass downstream checks). The Parse function pins
// the canonical lowercase form; everything else is refused at Register.
type BuildProfile string

const (
	// BuildProfileProduction is the canonical strict-PQ build. Audit-
	// signed-off; ships on mainnet. The verifier MUST be built without
	// debug instrumentation, race detector, or assert macros.
	BuildProfileProduction BuildProfile = "production"

	// BuildProfileAudit is the audit-engagement build. Identical to
	// production except for opt-in logging; never serves mainnet
	// traffic on its own. Used for external cryptanalysis.
	BuildProfileAudit BuildProfile = "audit"

	// BuildProfileDev is the testnet / devnet build. Allowed only on
	// profiles whose ForbidDevProofs is false.
	BuildProfileDev BuildProfile = "dev"
)

// IsKnown returns true iff p is one of the canonical BuildProfile values.
func (p BuildProfile) IsKnown() bool {
	return p == BuildProfileProduction || p == BuildProfileAudit || p == BuildProfileDev
}

// allowedBackendFormatPairs is the cross-validation table for
// (BackendID, ProofFormatID) at Register time. Pin the table; any new
// backend MUST add a row here before its first Register call.
var allowedBackendFormatPairs = map[config.ProofBackendID]map[config.ProofFormatID]struct{}{
	config.ProofBackendSP1CompressedSTARK: {
		config.ProofFormatSTARKFRIBinaryV1: {},
		config.ProofFormatSP1BinaryV1:      {},
	},
	config.ProofBackendRISC0SuccinctSTARK: {
		config.ProofFormatSTARKFRIBinaryV1: {},
		config.ProofFormatRISC0BinaryV1:    {},
	},
	config.ProofBackendP3QSTARKFRISHA3: {
		config.ProofFormatSTARKFRIBinaryV1: {},
		config.ProofFormatP3QBinaryV1:      {},
	},
	config.ProofBackendStoneCairoSTARK: {
		config.ProofFormatSTARKFRIBinaryV1:   {},
		config.ProofFormatStoneCairoBinaryV1: {},
	},
	config.ProofBackendStwoCircleSTARK: {
		config.ProofFormatSTARKFRIBinaryV1:   {},
		config.ProofFormatStwoCircleBinaryV1: {},
	},
	config.ProofBackendRISC0RawSTARKDev: {
		config.ProofFormatRISC0BinaryV1: {},
	},
	config.ProofBackendSP1CoreSTARKDev: {
		config.ProofFormatSP1BinaryV1: {},
	},
}

// isAllowedBackendFormatPair reports whether (backend, format) is in the
// allowedBackendFormatPairs table.
func isAllowedBackendFormatPair(backend config.ProofBackendID, format config.ProofFormatID) bool {
	if backend.IsForbiddenInPQMode() || format.IsForbiddenInPQMode() {
		return false
	}
	formats, ok := allowedBackendFormatPairs[backend]
	if !ok {
		return false
	}
	_, ok = formats[format]
	return ok
}

// VerifierManifestRegistry is the in-process pinned-verifier map. The
// zero value is unusable; construct via NewVerifierManifestRegistry.
//
// The registry is goroutine-safe: Register acquires a write lock,
// Lookup acquires a read lock. Boot-time wiring uses Register;
// per-envelope verification uses Lookup. There is no Unregister and
// no Replace — the registry is monotonic for the process lifetime.
type VerifierManifestRegistry struct {
	mu     sync.RWMutex
	byID   map[config.VerifierID]*VerifierManifest
	logger log.Logger
}

// NewVerifierManifestRegistry returns a fresh empty registry. Pass a
// luxfi/log.Logger for boot-time auditability; a nil logger is replaced
// with log.Noop() — every Register call emits one structured event so
// the boot path produces a reviewable manifest list.
func NewVerifierManifestRegistry(logger log.Logger) *VerifierManifestRegistry {
	if logger == nil {
		logger = log.Noop()
	}
	return &VerifierManifestRegistry{
		byID:   make(map[config.VerifierID]*VerifierManifest),
		logger: logger,
	}
}

// Register installs m in the registry. Refuses VerifierNone, refuses
// duplicates, refuses manifests with missing required fields, refuses
// manifests whose BackendID / ProofFormatID is on the PQ-forbidden list,
// refuses SupportsPolicyIDs that contains a forbidden policy, and
// refuses any (BackendID, ProofFormatID) pair not in the canonical
// table. On success emits one info-level log line so the boot path
// produces a reviewable manifest list.
//
// Closes F82 (forbidden policy in SupportsPolicyIDs), F83 (backend /
// format mismatch), F87 (manifest claiming forbidden backend with
// "safe" self-attest flags), F88 (free-form BuildProfile).
func (r *VerifierManifestRegistry) Register(m *VerifierManifest) error {
	if m == nil {
		return ErrVerifierManifestNil
	}
	if m.VerifierID == config.VerifierNone {
		return ErrVerifierManifestInvalidID
	}
	if m.Version == "" {
		return fmt.Errorf("%w: Version", ErrVerifierManifestMissingField)
	}
	if m.BuildProfile == "" {
		return fmt.Errorf("%w: BuildProfile", ErrVerifierManifestMissingField)
	}
	if !BuildProfile(m.BuildProfile).IsKnown() {
		return fmt.Errorf("%w: %q", ErrVerifierManifestUnknownBuildProfile, m.BuildProfile)
	}
	if m.BackendID == config.ProofBackendNone {
		return fmt.Errorf("%w: BackendID", ErrVerifierManifestMissingField)
	}
	if m.BackendID.IsForbiddenInPQMode() {
		return fmt.Errorf("%w: %s", ErrVerifierManifestForbiddenBackend, m.BackendID.String())
	}
	if m.ProofFormatID == config.ProofFormatNone {
		return fmt.Errorf("%w: ProofFormatID", ErrVerifierManifestMissingField)
	}
	if m.ProofFormatID.IsForbiddenInPQMode() {
		return fmt.Errorf("%w: %s", ErrVerifierManifestForbiddenFormat, m.ProofFormatID.String())
	}
	if !isAllowedBackendFormatPair(m.BackendID, m.ProofFormatID) {
		return fmt.Errorf("%w: backend=%s format=%s",
			ErrVerifierManifestBackendFormatPair,
			m.BackendID.String(), m.ProofFormatID.String())
	}
	if len(m.SupportsPolicyIDs) == 0 {
		return fmt.Errorf("%w: SupportsPolicyIDs", ErrVerifierManifestMissingField)
	}
	for _, pid := range m.SupportsPolicyIDs {
		if pid.IsForbiddenInPQMode() {
			return fmt.Errorf("%w: %s", ErrVerifierManifestForbiddenPolicy, pid.String())
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.byID[m.VerifierID]; exists {
		return fmt.Errorf("%w: %s", ErrVerifierManifestDuplicate, m.VerifierID.String())
	}
	// Defensive copy: the registry owns the stored value so callers
	// cannot mutate a manifest after Register returns.
	cp := *m
	cp.SupportsPolicyIDs = append([]config.ProofPolicyID(nil), m.SupportsPolicyIDs...)
	r.byID[m.VerifierID] = &cp

	r.logger.Info("zchain: registered verifier manifest",
		"verifier_id", m.VerifierID.String(),
		"backend_id", m.BackendID.String(),
		"version", m.Version,
		"build_profile", m.BuildProfile,
		"proof_format_id", m.ProofFormatID.String(),
		"soundness_bits_reviewed", m.SoundnessBitsReviewed,
	)
	return nil
}

// Lookup returns a defensive copy of the manifest for vid plus a boolean
// indicating presence. The caller owns the returned pointer; mutating it
// has no effect on the registry's stored value. Closes F81 (a caller
// who mutated the stored manifest could otherwise overwrite the pinned
// VerifierKeyHash or SupportsPolicyIDs at runtime).
func (r *VerifierManifestRegistry) Lookup(vid config.VerifierID) (*VerifierManifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	stored, ok := r.byID[vid]
	if !ok {
		return nil, false
	}
	out := *stored
	out.SupportsPolicyIDs = append([]config.ProofPolicyID(nil), stored.SupportsPolicyIDs...)
	return &out, true
}

// Len returns the number of manifests currently registered.
func (r *VerifierManifestRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}
