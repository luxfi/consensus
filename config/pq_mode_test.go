// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package config

import "testing"

// TestPQMode_String pins the canonical wire name for every mode.
// Anyone who renames a constant without updating the env-var docs / HIP
// breaks this test.
func TestPQMode_String(t *testing.T) {
	cases := []struct {
		mode PQMode
		name string
	}{
		{PQModeBLS, "bls"},
		{PQModeNasua, "nasua"},
		{PQModePulsar, "pulsar"},
		{PQModeQuasar, "quasar"},
		{PQModeMLDSA, "mldsa"},
	}
	for _, c := range cases {
		if got := c.mode.String(); got != c.name {
			t.Errorf("PQMode(%d).String() = %q, want %q", c.mode, got, c.name)
		}
	}
}

// TestParsePQMode_Canonical accepts every canonical lower-case name.
func TestParsePQMode_Canonical(t *testing.T) {
	for _, m := range []PQMode{
		PQModeBLS, PQModeNasua, PQModePulsar, PQModeQuasar, PQModeMLDSA,
	} {
		got, err := ParsePQMode(m.String())
		if err != nil {
			t.Errorf("ParsePQMode(%q) errored: %v", m.String(), err)
			continue
		}
		if got != m {
			t.Errorf("ParsePQMode(%q) = %v, want %v", m.String(), got, m)
		}
	}
}

