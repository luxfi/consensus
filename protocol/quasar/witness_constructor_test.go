// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// TestNewWitnessSet_RefusesNilProfile is the CR-4 regression: the
// zero-value WitnessSet{} (MinPolicy=0) was the silent-downgrade attack
// surface. The canonical constructor MUST refuse a nil profile so every
// production caller is forced into the audited mapping path.
func TestNewWitnessSet_RefusesNilProfile(t *testing.T) {
	_, err := NewWitnessSet(nil, fakeP{}, nil, nil)
	if !errors.Is(err, ErrNilProfile) {
		t.Fatalf("NewWitnessSet(nil profile) = %v, want ErrNilProfile", err)
	}
}

// TestNewWitnessSet_RefusesNilPProducer asserts the constructor refuses a
// nil P producer — P is mandatory at every floor, so the misconfiguration
// must be caught at construction rather than on the first call to Run().
func TestNewWitnessSet_RefusesNilPProducer(t *testing.T) {
	_, err := NewWitnessSet(config.StrictPQ(), nil, nil, nil)
	if err == nil {
		t.Fatal("NewWitnessSet(nil P) returned no error")
	}
}

// TestNewWitnessSet_StrictProfilePinsQuantumFloor is the headline CR-4
// fix: a chain that declared strict-PQ in its genesis MUST end up with
// MinPolicy=PolicyQuantum so Run() refuses any cert that omits Q or Z.
// Without this, a permissive zero-value WitnessSet{} on a strict chain
// would silently emit downgraded certs and the receiver would accept
// them as if the chain were declared at the lower floor.
func TestNewWitnessSet_StrictProfilePinsQuantumFloor(t *testing.T) {
	ws, err := NewWitnessSet(config.StrictPQ(), fakeP{}, goodQ{}, goodZ{})
	if err != nil {
		t.Fatalf("NewWitnessSet(strict) returned %v", err)
	}
	if ws.MinPolicy != 4 { // PolicyQuantum
		t.Fatalf("strict profile yielded MinPolicy=%d, want 4 (PolicyQuantum)", ws.MinPolicy)
	}
	if ws.Mode != config.PQModeQuasar {
		t.Fatalf("strict profile yielded Mode=%s, want PQModeQuasar", ws.Mode)
	}
}

// TestNewWitnessSet_FIPSProfilePinsQuantumFloor — FIPS is a strict
// superset of strict-PQ; the witness floor stays at PolicyQuantum.
func TestNewWitnessSet_FIPSProfilePinsQuantumFloor(t *testing.T) {
	ws, err := NewWitnessSet(config.FIPS(), fakeP{}, goodQ{}, goodZ{})
	if err != nil {
		t.Fatalf("NewWitnessSet(fips) returned %v", err)
	}
	if ws.MinPolicy != 4 { // PolicyQuantum
		t.Fatalf("fips profile yielded MinPolicy=%d, want 4 (PolicyQuantum)", ws.MinPolicy)
	}
	if ws.Mode != config.PQModeQuasar {
		t.Fatalf("fips profile yielded Mode=%s, want PQModeQuasar", ws.Mode)
	}
}

// TestNewWitnessSet_PermissiveFloorAtQuorum — permissive testnet/devnet
// chains finalise on P alone. The MinPolicy floor is PolicyQuorum so a
// BLS-only cert is admissible; chains that need stronger guarantees must
// upgrade their profile.
func TestNewWitnessSet_PermissiveFloorAtQuorum(t *testing.T) {
	ws, err := NewWitnessSet(config.Permissive(), fakeP{}, nil, nil)
	if err != nil {
		t.Fatalf("NewWitnessSet(permissive) returned %v", err)
	}
	if ws.MinPolicy != 1 { // PolicyQuorum
		t.Fatalf("permissive profile yielded MinPolicy=%d, want 1 (PolicyQuorum)", ws.MinPolicy)
	}
	if ws.Mode != config.PQModeBLS {
		t.Fatalf("permissive profile yielded Mode=%s, want PQModeBLS", ws.Mode)
	}
}

