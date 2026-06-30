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
	"encoding/binary"
	"errors"
	"time"

	"github.com/luxfi/crypto/bls"
	magnetar "github.com/luxfi/magnetar/ref/go/pkg/magnetar"
	pulsarwire "github.com/luxfi/pulsar/pkg/pulsar"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
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
	Pulsar *pulsar.Signature

	// Corona is the Ring-LWE threshold signature for the round.
	// Produced by the corona/threshold.Signer ceremony.
	Corona *coronaThreshold.Signature

	// Magnetar carries the per-validator standalone SLH-DSA aggregate
	// over the round digest. Built via magnetar.BuildAggregateCert
	// from individual ValidatorSign outputs.
	Magnetar *magnetar.ValidatorAggregateCert

	// MLDSARollup is the succinct strict-PQ STARK/FRI attestation of N
	// per-validator ML-DSA-65 signatures (via P3Q). Optional; populated
	// when the Z-Chain rollup prover is wired.
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
// into the canonical wire form embedded in a QuasarCert's Magnetar
// slot.
//
// Wire layout (big-endian throughout):
//
//	mode(1)
//	signer_count(4)
//	signer_id[i] for i in [0, N): 32 bytes each
//	pubkey[i] for i in [0, N): PublicKeySize(mode) bytes each
//	sig[i] for i in [0, N): SignatureSize(mode) bytes each
//
// All four shape fields are tightly bound: a flipped mode byte
// changes the per-entry byte width and rejects in DecodeMagnetarAggregate
// before any signature dispatch.
//
// Returns ErrAggregateCertShape if the cert's parallel slices are
// misaligned.
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

	total := 1 + 4 + n*(32+params.PublicKeySize+params.SignatureSize)
	out := make([]byte, 0, total)
	out = append(out, byte(cert.Mode))

	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(n))
	out = append(out, u32[:]...)

	for i := 0; i < n; i++ {
		out = append(out, cert.Signers[i][:]...)
	}
	for i := 0; i < n; i++ {
		out = append(out, cert.PubKeys[i]...)
	}
	for i := 0; i < n; i++ {
		out = append(out, cert.Sigs[i]...)
	}
	return out, nil
}

// DecodeMagnetarAggregate is the inverse of EncodeMagnetarAggregate.
// Strict trailing-bytes policy: any byte left after the declared
// frame rejects the cert as malformed (matches pulsar / corona /
// magnetar wire policy).
func DecodeMagnetarAggregate(data []byte) (*magnetar.ValidatorAggregateCert, error) {
	if len(data) < 5 {
		return nil, ErrCertCorrupt
	}
	mode := magnetar.Mode(data[0])
	params, err := magnetar.ParamsFor(mode)
	if err != nil {
		return nil, ErrCertCorrupt
	}
	n := int(binary.BigEndian.Uint32(data[1:5]))
	if n == 0 {
		return nil, ErrCertCorrupt
	}
	want := 5 + n*(32+params.PublicKeySize+params.SignatureSize)
	if len(data) != want {
		return nil, ErrCertCorrupt
	}
	off := 5
	signers := make([]magnetar.NodeID, n)
	for i := 0; i < n; i++ {
		copy(signers[i][:], data[off:off+32])
		off += 32
	}
	pubKeys := make([][]byte, n)
	for i := 0; i < n; i++ {
		pk := make([]byte, params.PublicKeySize)
		copy(pk, data[off:off+params.PublicKeySize])
		pubKeys[i] = pk
		off += params.PublicKeySize
	}
	sigs := make([][]byte, n)
	for i := 0; i < n; i++ {
		sg := make([]byte, params.SignatureSize)
		copy(sg, data[off:off+params.SignatureSize])
		sigs[i] = sg
		off += params.SignatureSize
	}
	return &magnetar.ValidatorAggregateCert{
		Mode:    mode,
		Signers: signers,
		PubKeys: pubKeys,
		Sigs:    sigs,
	}, nil
}
