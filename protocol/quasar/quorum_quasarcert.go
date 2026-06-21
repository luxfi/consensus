// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_quasarcert.go — wiring the weighted quorum certificate into the
// QuasarCert envelope as the honest, full-node-verifiable realization of
// the per-validator-signature legs.
//
// A QuasarCert's MLDSARollup leg is, semantically, "a proof that every one
// of N validators produced a valid independent ML-DSA-65 signature over the
// round digest" (types.go). The STARK/Groth16 backend realizes that as a
// succinct proof. The WeightedQuorumCert realizes the SAME claim DIRECTLY:
// the cert carries the per-validator records and a full node verifies them
// itself (no trusted setup, no succinctness assumption). This is the
// "full-node-verifiable mode" — the conservative production posture.
//
// Likewise the Magnetar leg is a magnetar.ValidatorAggregateCert (N
// independent SLH-DSA signatures). A mixed-scheme WeightedQuorumCert can
// carry ML-DSA and SLH-DSA signer records side by side, so it is also the
// honest realization of a combined ML-DSA ∧ SLH-DSA per-validator surface.
//
// To keep the MLDSARollup slot unambiguous (it may carry a STARK proof OR a
// direct WQC), the embedded bytes are tagged with a 1-byte discriminator.
// Decoders dispatch on the tag; a STARK verifier and a WQC verifier never
// confuse each other's bytes.
package quasar

import (
	"errors"
	"fmt"
)

// mldsaRollupTagWQC is the 1-byte discriminator prefix that marks the
// MLDSARollup leg as carrying a direct WeightedQuorumCert (the
// full-node-verifiable realization) rather than a STARK/Groth16 succinct
// proof. The value 0x57 ('W') is chosen to be visually distinct and to lie
// outside the typical leading bytes of a STARK proof container; decoders
// MUST check this tag before parsing.
const mldsaRollupTagWQC byte = 0x57

var (
	// ErrMLDSARollupNotWQC is returned by ExtractWeightedQuorumLeg when the
	// MLDSARollup leg does not carry a WQC discriminator (e.g. it holds a
	// STARK proof or per-validator sig concatenation instead).
	ErrMLDSARollupNotWQC = errors.New("quasar: MLDSARollup leg is not a direct weighted quorum cert")

	// ErrQuasarCertNoWQCLeg is returned by VerifyWeightedQuorumLeg when the
	// cert carries no MLDSARollup leg at all.
	ErrQuasarCertNoWQCLeg = errors.New("quasar: QuasarCert has no MLDSARollup leg to verify")
)

// EmbedWeightedQuorumLeg encodes a WeightedQuorumCert as the MLDSARollup
// leg of a QuasarCert, tagged with the WQC discriminator. The cert is
// emitted in FULL form (records included) when it carries records, so the
// leg is self-contained and a full node can verify it without a DA fetch;
// pass a Compact() cert to emit the commitment-only leg (DA-hybrid mode),
// in which case verification requires re-attaching records first.
//
// Returns the leg bytes (discriminator || wqc-wire) to assign to
// QuasarCert.MLDSARollup. Pure function; no secrets.
func EmbedWeightedQuorumLeg(cert *WeightedQuorumCert) ([]byte, error) {
	if cert == nil {
		return nil, ErrQCNil
	}
	wire, err := cert.MarshalBinary()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, 1+len(wire))
	out = append(out, mldsaRollupTagWQC)
	out = append(out, wire...)
	return out, nil
}

// ExtractWeightedQuorumLeg decodes the WeightedQuorumCert from a QuasarCert
// MLDSARollup leg. Returns ErrMLDSARollupNotWQC if the leg is empty or does
// not carry the WQC discriminator (so a STARK-rollup leg is cleanly
// distinguished, not misparsed).
func ExtractWeightedQuorumLeg(mldsaRollup []byte) (*WeightedQuorumCert, error) {
	if len(mldsaRollup) == 0 || mldsaRollup[0] != mldsaRollupTagWQC {
		return nil, ErrMLDSARollupNotWQC
	}
	return UnmarshalWeightedQuorumCert(mldsaRollup[1:])
}