// TestParsePQMode_Aliases ensures legacy env strings still parse to the
// right mode. Drop entries here only when the alias has been removed
// from a major release.
func TestParsePQMode_Aliases(t *testing.T) {
	cases := []struct {
		alias string
		want  PQMode
	}{
		// BLS
		{"", PQModeBLS},
		{"BLS", PQModeBLS},
		{"bls-only", PQModeBLS},
		{"classical", PQModeBLS},

		// Corona (academic, BLAKE3, trusted dealer)
		{"rt", PQModeNasua},
		{"academic", PQModeNasua},
		{"bls-rt", PQModeNasua},
		{"sha256-rt", PQModeNasua},

		// Pulsar (production fork, SHA-3, Pedersen DKG)
		{"sha3-rt", PQModePulsar},
		{"production-rt", PQModePulsar},
		{"bls-pulsar", PQModePulsar},

		// Quasar — component-named aliases only (no "triple"; tells callers
		// nothing about what's in the cert).
		{"rollup", PQModeQuasar},
		{"groth16", PQModeQuasar},
		{"bls-z", PQModeQuasar},
		{"bls-zk", PQModeQuasar},
		{"z-chain", PQModeQuasar},
		{"pulsar-z", PQModeQuasar},

		// MLDSA fallback
		{"audit", PQModeMLDSA},
		{"bls-mldsa", PQModeMLDSA},
	}
	for _, c := range cases {
		got, err := ParsePQMode(c.alias)
		if err != nil {
			t.Errorf("ParsePQMode(%q) errored: %v", c.alias, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParsePQMode(%q) = %v, want %v", c.alias, got, c.want)
		}
	}
}

func TestParsePQMode_Unknown(t *testing.T) {
	if _, err := ParsePQMode("nonsense"); err == nil {
		t.Errorf("ParsePQMode(\"nonsense\") accepted; want error")
	}
}

// TestParsePQMode_NoCountingWords pins the policy that mode names must
// describe what's in the cert. "triple" / "triple-quantum" / "double" /
// "stack" tell a caller nothing about which witnesses are signed and are
// rejected. Drop a name from the rejected list only when there is a
// concrete deployment requiring it AND a HIP amendment justifying it.
func TestParsePQMode_NoCountingWords(t *testing.T) {
	rejected := []string{"triple", "triple-quantum", "double", "stack", "all", "max"}
	for _, s := range rejected {
		if _, err := ParsePQMode(s); err == nil {
			t.Errorf("ParsePQMode(%q) accepted; counting words must be rejected per HIP-0077", s)
		}
	}
}

// TestPQMode_PolicyID pins the wire policy ID per mode. Anyone changing
// the wire layer must update this table at the same time.
func TestPQMode_PolicyID(t *testing.T) {
	cases := []struct {
		mode PQMode
		want uint16
	}{
		{PQModeBLS, 1},      // PolicyQuorum
		{PQModeNasua, 5}, // PolicyPQ
		{PQModePulsar, 5},   // PolicyPQ (same wire shape, SHA-3 negotiated out-of-band)
		{PQModeQuasar, 4},   // PolicyQuantum
		{PQModeMLDSA, 6},    // PolicyPZ
	}
	for _, c := range cases {
		if got := c.mode.PolicyID(); got != c.want {
			t.Errorf("PQMode(%d).PolicyID() = %d, want %d", c.mode, got, c.want)
		}
	}
}

// TestPQMode_HashProfile asserts the canonical hash family per mode.
// Strings are the human-readable form of HashSuiteID and must stay in
// lockstep with TestPQMode_HashSuiteID below.
//
// NIST-aligned mapping (HIP-0077 §"Lux consensus PQ modes"):
//
//	bls       -> "none"            no hash-family commitment
//	corona  -> "blake3-legacy"   academic kernel, non-normative
//	pulsar    -> "sha3-nist"       FIPS 202 + SP 800-185 (cSHAKE256 family)
//	quasar    -> "sha3-nist"       same kernel as Pulsar
//	mldsa     -> "sha3-nist"       ML-DSA-65 uses SHAKE256 (FIPS 202)
func TestPQMode_HashProfile(t *testing.T) {
	cases := []struct {
		mode PQMode
		want string
	}{
		{PQModeBLS, "none"},
		{PQModeNasua, "blake3-legacy"},
		{PQModePulsar, "sha3-nist"},
		{PQModeQuasar, "sha3-nist"},
		{PQModeMLDSA, "sha3-nist"},
	}
	for _, c := range cases {
		if got := c.mode.HashProfile(); got != c.want {
			t.Errorf("PQMode(%d).HashProfile() = %q, want %q", c.mode, got, c.want)
		}
	}
}

// TestPQMode_HashSuiteID pins the wire HashSuiteID byte for every mode.
// Closes HIP-0077 red-review F1 (silent finality forks between hash-profile
// islands sharing the same PolicyID).
//
// Numbering per the NIST submission prescription:
//
//	0x00  HashSuiteNone           BLS-only, no hash commitment
//	0x01  HashSuiteSHA3NIST       FIPS 202 family (normative)
//	0x02  HashSuiteBLAKE3Legacy   academic / federation, non-normative
//
// Renumbering an existing entry breaks every cert ever signed under
// that mode; ADDING a new entry only requires a new free integer.
func TestPQMode_HashSuiteID(t *testing.T) {
	cases := []struct {
		mode PQMode
		want HashSuiteID
	}{
		{PQModeBLS, HashSuiteNone},                  // 0x00
		{PQModeNasua, HashSuiteBLAKE3Legacy},     // 0x02
		{PQModePulsar, HashSuiteSHA3NIST},           // 0x01
		{PQModeQuasar, HashSuiteSHA3NIST},           // 0x01
		{PQModeMLDSA, HashSuiteSHA3NIST},            // 0x01 (SHAKE256 ∈ FIPS 202)
	}
	for _, c := range cases {
		if got := c.mode.HashSuiteID(); got != c.want {
			t.Errorf("PQMode(%d).HashSuiteID() = %d (%q), want %d (%q)",
				c.mode, got, got.String(), c.want, c.want.String())
		}
	}
}

// TestHashSuiteID_StableIntegers locks the integer values themselves.
// The HashSuiteID byte is on the wire of every Quasar finality cert;
// renumbering breaks forward and backward compatibility simultaneously.
// New suites MUST claim the next free integer.
func TestHashSuiteID_StableIntegers(t *testing.T) {
	cases := []struct {
		suite HashSuiteID
		want  uint8
	}{
		{HashSuiteNone, 0x00},
		{HashSuiteSHA3NIST, 0x01},
		{HashSuiteBLAKE3Legacy, 0x02},
	}
	for _, c := range cases {
		if got := uint8(c.suite); got != c.want {
			t.Errorf("HashSuiteID %q = 0x%02x, want 0x%02x", c.suite.String(), got, c.want)
		}
	}
}

// TestHashSuiteID_String pins the human-readable name for every defined
// suite. The names appear in logs and error messages.
func TestHashSuiteID_String(t *testing.T) {
	cases := []struct {
		suite HashSuiteID
		want  string
	}{
		{HashSuiteNone, "none"},
		{HashSuiteSHA3NIST, "sha3-nist"},
		{HashSuiteBLAKE3Legacy, "blake3-legacy"},
	}
	for _, c := range cases {
		if got := c.suite.String(); got != c.want {
			t.Errorf("HashSuiteID(0x%02x).String() = %q, want %q", uint8(c.suite), got, c.want)
		}
	}
}

// TestHashSuiteID_IsNormative — production Lux meshes accept only
// FIPS-aligned suites. Non-normative suites may emit certs but not on
// any NIST-track production network.
func TestHashSuiteID_IsNormative(t *testing.T) {
	if !HashSuiteSHA3NIST.IsNormative() {
		t.Errorf("HashSuiteSHA3NIST must be normative")
	}
	if HashSuiteBLAKE3Legacy.IsNormative() {
		t.Errorf("HashSuiteBLAKE3Legacy must be non-normative")
	}
	if HashSuiteNone.IsNormative() {
		t.Errorf("HashSuiteNone (bls-only) must be non-normative")
	}
}

// TestPQMode_SigSchemeID pins the default signature scheme the consensus
// layer will request for each mode. Mainnet declares PQModeMLDSA →
// raw ML-DSA-65 (audit-grade); operators that want Pulsar-M-65 threshold
// must opt in explicitly via the cert assembler.
func TestPQMode_SigSchemeID(t *testing.T) {
	cases := []struct {
		mode PQMode
		want SigSchemeID
	}{
		{PQModeBLS, SigSchemeNone},
		{PQModeNasua, SigSchemeNasua},
		{PQModePulsar, SigSchemePulsarR},
		{PQModeQuasar, SigSchemePulsarR},
		{PQModeMLDSA, SigSchemeMLDSA65}, // raw ML-DSA-65 default; ops opt-in to Pulsar-M-65
	}
	for _, c := range cases {
		if got := c.mode.SigSchemeID(); got != c.want {
			t.Errorf("PQMode(%d).SigSchemeID() = 0x%02x (%q), want 0x%02x (%q)",
				c.mode, uint8(got), got.String(), uint8(c.want), c.want.String())
		}
	}
}

// TestSigSchemeID_StableIntegers locks the byte values per HIP-0077
// numbering blocks. Renumbering breaks every cert ever emitted; new
// schemes claim the next free integer in their block:
//
//	0x10 BLS / 0x20 Corona / 0x30 Pulsar.R / 0x40 raw ML-DSA / 0x50 Pulsar-M
func TestSigSchemeID_StableIntegers(t *testing.T) {
	cases := []struct {
		scheme SigSchemeID
		want   uint8
	}{
		{SigSchemeNone, 0x00},
		{SigSchemeBLS12381, 0x10},
		{SigSchemeNasua, 0x20},
		{SigSchemePulsarR, 0x30},
		{SigSchemeMLDSA44, 0x41},
		{SigSchemeMLDSA65, 0x42},
		{SigSchemeMLDSA87, 0x43},
		{SigSchemePulsarM44, 0x51},
		{SigSchemePulsarM65, 0x52}, // production default
		{SigSchemePulsarM87, 0x53},
	}
	for _, c := range cases {
		if got := uint8(c.scheme); got != c.want {
			t.Errorf("SigSchemeID %q = 0x%02x, want 0x%02x", c.scheme.String(), got, c.want)
		}
	}
}

// TestSigSchemeID_String pins the canonical wire name.
func TestSigSchemeID_String(t *testing.T) {
	cases := []struct {
		scheme SigSchemeID
		want   string
	}{
		{SigSchemeNone, "none"},
		{SigSchemeBLS12381, "bls12-381"},
		{SigSchemeNasua, "nasua"},
		{SigSchemePulsarR, "pulsar-r"},
		{SigSchemeMLDSA44, "ml-dsa-44"},
		{SigSchemeMLDSA65, "ml-dsa-65"},
		{SigSchemeMLDSA87, "ml-dsa-87"},
		{SigSchemePulsarM44, "pulsar-m-44"},
		{SigSchemePulsarM65, "pulsar-m-65"},
		{SigSchemePulsarM87, "pulsar-m-87"},
	}
	for _, c := range cases {
		if got := c.scheme.String(); got != c.want {
			t.Errorf("SigSchemeID(0x%02x).String() = %q, want %q", uint8(c.scheme), got, c.want)
		}
	}
}

// TestSigSchemeID_IsPulsarM separates the Pulsar-M threshold family from
// everything else. Pulsar-M outputs verify under unmodified FIPS 204
// ML-DSA.Verify but are produced by a threshold protocol.
func TestSigSchemeID_IsPulsarM(t *testing.T) {
	pulsarM := []SigSchemeID{SigSchemePulsarM44, SigSchemePulsarM65, SigSchemePulsarM87}
	for _, s := range pulsarM {
		if !s.IsPulsarM() {
			t.Errorf("%q must be Pulsar-M", s.String())
		}
	}
	notPulsarM := []SigSchemeID{
		SigSchemeNone, SigSchemeBLS12381, SigSchemeNasua,
		SigSchemePulsarR,
		SigSchemeMLDSA44, SigSchemeMLDSA65, SigSchemeMLDSA87,
	}
	for _, s := range notPulsarM {
		if s.IsPulsarM() {
			t.Errorf("%q must not be Pulsar-M", s.String())
		}
	}
}

// TestSigSchemeID_IsRawMLDSA covers the 0x40 block — single-party FIPS 204
// signatures (no threshold protocol).
func TestSigSchemeID_IsRawMLDSA(t *testing.T) {
	rawMLDSA := []SigSchemeID{SigSchemeMLDSA44, SigSchemeMLDSA65, SigSchemeMLDSA87}
	for _, s := range rawMLDSA {
		if !s.IsRawMLDSA() {
			t.Errorf("%q must be raw ML-DSA", s.String())
		}
	}
	notRaw := []SigSchemeID{
		SigSchemeNone, SigSchemeBLS12381, SigSchemeNasua, SigSchemePulsarR,
		SigSchemePulsarM44, SigSchemePulsarM65, SigSchemePulsarM87,
	}
	for _, s := range notRaw {
		if s.IsRawMLDSA() {
			t.Errorf("%q must not be raw ML-DSA", s.String())
		}
	}
}

// TestSigSchemeID_VerifiesUnderFIPS204 — both raw ML-DSA (0x40) and
// Pulsar-M (0x50) verify under FIPS 204 ML-DSA.Verify.
func TestSigSchemeID_VerifiesUnderFIPS204(t *testing.T) {
	yes := []SigSchemeID{
		SigSchemeMLDSA44, SigSchemeMLDSA65, SigSchemeMLDSA87,
		SigSchemePulsarM44, SigSchemePulsarM65, SigSchemePulsarM87,
	}
	for _, s := range yes {
		if !s.VerifiesUnderFIPS204() {
			t.Errorf("%q must verify under FIPS 204", s.String())
		}
	}
	no := []SigSchemeID{
		SigSchemeNone, SigSchemeBLS12381, SigSchemeNasua, SigSchemePulsarR,
	}
	for _, s := range no {
		if s.VerifiesUnderFIPS204() {
			t.Errorf("%q must not verify under FIPS 204", s.String())
		}
	}
}

// TestProofPolicyID_StableIntegers locks the wire bytes for the ZK proof
// policy enum (security class). Z-Chain v1 ships with STARK_FRI_SHA3_PQ
// (0x10); classical IDs exist on the wire so audit pipelines flag
// misconfigurations.
//
// Renumbering an entry breaks every cert ever signed under it. Adding a
// new entry only requires a new free integer in the appropriate block.
func TestProofPolicyID_StableIntegers(t *testing.T) {
	cases := []struct {
		ps   ProofPolicyID
		want uint8
	}{
		{ProofPolicyNone, 0x00},
		{ProofPolicySTARKFRISHA3PQ, 0x10},
		{ProofPolicySTARKFRIKeccak, 0x11},
		{ProofPolicyGroth16BN254Forbid, 0x80},
		{ProofPolicyPLONKKZGForbid, 0x81},
		{ProofPolicyHaloECForbid, 0x82},
		{ProofPolicyIPAECForbid, 0x83},
	}
	for _, c := range cases {
		if got := uint8(c.ps); got != c.want {
			t.Errorf("ProofPolicyID %q = 0x%02x, want 0x%02x", c.ps.String(), got, c.want)
		}
	}
}

// TestProofPolicyID_String pins canonical wire names.
func TestProofPolicyID_String(t *testing.T) {
	cases := []struct {
		ps   ProofPolicyID
		want string
	}{
		{ProofPolicyNone, "none"},
		{ProofPolicySTARKFRISHA3PQ, "stark-fri-sha3-pq"},
		{ProofPolicySTARKFRIKeccak, "stark-fri-keccak-pq"},
		{ProofPolicyGroth16BN254Forbid, "groth16-bn254-classical-forbidden-in-pq"},
		{ProofPolicyPLONKKZGForbid, "plonk-kzg-classical-forbidden-in-pq"},
		{ProofPolicyHaloECForbid, "halo-ec-classical-forbidden-in-pq"},
		{ProofPolicyIPAECForbid, "ipa-ec-classical-forbidden-in-pq"},
	}
	for _, c := range cases {
		if got := c.ps.String(); got != c.want {
			t.Errorf("ProofPolicyID(0x%02x).String() = %q, want %q", uint8(c.ps), got, c.want)
		}
	}
}

// TestProofPolicyID_IsPostQuantum gates strict-PQ verifier acceptance.
// Lux primary-network producers MUST emit only IsPostQuantum=true; any
// classical wire byte makes it onto the wire only as a forbidden marker.
func TestProofPolicyID_IsPostQuantum(t *testing.T) {
	pq := []ProofPolicyID{
		ProofPolicySTARKFRISHA3PQ,
		ProofPolicySTARKFRIKeccak,
	}
	for _, p := range pq {
		if !p.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=true", p.String())
		}
	}
	notPQ := []ProofPolicyID{
		ProofPolicyNone, // None is not PQ-positive; it's "no commitment".
		ProofPolicyGroth16BN254Forbid,
		ProofPolicyPLONKKZGForbid,
		ProofPolicyHaloECForbid,
		ProofPolicyIPAECForbid,
	}
	for _, p := range notPQ {
		if p.IsPostQuantum() {
			t.Errorf("%q must be IsPostQuantum=false", p.String())
		}
	}
}

