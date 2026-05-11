// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import (
	"encoding/hex"
	"errors"
	"testing"
)

// =============================================================================
// E2E PQ enums — stable integers, canonical strings, post-quantum classification
// =============================================================================

// TestWalletSchemeID_StableIntegers locks the wire bytes for the wallet
// signature scheme enum. Renumbering breaks every wallet address ever
// derived under this byte; new schemes claim the next free integer.
func TestWalletSchemeID_StableIntegers(t *testing.T) {
	cases := []struct {
		s    WalletSchemeID
		want uint8
	}{
		{WalletSchemeInvalid, 0x00},
		{WalletSchemeMLDSA65, 0x42},
		{WalletSchemeMLDSA87, 0x43},
		{WalletSchemeECDSAUnsafe, 0x90},
	}
	for _, c := range cases {
		if got := uint8(c.s); got != c.want {
			t.Errorf("WalletSchemeID %q = 0x%02x, want 0x%02x", c.s.String(), got, c.want)
		}
	}
}

// TestWalletSchemeID_String pins the canonical wire name.
func TestWalletSchemeID_String(t *testing.T) {
	cases := []struct {
		s    WalletSchemeID
		want string
	}{
		{WalletSchemeInvalid, "invalid"},
		{WalletSchemeMLDSA65, "ml-dsa-65"},
		{WalletSchemeMLDSA87, "ml-dsa-87"},
		{WalletSchemeECDSAUnsafe, "ecdsa-unsafe-classical-forbidden-in-pq"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("WalletSchemeID(0x%02x).String() = %q, want %q", uint8(c.s), got, c.want)
		}
	}
}

// TestWalletSchemeID_IsPostQuantum — ML-DSA-65 / ML-DSA-87 are PQ;
// ECDSA and Invalid are not.
func TestWalletSchemeID_IsPostQuantum(t *testing.T) {
	for _, s := range []WalletSchemeID{WalletSchemeMLDSA65, WalletSchemeMLDSA87} {
		if !s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=true", s.String())
		}
	}
	for _, s := range []WalletSchemeID{WalletSchemeInvalid, WalletSchemeECDSAUnsafe} {
		if s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=false", s.String())
		}
	}
}

// TestWalletSchemeID_IsForbiddenInPQMode — the 0x90 classical marker
// is forbidden; everything else is not.
func TestWalletSchemeID_IsForbiddenInPQMode(t *testing.T) {
	if !WalletSchemeECDSAUnsafe.IsForbiddenInPQMode() {
		t.Errorf("WalletSchemeECDSAUnsafe must be forbidden in PQ mode")
	}
	for _, s := range []WalletSchemeID{WalletSchemeInvalid, WalletSchemeMLDSA65, WalletSchemeMLDSA87} {
		if s.IsForbiddenInPQMode() {
			t.Errorf("%q must NOT be forbidden in PQ mode", s.String())
		}
	}
}

// TestTxSchemeID_StableIntegers mirrors the wallet enum tests on the
// tx-auth axis. The two enums share the 0x00/0x42/0x43/0x90 pattern
// intentionally — the byte is independent on the wire but the pattern
// keeps audit readers aligned.
func TestTxSchemeID_StableIntegers(t *testing.T) {
	cases := []struct {
		s    TxSchemeID
		want uint8
	}{
		{TxSchemeInvalid, 0x00},
		{TxSchemeMLDSA65, 0x42},
		{TxSchemeMLDSA87, 0x43},
		{TxSchemeECDSAUnsafe, 0x90},
	}
	for _, c := range cases {
		if got := uint8(c.s); got != c.want {
			t.Errorf("TxSchemeID %q = 0x%02x, want 0x%02x", c.s.String(), got, c.want)
		}
	}
}