// TestNewWitnessSet_StrictProfile_RefusesMissingZ — the structural
// guarantee of CR-4: under a strict-PQ profile, every Run() call MUST
// refuse silent downgrade to PolicyPQ when the Z producer is nil. The
// constructor itself accepts a nil Z (mis-deployment will surface at
// first round), and Run() enforces the floor.
func TestNewWitnessSet_StrictProfile_RefusesMissingZ(t *testing.T) {
	ws, err := NewWitnessSet(config.StrictPQ(), fakeP{}, goodQ{}, nil)
	if err != nil {
		t.Fatalf("NewWitnessSet(strict, nil Z) returned %v", err)
	}
	_, err = ws.Run(context.Background(), testNonZeroDigest(), nil)
	if !errors.Is(err, ErrWitnessFloorBreached) {
		t.Fatalf("strict profile with nil Z producer: Run() returned %v, want ErrWitnessFloorBreached", err)
	}
}

// TestNewWitnessSet_StrictProfile_RefusesMissingQ — mirror of the
// "missing Z" case for the Q lane. Same floor, same refusal.
func TestNewWitnessSet_StrictProfile_RefusesMissingQ(t *testing.T) {
	ws, err := NewWitnessSet(config.StrictPQ(), fakeP{}, nil, goodZ{})
	if err != nil {
		t.Fatalf("NewWitnessSet(strict, nil Q) returned %v", err)
	}
	_, err = ws.Run(context.Background(), testNonZeroDigest(), nil)
	if !errors.Is(err, ErrWitnessFloorBreached) {
		t.Fatalf("strict profile with nil Q producer: Run() returned %v, want ErrWitnessFloorBreached", err)
	}
}

// TestNewWitnessSet_StrictProfile_AcceptsFullWitnessSet — happy path:
// a strict-PQ chain with all three producers wired produces a
// PolicyQuantum cert.
func TestNewWitnessSet_StrictProfile_AcceptsFullWitnessSet(t *testing.T) {
	ws, err := NewWitnessSet(config.StrictPQ(), fakeP{}, goodQ{}, goodZ{})
	if err != nil {
		t.Fatalf("NewWitnessSet(strict, full) returned %v", err)
	}
	rw, err := ws.Run(context.Background(), testNonZeroDigest(), nil)
	if err != nil {
		t.Fatalf("strict profile with full witness set: Run() returned %v", err)
	}
	if rw.Q == nil || rw.Z == nil {
		t.Fatalf("expected Q and Z to be populated, got Q=%v Z=%v", rw.Q != nil, rw.Z != nil)
	}
	if rw.HashSuiteID != config.HashSuiteSHA3NIST {
		t.Fatalf("expected SHA3_NIST hash suite, got %s", rw.HashSuiteID)
	}
}

// TestNewWitnessSet_RefusesProfileNone — ProfileNone (0x00) is the
// rejected zero value of the profile-ID enum; the constructor must
// refuse it rather than mapping to a default.
func TestNewWitnessSet_RefusesProfileNone(t *testing.T) {
	bogus := &config.ChainSecurityProfile{ProfileID: uint32(config.ProfileNone)}
	_, err := NewWitnessSet(bogus, fakeP{}, nil, nil)
	if err == nil {
		t.Fatal("NewWitnessSet(ProfileNone) returned no error")
	}
}

// TestNewWitnessSet_RefusesUnknownProfileID — an unknown profile byte
// (e.g. a downstream white-label that forgot to register) must surface
// at construction, not at first cert.
func TestNewWitnessSet_RefusesUnknownProfileID(t *testing.T) {
	bogus := &config.ChainSecurityProfile{ProfileID: 0xFF} // not in registry
	_, err := NewWitnessSet(bogus, fakeP{}, nil, nil)
	if err == nil {
		t.Fatal("NewWitnessSet(unknown ProfileID) returned no error")
	}
}