// TestProofPolicyID_IsForbiddenInPQMode — strict-PQ deployments use this
// to refuse misconfigured certs. The forbidden markers exist as explicit
// wire bytes so audit tooling can name them precisely.
func TestProofPolicyID_IsForbiddenInPQMode(t *testing.T) {
	for _, p := range []ProofPolicyID{
		ProofPolicyGroth16BN254Forbid,
		ProofPolicyPLONKKZGForbid,
		ProofPolicyHaloECForbid,
		ProofPolicyIPAECForbid,
	} {
		if !p.IsForbiddenInPQMode() {
			t.Errorf("%q must be forbidden in PQ mode", p.String())
		}
	}
	if ProofPolicySTARKFRISHA3PQ.IsForbiddenInPQMode() {
		t.Errorf("STARK_FRI_SHA3_PQ must NOT be forbidden")
	}
}

// TestProofPolicyID_LegacyAliases keeps the deprecated ProofSystem* names
// compiling for one release. New code must use the ProofPolicy* names.
// Drop this test when the aliases are removed at the 2026-Aug-31 freeze.
func TestProofPolicyID_LegacyAliases(t *testing.T) {
	if ProofSystemNone != ProofPolicyNone {
		t.Errorf("legacy alias ProofSystemNone must equal ProofPolicyNone")
	}
	if ProofSystemSTARKFRISHA3PQ != ProofPolicySTARKFRISHA3PQ {
		t.Errorf("legacy alias ProofSystemSTARKFRISHA3PQ must equal ProofPolicySTARKFRISHA3PQ")
	}
	if ProofSystemSTARKFRIKeccakPQ != ProofPolicySTARKFRIKeccak {
		t.Errorf("legacy alias ProofSystemSTARKFRIKeccakPQ must equal ProofPolicySTARKFRIKeccak")
	}
	if ProofSystemGroth16BN254Forbid != ProofPolicyGroth16BN254Forbid {
		t.Errorf("legacy alias ProofSystemGroth16BN254Forbid must equal ProofPolicyGroth16BN254Forbid")
	}
	if ProofSystemKZGForbid != ProofPolicyPLONKKZGForbid {
		t.Errorf("legacy alias ProofSystemKZGForbid must equal ProofPolicyPLONKKZGForbid")
	}
}

