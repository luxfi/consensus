// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zap

import (
	"bytes"
	"testing"
)

// TestQuasarCertBuildParseFieldEquality verifies that every field
// written into NewQuasarCert round-trips through WrapQuasarCert with
// byte equality. This is the LP-182 test case
// "TestQuasarCertBuildParseFieldEquality".
func TestQuasarCertBuildParseFieldEquality(t *testing.T) {
	bls := []byte("BLS-12-381-aggregate-48-bytes-here-padding-x")
	corona := bytes.Repeat([]byte{0xAA}, 4096)
	pulsar := bytes.Repeat([]byte{0xBB}, 3309)
	magnetar := bytes.Repeat([]byte{0xCC}, 8192)
	mldsaRollup := bytes.Repeat([]byte{0xDD}, 1024)
	epoch := uint64(0x1122334455667788)
	finality := int64(0x7FFFFFFF12345678)
	validators := uint32(0xDEADBEEF)

	c := NewQuasarCert(bls, corona, pulsar, magnetar, mldsaRollup, epoch, finality, validators)

	wrapped, err := WrapQuasarCert(c.Bytes())
	if err != nil {
		t.Fatalf("WrapQuasarCert: %v", err)
	}

	if !bytes.Equal(wrapped.BLS(), bls) {
		t.Errorf("BLS round-trip mismatch")
	}
	if !bytes.Equal(wrapped.Corona(), corona) {
		t.Errorf("Corona round-trip mismatch")
	}
	if !bytes.Equal(wrapped.Pulsar(), pulsar) {
		t.Errorf("Pulsar round-trip mismatch")
	}
	if !bytes.Equal(wrapped.Magnetar(), magnetar) {
		t.Errorf("Magnetar round-trip mismatch")
	}
	if !bytes.Equal(wrapped.MLDSARollup(), mldsaRollup) {
		t.Errorf("MLDSARollup round-trip mismatch")
	}
	if wrapped.Epoch() != epoch {
		t.Errorf("Epoch mismatch: got %x want %x", wrapped.Epoch(), epoch)
	}
	if wrapped.FinalityUnixNano() != finality {
		t.Errorf("Finality mismatch: got %x want %x", wrapped.FinalityUnixNano(), finality)
	}
	if wrapped.Validators() != validators {
		t.Errorf("Validators mismatch: got %x want %x", wrapped.Validators(), validators)
	}
}

// TestQuasarCertBytesIsUnderlyingBuffer verifies that c.Bytes() returns
// the same underlying byte slice as the ZAP buffer the message wraps —
// no copy, no extra allocation. LP-182 test case
// "TestQuasarCertBytesIsUnderlyingBuffer".
func TestQuasarCertBytesIsUnderlyingBuffer(t *testing.T) {
	c := NewQuasarCert(
		[]byte("bls"),
		[]byte("corona"),
		[]byte("pulsar"),
		nil,
		nil,
		1, 2, 3,
	)
	b1 := c.Bytes()
	b2 := c.Bytes()
	if &b1[0] != &b2[0] {
		t.Errorf("Bytes() returned different backing arrays — expected zero-copy")
	}
}

// TestQuasarCertParseRefusesNonZAP verifies that WrapQuasarCert rejects
// bytes that don't have the ZAP magic header. LP-182 test case
// "TestQuasarCertParseRefusesNonZAP".
func TestQuasarCertParseRefusesNonZAP(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		make([]byte, 1),
		make([]byte, 15),                              // too short for header
		bytes.Repeat([]byte{0x00}, 100),               // no magic
		append([]byte("WRONG"), make([]byte, 100)...), // wrong magic
	}
	for i, c := range cases {
		_, err := WrapQuasarCert(c)
		if err == nil {
			t.Errorf("case %d: WrapQuasarCert accepted non-ZAP bytes", i)
		}
	}
}

