// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_scheme.go — the per-signer scheme axis of a weighted quorum
// certificate, plus the pluggable FIPS verifier registry.
//
// A weighted quorum certificate is scheme-agnostic: it proves that a
// quorum-weight subset of validators each produced a valid INDEPENDENT
// signature over the same domain-separated consensus message. "Independent"
// is the whole point — there is no key sharing, no reconstruction, no seed /
// WOTS+ / FORS / secret material combined. Each signer record names the
// scheme it signed under; the verifier dispatches to the unmodified FIPS
// verifier for that scheme.
//
// This is threshold CERTIFICATION, not threshold signing. The contrast with
// the Pulsar / Corona threshold legs of QuasarCert is deliberate: those run
// a DKG and emit ONE group signature; this runs no ceremony and emits N
// per-validator signatures that any peer can check with a stock FIPS
// verifier and no Lux dependency.
package quasar

import (
	"errors"
	"fmt"

	"github.com/luxfi/crypto/mldsa"
	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
)

// QuorumSchemeID names the signature scheme a single quorum-certificate
// signer record was produced under. Orthogonal to config.SigSchemeID (the
// threshold-finality axis): a quorum cert's signer signs INDEPENDENTLY, so
// this axis names the per-signer FIPS relation the verifier dispatches to.
//
// The byte values intentionally MIRROR the canonical Lux wire bytes for the
// same primitives so a reader sees consistent numbering across the codebase:
//
//	0x41..0x43 — ML-DSA per FIPS 204 (matches config.SigSchemeID 0x40 block)
//	0x05..0x07 — SLH-DSA per FIPS 205 (matches config.RecoverySchemeID block)
//
// New schemes (e.g. an independent-signature Corona) claim the next free
// integer in their family block and MUST register a verifier in
// defaultQuorumVerifiers before any cert references them. Never reuse a
// retired id.
type QuorumSchemeID uint8

const (
	QuorumSchemeNone QuorumSchemeID = 0x00

	// FIPS 204 ML-DSA (Module-LWE lattice signature). Independent per-signer.
	QuorumSchemeMLDSA44 QuorumSchemeID = 0x41
	QuorumSchemeMLDSA65 QuorumSchemeID = 0x42 // production identity default
	QuorumSchemeMLDSA87 QuorumSchemeID = 0x43

	// FIPS 205 SLH-DSA (hash-based stateless signature). Independent
	// per-signer; the cross-family backstop. Byte values match
	// config.RecoverySchemeID's SLH-DSA block.
	QuorumSchemeSLHDSA192s QuorumSchemeID = 0x06 // Magnetar production (SHAKE-192s)
	QuorumSchemeSLHDSA192f QuorumSchemeID = 0x16 // SHAKE-192f
	QuorumSchemeSLHDSA256s QuorumSchemeID = 0x07 // SHAKE-256s
)

// String returns the canonical wire name.
func (s QuorumSchemeID) String() string {
	switch s {
	case QuorumSchemeNone:
		return "none"
	case QuorumSchemeMLDSA44:
		return "ml-dsa-44"
	case QuorumSchemeMLDSA65:
		return "ml-dsa-65"
	case QuorumSchemeMLDSA87:
		return "ml-dsa-87"
	case QuorumSchemeSLHDSA192s:
		return "slh-dsa-192s"
	case QuorumSchemeSLHDSA192f:
		return "slh-dsa-192f"
	case QuorumSchemeSLHDSA256s:
		return "slh-dsa-256s"
	default:
		return fmt.Sprintf("quorum-scheme(0x%02x)", uint8(s))
	}
}

// FIPS reports the FIPS standard the scheme's verifier implements. "204"
// for ML-DSA, "205" for SLH-DSA, "" for unknown. The certificate's NIST
// posture rests on these being unmodified FIPS verifiers.
func (s QuorumSchemeID) FIPS() string {
	switch s {
	case QuorumSchemeMLDSA44, QuorumSchemeMLDSA65, QuorumSchemeMLDSA87:
		return "204"
	case QuorumSchemeSLHDSA192s, QuorumSchemeSLHDSA192f, QuorumSchemeSLHDSA256s:
		return "205"
	default:
		return ""
	}
}

var (
	// ErrUnknownQuorumScheme is returned when a signer record names a
	// scheme with no registered verifier. Treated as a per-record INVALID
	// (not a fatal/DoS) by the certificate verifier.
	ErrUnknownQuorumScheme = errors.New("quasar: unknown quorum signature scheme")

	// ErrSchemeNotAllowed is returned (as a per-record invalidation) when a
	// signer record's scheme is not in the verifier's allowed set.
	ErrSchemeNotAllowed = errors.New("quasar: quorum signature scheme not in allowed set")
)

