// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

// kmac256.go — SP 800-185 KMAC256 wrapper for canonical Quasar MACs.
//
// The DAG event-horizon attestations in bls.go bind (block_id, vote_map)
// to a validator's key material with a keyed MAC. Previously the kernel
// was HMAC-SHA256 (SHA-2 family). That is fine for classical interop
// but is NOT the canonical SHA-3-family MAC required by the strict-PQ
// profile (ChainSecurityProfile.HashSuiteID = HashSuiteSHA3NIST), which
// pins cSHAKE256 / KMAC256 / TupleHash256 (FIPS 202 + SP 800-185).
//
// KMAC256 is the FIPS-aligned MAC. It accepts a built-in customization
// string for unambiguous domain separation, so every Quasar MAC call
// site advertises its purpose on the wire:
//
//   "QUASAR_EVENT_HORIZON_BLS_MAC_V1"   — phaseII BLS aggregate commitment
//   "QUASAR_EVENT_HORIZON_PQ_MAC_V1"    — phaseII PQ certificate
//   "QUASAR_HORIZON_SIG_BLS_MAC_V1"     — createHorizonSignature BLS half
//   "QUASAR_HORIZON_SIG_PQ_MAC_V1"      — createHorizonSignature PQ half
//
// Two MACs computed with different customization strings are independent
// random oracles even when the key and message are byte-identical.
// Bumping any "_V1" tag invalidates every prior MAC under it; that is
// the correct behaviour when a wire-format breaking change ships.
//
// The implementation reuses the SP 800-185 encoders already vendored
// into this package by round_digest.go (encodeStringSP800185,
// rightEncodeSP800185). One implementation, one customization per call
// site, zero hidden state — kept inside the quasar package so the
// consensus module has no upward dependency on pulsar.

import (
	"crypto/subtle"

	"github.com/luxfi/consensus/config"
	"golang.org/x/crypto/sha3"
)

// Canonical MAC byte width — 48 bytes (SHA3-384 width). Matches
// MinHashOutputBits=384 on the LuxStrictPQ profile and keeps the MAC
// width orthogonal to anything in the SHA-2 family (whose
// distinguishing output sizes are 32 / 64).
const kmacMACOutLen = 48

// Customization strings — each MAC call site declares its purpose.
// Distinct strings make two MACs over the same (key, message) pair
// independent. Tags are pinned at "_V1"; a breaking layout change
// claims "_V2" and invalidates every prior MAC under the old tag.
const (
	customQuasarEventHorizonBLSMAC = "QUASAR_EVENT_HORIZON_BLS_MAC_V1"
	customQuasarEventHorizonPQMAC  = "QUASAR_EVENT_HORIZON_PQ_MAC_V1"
	customQuasarHorizonSigBLSMAC   = "QUASAR_HORIZON_SIG_BLS_MAC_V1"
	customQuasarHorizonSigPQMAC    = "QUASAR_HORIZON_SIG_PQ_MAC_V1"
)

// kmac256 returns KMAC256(K, X, outLen, S) per NIST SP 800-185 §4.
//
// Encoded as cSHAKE256 with the function name "KMAC" and the
// customization string S; absorption is bytepad(encode_string(K), 136)
// || X || right_encode(outLen*8). Output is L = outLen bytes read from
// the XOF.
//
// The encoders (encodeStringSP800185, rightEncodeSP800185) are vendored
// in this package by round_digest.go; this function depends on them
// directly so the implementation is reviewable in one diff.
func kmac256(key, msg []byte, outLen int, customization string) []byte {
	x := bytepadSP800185(encodeStringSP800185(key), 136)
	x = append(x, msg...)
	x = append(x, rightEncodeSP800185(uint64(outLen)*8)...)

	h := sha3.NewCShake256([]byte("KMAC"), []byte(customization))
	_, _ = h.Write(x)
	out := make([]byte, outLen)
	_, _ = h.Read(out)
	return out
}

// bytepadSP800185 pads x with zeros so the result is a multiple of w
// bytes, prefixed by left_encode(w). SP 800-185 §2.3.3.
//
// rate-136 byte (1088-bit) is the cSHAKE256 absorption rate; KMAC256
// always passes 136 here.
func bytepadSP800185(x []byte, w int) []byte {
	prefix := leftEncodeSP800185(uint64(w))
	out := make([]byte, 0, len(prefix)+len(x)+w)
	out = append(out, prefix...)
	out = append(out, x...)
	for len(out)%w != 0 {
		out = append(out, 0x00)
	}
	return out
}

// macEqual is a constant-time equality check over two MAC outputs.
// Wrapper around crypto/subtle so call sites read clearly and we keep
// the constant-time invariant in one place.
func macEqual(a, b []byte) bool {
	return subtle.ConstantTimeCompare(a, b) == 1
}

// strictPQRejectsLegacyMAC reports whether a profile forbids accepting
// a MAC byte string of legacy width (32 bytes — HMAC-SHA256 output).
// LuxStrictPQ / LuxFIPS profiles set HashSuiteID = HashSuiteSHA3NIST,
// at which point only KMAC256 widths (48 bytes) are admissible.
//
// A nil profile defaults to permissive: lets legacy bytes through so
// the consensus engine can roundtrip a CertBundle built under an older
// classical-compat profile in test / migration scenarios.
func strictPQRejectsLegacyMAC(profile *config.ChainSecurityProfile) bool {
	if profile == nil {
		return false
	}
	return profile.HashSuiteID == config.HashSuiteSHA3NIST
}
