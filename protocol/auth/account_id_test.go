// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import (
	"bytes"
	"testing"
)

// TestDeriveAccountID_Deterministic — identical inputs MUST produce
// identical outputs across every call. Closes the "non-deterministic
// derivation" class so wallets and contracts can rederive the same
// AccountID without coordinating randomness.
func TestDeriveAccountID_Deterministic(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0xAB}, 1952) // ML-DSA-65 pubkey size
	profileID := uint32(0x01)                  // StrictPQ
	chainID := uint32(96369)
	scheme := WalletSchemeMLDSA65

	id1 := DeriveAccountID(profileID, chainID, scheme, pubkey)
	id2 := DeriveAccountID(profileID, chainID, scheme, pubkey)
	id3 := DeriveAccountID(profileID, chainID, scheme, pubkey)

	if id1 != id2 {
		t.Fatalf("DeriveAccountID not deterministic across two calls: %x vs %x", id1, id2)
	}
	if id2 != id3 {
		t.Fatalf("DeriveAccountID not deterministic across three calls: %x vs %x", id2, id3)
	}
	// Outputs must be 48 bytes wide.
	if len(id1) != 48 {
		t.Fatalf("DeriveAccountID output width = %d, want 48", len(id1))
	}
}

// TestDeriveAccountID_DistinctProfilesSameKey — the SAME (chainID,
// scheme, pubkey) on DIFFERENT profileIDs MUST yield DIFFERENT
// AccountIDs. Closes the cross-profile replay class — a wallet
// registered on the StrictPQ profile cannot be impersonated on the
// FIPS or Permissive profile even with the same pubkey.
func TestDeriveAccountID_DistinctProfilesSameKey(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0x42}, 1952)
	chainID := uint32(96369)
	scheme := WalletSchemeMLDSA65

	idStrict := DeriveAccountID(0x01, chainID, scheme, pubkey)
	idPermissive := DeriveAccountID(0x02, chainID, scheme, pubkey)
	idFIPS := DeriveAccountID(0x03, chainID, scheme, pubkey)
	idFork := DeriveAccountID(0x80, chainID, scheme, pubkey)

	all := []struct {
		name string
		id   [48]byte
	}{
		{"strict", idStrict},
		{"permissive", idPermissive},
		{"fips", idFIPS},
		{"fork", idFork},
	}
	for i := range all {
		for j := i + 1; j < len(all); j++ {
			if all[i].id == all[j].id {
				t.Errorf("AccountID(profile=%s) == AccountID(profile=%s) — profile separation broken",
					all[i].name, all[j].name)
			}
		}
	}
}

// TestDeriveAccountID_DistinctChainsSameKey — the SAME (profileID,
// scheme, pubkey) on DIFFERENT chainIDs MUST yield DIFFERENT
// AccountIDs. Closes the cross-chain pubkey-reuse replay class.
func TestDeriveAccountID_DistinctChainsSameKey(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0x42}, 1952)
	profileID := uint32(0x01)
	scheme := WalletSchemeMLDSA65

	// Three distinct chain IDs (mainnet=1, testnet=5, devnet=1337-like).
	id1 := DeriveAccountID(profileID, 1, scheme, pubkey)
	id5 := DeriveAccountID(profileID, 5, scheme, pubkey)
	id1337 := DeriveAccountID(profileID, 1337, scheme, pubkey)

	if id1 == id5 {
		t.Errorf("AccountID(chainID=1) == AccountID(chainID=5) — chain separation broken")
	}
	if id1 == id1337 {
		t.Errorf("AccountID(chainID=1) == AccountID(chainID=1337) — chain separation broken")
	}
	if id5 == id1337 {
		t.Errorf("AccountID(chainID=5) == AccountID(chainID=1337) — chain separation broken")
	}
}

// TestDeriveAccountID_DistinctSchemesSameKey — the SAME (profileID,
// chainID, pubkey) under DIFFERENT WalletSchemeIDs MUST yield
// DIFFERENT AccountIDs. Closes the "scheme confusion" class.
func TestDeriveAccountID_DistinctSchemesSameKey(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0x7E}, 1952)
	profileID := uint32(0x01)
	chainID := uint32(96369)

	id44 := DeriveAccountID(profileID, chainID, WalletSchemeMLDSA44, pubkey)
	id65 := DeriveAccountID(profileID, chainID, WalletSchemeMLDSA65, pubkey)
	id87 := DeriveAccountID(profileID, chainID, WalletSchemeMLDSA87, pubkey)
	idSLH := DeriveAccountID(profileID, chainID, WalletSchemeSLHDSA192f, pubkey)

	cases := []struct {
		a, b [48]byte
		name string
	}{
		{id44, id65, "ML-DSA-44 vs ML-DSA-65"},
		{id44, id87, "ML-DSA-44 vs ML-DSA-87"},
		{id44, idSLH, "ML-DSA-44 vs SLH-DSA-192f"},
		{id65, id87, "ML-DSA-65 vs ML-DSA-87"},
		{id65, idSLH, "ML-DSA-65 vs SLH-DSA-192f"},
		{id87, idSLH, "ML-DSA-87 vs SLH-DSA-192f"},
	}
	for _, c := range cases {
		if c.a == c.b {
			t.Errorf("scheme separation broken: %s yielded identical AccountIDs", c.name)
		}
	}
}

// TestDeriveAccountID_DistinctKeysSameChain — DIFFERENT pubkeys on the
// SAME (profileID, chainID, scheme) MUST yield DIFFERENT AccountIDs.
// Basic collision-resistance smoke test.
func TestDeriveAccountID_DistinctKeysSameChain(t *testing.T) {
	profileID := uint32(0x01)
	chainID := uint32(96369)
	scheme := WalletSchemeMLDSA65

	pkA := bytes.Repeat([]byte{0x01}, 1952)
	pkB := bytes.Repeat([]byte{0x02}, 1952)

	idA := DeriveAccountID(profileID, chainID, scheme, pkA)
	idB := DeriveAccountID(profileID, chainID, scheme, pkB)

	if idA == idB {
		t.Fatalf("distinct pubkeys produced identical AccountIDs — pubkey not bound into digest")
	}
}
