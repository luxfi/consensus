// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// errors.go -- typed errors for the Pulsar cert profile. These are the
// fallback / retry signals the Quasar orchestrator matches against to decide
// whether to fall back to Corona, retry with a fresh nonce, or slash.

package pulsar

import "errors"

var (
	// ErrNonceCertPoolEmpty signals the background NonceCert pool is empty.
	// The orchestrator MUST fall back to Corona (or retry once the pool
	// refills); the hot path NEVER generates a nonce inline.
	ErrNonceCertPoolEmpty = errors.New("quasar/pulsar: NonceCert pool empty; fall back to Corona")

	// ErrProfileNotReady signals the secret-bearing FIPS 204 signature-assembly
	// core is not registered (pulsar.ProductionBCCSigningReady() is false: the
	// BCC/CEF ZK proofs are fail-closed). Finalize still returns a
	// structurally-complete ConsensusCert with an EMPTY Signature; the
	// orchestrator falls back to Corona.
	ErrProfileNotReady = errors.New("quasar/pulsar: BCC signing core not registered; fall back to Corona")

	// ErrSessionFieldUnset rejects a PulsarSession with an all-zero
	// security-relevant root (unbound field). Signing under it would allow
	// cross-round replay.
	ErrSessionFieldUnset = errors.New("quasar/pulsar: session has an unset security-relevant field")

	// ErrNonCanonicalNonce signals the supplied nonce id is not the canonical
	// pool selection for this session. Every signer recomputes the canonical
	// index and refuses a non-canonical nonce (anti-grind).
	ErrNonCanonicalNonce = errors.New("quasar/pulsar: non-canonical nonce for session")

	// ErrInsufficientShares signals fewer collected shares than the threshold.
	ErrInsufficientShares = errors.New("quasar/pulsar: fewer shares than threshold")

	// ErrNoNonceCert signals Round2/Finalize were called without a bound
	// Round1 (no NonceCert).
	ErrNoNonceCert = errors.New("quasar/pulsar: no bound NonceCert (call Round1 first)")
)
