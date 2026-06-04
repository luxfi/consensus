// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// testNonZeroDigest returns a fixed non-zero RoundDigest for tests
// that just need a valid (non-zero) subject. Production code MUST
// build the digest via ComputeRoundDigest; this helper bypasses that
// constructor only because the surrounding test isolates a different
// behaviour (witness floor, hash-suite propagation) and not the
// digest construction itself.
func testNonZeroDigest() RoundDigest {
	return RoundDigest{
		0xfe, 0xed, 0xfa, 0xce, 0xde, 0xad, 0xbe, 0xef,
		0xfe, 0xed, 0xfa, 0xce, 0xde, 0xad, 0xbe, 0xef,
		0xfe, 0xed, 0xfa, 0xce, 0xde, 0xad, 0xbe, 0xef,
		0xfe, 0xed, 0xfa, 0xce, 0xde, 0xad, 0xbe, 0xef,
	}
}

// fakeP always succeeds (we want to isolate Q/Z floor behaviour).
type fakeP struct{}

func (fakeP) Witness(ctx context.Context, digest RoundDigest) ([]byte, []byte, error) {
	return []byte("p-sig"), []byte("p-signers"), nil
}

// failQ always returns ErrWitnessUnavailable.
type failQ struct{}

func (failQ) Witness(ctx context.Context, digest RoundDigest) ([]byte, error) {
	return nil, ErrWitnessUnavailable
}

// failZ always returns ErrWitnessUnavailable.
type failZ struct{}

func (failZ) Witness(ctx context.Context, digest RoundDigest, vk [][]byte) ([]byte, error) {
	return nil, ErrWitnessUnavailable
}

// goodQ returns a non-nil signature.
type goodQ struct{}

func (goodQ) Witness(ctx context.Context, digest RoundDigest) ([]byte, error) {
	return []byte("q-sig"), nil
}

// goodZ returns a non-nil proof.
type goodZ struct{}

func (goodZ) Witness(ctx context.Context, digest RoundDigest, vk [][]byte) ([]byte, error) {
	return []byte("z-proof"), nil
}

// TestRun_LegacyBestEffort_NoFloor preserves the pre-floor behaviour for
// callers that haven't migrated. MinPolicy=0 → missing optional witnesses
// produce a downgraded bundle without error.
func TestRun_LegacyBestEffort_NoFloor(t *testing.T) {
	ws := WitnessSet{P: fakeP{}, Q: failQ{}, Z: failZ{}}
	rw, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if err != nil {
		t.Fatalf("legacy mode: unexpected error: %v", err)
	}
	if rw.Q != nil || rw.Z != nil {
		t.Fatalf("legacy mode: expected nil Q/Z, got Q=%v Z=%v", rw.Q != nil, rw.Z != nil)
	}
	if string(rw.PSig) != "p-sig" {
		t.Fatalf("legacy mode: expected P=p-sig, got %s", rw.PSig)
	}
}

// TestRun_QuasarFloor_RefusesWhenZFails proves the headline F2 fix:
// a Quasar-declared chain whose Z producer fails MUST NOT silently
// emit a PolicyPQ cert. It MUST refuse the round entirely.
func TestRun_QuasarFloor_RefusesWhenZFails(t *testing.T) {
	ws := WitnessSet{
		P:         fakeP{},
		Q:         goodQ{},
		Z:         failZ{},
		MinPolicy: 4, // PolicyQuantum
	}
	_, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if !errors.Is(err, ErrWitnessFloorBreached) {
		t.Fatalf("expected ErrWitnessFloorBreached, got: %v", err)
	}
}

// TestRun_QuasarFloor_RefusesWhenQFails — same protection in the other
// direction. Quasar requires BOTH lattice witnesses; either one missing
// → refused round.
func TestRun_QuasarFloor_RefusesWhenQFails(t *testing.T) {
	ws := WitnessSet{
		P:         fakeP{},
		Q:         failQ{},
		Z:         goodZ{},
		MinPolicy: 4, // PolicyQuantum
	}
	_, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if !errors.Is(err, ErrWitnessFloorBreached) {
		t.Fatalf("expected ErrWitnessFloorBreached, got: %v", err)
	}
}

// TestRun_QuasarFloor_AcceptsWhenBothPresent — happy path: both lattice
// witnesses available → cert finalises at PolicyQuantum.
func TestRun_QuasarFloor_AcceptsWhenBothPresent(t *testing.T) {
	ws := WitnessSet{
		P:         fakeP{},
		Q:         goodQ{},
		Z:         goodZ{},
		MinPolicy: 4, // PolicyQuantum
	}
	rw, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if rw.Q == nil || rw.Z == nil {
		t.Fatalf("expected both Q and Z, got Q=%v Z=%v", rw.Q != nil, rw.Z != nil)
	}
}