// TestProofBackendID_StableIntegers locks the wire bytes for the proof
// **backend** (implementation) enum, orthogonal to ProofPolicyID. Backends
// covered:
//
//	0x20 RISC0_SUCCINCT_STARK   succinct receipt path (no Groth16 wrapper)
//	0x21 SP1_COMPRESSED_STARK   SP1 compressed STARK
//	0x22 P3Q_STARK_FRI_SHA3     Lux P3Q (Plonky3 fork, cSHAKE256 Merkle)
//	0x23 STONE_CAIRO_STARK      Cairo / Stone backend
//	0x24 STWO_CIRCLE_STARK      Stwo Circle STARK
//	0x70 RISC0_RAW_STARK_DEV    dev/CI only
//	0x71 SP1_CORE_STARK_DEV     dev/CI only
//	0x80 GROTH16_WRAP_FORBID    forbidden wrapper in strict-PQ
//	0x81 KZG_WRAP_FORBID        forbidden wrapper in strict-PQ
func TestProofBackendID_StableIntegers(t *testing.T) {
	cases := []struct {
		backend ProofBackendID
		want    uint8
	}{
		{ProofBackendNone, 0x00},
		{ProofBackendRISC0SuccinctSTARK, 0x20},
		{ProofBackendSP1CompressedSTARK, 0x21},
		{ProofBackendP3QSTARKFRISHA3, 0x22},
		{ProofBackendStoneCairoSTARK, 0x23},
		{ProofBackendStwoCircleSTARK, 0x24},
		{ProofBackendRISC0RawSTARKDev, 0x70},
		{ProofBackendSP1CoreSTARKDev, 0x71},
		{ProofBackendGroth16WrapForbid, 0x80},
		{ProofBackendKZGWrapForbid, 0x81},
	}
	for _, c := range cases {
		if got := uint8(c.backend); got != c.want {
			t.Errorf("ProofBackendID %q = 0x%02x, want 0x%02x",
				c.backend.String(), got, c.want)
		}
	}
}

