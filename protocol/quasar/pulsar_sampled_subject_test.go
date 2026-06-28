// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"bytes"
	"testing"
)

// baseSubjectParams returns a fully-populated, non-zero subject parameter set.
func baseSubjectParams() PulsarSampledSubjectParams {
	p := PulsarSampledSubjectParams{
		ChainID:      96369,
		Height:       1_000_000,
		Round:        7,
		PChainHeight: 4242,
		PolicyID:     0x0C0DE002,
	}
	for i := range p.BlockID {
		p.BlockID[i] = byte(i + 1)
	}
	for i := range p.StateRoot {
		p.StateRoot[i] = byte(0x40 + i)
	}
	for i := range p.BeamQCHash {
		p.BeamQCHash[i] = byte(0x80 + i)
	}
	for i := range p.SignerSetID {
		p.SignerSetID[i] = byte(0xC0 + i)
	}
	p.CommitteePlanHash = bytes.Repeat([]byte{0xAB}, 32)
	return p
}

func TestSampledSubject_Deterministic(t *testing.T) {
	p := baseSubjectParams()
	m1 := PulsarSampledSubject(p)
	m2 := PulsarSampledSubject(p)
	if !bytes.Equal(m1, m2) {
		t.Fatal("subject M not deterministic")
	}
	if len(m1) != 32 {
		t.Fatalf("subject M length = %d, want 32", len(m1))
	}
}

// TestSampledSubject_EveryFieldBinds flips each field in turn and asserts M
// changes — the committees' signatures bind the FULL finality position plus the
// Beam-QC commitment plus the frozen committee plan. A field that did not change
// M would be a field a committee signature could be replayed across.
func TestSampledSubject_EveryFieldBinds(t *testing.T) {
	base := baseSubjectParams()
	baseM := PulsarSampledSubject(base)

	mutate := map[string]func(*PulsarSampledSubjectParams){
		"chainID":           func(p *PulsarSampledSubjectParams) { p.ChainID ^= 1 },
		"height":            func(p *PulsarSampledSubjectParams) { p.Height ^= 1 },
		"round":             func(p *PulsarSampledSubjectParams) { p.Round ^= 1 },
		"blockID":           func(p *PulsarSampledSubjectParams) { p.BlockID[0] ^= 1 },
		"stateRoot":         func(p *PulsarSampledSubjectParams) { p.StateRoot[0] ^= 1 },
		"beamQCHash":        func(p *PulsarSampledSubjectParams) { p.BeamQCHash[0] ^= 1 },
		"signerSetID":       func(p *PulsarSampledSubjectParams) { p.SignerSetID[0] ^= 1 },
		"pChainHeight":      func(p *PulsarSampledSubjectParams) { p.PChainHeight ^= 1 },
		"policyID":          func(p *PulsarSampledSubjectParams) { p.PolicyID ^= 1 },
		"committeePlanHash": func(p *PulsarSampledSubjectParams) { p.CommitteePlanHash[0] ^= 1 },
	}
	for name, f := range mutate {
		p := base
		// CommitteePlanHash is a slice — copy it so a per-case mutation does not
		// alias the base.
		p.CommitteePlanHash = append([]byte(nil), base.CommitteePlanHash...)
		f(&p)
		if bytes.Equal(baseM, PulsarSampledSubject(p)) {
			t.Fatalf("flipping %s did not change the subject M — field is not bound", name)
		}
	}
}

// TestSampledSubject_NonTransferableVsEnvelope asserts the sampled subject is
// NOT byte-equal to the canonical single-committee Pulsar finality message for
// the same finalized position. The distinct subject domain guarantees a
// single-committee threshold signature can never be replayed as a sampled
// committee signature (and vice versa) — different evidence types, different
// subjects.
func TestSampledSubject_NonTransferableVsEnvelope(t *testing.T) {
	base := baseSubjectParams()
	sampledM := PulsarSampledSubject(base)

	envelopeM := QuasarFinalityMessage(QuasarFinalityParams{
		ChainID:          base.ChainID,
		Height:           base.Height,
		Round:            base.Round,
		BlockID:          base.BlockID,
		StateRoot:        base.StateRoot,
		SignerSetID:      base.SignerSetID,
		EvidencePolicyID: base.PolicyID,
	})
	if bytes.Equal(sampledM, envelopeM) {
		t.Fatal("sampled subject collides with the envelope Pulsar finality message — non-transferability broken")
	}
}

// TestSampledSubject_PlanHashLengthDistinct asserts a length difference in the
// committeePlanHash changes M (TupleHash length-prefixing), so a short/empty
// plan-hash binding cannot be confused with a full 32-byte one.
func TestSampledSubject_PlanHashLengthDistinct(t *testing.T) {
	full := baseSubjectParams()
	short := baseSubjectParams()
	short.CommitteePlanHash = full.CommitteePlanHash[:16]
	if bytes.Equal(PulsarSampledSubject(full), PulsarSampledSubject(short)) {
		t.Fatal("plan-hash length is not bound — TupleHash length-prefixing broken")
	}
	empty := baseSubjectParams()
	empty.CommitteePlanHash = nil
	if bytes.Equal(PulsarSampledSubject(full), PulsarSampledSubject(empty)) {
		t.Fatal("empty plan hash collides with full plan hash")
	}
}