// TestTxSchemeID_String pins the canonical wire name.
func TestTxSchemeID_String(t *testing.T) {
	cases := []struct {
		s    TxSchemeID
		want string
	}{
		{TxSchemeInvalid, "invalid"},
		{TxSchemeMLDSA65, "ml-dsa-65"},
		{TxSchemeMLDSA87, "ml-dsa-87"},
		{TxSchemeECDSAUnsafe, "ecdsa-unsafe-classical-forbidden-in-pq"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("TxSchemeID(0x%02x).String() = %q, want %q", uint8(c.s), got, c.want)
		}
	}
}

// TestTxSchemeID_IsPostQuantum / IsForbiddenInPQMode mirror the wallet tests.
func TestTxSchemeID_IsPostQuantum(t *testing.T) {
	for _, s := range []TxSchemeID{TxSchemeMLDSA65, TxSchemeMLDSA87} {
		if !s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=true", s.String())
		}
	}
	for _, s := range []TxSchemeID{TxSchemeInvalid, TxSchemeECDSAUnsafe} {
		if s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=false", s.String())
		}
	}
}

func TestTxSchemeID_IsForbiddenInPQMode(t *testing.T) {
	if !TxSchemeECDSAUnsafe.IsForbiddenInPQMode() {
		t.Errorf("TxSchemeECDSAUnsafe must be forbidden in PQ mode")
	}
	for _, s := range []TxSchemeID{TxSchemeInvalid, TxSchemeMLDSA65, TxSchemeMLDSA87} {
		if s.IsForbiddenInPQMode() {
			t.Errorf("%q must NOT be forbidden in PQ mode", s.String())
		}
	}
}

// TestContractAuthID_StableIntegers pins the byte values. The enum has
// the widest spread because contract auth covers proof-carrying,
// multi-party, session-scoped, raw lattice, and classical primitives.
func TestContractAuthID_StableIntegers(t *testing.T) {
	cases := []struct {
		s    ContractAuthID
		want uint8
	}{
		{ContractAuthInvalid, 0x00},
		{ContractAuthZChainProof, 0x10},
		{ContractAuthMultisigMLDSA, 0x20},
		{ContractAuthSessionPQ, 0x21},
		{ContractAuthMLDSA65, 0x42},
		{ContractAuthMLDSA87, 0x43},
		{ContractAuthECDSAUnsafe, 0x90},
		{ContractAuthBLSUnsafe, 0x91},
	}
	for _, c := range cases {
		if got := uint8(c.s); got != c.want {
			t.Errorf("ContractAuthID %q = 0x%02x, want 0x%02x", c.s.String(), got, c.want)
		}
	}
}

// TestContractAuthID_String pins canonical wire names.
func TestContractAuthID_String(t *testing.T) {
	cases := []struct {
		s    ContractAuthID
		want string
	}{
		{ContractAuthInvalid, "invalid"},
		{ContractAuthZChainProof, "z-chain-proof"},
		{ContractAuthMultisigMLDSA, "multisig-ml-dsa"},
		{ContractAuthSessionPQ, "session-pq"},
		{ContractAuthMLDSA65, "ml-dsa-65"},
		{ContractAuthMLDSA87, "ml-dsa-87"},
		{ContractAuthECDSAUnsafe, "ecdsa-unsafe-classical-forbidden-in-pq"},
		{ContractAuthBLSUnsafe, "bls-unsafe-classical-forbidden-in-pq"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("ContractAuthID(0x%02x).String() = %q, want %q", uint8(c.s), got, c.want)
		}
	}
}

// TestContractAuthID_IsPostQuantum — every non-classical primitive
// (proof-carrying, multi-party, session-PQ, raw ML-DSA) qualifies.
func TestContractAuthID_IsPostQuantum(t *testing.T) {
	for _, s := range []ContractAuthID{
		ContractAuthZChainProof,
		ContractAuthMultisigMLDSA,
		ContractAuthSessionPQ,
		ContractAuthMLDSA65,
		ContractAuthMLDSA87,
	} {
		if !s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=true", s.String())
		}
	}
	for _, s := range []ContractAuthID{
		ContractAuthInvalid,
		ContractAuthECDSAUnsafe,
		ContractAuthBLSUnsafe,
	} {
		if s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=false", s.String())
		}
	}
}

