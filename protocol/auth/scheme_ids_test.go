// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import "testing"

// TestWalletSchemeID_String pins the wire-name table so a rename is
// immediately visible in CI. Audit tooling and logs read these strings;
// flipping one silently is a backwards-compat break we want loud.
func TestWalletSchemeID_String(t *testing.T) {
	cases := []struct {
		id   WalletSchemeID
		want string
	}{
		{WalletSchemeNone, "none"},
		{WalletSchemeECDSASecp256k1Legacy, "ecdsa-secp256k1-legacy"},
		{WalletSchemeMLDSA44, "ml-dsa-44"},
		{WalletSchemeMLDSA65, "ml-dsa-65"},
		{WalletSchemeMLDSA87, "ml-dsa-87"},
		{WalletSchemeSLHDSA128f, "slh-dsa-128f"},
		{WalletSchemeSLHDSA192f, "slh-dsa-192f"},
		{WalletSchemeSLHDSA256f, "slh-dsa-256f"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("WalletSchemeID(0x%02x).String() = %q, want %q", uint8(c.id), got, c.want)
		}
	}
	unknown := WalletSchemeID(0xFE)
	if got := unknown.String(); got != "wallet-scheme(0xfe)" {
		t.Errorf("unknown WalletSchemeID.String() = %q, want wallet-scheme(0xfe)", got)
	}
}

// TestWalletSchemeID_Predicates pins the predicate truth table.
func TestWalletSchemeID_Predicates(t *testing.T) {
	rawMLDSA := []WalletSchemeID{WalletSchemeMLDSA44, WalletSchemeMLDSA65, WalletSchemeMLDSA87}
	slhDSA := []WalletSchemeID{WalletSchemeSLHDSA128f, WalletSchemeSLHDSA192f, WalletSchemeSLHDSA256f}
	classical := []WalletSchemeID{WalletSchemeECDSASecp256k1Legacy}

	for _, s := range rawMLDSA {
		if !s.IsRawMLDSA() {
			t.Errorf("%s should IsRawMLDSA", s)
		}
		if s.IsSLHDSA() {
			t.Errorf("%s should not IsSLHDSA", s)
		}
		if !s.IsPostQuantum() {
			t.Errorf("%s should IsPostQuantum", s)
		}
		if s.IsLegacyClassical() {
			t.Errorf("%s should not IsLegacyClassical", s)
		}
	}
	for _, s := range slhDSA {
		if s.IsRawMLDSA() {
			t.Errorf("%s should not IsRawMLDSA", s)
		}
		if !s.IsSLHDSA() {
			t.Errorf("%s should IsSLHDSA", s)
		}
		if !s.IsPostQuantum() {
			t.Errorf("%s should IsPostQuantum", s)
		}
	}
	for _, s := range classical {
		if s.IsPostQuantum() {
			t.Errorf("%s should not IsPostQuantum", s)
		}
		if !s.IsLegacyClassical() {
			t.Errorf("%s should IsLegacyClassical", s)
		}
	}
	// None is neither PQ nor classical-legacy: it's the unspecified
	// wire byte that every locked profile refuses.
	none := WalletSchemeNone
	if none.IsPostQuantum() || none.IsLegacyClassical() || none.IsRawMLDSA() || none.IsSLHDSA() {
		t.Errorf("WalletSchemeNone predicates must all be false; got PQ=%v classical=%v rawML=%v slh=%v",
			none.IsPostQuantum(), none.IsLegacyClassical(), none.IsRawMLDSA(), none.IsSLHDSA())
	}
}

// TestTxSchemeID_String pins the wire-name table.
func TestTxSchemeID_String(t *testing.T) {
	cases := []struct {
		id   TxSchemeID
		want string
	}{
		{TxSchemeNone, "none"},
		{TxSchemeLegacyECDSA, "tx-legacy-ecdsa"},
		{TxSchemePQAuthV1, "tx-pq-auth-v1"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("TxSchemeID(0x%02x).String() = %q, want %q", uint8(c.id), got, c.want)
		}
	}
}

// TestTxSchemeID_IsPostQuantum pins the PQ predicate.
func TestTxSchemeID_IsPostQuantum(t *testing.T) {
	if !TxSchemePQAuthV1.IsPostQuantum() {
		t.Error("TxSchemePQAuthV1 must IsPostQuantum")
	}
	if TxSchemeLegacyECDSA.IsPostQuantum() {
		t.Error("TxSchemeLegacyECDSA must not IsPostQuantum")
	}
	if TxSchemeNone.IsPostQuantum() {
		t.Error("TxSchemeNone must not IsPostQuantum")
	}
}

