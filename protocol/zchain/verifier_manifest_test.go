// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zchain

import (
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// canonicalManifest returns a valid manifest for use as a register-able
// baseline. Tests that exercise rejection paths mutate one field at a
// time and assert the registry surfaces the expected typed error.
func canonicalManifest() *VerifierManifest {
	return &VerifierManifest{
		VerifierID:            config.VerifierP3QSTARKFRISHA3PQ,
		BackendID:             config.ProofBackendP3QSTARKFRISHA3,
		Version:               "v0.1.0",
		SourceCommit:          padBytes20(0xDE),
		BuildProfile:          "production",
		ProofFormatID:         config.ProofFormatP3QBinaryV1,
		ProgramOrAirHash:      padBytes48(0x44),
		VerifierKeyHash:       padBytes48(0x22),
		SupportsPolicyIDs:     []config.ProofPolicyID{config.ProofPolicySTARKFRISHA3PQ},
		SoundnessBitsReviewed: 128,
		HashOutputBits:        384,
	}
}

func TestVerifierManifestRegistry_RegisterAndLookup(t *testing.T) {
	r := NewVerifierManifestRegistry(nil)
	m := canonicalManifest()
	if err := r.Register(m); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if r.Len() != 1 {
		t.Errorf("Len() = %d, want 1", r.Len())
	}
	got, ok := r.Lookup(m.VerifierID)
	if !ok {
		t.Fatalf("Lookup(%s) returned !ok", m.VerifierID.String())
	}
	if got.BackendID != m.BackendID {
		t.Errorf("Lookup returned wrong BackendID: %s vs %s", got.BackendID.String(), m.BackendID.String())
	}
}

func TestVerifierManifestRegistry_DefensiveCopy(t *testing.T) {
	r := NewVerifierManifestRegistry(nil)
	m := canonicalManifest()
	if err := r.Register(m); err != nil {
		t.Fatalf("Register: %v", err)
	}
	// Mutate the original AFTER Register. The registry MUST have taken
	// a defensive copy; lookup MUST return the unmutated manifest.
	m.SupportsPolicyIDs[0] = config.ProofPolicySTARKFRIKeccak
	got, _ := r.Lookup(canonicalManifest().VerifierID)
	if got.SupportsPolicyIDs[0] != config.ProofPolicySTARKFRISHA3PQ {
		t.Errorf("registry did not defensively copy SupportsPolicyIDs: got %v",
			got.SupportsPolicyIDs)
	}
}

func TestVerifierManifestRegistry_RejectsNil(t *testing.T) {
	r := NewVerifierManifestRegistry(nil)
	if err := r.Register(nil); !errors.Is(err, ErrVerifierManifestNil) {
		t.Errorf("Register(nil) = %v; want ErrVerifierManifestNil", err)
	}
}

func TestVerifierManifestRegistry_RejectsVerifierNone(t *testing.T) {
	r := NewVerifierManifestRegistry(nil)
	m := canonicalManifest()
	m.VerifierID = config.VerifierNone
	if err := r.Register(m); !errors.Is(err, ErrVerifierManifestInvalidID) {
		t.Errorf("Register(VerifierNone) = %v; want ErrVerifierManifestInvalidID", err)
	}
}

func TestVerifierManifestRegistry_RejectsDuplicate(t *testing.T) {
	r := NewVerifierManifestRegistry(nil)
	m := canonicalManifest()
	if err := r.Register(m); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := r.Register(canonicalManifest()); !errors.Is(err, ErrVerifierManifestDuplicate) {
		t.Errorf("duplicate Register = %v; want ErrVerifierManifestDuplicate", err)
	}
}

func TestVerifierManifestRegistry_RejectsMissingFields(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(m *VerifierManifest)
	}{
		{"Version", func(m *VerifierManifest) { m.Version = "" }},
		{"BuildProfile", func(m *VerifierManifest) { m.BuildProfile = "" }},
		{"BackendID", func(m *VerifierManifest) { m.BackendID = config.ProofBackendNone }},
		{"ProofFormatID", func(m *VerifierManifest) { m.ProofFormatID = config.ProofFormatNone }},
		{"SupportsPolicyIDs", func(m *VerifierManifest) { m.SupportsPolicyIDs = nil }},
	}
	for _, c := range cases {
		r := NewVerifierManifestRegistry(nil)
		m := canonicalManifest()
		c.mutate(m)
		err := r.Register(m)
		if !errors.Is(err, ErrVerifierManifestMissingField) {
			t.Errorf("Register without %s = %v; want ErrVerifierManifestMissingField", c.name, err)
		}
	}
}

func TestVerifierManifestRegistry_Lookup_MissingReturnsFalse(t *testing.T) {
	r := NewVerifierManifestRegistry(nil)
	_, ok := r.Lookup(config.VerifierSP1CompressedSTARKPQ)
	if ok {
		t.Errorf("Lookup on empty registry returned ok=true")
	}
}

func TestVerifierManifest_SupportsPolicy(t *testing.T) {
	m := canonicalManifest()
	if !m.SupportsPolicy(config.ProofPolicySTARKFRISHA3PQ) {
		t.Errorf("SupportsPolicy(STARK_FRI_SHA3_PQ) = false; want true")
	}
	if m.SupportsPolicy(config.ProofPolicySTARKFRIKeccak) {
		t.Errorf("SupportsPolicy(STARK_FRI_Keccak) = true; want false")
	}
}