func TestContractAuthID_IsForbiddenInPQMode(t *testing.T) {
	for _, s := range []ContractAuthID{ContractAuthECDSAUnsafe, ContractAuthBLSUnsafe} {
		if !s.IsForbiddenInPQMode() {
			t.Errorf("%q must be forbidden in PQ mode", s.String())
		}
	}
	for _, s := range []ContractAuthID{
		ContractAuthInvalid,
		ContractAuthZChainProof,
		ContractAuthMultisigMLDSA,
		ContractAuthSessionPQ,
		ContractAuthMLDSA65,
		ContractAuthMLDSA87,
	} {
		if s.IsForbiddenInPQMode() {
			t.Errorf("%q must NOT be forbidden in PQ mode", s.String())
		}
	}
}

// TestKeyExchangeID_StableIntegers pins the KEM byte values.
func TestKeyExchangeID_StableIntegers(t *testing.T) {
	cases := []struct {
		s    KeyExchangeID
		want uint8
	}{
		{KeyExchangeInvalid, 0x00},
		{KeyExchangeMLKEM768, 0x01},
		{KeyExchangeMLKEM1024, 0x02},
		{KeyExchangeX25519Unsafe, 0x90},
	}
	for _, c := range cases {
		if got := uint8(c.s); got != c.want {
			t.Errorf("KeyExchangeID %q = 0x%02x, want 0x%02x", c.s.String(), got, c.want)
		}
	}
}

func TestKeyExchangeID_String(t *testing.T) {
	cases := []struct {
		s    KeyExchangeID
		want string
	}{
		{KeyExchangeInvalid, "invalid"},
		{KeyExchangeMLKEM768, "ml-kem-768"},
		{KeyExchangeMLKEM1024, "ml-kem-1024"},
		{KeyExchangeX25519Unsafe, "x25519-unsafe-classical-forbidden-in-pq"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("KeyExchangeID(0x%02x).String() = %q, want %q", uint8(c.s), got, c.want)
		}
	}
}

func TestKeyExchangeID_IsPostQuantum(t *testing.T) {
	for _, s := range []KeyExchangeID{KeyExchangeMLKEM768, KeyExchangeMLKEM1024} {
		if !s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=true", s.String())
		}
	}
	for _, s := range []KeyExchangeID{KeyExchangeInvalid, KeyExchangeX25519Unsafe} {
		if s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=false", s.String())
		}
	}
}

func TestKeyExchangeID_IsForbiddenInPQMode(t *testing.T) {
	if !KeyExchangeX25519Unsafe.IsForbiddenInPQMode() {
		t.Errorf("KeyExchangeX25519Unsafe must be forbidden in PQ mode")
	}
	for _, s := range []KeyExchangeID{
		KeyExchangeInvalid,
		KeyExchangeMLKEM768,
		KeyExchangeMLKEM1024,
	} {
		if s.IsForbiddenInPQMode() {
			t.Errorf("%q must NOT be forbidden in PQ mode", s.String())
		}
	}
}

// TestRecoverySchemeID_StableIntegers pins the recovery scheme bytes.
// SLH-DSA occupies 0x05..0x07 (FIPS 205 Cat 1/3/5), ML-DSA-87 shares
// 0x43 with the FIPS 204 enum, RecoverySchemeNone is 0xFF.
func TestRecoverySchemeID_StableIntegers(t *testing.T) {
	cases := []struct {
		s    RecoverySchemeID
		want uint8
	}{
		{RecoverySchemeInvalid, 0x00},
		{RecoverySchemeSLHDSA128, 0x05},
		{RecoverySchemeSLHDSA192, 0x06},
		{RecoverySchemeSLHDSA256, 0x07},
		{RecoverySchemeMLDSA87, 0x43},
		{RecoverySchemeNone, 0xFF},
	}
	for _, c := range cases {
		if got := uint8(c.s); got != c.want {
			t.Errorf("RecoverySchemeID %q = 0x%02x, want 0x%02x", c.s.String(), got, c.want)
		}
	}
}