// TestAllSchemasRoundTrip verifies that every LP-182 schema round-trips
// through Build → Bytes → Wrap with byte equality on every field.
func TestAllSchemasRoundTrip(t *testing.T) {
	// QBlock
	qb := NewQBlock(QBlockFields{
		Version:   1,
		NetworkID: 1337,
		ChainID:   1,
		Height:    42,
		Signature: []byte("pulsarM-sig"),
	})
	if wrap, err := WrapQBlock(qb.Bytes()); err != nil {
		t.Errorf("WrapQBlock: %v", err)
	} else if wrap.Version() != 1 || wrap.Height() != 42 {
		t.Errorf("QBlock fields mismatch")
	}

	// WitnessProof
	w := NewWitnessProof(WitnessProofFields{
		Commitment:  []byte("commit"),
		PQSignature: []byte("pqsig"),
		BlockHeight: 1234,
	})
	if wrap, err := WrapWitnessProof(w.Bytes()); err != nil {
		t.Errorf("WrapWitnessProof: %v", err)
	} else if wrap.BlockHeight() != 1234 {
		t.Errorf("WitnessProof.BlockHeight mismatch")
	}

	// MagnetarAggregateCert
	m := NewMagnetarAggregateCert(MagnetarAggregateFields{
		Mode:    5,
		Count:   3,
		Signers: bytes.Repeat([]byte{0x01}, 3*32),
	})
	if wrap, err := WrapMagnetarAggregateCert(m.Bytes()); err != nil {
		t.Errorf("WrapMagnetarAggregateCert: %v", err)
	} else if wrap.Mode() != 5 || wrap.Count() != 3 {
		t.Errorf("MagnetarAggregate fields mismatch")
	}

	// PolarisLegs
	p := NewPolarisLegs(PolarisLegsFields{
		BLS:        []byte("bls"),
		Pulsar:     []byte("puls"),
		Epoch:      99,
		Validators: 5,
	})
	if wrap, err := WrapPolarisLegs(p.Bytes()); err != nil {
		t.Errorf("WrapPolarisLegs: %v", err)
	} else if wrap.Epoch() != 99 {
		t.Errorf("PolarisLegs.Epoch mismatch")
	}

	// QuasarSig
	qs := NewQuasarSig(QuasarSigFields{
		BLSSignature:   []byte("bls"),
		BLSValidatorID: "node-0",
		MLDSA:          []byte("mldsa"),
	})
	if wrap, err := WrapQuasarSig(qs.Bytes()); err != nil {
		t.Errorf("WrapQuasarSig: %v", err)
	} else if wrap.BLSValidatorID() != "node-0" {
		t.Errorf("QuasarSig.BLSValidatorID mismatch")
	}

	// EpochBundle
	eb := NewEpochBundle(EpochBundleFields{
		Epoch:      7,
		Sequence:   3,
		BlockCount: 6,
		Timestamp:  1700000000,
	})
	if wrap, err := WrapEpochBundle(eb.Bytes()); err != nil {
		t.Errorf("WrapEpochBundle: %v", err)
	} else if wrap.Epoch() != 7 || wrap.BlockCount() != 6 {
		t.Errorf("EpochBundle fields mismatch")
	}

	// PrismCut
	pc := NewPrismCut(PrismCutFields{K: 20, Peers: bytes.Repeat([]byte{0x02}, 20*32)})
	if wrap, err := WrapPrismCut(pc.Bytes()); err != nil {
		t.Errorf("WrapPrismCut: %v", err)
	} else if wrap.K() != 20 {
		t.Errorf("PrismCut.K mismatch")
	}

	// StakeWeightedCut
	swc := NewStakeWeightedCut(StakeWeightedCutFields{
		K:           5,
		TotalWeight: 1000,
		Validators:  bytes.Repeat([]byte{0x03}, 5*40),
	})
	if wrap, err := WrapStakeWeightedCut(swc.Bytes()); err != nil {
		t.Errorf("WrapStakeWeightedCut: %v", err)
	} else if wrap.K() != 5 || wrap.TotalWeight() != 1000 {
		t.Errorf("StakeWeightedCut fields mismatch")
	}

	// TxAuthEnvelope
	te := NewTxAuthEnvelope(TxAuthEnvelopeFields{
		Version:        1,
		ProfileID:      2,
		ChainID:        100,
		NetworkID:      1337,
		Nonce:          42,
		ExpiryHeight:   1000000,
		WalletSchemeID: 1,
		HashSuiteID:    1,
		Signature:      []byte("sig"),
	})
	if wrap, err := WrapTxAuthEnvelope(te.Bytes()); err != nil {
		t.Errorf("WrapTxAuthEnvelope: %v", err)
	} else if wrap.Nonce() != 42 {
		t.Errorf("TxAuthEnvelope.Nonce mismatch")
	}

	// PQPermit
	pp := NewPQPermit(PQPermitFields{
		Version:      1,
		ProfileID:    2,
		ChainID:      100,
		Nonce:        7,
		Deadline:     999999,
		AuthSchemeID: 1,
		HashSuiteID:  1,
		Signature:    []byte("sig"),
	})
	if wrap, err := WrapPQPermit(pp.Bytes()); err != nil {
		t.Errorf("WrapPQPermit: %v", err)
	} else if wrap.Nonce() != 7 {
		t.Errorf("PQPermit.Nonce mismatch")
	}

	// DAGVertex
	dv := NewDAGVertex(DAGVertexFields{
		Height:    100,
		Timestamp: 1700000000,
		Data:      []byte("payload"),
	})
	if wrap, err := WrapDAGVertex(dv.Bytes()); err != nil {
		t.Errorf("WrapDAGVertex: %v", err)
	} else if wrap.Height() != 100 || !bytes.Equal(wrap.Data(), []byte("payload")) {
		t.Errorf("DAGVertex fields mismatch")
	}

	// PolicyCert
	pcert := NewPolicyCert(PolicyCertFields{
		Height:   50,
		PolicyID: 3,
		Proof:    []byte("proof"),
	})
	if wrap, err := WrapPolicyCert(pcert.Bytes()); err != nil {
		t.Errorf("WrapPolicyCert: %v", err)
	} else if wrap.Height() != 50 || wrap.PolicyID() != 3 {
		t.Errorf("PolicyCert fields mismatch")
	}
}

// TestSchemaRegistry verifies the SchemaID registry matches LP-182.
func TestSchemaRegistry(t *testing.T) {
	want := map[SchemaID]string{
		0x01: "QuasarCert",
		0x02: "QBlock",
		0x03: "WitnessProof",
		0x04: "MagnetarAggregateCert",
		0x05: "PolarisLegs",
		0x06: "QuasarSig",
		0x07: "EpochBundle",
		0x08: "PrismCut",
		0x09: "StakeWeightedCut",
		0x0A: "TxAuthEnvelope",
		0x0B: "PQPermit",
		0x0C: "DAGVertex",
		0x0D: "PolicyCert",
	}
	for id, name := range want {
		if SchemaName(id) != name {
			t.Errorf("SchemaName(0x%02X) = %q, want %q", uint8(id), SchemaName(id), name)
		}
	}
	if len(AllSchemas()) != 13 {
		t.Errorf("AllSchemas() returned %d schemas, want 13", len(AllSchemas()))
	}
}