// TestRun_PulsarFloor_AcceptsPQ — a chain that declared Pulsar mode
// (PolicyPQ floor) is happy with P+Q, even when Z is absent.
func TestRun_PulsarFloor_AcceptsPQ(t *testing.T) {
	ws := WitnessSet{
		P:         fakeP{},
		Q:         goodQ{},
		Z:         nil, // explicitly unconfigured
		MinPolicy: 5,   // PolicyPQ
	}
	rw, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if rw.Q == nil {
		t.Fatalf("expected Q, got nil")
	}
}

// TestRun_PulsarFloor_RefusesWhenQFails — Pulsar mode without Q can't
// downgrade silently to BLS-only.
func TestRun_PulsarFloor_RefusesWhenQFails(t *testing.T) {
	ws := WitnessSet{
		P:         fakeP{},
		Q:         failQ{},
		MinPolicy: 5, // PolicyPQ
	}
	_, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if !errors.Is(err, ErrWitnessFloorBreached) {
		t.Fatalf("expected ErrWitnessFloorBreached, got: %v", err)
	}
}

// TestRun_BLSFloor_AlwaysAccepts — chains that declared BLS-only mode
// finalise on P alone. (Lux mesh refuses this in production policy,
// but the consensus engine must support it for benchmarks/legacy.)
func TestRun_BLSFloor_AlwaysAccepts(t *testing.T) {
	ws := WitnessSet{
		P:         fakeP{},
		MinPolicy: 1, // PolicyQuorum
	}
	_, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if err != nil {
		t.Fatalf("BLS floor: unexpected error: %v", err)
	}
}

// TestRun_PropagatesHashSuiteID_FromMode is the F1 fix at the producer
// layer: every PQMode MUST emit a RoundWitnesses whose HashSuiteID matches
// config.PQMode.HashSuiteID(). The downstream cert assembler stamps the
// envelope's HashSuiteID byte from this field and binds it into the
// transcript.
//
// HIP-0077 §"Lux consensus PQ modes" red-review F1. Without this, a Pulsar
// (SHA-3) producer and a Corona (BLAKE3) producer at the same PolicyID 5
// emit indistinguishable certs and the receiver picks the wrong kernel.
func TestRun_PropagatesHashSuiteID_FromMode(t *testing.T) {
	cases := []struct {
		mode config.PQMode
		want config.HashSuiteID
	}{
		{config.PQModeBLS, config.HashSuiteNone},
		{config.PQModeCorona, config.HashSuiteBLAKE3Legacy},
		{config.PQModePulsar, config.HashSuiteSHA3NIST},
		{config.PQModeQuasar, config.HashSuiteSHA3NIST},
		{config.PQModeMLDSA, config.HashSuiteSHA3NIST},
	}
	for _, c := range cases {
		ws := WitnessSet{P: fakeP{}, Mode: c.mode}
		rw, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
		if err != nil {
			t.Fatalf("mode=%s: unexpected error: %v", c.mode, err)
		}
		if rw.HashSuiteID != c.want {
			t.Errorf("mode=%s: RoundWitnesses.HashSuiteID = %d (%q), want %d (%q)",
				c.mode, rw.HashSuiteID, rw.HashSuiteID.String(),
				c.want, c.want.String())
		}
	}
}

// TestRun_HashSuiteID_DefaultsToNoneWhenModeUnset preserves legacy callers
// that haven't migrated yet: a zero-valued WitnessSet.Mode (PQModeBLS) maps
// to HashSuiteNone on the wire, matching the historical BLS-only cert
// shape. New code MUST set Mode explicitly.
func TestRun_HashSuiteID_DefaultsToNoneWhenModeUnset(t *testing.T) {
	ws := WitnessSet{P: fakeP{}} // Mode zero-valued
	rw, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rw.HashSuiteID != config.HashSuiteNone {
		t.Errorf("unset Mode: HashSuiteID = %d, want HashSuiteNone (0)", rw.HashSuiteID)
	}
}

// TestEffectivePolicyID maps witness presence to wire policy ID. Direct
// inverse of config.PQMode.PolicyID().
func TestEffectivePolicyID(t *testing.T) {
	cases := []struct {
		name string
		rw   *RoundWitnesses
		want uint16
	}{
		{"P only", &RoundWitnesses{}, 1},
		{"P+Q", &RoundWitnesses{Q: []byte("x")}, 5},
		{"P+Z", &RoundWitnesses{Z: []byte("x")}, 6},
		{"P+Q+Z", &RoundWitnesses{Q: []byte("x"), Z: []byte("x")}, 4},
	}
	for _, c := range cases {
		if got := effectivePolicyID(c.rw); got != c.want {
			t.Errorf("%s: effectivePolicyID = %d, want %d", c.name, got, c.want)
		}
	}
}
