// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestValidatorSchemeID_PinsIdentityScheme — ValidatorSchemeID returns
// the chain's pinned IdentitySchemeID. The two are the same byte by
// design; the alias names the role for callers that consume the
// NodeIDScheme cross-axis check.
func TestValidatorSchemeID_PinsIdentityScheme(t *testing.T) {
	require := require.New(t)

	p := StrictPQ()
	require.Equal(p.IdentitySchemeID, p.ValidatorSchemeID())
	require.Equal(SigSchemeMLDSA65, p.ValidatorSchemeID())
}

// TestAcceptsValidatorScheme_Matched — the matched-scheme path: a peer
// presenting the same byte the chain pins is accepted.
func TestAcceptsValidatorScheme_Matched(t *testing.T) {
	require := require.New(t)

	p := StrictPQ()
	require.NoError(p.AcceptsValidatorScheme(SigSchemeMLDSA65, false))
}

// TestAcceptsValidatorScheme_StrictPQ_RejectsCrossPQScheme — a strict-PQ
// chain that pins ML-DSA-65 refuses a peer presenting ML-DSA-87 (and
// vice versa). The cross-axis gate is byte-equality; "different PQ
// scheme" is still a mismatch even when both bytes are PQ-positive.
func TestAcceptsValidatorScheme_StrictPQ_RejectsCrossPQScheme(t *testing.T) {
	require := require.New(t)

	p := StrictPQ() // pins ML-DSA-65
	err := p.AcceptsValidatorScheme(SigSchemeMLDSA87, false)
	require.ErrorIs(err, ErrValidatorSchemeMismatch)
}

// TestAcceptsValidatorScheme_StrictPQ_RejectsClassicalEvenUnderUnsafeFlag
// — under strict-PQ, the operator cannot bypass the cross-axis check
// even by setting classicalCompatUnsafe=true. This is the explicit
// defense in depth: strict-PQ chains refuse classical NodeIDs at
// every layer.
func TestAcceptsValidatorScheme_StrictPQ_RejectsClassicalEvenUnderUnsafeFlag(t *testing.T) {
	require := require.New(t)

	p := StrictPQ()
	err := p.AcceptsValidatorScheme(sigSchemeSecp256k1Classical, true)
	require.ErrorIs(err, ErrValidatorSchemeMismatch)
}

// TestAcceptsValidatorScheme_Permissive_AcceptsClassicalUnderUnsafeFlag
// — the permissive profile (testnet/devnet) allows classical NodeIDs
// when the operator explicitly opts in. Without the flag, classical
// is still refused.
func TestAcceptsValidatorScheme_Permissive_AcceptsClassicalUnderUnsafeFlag(t *testing.T) {
	require := require.New(t)

	p := Permissive()

	// Without the unsafe flag, classical is refused even on permissive.
	err := p.AcceptsValidatorScheme(sigSchemeSecp256k1Classical, false)
	require.ErrorIs(err, ErrValidatorSchemeMismatch)

	// With the unsafe flag on permissive, classical is accepted.
	require.NoError(p.AcceptsValidatorScheme(sigSchemeSecp256k1Classical, true))
}

// TestAcceptsValidatorScheme_RejectsUnknownByte — any byte outside the
// pinned and the named-classical schemes is refused regardless of the
// flag. The flag's effect is narrowly the named classical scheme, not
// "anything outside the PQ block".
func TestAcceptsValidatorScheme_RejectsUnknownByte(t *testing.T) {
	require := require.New(t)

	p := StrictPQ()
	for _, bad := range []SigSchemeID{
		SigSchemeNone,
		SigSchemeBLS12381,
		SigSchemeMLDSA44, // PQ-positive but Cat 2 only; not the pinned identity
		SigSchemePulsarM65,
		0x91, // 0x90+ but not the named classical byte
	} {
		err := p.AcceptsValidatorScheme(bad, true)
		require.ErrorIs(err, ErrValidatorSchemeMismatch,
			"byte 0x%02x should be refused", uint8(bad))
	}
}

// TestSigSchemeID_IsClassicalCompatUnsafe — only the named classical
// secp256k1 byte (0x90) is in the classical-compat block. Other 0x90+
// bytes are reserved and classify false.
func TestSigSchemeID_IsClassicalCompatUnsafe(t *testing.T) {
	require := require.New(t)

	require.True(sigSchemeSecp256k1Classical.IsClassicalCompatUnsafe())

	require.False(SigSchemeMLDSA65.IsClassicalCompatUnsafe())
	require.False(SigSchemeMLDSA87.IsClassicalCompatUnsafe())
	require.False(SigSchemeNone.IsClassicalCompatUnsafe())
	require.False(SigSchemeBLS12381.IsClassicalCompatUnsafe())
	require.False(SigSchemeID(0x91).IsClassicalCompatUnsafe(),
		"reserved 0x91 must not classify as classical-compat")
}

// TestValidatorScheme_AlignedWithIdsEnum — sanity check that the
// consensus SigSchemeID assignments and the documented luxfi/ids
// NodeIDScheme assignments agree on the wire byte. This test pins
// the alignment in source-of-truth form so a future change to either
// enum trips the gate. Numbers must match by-byte.
//
// (consensus/config cannot import luxfi/ids — ids depends on
// consensus/config conceptually but not in Go imports. The pin lives
// here in literal form.)
func TestValidatorScheme_AlignedWithIdsEnum(t *testing.T) {
	require := require.New(t)

	require.Equal(uint8(0x42), uint8(SigSchemeMLDSA65),
		"NodeIDSchemeMLDSA65 in ids must equal SigSchemeMLDSA65 here")
	require.Equal(uint8(0x43), uint8(SigSchemeMLDSA87),
		"NodeIDSchemeMLDSA87 in ids must equal SigSchemeMLDSA87 here")
	require.Equal(uint8(0x90), uint8(sigSchemeSecp256k1Classical),
		"NodeIDSchemeSecp256k1 in ids must equal sigSchemeSecp256k1Classical here")
}
