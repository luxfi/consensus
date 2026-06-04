// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// polaris.go — Polaris cert profile composition + wire helpers.
//
// Maps the production papers/lux-quasar-composition profile registry
// (§04) into concrete Go primitives. The three production profiles
// share one cert struct (QuasarCert in types.go); the profile
// selector decides which legs are populated.
//
//	Pulsar profile  — BLS ‖ Puls ‖ ZK            (minimum PQ posture)
//	Aurora profile  — BLS ‖ Puls ‖ Cor ‖ ZK      (intra-lattice diversity)
//	Polaris profile — BLS ‖ Puls ‖ Cor ‖ Mag ‖ ZK (cross-family maximum)
//
// All three use the same wire format (CertSchemeQuasar = 0x05). The
// absent legs are encoded with zero-length frames; an in-band
// composition predicate (IsPolaris, IsDoubleLattice, HasHashBased)
// names what's present.
//
// Composition function (ComposePolaris) takes already-produced
// signatures from the four primitives (BLS aggregate, Pulsar-M
// threshold sig, Corona threshold sig, Magnetar standalone signatures)
// and builds the cert. Each primitive ships its own threshold protocol;
// composition is a pure-function layering on top.

package quasar

import (
	"errors"
	"time"

	wirezap "github.com/luxfi/consensus/pkg/wire/zap"
	"github.com/luxfi/crypto/bls"
	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
	pulsarwire "github.com/luxfi/pulsar/ref/go/pkg/pulsar"
	corona "github.com/luxfi/threshold/protocols/corona"
	pulsar "github.com/luxfi/threshold/protocols/pulsar"
)

// ErrPolarisMissingLeg is returned by ComposePolaris when any of the
// three required PQ legs is empty. Polaris by definition is the
// all-three-legs profile; populate a partial cert via the lower-tier
// composition functions (ComposePulsar / ComposeAurora).
var ErrPolarisMissingLeg = errors.New("quasar: Polaris cert requires Pulsar, Corona, and Magnetar legs")

// PolarisLegs carries the four signature inputs and the validator-set
// metadata for one Polaris-profile cert. Each leg is the OUTPUT of an
// already-completed threshold (or per-validator) signing ceremony on
// the round digest; this struct is the pure-function input to
// ComposePolaris.
//
// The BLS leg may be nil for a pure-PQ Polaris posture (no classical
// fast-path). The papers' profile §04 invariant "BLS classical leg is
// always populated" applies to hybrid Polaris; pure-PQ Polaris drops
// it and the cert's HasClassicalFastPath() returns false.
type PolarisLegs struct {
	// BLS is the BLS-12-381 aggregate signature bytes (the classical
	// fast-path leg). nil/empty for pure-PQ profiles.
	BLS *bls.Signature

	// Pulsar is the Module-LWE threshold signature for the round.
	// Produced by the pulsar.ThresholdSigner ceremony.
	Pulsar *pulsar.Signature

	// Corona is the Ring-LWE threshold signature for the round.
	// Produced by the corona/threshold.Signer ceremony.
	Corona *corona.Signature

	// Magnetar carries the per-validator standalone SLH-DSA aggregate
	// over the round digest. Built via magnetar.BuildAggregateCert
	// from individual ValidatorSign outputs.
	Magnetar *magnetar.ValidatorAggregateCert

	// MLDSARollup is the succinct STARK/Groth16 attestation of N
	// per-validator ML-DSA-65 signatures. Optional; populated when
	// the Z-Chain rollup path is wired.
	MLDSARollup []byte

	// Epoch is the consensus epoch this cert finalises.
	Epoch uint64

	// Finality is the wall-clock time at which the cert was assembled.
	Finality time.Time

	// Validators is the count of distinct signing validators across
	// the configured legs. Bound into the cert for header inspection
	// only — verification routes through the per-leg quorum predicates.
	Validators int
}

// ComposePolaris builds a Polaris-profile QuasarCert from four legs.
//
// Pure function: given the same inputs the same cert bytes come out.
// No mutable state, no randomness, no hidden defaults.
//
// Returns ErrPolarisMissingLeg if any of {Pulsar, Corona, Magnetar} is
// nil. The BLS leg may be nil (pure-PQ Polaris). The MLDSARollup may
// be empty (rollup not wired in the calling deployment).
func ComposePolaris(legs PolarisLegs) (*QuasarCert, error) {
	if legs.Pulsar == nil || legs.Corona == nil || legs.Magnetar == nil {
		return nil, ErrPolarisMissingLeg
	}

	pulsarBytes, err := legs.Pulsar.MarshalBinary()
	if err != nil {
		return nil, err
	}
	coronaBytes, err := legs.Corona.MarshalBinary()
	if err != nil {
		return nil, err
	}
	magBytes, err := EncodeMagnetarAggregate(legs.Magnetar)
	if err != nil {
		return nil, err
	}

	var blsBytes []byte
	if legs.BLS != nil {
		blsBytes = bls.SignatureToBytes(legs.BLS)
	}

	return &QuasarCert{
		BLS:         blsBytes,
		Corona:      coronaBytes,
		Pulsar:      pulsarBytes,
		Magnetar:    magBytes,
		MLDSARollup: append([]byte(nil), legs.MLDSARollup...),
		Epoch:       legs.Epoch,
		Finality:    legs.Finality,
		Validators:  legs.Validators,
	}, nil
}