// HasWeightedQuorumLeg reports whether a QuasarCert's MLDSARollup leg
// carries a direct WeightedQuorumCert (vs a STARK proof or nothing).
func (c *QuasarCert) HasWeightedQuorumLeg() bool {
	if c == nil {
		return false
	}
	return len(c.MLDSARollup) > 0 && c.MLDSARollup[0] == mldsaRollupTagWQC
}

// VerifyWeightedQuorumLeg verifies the direct weighted quorum certificate
// carried in a QuasarCert's MLDSARollup leg against the chain envelope under
// cfg. This is the clean Verify entry point for the full-node-verifiable
// mode.
//
// It returns nil iff the leg is present, decodes as a WQC, and passes the
// full weighted-quorum predicate (proof-backend axis match + per-signer FIPS
// verify + Merkle inclusion + weight threshold floor + aggregate/commitment
// checks). Any failure is a typed error — never a panic, never unbounded
// work.
//
// envelope carries the chain's pinned posture axes; the extracted cert's
// Verify rebuilds the domain-separated signing message ITSELF from the
// envelope plus the cert's own position fields, so the caller never supplies
// an opaque message that could skip the position / threshold / proof-backend
// binding.
func (c *QuasarCert) VerifyWeightedQuorumLeg(envelope QuorumMessageEnvelope, cfg QuorumVerifierConfig) error {
	if c == nil {
		return ErrQCNil
	}
	if len(c.MLDSARollup) == 0 {
		return ErrQuasarCertNoWQCLeg
	}
	cert, err := ExtractWeightedQuorumLeg(c.MLDSARollup)
	if err != nil {
		return err
	}
	return cert.Verify(envelope, cfg)
}

// BuildQuasarCertFromWeightedQuorum builds a QuasarCert whose MLDSARollup
// leg is the direct full-node-verifiable WeightedQuorumCert. The other legs
// (BLS / Corona / Pulsar / Magnetar) are left empty — this is the pure
// "direct weighted quorum" posture where the per-validator-signature
// surface IS the finality evidence, with no threshold-group or classical
// fast-path leg layered on. Compose with the lower-tier ComposePulsar /
// ComposeAurora / ComposePolaris functions when a hybrid posture is wanted;
// those operate on the threshold legs, which are orthogonal to this one.
//
// Epoch / Validators / Finality are copied from the quorum cert so the
// QuasarCert header reflects the same epoch and signer count.
func BuildQuasarCertFromWeightedQuorum(cert *WeightedQuorumCert) (*QuasarCert, error) {
	if cert == nil {
		return nil, ErrQCNil
	}
	leg, err := EmbedWeightedQuorumLeg(cert)
	if err != nil {
		return nil, err
	}
	return &QuasarCert{
		MLDSARollup: leg,
		Epoch:       cert.Epoch,
		Validators:  int(cert.SignerCount),
	}, nil
}

// AttachWeightedQuorumLeg returns a copy of the QuasarCert with its
// MLDSARollup leg replaced by the direct WeightedQuorumCert, preserving the
// other legs. Use when layering the full-node-verifiable per-validator
// surface onto an already-composed cert (e.g. one that already carries the
// BLS classical fast-path or a Pulsar threshold leg).
func (c *QuasarCert) AttachWeightedQuorumLeg(cert *WeightedQuorumCert) (*QuasarCert, error) {
	if c == nil {
		return nil, errors.New("quasar: nil QuasarCert")
	}
	leg, err := EmbedWeightedQuorumLeg(cert)
	if err != nil {
		return nil, fmt.Errorf("attach weighted quorum leg: %w", err)
	}
	cp := *c
	cp.MLDSARollup = leg
	if cp.Epoch == 0 {
		cp.Epoch = cert.Epoch
	}
	if cp.Validators == 0 {
		cp.Validators = int(cert.SignerCount)
	}
	return &cp, nil
}