// TestProofBackendID_String pins canonical wire names so logs and audit
// pipelines see the same string in every cluster.
func TestProofBackendID_String(t *testing.T) {
	cases := []struct {
		backend ProofBackendID
		want    string
	}{
		{ProofBackendNone, "none"},
		{ProofBackendRISC0SuccinctSTARK, "risc0-succinct-stark-pq"},
		{ProofBackendSP1CompressedSTARK, "sp1-compressed-stark-pq"},
		{ProofBackendP3QSTARKFRISHA3, "p3q-stark-fri-sha3-pq"},
		{ProofBackendStoneCairoSTARK, "stone-cairo-stark-pq"},
		{ProofBackendStwoCircleSTARK, "stwo-circle-stark-pq"},
		{ProofBackendRISC0RawSTARKDev, "risc0-raw-stark-dev"},
		{ProofBackendSP1CoreSTARKDev, "sp1-core-stark-dev"},
		{ProofBackendGroth16WrapForbid, "groth16-wrap-classical-forbidden-in-pq"},
		{ProofBackendKZGWrapForbid, "kzg-wrap-classical-forbidden-in-pq"},
	}
	for _, c := range cases {
		if got := c.backend.String(); got != c.want {
			t.Errorf("ProofBackendID(0x%02x).String() = %q, want %q",
				uint8(c.backend), got, c.want)
		}
	}
}