func TestRecoverySchemeID_String(t *testing.T) {
	cases := []struct {
		s    RecoverySchemeID
		want string
	}{
		{RecoverySchemeInvalid, "invalid"},
		{RecoverySchemeSLHDSA128, "slh-dsa-128"},
		{RecoverySchemeSLHDSA192, "slh-dsa-192"},
		{RecoverySchemeSLHDSA256, "slh-dsa-256"},
		{RecoverySchemeMLDSA87, "ml-dsa-87"},
		{RecoverySchemeNone, "none"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("RecoverySchemeID(0x%02x).String() = %q, want %q", uint8(c.s), got, c.want)
		}
	}
}

// TestRecoverySchemeID_IsPostQuantum — every defined non-Invalid
// recovery scheme is PQ-acceptable. None is a policy-class sentinel
// (acceptable only when high-value is Cat 5) and counts as PQ for the
// predicate; the cross-axis validation gate refuses it when the
// high-value scheme doesn't permit it.
func TestRecoverySchemeID_IsPostQuantum(t *testing.T) {
	for _, s := range []RecoverySchemeID{
		RecoverySchemeSLHDSA128,
		RecoverySchemeSLHDSA192,
		RecoverySchemeSLHDSA256,
		RecoverySchemeMLDSA87,
		RecoverySchemeNone,
	} {
		if !s.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=true", s.String())
		}
	}
	if RecoverySchemeInvalid.IsPostQuantum() {
		t.Errorf("RecoverySchemeInvalid must be IsPostQuantum=false")
	}
}

func TestRecoverySchemeID_IsForbiddenInPQMode(t *testing.T) {
	// No 0x90+ classical recovery scheme is currently defined; the
	// predicate is trivially false for every entry. Lock the behaviour
	// so future additions surface here.
	for _, s := range []RecoverySchemeID{
		RecoverySchemeInvalid,
		RecoverySchemeSLHDSA128,
		RecoverySchemeSLHDSA192,
		RecoverySchemeSLHDSA256,
		RecoverySchemeMLDSA87,
		RecoverySchemeNone,
	} {
		if s.IsForbiddenInPQMode() {
			t.Errorf("%q must NOT be forbidden in PQ mode", s.String())
		}
	}
}

// TestRecoverySchemeID_IsStateless — only SLH-DSA schemes are stateless
// hash-based; ML-DSA-87 and None are not.
func TestRecoverySchemeID_IsStateless(t *testing.T) {
	for _, s := range []RecoverySchemeID{
		RecoverySchemeSLHDSA128,
		RecoverySchemeSLHDSA192,
		RecoverySchemeSLHDSA256,
	} {
		if !s.IsStateless() {
			t.Errorf("%q must be IsStateless=true", s.String())
		}
	}
	for _, s := range []RecoverySchemeID{
		RecoverySchemeInvalid,
		RecoverySchemeMLDSA87,
		RecoverySchemeNone,
	} {
		if s.IsStateless() {
			t.Errorf("%q must be IsStateless=false", s.String())
		}
	}
}

// =============================================================================
// ChainSecurityProfile.Validate — E2E PQ axis enforcement
// =============================================================================

// TestChainSecurityProfile_Validate_LuxStrictPQ_RejectsECDSAWallets proves
// the strict-PQ profile refuses a wallet scheme set to the 0x90 classical
// marker. Two-stage check: setting WalletSchemeECDSAUnsafe directly is
// refused by validatePolicy's IsForbiddenInPQMode gate; setting
// ForbidECDSAWallets=false (and keeping the lattice wallet scheme) is
// refused by the operator-policy check.
func TestChainSecurityProfile_Validate_LuxStrictPQ_RejectsECDSAWallets(t *testing.T) {
	// Case 1: classical scheme byte on the wire.
	p := LuxStrictPQ()
	p.WalletSchemeID = WalletSchemeECDSAUnsafe
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with WalletSchemeECDSAUnsafe returned %v; want ErrProfileFieldInvalid", err)
	}

	// Case 2: Forbid bit cleared while the scheme stays lattice. The
	// operator-policy gate must catch the cleared bit on a strict-PQ
	// profile even though the scheme itself is lattice.
	q := LuxStrictPQ()
	q.ForbidECDSAWallets = false
	if err := q.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with ForbidECDSAWallets=false returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_LuxStrictPQ_RejectsX25519KEM proves the
