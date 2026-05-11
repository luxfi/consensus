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
	chainID := uint32(43114)
	scheme := WalletSchemeMLDSA65

	id1 := DeriveAccountID(chainID, scheme, pubkey)
	id2 := DeriveAccountID(chainID, scheme, pubkey)
	id3 := DeriveAccountID(chainID, scheme, pubkey)

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

// TestDeriveAccountID_DistinctChainsSameKey — the SAME (scheme, pubkey)
// on DIFFERENT chainIDs MUST yield DIFFERENT AccountIDs. Closes the
// cross-chain pubkey-reuse replay class.
func TestDeriveAccountID_DistinctChainsSameKey(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0x42}, 1952)
	scheme := WalletSchemeMLDSA65

	// Three distinct chain IDs (mainnet=1, testnet=5, devnet=1337-like).
	id1 := DeriveAccountID(1, scheme, pubkey)
	id5 := DeriveAccountID(5, scheme, pubkey)
	id1337 := DeriveAccountID(1337, scheme, pubkey)

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

// TestDeriveAccountID_DistinctSchemesSameKey — the SAME (chainID,
// pubkey) under DIFFERENT WalletSchemeIDs MUST yield DIFFERENT
// AccountIDs. Closes the "scheme confusion" class.
func TestDeriveAccountID_DistinctSchemesSameKey(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0x7E}, 1952)
	chainID := uint32(43114)

	id44 := DeriveAccountID(chainID, WalletSchemeMLDSA44, pubkey)
	id65 := DeriveAccountID(chainID, WalletSchemeMLDSA65, pubkey)
	id87 := DeriveAccountID(chainID, WalletSchemeMLDSA87, pubkey)
	idSLH := DeriveAccountID(chainID, WalletSchemeSLHDSA192f, pubkey)

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
// SAME (chainID, scheme) MUST yield DIFFERENT AccountIDs. Basic
// collision-resistance smoke test.
func TestDeriveAccountID_DistinctKeysSameChain(t *testing.T) {
	chainID := uint32(43114)
	scheme := WalletSchemeMLDSA65

	pkA := bytes.Repeat([]byte{0x01}, 1952)
	pkB := bytes.Repeat([]byte{0x02}, 1952)

	idA := DeriveAccountID(chainID, scheme, pkA)
	idB := DeriveAccountID(chainID, scheme, pkB)

	if idA == idB {
		t.Fatalf("distinct pubkeys produced identical AccountIDs — pubkey not bound into digest")
	}
}