// TestProofBackendID_IsProductionPQ separates the production block (0x20)
// from dev (0x70) and forbidden (0x80). Strict-PQ profiles MUST allow only
// production backends.
func TestProofBackendID_IsProductionPQ(t *testing.T) {
	prod := []ProofBackendID{
		ProofBackendRISC0SuccinctSTARK,
		ProofBackendSP1CompressedSTARK,
		ProofBackendP3QSTARKFRISHA3,
		ProofBackendStoneCairoSTARK,
		ProofBackendStwoCircleSTARK,
	}
	for _, b := range prod {
		if !b.IsProductionPQ() {
			t.Errorf("%q must be IsProductionPQ=true", b.String())
		}
	}
	nonProd := []ProofBackendID{
		ProofBackendNone,
		ProofBackendRISC0RawSTARKDev,
		ProofBackendSP1CoreSTARKDev,
		ProofBackendGroth16WrapForbid,
		ProofBackendKZGWrapForbid,
	}
	for _, b := range nonProd {
		if b.IsProductionPQ() {
			t.Errorf("%q must be IsProductionPQ=false", b.String())
		}
	}
}

// TestProofBackendID_IsForbiddenInPQMode mirrors the policy-layer test.
func TestProofBackendID_IsForbiddenInPQMode(t *testing.T) {
	for _, b := range []ProofBackendID{
		ProofBackendGroth16WrapForbid,
		ProofBackendKZGWrapForbid,
	} {
		if !b.IsForbiddenInPQMode() {
			t.Errorf("%q must be forbidden in PQ mode", b.String())
		}
	}
	for _, b := range []ProofBackendID{
		ProofBackendNone,
		ProofBackendRISC0SuccinctSTARK,
		ProofBackendSP1CompressedSTARK,
		ProofBackendP3QSTARKFRISHA3,
		ProofBackendStoneCairoSTARK,
		ProofBackendStwoCircleSTARK,
		ProofBackendRISC0RawSTARKDev,
		ProofBackendSP1CoreSTARKDev,
	} {
		if b.IsForbiddenInPQMode() {
			t.Errorf("%q must NOT carry the forbidden marker", b.String())
		}
	}
}