// strict-PQ profile refuses both classical KEM scheme bytes and a cleared
// ForbidClassicalKEM bit.
func TestChainSecurityProfile_Validate_LuxStrictPQ_RejectsX25519KEM(t *testing.T) {
	p := LuxStrictPQ()
	p.KeyExchangeID = KeyExchangeX25519Unsafe
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with KeyExchangeX25519Unsafe returned %v; want ErrProfileFieldInvalid", err)
	}

	q := LuxStrictPQ()
	q.HighValueKEM = KeyExchangeX25519Unsafe
	if err := q.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with HighValueKEM=KeyExchangeX25519Unsafe returned %v; want ErrProfileFieldInvalid", err)
	}

	r := LuxStrictPQ()
	r.ForbidClassicalKEM = false
	if err := r.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with ForbidClassicalKEM=false returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_LuxStrictPQ_RejectsECDSATxScheme covers
// the tx axis explicitly so an audit reader sees the gate is per-axis.
func TestChainSecurityProfile_Validate_LuxStrictPQ_RejectsECDSATxScheme(t *testing.T) {
	p := LuxStrictPQ()
	p.TxSchemeID = TxSchemeECDSAUnsafe
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with TxSchemeECDSAUnsafe returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_LuxStrictPQ_RejectsBLSContractAuth proves
// the strict-PQ profile refuses classical BLS aggregates at the contract
// authorisation layer.
func TestChainSecurityProfile_Validate_LuxStrictPQ_RejectsBLSContractAuth(t *testing.T) {
	p := LuxStrictPQ()
	p.ContractAuthID = ContractAuthBLSUnsafe
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with ContractAuthBLSUnsafe returned %v; want ErrProfileFieldInvalid", err)
	}

	q := LuxStrictPQ()
	q.ForbidBLSContractAuth = false
	if err := q.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with ForbidBLSContractAuth=false returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_LuxStrictPQ_RequiresTypedTxAuth proves
// strict-PQ demands the typed-auth byte on the wire.
func TestChainSecurityProfile_Validate_LuxStrictPQ_RequiresTypedTxAuth(t *testing.T) {
	p := LuxStrictPQ()
	p.RequireTypedTxAuth = false
	if err := p.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with RequireTypedTxAuth=false returned %v; want ErrProfileFieldInvalid", err)
	}
}

// TestChainSecurityProfile_Validate_RecoverySchemeNone_RequiresMLDSA87 proves
// the cross-axis rule: RecoverySchemeNone is only acceptable when the
// chain's high-value scheme is Pulsar-M-87 or raw ML-DSA-87 (both Cat 5).
//
// In practice the existing HighValueSchemeID invariant restricts the
// field to Pulsar-M-65 / Pulsar-M-87 (raw ML-DSA is identity, not
// high-value); the ML-DSA-87 branch of the rule is therefore defensive
// — it covers a future profile-class that opts out of the Pulsar-M
// constraint. This test exercises only the reachable Pulsar-M-87 path
// and the Pulsar-M-65 negative case.
func TestChainSecurityProfile_Validate_RecoverySchemeNone_RequiresMLDSA87(t *testing.T) {
	// Strict-PQ is at HighValue=Pulsar-M-87 → RecoverySchemeNone is OK.
	p := LuxStrictPQ()
	p.RecoverySchemeID = RecoverySchemeNone
	if err := p.Validate(); err != nil {
		t.Errorf("Validate() with RecoverySchemeNone + HighValue=Pulsar-M-87 returned %v; want nil", err)
	}

	// Drop HighValue to Pulsar-M-65 → must be rejected. Pulsar-M-65 is
	// NIST PQ Cat 3 (acceptable for high-value finality) but the
	// recovery-fallback rule demands Cat 5, so the cross-axis gate
	// trips.
	q := LuxStrictPQ()
	q.HighValueSchemeID = SigSchemePulsarM65
	q.RecoverySchemeID = RecoverySchemeNone
	if err := q.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("Validate() with RecoverySchemeNone + HighValue=Pulsar-M-65 returned %v; want ErrProfileFieldInvalid", err)
	}

	// The fork profile demonstrates RecoverySchemeNone in production —
	// HighValueSchemeID=Pulsar-M-87 carries the strong fallback that
	// would otherwise require a stateless backstop.
	f := ForkClassicalCompatUnsafeProfile
	if f.RecoverySchemeID != RecoverySchemeNone {
		t.Errorf("ForkClassicalCompatUnsafeProfile.RecoverySchemeID = %s; want none", f.RecoverySchemeID.String())
	}
	if err := f.Validate(); err != nil {
		t.Errorf("ForkClassicalCompatUnsafeProfile.Validate() = %v; want nil", err)
	}
}