// quorumVerifyFn is the pluggable per-signature verification function. It
// MUST be a thin dispatch over an UNMODIFIED FIPS verifier — adding logic
// here breaks the wire-identity property that lets any peer verify a record
// with a stock FIPS library and no Lux dependency.
//
// Contract:
//   - pubKey is the signer's canonical public-key bytes (exactly as bound
//     into the validator-set leaf).
//   - message is the domain-separated consensus message (the round digest
//     subject — see quorum_message.go).
//   - ctx is the FIPS context string (≤255 bytes), bound at sign time.
//   - returns true iff the signature verifies; never panics.
type quorumVerifyFn func(pubKey, message, ctx, sig []byte) bool

// defaultQuorumVerifiers is the canonical scheme→verifier registry. Each
// entry is a thin dispatch over a stock FIPS verifier:
//
//	ML-DSA  → github.com/luxfi/crypto/mldsa  PublicKey.VerifySignatureCtx (FIPS 204)
//	SLH-DSA → github.com/luxfi/magnetar      slhdsa.Verify via VerifyCtx  (FIPS 205)
//
// The map is the single source of truth for which schemes a quorum cert
// can carry. A QuorumVerifierConfig may restrict to a subset but never
// adds schemes the registry does not know.
var defaultQuorumVerifiers = map[QuorumSchemeID]quorumVerifyFn{
	QuorumSchemeMLDSA44: mldsaVerifier(mldsa.MLDSA44),
	QuorumSchemeMLDSA65: mldsaVerifier(mldsa.MLDSA65),
	QuorumSchemeMLDSA87: mldsaVerifier(mldsa.MLDSA87),

	QuorumSchemeSLHDSA192s: slhdsaVerifier(magnetar.ModeM192s),
	QuorumSchemeSLHDSA192f: slhdsaVerifier(magnetar.ModeM192f),
	QuorumSchemeSLHDSA256s: slhdsaVerifier(magnetar.ModeM256s),
}

// mldsaVerifier builds a quorumVerifyFn over the unmodified FIPS 204
// ML-DSA verifier for a fixed parameter set. The public key is parsed
// per-call from its canonical bytes; a parse failure or wrong-length key is
// a clean false (the record is INVALID, not fatal).
func mldsaVerifier(mode mldsa.Mode) quorumVerifyFn {
	return func(pubKey, message, ctx, sig []byte) bool {
		pk, err := mldsa.PublicKeyFromBytes(pubKey, mode)
		if err != nil || pk == nil {
			return false
		}
		// VerifySignatureCtx is the stock FIPS 204 verify with a context
		// string per §5.4; an empty ctx (nil) is the empty-context case.
		return pk.VerifySignatureCtx(message, sig, ctx)
	}
}

// slhdsaVerifier builds a quorumVerifyFn over the unmodified FIPS 205
// SLH-DSA verifier (via magnetar's thin VerifyCtx dispatch over
// circl/slhdsa) for a fixed parameter set.
func slhdsaVerifier(mode magnetar.Mode) quorumVerifyFn {
	params, perr := magnetar.ParamsFor(mode)
	return func(pubKey, message, ctx, sig []byte) bool {
		if perr != nil || params == nil {
			return false
		}
		if len(pubKey) != params.PublicKeySize || len(sig) != params.SignatureSize {
			return false
		}
		pk := &magnetar.PublicKey{Mode: mode, Bytes: append([]byte(nil), pubKey...)}
		s := &magnetar.Signature{Mode: mode, Bytes: append([]byte(nil), sig...)}
		// VerifyCtx returns nil iff the signature is valid under
		// FIPS 205 SLH-DSA.Verify; any error => invalid record.
		return magnetar.VerifyCtx(params, pk, message, ctx, s) == nil
	}
}

// expectedPubKeyLen returns the canonical public-key byte length for a
// scheme, or (0, false) if the scheme is unknown. Used by the certificate
// verifier to reject a record whose embedded key length cannot match the
// scheme before any signature dispatch.
func expectedPubKeyLen(scheme QuorumSchemeID) (int, bool) {
	switch scheme {
	case QuorumSchemeMLDSA44:
		return mldsa.GetPublicKeySize(mldsa.MLDSA44), true
	case QuorumSchemeMLDSA65:
		return mldsa.GetPublicKeySize(mldsa.MLDSA65), true
	case QuorumSchemeMLDSA87:
		return mldsa.GetPublicKeySize(mldsa.MLDSA87), true
	case QuorumSchemeSLHDSA192s:
		return magnetar.ParamsM192s.PublicKeySize, true
	case QuorumSchemeSLHDSA192f:
		return magnetar.ParamsM192f.PublicKeySize, true
	case QuorumSchemeSLHDSA256s:
		return magnetar.ParamsM256s.PublicKeySize, true
	default:
		return 0, false
	}
}