// TestIdentitySchemeID_StableIntegers locks the wire bytes for validator
// identity schemes. Block 0x40 mirrors raw ML-DSA in SigSchemeID so the
// byte pattern is consistent across the wire.
//
//	0x00 None    no identity scheme committed
//	0x41 ML-DSA-44  NIST PQ Cat 2 (devnet only)
//	0x42 ML-DSA-65  NIST PQ Cat 3 (production identity default)
//	0x43 ML-DSA-87  NIST PQ Cat 5 (high-value root identities)
func TestIdentitySchemeID_StableIntegers(t *testing.T) {
	cases := []struct {
		ident IdentitySchemeID
		want  uint8
	}{
		{IdentitySchemeNone, 0x00},
		{IdentitySchemeMLDSA44, 0x41},
		{IdentitySchemeMLDSA65, 0x42}, // production default
		{IdentitySchemeMLDSA87, 0x43},
	}
	for _, c := range cases {
		if got := uint8(c.ident); got != c.want {
			t.Errorf("IdentitySchemeID %q = 0x%02x, want 0x%02x",
				c.ident.String(), got, c.want)
		}
	}
}

// TestIdentitySchemeID_String pins canonical wire names.
func TestIdentitySchemeID_String(t *testing.T) {
	cases := []struct {
		ident IdentitySchemeID
		want  string
	}{
		{IdentitySchemeNone, "none"},
		{IdentitySchemeMLDSA44, "ml-dsa-44"},
		{IdentitySchemeMLDSA65, "ml-dsa-65"},
		{IdentitySchemeMLDSA87, "ml-dsa-87"},
	}
	for _, c := range cases {
		if got := c.ident.String(); got != c.want {
			t.Errorf("IdentitySchemeID(0x%02x).String() = %q, want %q",
				uint8(c.ident), got, c.want)
		}
	}
}