// TestChainSecurityProfile_Validate_RejectsInvalidEnumZeros proves the
// structural gate rejects a zero-init value on every new enum.
func TestChainSecurityProfile_Validate_RejectsInvalidEnumZeros(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *ChainSecurityProfile)
	}{
		{"WalletSchemeID", func(p *ChainSecurityProfile) { p.WalletSchemeID = WalletSchemeInvalid }},
		{"TxSchemeID", func(p *ChainSecurityProfile) { p.TxSchemeID = TxSchemeInvalid }},
		{"ContractAuthID", func(p *ChainSecurityProfile) { p.ContractAuthID = ContractAuthInvalid }},
		{"KeyExchangeID", func(p *ChainSecurityProfile) { p.KeyExchangeID = KeyExchangeInvalid }},
		{"HighValueKEM", func(p *ChainSecurityProfile) { p.HighValueKEM = KeyExchangeInvalid }},
		{"RecoverySchemeID", func(p *ChainSecurityProfile) { p.RecoverySchemeID = RecoverySchemeInvalid }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := LuxStrictPQ()
			c.mutate(p)
			if err := p.Validate(); !errors.Is(err, ErrProfileFieldUnset) {
				t.Errorf("Validate() with %s=Invalid returned %v; want ErrProfileFieldUnset",
					c.name, err)
			}
		})
	}
}

// TestChainSecurityProfile_Validate_RejectsUnknownEnumByte proves
// out-of-range bytes (no known enum entry) are refused. A real wire
// could carry an attacker-supplied byte the local toolkit doesn't know.
func TestChainSecurityProfile_Validate_RejectsUnknownEnumByte(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(p *ChainSecurityProfile)
	}{
		{"WalletSchemeID", func(p *ChainSecurityProfile) { p.WalletSchemeID = WalletSchemeID(0x77) }},
		{"TxSchemeID", func(p *ChainSecurityProfile) { p.TxSchemeID = TxSchemeID(0x77) }},
		{"ContractAuthID", func(p *ChainSecurityProfile) { p.ContractAuthID = ContractAuthID(0x77) }},
		{"KeyExchangeID", func(p *ChainSecurityProfile) { p.KeyExchangeID = KeyExchangeID(0x77) }},
		{"HighValueKEM", func(p *ChainSecurityProfile) { p.HighValueKEM = KeyExchangeID(0x77) }},
		{"RecoverySchemeID", func(p *ChainSecurityProfile) { p.RecoverySchemeID = RecoverySchemeID(0x77) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := LuxStrictPQ()
			c.mutate(p)
			if err := p.Validate(); !errors.Is(err, ErrProfileFieldUnknown) {
				t.Errorf("Validate() with %s=0x77 returned %v; want ErrProfileFieldUnknown",
					c.name, err)
			}
		})
	}
}

