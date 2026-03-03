// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Parallel finality witness producers for Quasar (LP-020).
//
// Lux Quasar finality is layered, parallel witnesses. P-Chain BLS is the
// always-on finality witness. Q-Chain (Ringtail threshold) and Z-Chain
// (MLDSAGroth16 rollup) are independently toggleable parallel witnesses
// that produce additional finality artifacts at the same round-rate as P.
//
// Each round, the consensus driver computes a 32-byte round digest binding
// chain id, epoch, round, mode, and parent state. It then asks each enabled
// witness producer for that round's witness in parallel. Witnesses do not
// pipeline -- adding Q and/or Z does not change finality latency, only
// parallel verification cost.

package quasar

import (
	"context"
	"errors"
)

// RoundDigest is the 32-byte certificate subject for a consensus round.
// See LP-020 §2.3 (Definition: Certificate Subject).
type RoundDigest [32]byte

// ErrWitnessUnavailable is returned by a witness producer when the underlying
// chain cannot produce a witness for the given round (e.g. Q-Chain DKG not
// complete, Z-Chain prover missed deadline). The consensus driver downgrades
// to the next-lower witness set per operator policy when this occurs.
var ErrWitnessUnavailable = errors.New("parallel witness unavailable for round")

// PWitnessProducer produces P-Chain BLS aggregate witnesses. Always required.
//
// Implementations live in P-Chain's consensus path; this interface exists for
// symmetry with Q and Z and lets test harnesses substitute fakes.
type PWitnessProducer interface {
	// Witness returns a BLS12-381 aggregate signature plus a signer bitmap
	// over the round digest. Returns ErrWitnessUnavailable on quorum failure.
	Witness(ctx context.Context, digest RoundDigest) (sig []byte, signers []byte, err error)
}

// QWitnessProducer produces Q-Chain Ringtail threshold-signature witnesses.
//
// The Q-Chain VM (chains/quantumvm) implements this by driving a 2-round
// Ringtail threshold ceremony per consensus round once a t-of-n DKG has
// produced the combined public key recorded in qchain_ceremony_root.
type QWitnessProducer interface {
	// Witness returns a Ringtail threshold signature over the round digest,
	// or ErrWitnessUnavailable if the ceremony fails or quorum is missed.
	Witness(ctx context.Context, digest RoundDigest) ([]byte, error)
}

// ZWitnessProducer produces Z-Chain MLDSAGroth16 rollup witnesses.
//
// The Z-Chain VM (chains/zkvm) implements this by collecting per-validator
// ML-DSA-65 signatures over the round digest and producing a single Groth16
// proof attesting "for every i in [N], MLDSA.Verify(pk_i, digest, sig_i) = 1".
//
// The validator ML-DSA public-key list is bound to pchain_validator_root for
// the round; the prover takes those keys as a public input. The proof is
// verified by the Groth16 (bn254) precompile on Z-Chain.
type ZWitnessProducer interface {
	// Witness returns a Groth16 proof aggregating per-validator ML-DSA-65
	// signatures over the round digest. validatorMLDSAPubs is the canonical
	// public-key list rooted in pchain_validator_root for the round.
	Witness(ctx context.Context, digest RoundDigest, validatorMLDSAPubs [][]byte) ([]byte, error)
}

// WitnessSet bundles the witness producers configured for a network. Nil Q
// and/or Z producers are valid; the driver simply skips them and produces
// the corresponding lower-level certificate (PolicyQuorum, PolicyPQ, or
// PolicyPZ instead of PolicyQuantum).
type WitnessSet struct {
	P PWitnessProducer
	Q QWitnessProducer // optional
	Z ZWitnessProducer // optional
}

// RoundWitnesses is the result of running a WitnessSet for one round.
// Q and/or Z may be nil if their producer was disabled, returned
// ErrWitnessUnavailable, or missed the round deadline.
type RoundWitnesses struct {
	PSig     []byte
	PSigners []byte
	Q        []byte // nil if no Q producer or unavailable
	Z        []byte // nil if no Z producer or unavailable
}

// Run executes the configured witness producers in parallel against digest.
// P is mandatory: a P failure aborts the round. Q and Z run concurrently;
// if either returns ErrWitnessUnavailable (or any error), its slot is left
// nil and the round still finalizes at the next-lower witness level.
//
// The caller is responsible for bounding ctx with the round window.
func (ws WitnessSet) Run(ctx context.Context, digest RoundDigest, validatorMLDSAPubs [][]byte) (*RoundWitnesses, error) {
	if ws.P == nil {
		return nil, errors.New("WitnessSet: P producer required")
	}

	type qz struct {
		sig []byte
		err error
	}
	qch := make(chan qz, 1)
	zch := make(chan qz, 1)

	go func() {
		if ws.Q == nil {
			qch <- qz{}
			return
		}
		sig, err := ws.Q.Witness(ctx, digest)
		qch <- qz{sig: sig, err: err}
	}()

	go func() {
		if ws.Z == nil {
			zch <- qz{}
			return
		}
		sig, err := ws.Z.Witness(ctx, digest, validatorMLDSAPubs)
		zch <- qz{sig: sig, err: err}
	}()

	pSig, pSigners, err := ws.P.Witness(ctx, digest)
	if err != nil {
		return nil, err
	}

	q := <-qch
	z := <-zch

	out := &RoundWitnesses{PSig: pSig, PSigners: pSigners}
	if q.err == nil {
		out.Q = q.sig
	}
	if z.err == nil {
		out.Z = z.sig
	}
	return out, nil
}

// DisabledZWitnessProducer is the explicit "no Z lane" sentinel producer.
// Networks that do not run a Z-Chain Groth16 prover (smaller deployments,
// classical-only configurations, or chains during their pre-Z bootstrap
// window) install this producer; the driver treats every call as
// ErrWitnessUnavailable and finalises at the next-lower witness set.
//
// The chains/zkvm package supplies the active-Z producer that proves
// "for each i, MLDSA.Verify(pk_i, digest, sig_i) = 1" via Groth16/bn254
// with the validator ML-DSA pubkey list as a public input. See LP-020 §6
// and proofs/quasar-cert-soundness.tex App. B for the R1CS constraint
// count and prover-cost analysis.
type DisabledZWitnessProducer struct{}

// Witness always returns ErrWitnessUnavailable so the driver downgrades
// uniformly with the rest of the optional-lane fallback path.
func (DisabledZWitnessProducer) Witness(ctx context.Context, digest RoundDigest, validatorMLDSAPubs [][]byte) ([]byte, error) {
	return nil, ErrWitnessUnavailable
}
