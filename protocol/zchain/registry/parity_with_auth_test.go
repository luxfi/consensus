// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry_test

import (
	"bytes"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/auth"
	"github.com/luxfi/consensus/protocol/zchain/registry"
)

// TestDeriveAccountID_MatchesRegistryDerivation proves auth.DeriveAccountID
// and registry.DeriveAccountID produce byte-identical 48-byte outputs for
// the same (profileID, chainID, scheme, compactPubkey) inputs.
//
// Both functions vendor their own hash kernels (each package owns a
// local TupleHash256 / SHAKE256 helper to avoid an upward dependency
// on its parent) but MUST agree byte-for-byte; otherwise a wallet
// registered through the Z-Chain registry path would be rejected by
// VerifyTxAuthEnvelope (and vice versa) — every tx from that account
// would fail with ErrTxAuthAccountIDMismatch.
//
// Before the Bug 2 fix this test would FAIL: auth's customization tag
// was "LUX_ACCOUNT_V1" and its input layout omitted the profile_id
// prefix, while registry's customization was "LUX_ACCOUNT_ID_V1" with
// the profile prefix bound first.
//
// Parity is a wire-format invariant, not just a correctness niceety.
// A divergence here is a critical-severity finding.
func TestDeriveAccountID_MatchesRegistryDerivation(t *testing.T) {
	cases := []struct {
		name      string
		profileID uint32
		chainID   uint32
		scheme    config.WalletSchemeID
		pubkey    []byte
	}{
		{
			name:      "lux-strict-pq mainnet ML-DSA-65",
			profileID: uint32(config.ProfileStrictPQ),
			chainID:   43114,
			scheme:    config.WalletSchemeMLDSA65,
			pubkey:    bytes.Repeat([]byte{0xAB}, 1952),
		},
		{
			name:      "zoo-strict-pq mainnet ML-DSA-65",
			profileID: uint32(config.ProfileZooStrictPQ),
			chainID:   200200,
			scheme:    config.WalletSchemeMLDSA65,
			pubkey:    bytes.Repeat([]byte{0xCD}, 1952),
		},
		{
			name:      "hanzo-strict-pq mainnet ML-DSA-87",
			profileID: uint32(config.ProfileHanzoStrictPQ),
			chainID:   300300,
			scheme:    config.WalletSchemeMLDSA87,
			pubkey:    bytes.Repeat([]byte{0xEF}, 2592),
		},
		{
			name:      "lux-permissive testnet ML-DSA-65",
			profileID: uint32(config.ProfilePermissive),
			chainID:   43113,
			scheme:    config.WalletSchemeMLDSA65,
			pubkey:    bytes.Repeat([]byte{0x42}, 1952),
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// Registry path — the one the Z-Chain Apply step uses to
			// derive the AccountID a record is keyed under.
			fromRegistry := registry.DeriveAccountID(c.profileID, c.chainID, c.scheme, c.pubkey)

			// Auth path — the one VerifyTxAuthEnvelope recomputes to
			// match against env.AccountID. The auth package types
			// WalletSchemeID parallel to config.WalletSchemeID; for the
			// numeric values used by strict-PQ wallets (0x42, 0x43)
			// they're byte-equal so a uint8 conversion is sufficient.
			fromAuth := auth.DeriveAccountID(c.profileID, c.chainID, auth.WalletSchemeID(c.scheme), c.pubkey)

			if fromRegistry != fromAuth {
				t.Errorf("DeriveAccountID divergence:\n  registry path: %x\n  auth path:     %x",
					fromRegistry, fromAuth)
			}
		})
	}
}

// TestDeriveAccountID_CustomizationTagsMatch pins the customization-tag
// agreement between auth and registry packages. The tag is what
// produces the canonical digest stream; if the two packages ever drift
// to different tags, every AccountID computed through one path would
// fail to validate through the other.
func TestDeriveAccountID_CustomizationTagsMatch(t *testing.T) {
	if registry.AccountIDCustomization != "LUX_ACCOUNT_ID_V1" {
		t.Errorf("registry.AccountIDCustomization = %q, want %q",
			registry.AccountIDCustomization, "LUX_ACCOUNT_ID_V1")
	}
	// auth's tag is unexported; the parity test above proves it agrees
	// transitively. This test pins the registry side as the single
	// canonical tag a reviewer can grep for.
}