// TestChainSecurityProfile_LuxPermissive_AllowsExperiments proves the
// permissive profile accepts the same lattice defaults as strict-PQ but
// keeps the new Forbid* bits cleared and RequireTypedTxAuth false.
func TestChainSecurityProfile_LuxPermissive_AllowsExperiments(t *testing.T) {
	p := LuxPermissive()
	if err := p.Validate(); err != nil {
		t.Fatalf("LuxPermissive().Validate() = %v; want nil", err)
	}
	if p.ForbidECDSAWallets {
		t.Errorf("LuxPermissive() must NOT set ForbidECDSAWallets")
	}
	if p.ForbidECDSAContractAuth {
		t.Errorf("LuxPermissive() must NOT set ForbidECDSAContractAuth")
	}
	if p.ForbidBLSContractAuth {
		t.Errorf("LuxPermissive() must NOT set ForbidBLSContractAuth")
	}
	if p.ForbidClassicalKEM {
		t.Errorf("LuxPermissive() must NOT set ForbidClassicalKEM")
	}
	if p.RequireTypedTxAuth {
		t.Errorf("LuxPermissive() must NOT require typed tx auth")
	}
	if p.WalletSchemeID != WalletSchemeMLDSA65 {
		t.Errorf("LuxPermissive().WalletSchemeID = %s; want ml-dsa-65", p.WalletSchemeID.String())
	}
}

// TestChainSecurityProfile_ForkClassicalCompatUnsafe_AcceptsClassical proves
// the fork profile passes Validate even with every new axis pinned to
// the 0x90 classical marker. Only LuxStrictPQ and LuxFIPS enforce the
// strict-PQ refusals; the fork is allowed to opt out.
func TestChainSecurityProfile_ForkClassicalCompatUnsafe_AcceptsClassical(t *testing.T) {
	if err := ForkClassicalCompatUnsafeProfile.Validate(); err != nil {
		t.Fatalf("ForkClassicalCompatUnsafeProfile.Validate() = %v; want nil", err)
	}
	if ForkClassicalCompatUnsafeProfile.WalletSchemeID != WalletSchemeECDSAUnsafe {
		t.Errorf("fork.WalletSchemeID = %s; want ECDSA-unsafe", ForkClassicalCompatUnsafeProfile.WalletSchemeID.String())
	}
	if ForkClassicalCompatUnsafeProfile.KeyExchangeID != KeyExchangeX25519Unsafe {
		t.Errorf("fork.KeyExchangeID = %s; want X25519-unsafe", ForkClassicalCompatUnsafeProfile.KeyExchangeID.String())
	}
	if ForkClassicalCompatUnsafeProfile.RecoverySchemeID != RecoverySchemeNone {
		t.Errorf("fork.RecoverySchemeID = %s; want none", ForkClassicalCompatUnsafeProfile.RecoverySchemeID.String())
	}
}

// =============================================================================
// ComputeHash — every new field must be bound into the TupleHash transcript
// =============================================================================