// TestContractAuthID_String pins the wire-name table.
func TestContractAuthID_String(t *testing.T) {
	cases := []struct {
		id   ContractAuthID
		want string
	}{
		{ContractAuthNone, "none"},
		{ContractAuthECDSASecp256k1Legacy, "ecdsa-secp256k1-legacy"},
		{ContractAuthMLDSA44, "ml-dsa-44"},
		{ContractAuthMLDSA65, "ml-dsa-65"},
		{ContractAuthMLDSA87, "ml-dsa-87"},
		{ContractAuthSLHDSA128f, "slh-dsa-128f"},
		{ContractAuthSLHDSA192f, "slh-dsa-192f"},
		{ContractAuthSLHDSA256f, "slh-dsa-256f"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("ContractAuthID(0x%02x).String() = %q, want %q", uint8(c.id), got, c.want)
		}
	}
}

// TestKeyExchangeID_String pins the wire-name table.
func TestKeyExchangeID_String(t *testing.T) {
	cases := []struct {
		id   KeyExchangeID
		want string
	}{
		{KeyExchangeNone, "none"},
		{KeyExchangeX25519Legacy, "x25519-legacy"},
		{KeyExchangeMLKEM512, "ml-kem-512"},
		{KeyExchangeMLKEM768, "ml-kem-768"},
		{KeyExchangeMLKEM1024, "ml-kem-1024"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("KeyExchangeID(0x%02x).String() = %q, want %q", uint8(c.id), got, c.want)
		}
	}
	// IsPostQuantum: ML-KEM family yes, X25519 no.
	pq := []KeyExchangeID{KeyExchangeMLKEM512, KeyExchangeMLKEM768, KeyExchangeMLKEM1024}
	for _, k := range pq {
		if !k.IsPostQuantum() {
			t.Errorf("%s should IsPostQuantum", k)
		}
	}
	if KeyExchangeX25519Legacy.IsPostQuantum() {
		t.Error("X25519 legacy must not IsPostQuantum")
	}
}

// TestRecoverySchemeID_String pins the wire-name table.
func TestRecoverySchemeID_String(t *testing.T) {
	cases := []struct {
		id   RecoverySchemeID
		want string
	}{
		{RecoverySchemeNone, "none"},
		{RecoverySchemeNoneByDesign, "recovery-none-by-design"},
		{RecoverySchemeSocialKofN, "recovery-social-k-of-n"},
		{RecoverySchemeTimelockPQ, "recovery-timelock-pq"},
	}
	for _, c := range cases {
		if got := c.id.String(); got != c.want {
			t.Errorf("RecoverySchemeID(0x%02x).String() = %q, want %q", uint8(c.id), got, c.want)
		}
	}
}

// TestRecoverySchemeID_IsKnown pins the membership predicate.
func TestRecoverySchemeID_IsKnown(t *testing.T) {
	for _, s := range []RecoverySchemeID{
		RecoverySchemeNone,
		RecoverySchemeNoneByDesign,
		RecoverySchemeSocialKofN,
		RecoverySchemeTimelockPQ,
	} {
		if !s.IsKnown() {
			t.Errorf("%s should be IsKnown", s)
		}
	}
	if (RecoverySchemeID(0x99)).IsKnown() {
		t.Error("unknown byte 0x99 reported IsKnown")
	}
}

// TestContractAuthID_PostQuantumPredicate pins the truth table.
func TestContractAuthID_PostQuantumPredicate(t *testing.T) {
	pq := []ContractAuthID{
		ContractAuthMLDSA44, ContractAuthMLDSA65, ContractAuthMLDSA87,
		ContractAuthSLHDSA128f, ContractAuthSLHDSA192f, ContractAuthSLHDSA256f,
	}
	for _, c := range pq {
		if !c.IsPostQuantum() {
			t.Errorf("%s should IsPostQuantum", c)
		}
	}
	if ContractAuthECDSASecp256k1Legacy.IsPostQuantum() {
		t.Error("ECDSA legacy must not IsPostQuantum")
	}
	if !ContractAuthECDSASecp256k1Legacy.IsLegacyClassical() {
		t.Error("ECDSA legacy must IsLegacyClassical")
	}
	if ContractAuthNone.IsPostQuantum() {
		t.Error("None must not IsPostQuantum")
	}
}

// TestPrecompileAddresses pins the canonical EVM precompile addresses
// so a renumbering accident shows up in CI rather than at the wiring
// layer (~/work/lux/coreth/precompile/pqverify).
func TestPrecompileAddresses(t *testing.T) {
	cases := []struct {
		name string
		got  uint16
		want uint16
	}{
		{"pq_verify_mldsa65", PrecompileAddrPQVerifyMLDSA65, 0x301},
		{"pq_verify_mldsa87", PrecompileAddrPQVerifyMLDSA87, 0x302},
		{"pq_verify_slh_dsa", PrecompileAddrPQVerifySLHDSA, 0x303},
		{"pq_verify_z_auth_proof", PrecompileAddrPQVerifyZAuthProof, 0x304},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("precompile %s: got 0x%03x want 0x%03x", c.name, c.got, c.want)
		}
	}
}