// verifyPulsarLeg routes the cert's Pulsar bytes through the pulsar
// package's stateless VerifyBytes path. Returns false if the leg
// fails or the group key is missing/empty.
func verifyPulsarLeg(message []byte, groupKey []byte, pulsarSigBytes []byte) bool {
	if len(groupKey) == 0 || len(pulsarSigBytes) == 0 {
		return false
	}
	return pulsarwire.VerifyBytes(groupKey, message, pulsarSigBytes)
}

// EncodeMagnetarAggregate serialises a magnetar.ValidatorAggregateCert
// into LP-182 schema 0x04 ZAP wire-bytes, embedded in a QuasarCert's
// Magnetar slot.
//
// The three parallel slices (Signers, PubKeys, Sigs) are concatenated
// into three variable-length ZAP byte fields. Per-element byte widths
// are derived from magnetar.ParamsFor(Mode) at both encode and decode,
// so a flipped mode byte changes the expected stride and rejects in
// DecodeMagnetarAggregate before any signature dispatch.
//
// Returns magnetar.ErrAggregateCertShape if the cert's parallel slices
// are misaligned.
func EncodeMagnetarAggregate(cert *magnetar.ValidatorAggregateCert) ([]byte, error) {
	if cert == nil {
		return nil, magnetar.ErrAggregateCertEmpty
	}
	params, err := magnetar.ParamsFor(cert.Mode)
	if err != nil {
		return nil, err
	}
	n := len(cert.Signers)
	if n == 0 {
		return nil, magnetar.ErrAggregateCertEmpty
	}
	if n != len(cert.PubKeys) || n != len(cert.Sigs) {
		return nil, magnetar.ErrAggregateCertShape
	}
	for i := 0; i < n; i++ {
		if len(cert.PubKeys[i]) != params.PublicKeySize {
			return nil, magnetar.ErrAggregateCertShape
		}
		if len(cert.Sigs[i]) != params.SignatureSize {
			return nil, magnetar.ErrAggregateCertShape
		}
	}

	signers := make([]byte, 0, n*32)
	for i := 0; i < n; i++ {
		signers = append(signers, cert.Signers[i][:]...)
	}
	pubKeys := make([]byte, 0, n*params.PublicKeySize)
	for i := 0; i < n; i++ {
		pubKeys = append(pubKeys, cert.PubKeys[i]...)
	}
	sigs := make([]byte, 0, n*params.SignatureSize)
	for i := 0; i < n; i++ {
		sigs = append(sigs, cert.Sigs[i]...)
	}

	return wirezap.NewMagnetarAggregateCert(wirezap.MagnetarAggregateFields{
		Mode:    uint8(cert.Mode),
		Count:   uint32(n),
		Signers: signers,
		PubKeys: pubKeys,
		Sigs:    sigs,
	}).Bytes(), nil
}

// DecodeMagnetarAggregate is the inverse of EncodeMagnetarAggregate.
// Wraps LP-182 schema 0x04 ZAP wire-bytes and projects them back into
// a magnetar.ValidatorAggregateCert.
//
// Strict shape policy: the concatenated Signers / PubKeys / Sigs byte
// lengths MUST equal Count * stride for the declared Mode. Anything
// else rejects as malformed.
func DecodeMagnetarAggregate(data []byte) (*magnetar.ValidatorAggregateCert, error) {
	wrap, err := wirezap.WrapMagnetarAggregateCert(data)
	if err != nil {
		return nil, ErrCertCorrupt
	}
	mode := magnetar.Mode(wrap.Mode())
	params, perr := magnetar.ParamsFor(mode)
	if perr != nil {
		return nil, ErrCertCorrupt
	}
	n := int(wrap.Count())
	if n == 0 {
		return nil, ErrCertCorrupt
	}
	signersBytes := wrap.Signers()
	pubKeysBytes := wrap.PubKeys()
	sigsBytes := wrap.Sigs()
	if len(signersBytes) != n*32 ||
		len(pubKeysBytes) != n*params.PublicKeySize ||
		len(sigsBytes) != n*params.SignatureSize {
		return nil, ErrCertCorrupt
	}
	signers := make([]magnetar.NodeID, n)
	for i := 0; i < n; i++ {
		copy(signers[i][:], signersBytes[i*32:(i+1)*32])
	}
	pubKeys := make([][]byte, n)
	for i := 0; i < n; i++ {
		pk := make([]byte, params.PublicKeySize)
		copy(pk, pubKeysBytes[i*params.PublicKeySize:(i+1)*params.PublicKeySize])
		pubKeys[i] = pk
	}
	sigs := make([][]byte, n)
	for i := 0; i < n; i++ {
		sg := make([]byte, params.SignatureSize)
		copy(sg, sigsBytes[i*params.SignatureSize:(i+1)*params.SignatureSize])
		sigs[i] = sg
	}
	return &magnetar.ValidatorAggregateCert{
		Mode:    mode,
		Signers: signers,
		PubKeys: pubKeys,
		Sigs:    sigs,
	}, nil
}