// TestChainSecurityProfile_ComputeHash_BindsNewFields walks every new
// E2E PQ field on the profile and proves mutating it changes the
// ComputeHash output. The test is the last line of defence against a
// refactor accidentally dropping a field from the canonical encoding.
//
// Mutations are chosen to keep Validate happy (so ComputeHash's internal
// validateStructural pass succeeds) while still being a distinct value
// from the canonical profile.
func TestChainSecurityProfile_ComputeHash_BindsNewFields(t *testing.T) {
	baseHash, err := LuxStrictPQ().ComputeHash()
	if err != nil {
		t.Fatalf("base: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(p *ChainSecurityProfile)
	}{
		{"WalletSchemeID", func(p *ChainSecurityProfile) { p.WalletSchemeID = WalletSchemeMLDSA87 }},
		{"TxSchemeID", func(p *ChainSecurityProfile) { p.TxSchemeID = TxSchemeMLDSA87 }},
		{"ContractAuthID", func(p *ChainSecurityProfile) { p.ContractAuthID = ContractAuthMLDSA87 }},
		{"KeyExchangeID", func(p *ChainSecurityProfile) { p.KeyExchangeID = KeyExchangeMLKEM1024 }},
		{"HighValueKEM", func(p *ChainSecurityProfile) { p.HighValueKEM = KeyExchangeMLKEM768 }},
		{"RecoverySchemeID", func(p *ChainSecurityProfile) { p.RecoverySchemeID = RecoverySchemeSLHDSA256 }},
		// Forbid bits and RequireTypedTxAuth — these mutations make
		// Validate fail under strict-PQ but ComputeHash only calls
		// validateStructural, which does not enforce the strict-PQ
		// operator-policy. Each flip MUST still change the transcript.
		{"ForbidECDSAWallets", func(p *ChainSecurityProfile) { p.ForbidECDSAWallets = false }},
		{"ForbidECDSAContractAuth", func(p *ChainSecurityProfile) { p.ForbidECDSAContractAuth = false }},
		{"ForbidBLSContractAuth", func(p *ChainSecurityProfile) { p.ForbidBLSContractAuth = false }},
		{"ForbidClassicalKEM", func(p *ChainSecurityProfile) { p.ForbidClassicalKEM = false }},
		{"RequireTypedTxAuth", func(p *ChainSecurityProfile) { p.RequireTypedTxAuth = false }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			mutated := LuxStrictPQ()
			c.mutate(mutated)
			got, err := mutated.ComputeHash()
			if err != nil {
				t.Fatalf("ComputeHash(mutated %s): %v", c.name, err)
			}
			if got == baseHash {
				t.Errorf("mutating %s did not change ComputeHash output", c.name)
			}
		})
	}
}

// =============================================================================
// Golden vector — the re-pinned canonical LuxStrictPQ profile hash.
//
// The hash binds every field of ChainSecurityProfile under the
// TupleHash256 / cSHAKE256 customisation "LUX-CHAIN-SECURITY-PROFILE-V1".
// Any change to the canonical profile, the field set, or the encoding
// produces a new digest — and re-pinning this constant is the explicit
// gate that says "I intentionally rolled the canonical profile."
//
// Update path: when the canonical profile or encoding changes, re-run
//
//	cd ~/work/lux/consensus && env GOWORK=off go test -count=1 \
//	    -run TestChainSecurityProfile_GoldenVector_LuxStrictPQ -v ./config/
//
// Copy the digest from the failure message into the constant below in
// the SAME COMMIT that landed the schema change.
// =============================================================================

// luxStrictPQGoldenHashHex is the pinned hex of LuxStrictPQ.ComputeHash().
// Re-pinned after extending ChainSecurityProfile with the E2E PQ axes
// (WalletSchemeID, TxSchemeID, ContractAuthID, KeyExchangeID,
// HighValueKEM, RecoverySchemeID + the four Forbid* bits + RequireTypedTxAuth).
const luxStrictPQGoldenHashHex = "93efd103aaf4b85eb2ea37b451da5a8f6e36af0745b80837c1942ffd4be9eead59c22fc191298b381ee0a4b0323bfcd0"

// TestChainSecurityProfile_GoldenVector_LuxStrictPQ pins the canonical
// LuxStrictPQ profile hash. A failing test means either the canonical
// profile changed, the encoding changed, or both — every such change
// MUST be intentional and re-pinned here in the same commit.
func TestChainSecurityProfile_GoldenVector_LuxStrictPQ(t *testing.T) {
	got, err := LuxStrictPQ().ComputeHash()
	if err != nil {
		t.Fatalf("ComputeHash: %v", err)
	}
	gotHex := hex.EncodeToString(got[:])
	if gotHex != luxStrictPQGoldenHashHex {
		t.Errorf("LuxStrictPQ.ComputeHash() golden vector drift.\n got: %s\nwant: %s\nIf the schema change is intentional, re-pin luxStrictPQGoldenHashHex to the 'got' value above.",
			gotHex, luxStrictPQGoldenHashHex)
	}
}