// TestIdentitySchemeID_IsFIPS204 — every non-None identity scheme uses
// unmodified FIPS 204 ML-DSA verification.
func TestIdentitySchemeID_IsFIPS204(t *testing.T) {
	for _, i := range []IdentitySchemeID{
		IdentitySchemeMLDSA44,
		IdentitySchemeMLDSA65,
		IdentitySchemeMLDSA87,
	} {
		if !i.IsFIPS204() {
			t.Errorf("%q must verify under FIPS 204", i.String())
		}
	}
	if IdentitySchemeNone.IsFIPS204() {
		t.Errorf("IdentitySchemeNone must not advertise FIPS 204")
	}
}

// TestPQMode_DKGRequired asserts the DKG class. Public chains MUST pick
// modes whose DKG class != "trusted-dealer".
func TestPQMode_DKGRequired(t *testing.T) {
	cases := []struct {
		mode PQMode
		want string
	}{
		{PQModeBLS, "none"},
		{PQModeNasua, "trusted-dealer"},
		{PQModePulsar, "pedersen-dkg-over-rq"},
		{PQModeQuasar, "pedersen-dkg-over-rq"},
		{PQModeMLDSA, "none"},
	}
	for _, c := range cases {
		if got := c.mode.DKGRequired(); got != c.want {
			t.Errorf("PQMode(%d).DKGRequired() = %q, want %q", c.mode, got, c.want)
		}
	}
}

// TestPQMode_SuitableForPublicChain enforces HIP-0077 §"PQ defaults":
// production Lux meshes accept Pulsar / Quasar / MLDSA. BLS (no PQ)
// and Corona (trusted-dealer DKG) are rejected.
func TestPQMode_SuitableForPublicChain(t *testing.T) {
	cases := []struct {
		mode PQMode
		want bool
	}{
		{PQModeBLS, false},
		{PQModeNasua, false},
		{PQModePulsar, true},
		{PQModeQuasar, true},
		{PQModeMLDSA, true},
	}
	for _, c := range cases {
		if got := c.mode.SuitableForPublicChain(); got != c.want {
			t.Errorf("PQMode(%d).SuitableForPublicChain() = %v, want %v", c.mode, got, c.want)
		}
	}
}

// TestPQModeFromBool pins the boolean-shorthand contract.
func TestPQModeFromBool(t *testing.T) {
	if got := PQModeFromBool(true); got != PQModeQuasar {
		t.Errorf("PQModeFromBool(true) = %v, want PQModeQuasar", got)
	}
	if got := PQModeFromBool(false); got != PQModeBLS {
		t.Errorf("PQModeFromBool(false) = %v, want PQModeBLS", got)
	}
}

// TestPQMode_IsPostQuantum is true for everything except plain BLS.
func TestPQMode_IsPostQuantum(t *testing.T) {
	cases := []struct {
		mode PQMode
		want bool
	}{
		{PQModeBLS, false},
		{PQModeNasua, true},
		{PQModePulsar, true},
		{PQModeQuasar, true},
		{PQModeMLDSA, true},
	}
	for _, c := range cases {
		if got := c.mode.IsPostQuantum(); got != c.want {
			t.Errorf("PQMode(%d).IsPostQuantum() = %v, want %v", c.mode, got, c.want)
		}
	}
}

// TestPQModeFromEnv covers the env-var override path.
func TestPQModeFromEnv(t *testing.T) {
	t.Setenv("LUX_CONSENSUS_PQ_MODE", "")
	if got, err := PQModeFromEnv(PQModeQuasar); err != nil || got != PQModeQuasar {
		t.Errorf("empty env: got (%v, %v), want (PQModeQuasar, nil)", got, err)
	}

	t.Setenv("LUX_CONSENSUS_PQ_MODE", "pulsar")
	if got, err := PQModeFromEnv(PQModeBLS); err != nil || got != PQModePulsar {
		t.Errorf("env=pulsar: got (%v, %v), want (PQModePulsar, nil)", got, err)
	}

	t.Setenv("LUX_CONSENSUS_PQ_MODE", "garbage")
	got, err := PQModeFromEnv(PQModeQuasar)
	if err == nil {
		t.Errorf("env=garbage: expected error, got nil; mode=%v", got)
	}
	if got != PQModeQuasar {
		t.Errorf("env=garbage: fell back to %v, want PQModeQuasar (the supplied default)", got)
	}
}
