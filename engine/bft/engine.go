// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package bft is the sibling-engine adapter that lets the consensus
// toolkit expose `luxfi/bft` (epoch-based leader-rotation BFT, Simplex
// family) alongside Quasar's metastable engines (NewChain / NewDAG /
// NewPQ).
//
// The four engine factories cover four orthogonal consensus paradigms:
//
//	NewChain — linear Snow-family metastable (Nova)
//	NewDAG   — DAG Snow-family metastable (Nebula)
//	NewPQ    — post-quantum threshold Quasar
//	NewBFT   — classical leader-rotation BFT (this package)
//
// The Engine here satisfies github.com/luxfi/consensus/engine/interfaces.Engine
// by wrapping a *bft.Epoch and routing Start/Stop/HealthCheck/
// IsBootstrapped through the underlying epoch lifecycle.
//
// Orthogonal axes:
//
//	consensus paradigm       — picks engine (Chain/DAG/PQ/BFT)
//	ChainSecurityProfile     — picks admissible primitives (strict/permissive/fips)
//	FinalitySchemeID         — picks the threshold kernel (Pulsar/Corona/MPTC-MLDSA)
//
// A BFT engine under a strict profile with Pulsar-M-65 finality is a
// valid production combination: leader-rotation ordering with
// PQ-threshold-signed cert envelopes.
//
// CR-11 PQ envelope enforcement: under a strict-PQ / FIPS profile,
// NewEngineWithProfile refuses any bft.Signer that doesn't satisfy the
// PQSigner marker. The check is structural at construction — a
// misconfigured chain fails to launch rather than running an Ed25519
// leader-rotation chain that believes it's PQ-signed.
package bft

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	"github.com/luxfi/bft"

	"github.com/luxfi/consensus/config"
)

// PQSigner is the marker interface a bft.Signer implements to declare
// itself PQ-grade for CR-11. Implementations sign with a lattice scheme
// (ML-DSA-65 / Pulsar-M-65) rather than classical Ed25519 / ECDSA.
//
// Any concrete PQ signer in luxfi/* embeds this interface or implements
// it directly. The marker has zero runtime cost — it's a compile-time
// assertion that the operator wired in a vetted PQ signer.
//
// Implementations live in caller code (the node-side adapter that
// constructs the bft.Epoch). The wrapper here only checks the type
// at construction; it does not produce signatures itself.
type PQSigner interface {
	bft.Signer

	// PQSchemeID returns the wire byte identifying which lattice scheme
	// this signer emits (ML-DSA-65, ML-DSA-87, Pulsar-M-65, ...). Bound
	// into the cert transcript by the BFT layer's vote/finalisation
	// path so a flipped byte breaks signature verification.
	PQSchemeID() config.SigSchemeID
}

// PQVerifier is the marker interface a bft.SignatureVerifier implements
// to declare itself PQ-grade. Pair to PQSigner — chain operators that
// pin a strict-PQ profile must wire both sides.
type PQVerifier interface {
	bft.SignatureVerifier

	// PQSchemeID returns the wire byte identifying which lattice scheme
	// this verifier accepts. MUST match the Signer's scheme on a single
	// chain — the adapter checks both bytes at construction.
	PQSchemeID() config.SigSchemeID
}

// ErrClassicalSignerUnderStrictPQ is returned by NewEngineWithProfile
// when the chain's ChainSecurityProfile is strict-PQ or FIPS but the
// bft.Signer wired into the Epoch is not a PQSigner. Closes CR-11:
// previously the wrapper accepted any Signer/Verifier and ran the
// epoch with an Ed25519 leader-rotation kernel even on a strict-PQ
// chain.
var ErrClassicalSignerUnderStrictPQ = errors.New(
	"engine/bft: strict-PQ profile refuses a classical bft.Signer; install a PQSigner")

// ErrClassicalVerifierUnderStrictPQ mirrors ErrClassicalSignerUnderStrictPQ
// for the verifier side. Returned when the bft.SignatureVerifier does
// not implement PQVerifier under a strict-PQ profile.
var ErrClassicalVerifierUnderStrictPQ = errors.New(
	"engine/bft: strict-PQ profile refuses a classical bft.SignatureVerifier; install a PQVerifier")

// ErrPQSchemeMismatch is returned when both sides implement PQSigner /
// PQVerifier but advertise different SigSchemeIDs. A chain MUST sign
// and verify under the same scheme; a mismatch is a misconfiguration
// the adapter surfaces at construction.
var ErrPQSchemeMismatch = errors.New(
	"engine/bft: PQSigner and PQVerifier advertise different SigSchemeIDs")

// Engine is the consensus-toolkit-facing wrapper around a bft.Epoch.
// Constructed via NewEngine; the returned value satisfies
// github.com/luxfi/consensus/engine/interfaces.Engine.
//
// The wrapper keeps the BFT epoch encapsulated so callers do not need
// to know whether they hold a Chain/DAG/PQ/BFT engine — the engine
// surface is uniform.
type Engine struct {
	epoch        *bft.Epoch
	bootstrapped atomic.Bool
	stopCh       chan struct{}
	stopOnce     atomic.Bool
}

// NewEngine wraps a bft.Epoch as a consensus.Engine. The caller is
// responsible for constructing the Epoch with a fully-wired
// EpochConfig (Storage, Communication, Signer, BlockBuilder, WAL,
// etc.); this adapter takes ownership of its lifecycle.
//
// Returns nil if epoch is nil — every other call site assumes the
// embedded epoch is non-nil after construction.
//
// NewEngine performs no PQ-envelope check. Production callers under a
// strict-PQ profile MUST use NewEngineWithProfile, which refuses a
// classical bft.Signer at construction (CR-11). NewEngine is the
// non-strict / legacy / test path.
func NewEngine(epoch *bft.Epoch) *Engine {
	if epoch == nil {
		return nil
	}
	return &Engine{
		epoch:  epoch,
		stopCh: make(chan struct{}),
	}
}

// NewEngineWithProfile wraps a bft.Epoch as a consensus.Engine while
// enforcing the chain's ChainSecurityProfile at construction (CR-11).
//
// Under a strict-PQ / FIPS profile the embedded epoch's Signer and
// SignatureVerifier MUST satisfy PQSigner / PQVerifier — i.e. produce
// and accept lattice signatures (ML-DSA-65 / Pulsar-M-65), not
// classical Ed25519. A misconfiguration surfaces as a typed error here,
// not as silently classical signatures riding on a strict-PQ chain.
//
// Non-strict profiles (Permissive, nil) preserve the existing path:
// any Signer / Verifier is admitted.
//
// Returns nil + ErrNilEpoch when epoch is nil; nil + the strict-PQ
// sentinel when the profile demands PQ and the signer/verifier is
// classical; otherwise a wrapper ready for Start.
func NewEngineWithProfile(epoch *bft.Epoch, profile *config.ChainSecurityProfile) (*Engine, error) {
	if epoch == nil {
		return nil, ErrNilEpoch
	}

	// Non-strict profiles preserve the legacy admission rules.
	if profile == nil || !profile.IsPQ() {
		return &Engine{
			epoch:  epoch,
			stopCh: make(chan struct{}),
		}, nil
	}

	// Strict-PQ / FIPS: refuse any classical signer or verifier.
	pqSigner, sok := epoch.Signer.(PQSigner)
	if !sok {
		return nil, ErrClassicalSignerUnderStrictPQ
	}
	pqVerifier, vok := epoch.Verifier.(PQVerifier)
	if !vok {
		return nil, ErrClassicalVerifierUnderStrictPQ
	}

	// Both sides are PQ — but they must advertise the SAME scheme.
	// A signer at ML-DSA-65 with a verifier at ML-DSA-87 is a
	// misconfiguration; sign/verify will fail at runtime, but the
	// adapter catches it now so chains fail to launch rather than
	// crash on first vote.
	if pqSigner.PQSchemeID() != pqVerifier.PQSchemeID() {
		return nil, ErrPQSchemeMismatch
	}

	return &Engine{
		epoch:  epoch,
		stopCh: make(chan struct{}),
	}, nil
}

// ErrNilEpoch is returned by Start when the wrapper holds no Epoch.
// Should never happen in practice — NewEngine refuses nil — but
// callers that construct the struct literally must still handle it.
var ErrNilEpoch = errors.New("consensus/engine/bft: epoch is nil")

// Start launches the wrapped Epoch and marks the engine as
// bootstrapped. The requestID argument exists for engine-interface
// compatibility with the Quasar engines; BFT epochs do not consume
// it but the parameter is preserved so a caller can swap engines
// without changing call sites.
//
// One-way: Start may be called once. Subsequent Starts are no-ops.
func (e *Engine) Start(ctx context.Context, _ uint32) error {
	if e == nil || e.epoch == nil {
		return ErrNilEpoch
	}
	if e.bootstrapped.Load() {
		return nil
	}
	if err := e.epoch.Start(); err != nil {
		return err
	}
	e.bootstrapped.Store(true)
	return nil
}

// Stop halts the wrapped Epoch. Idempotent: subsequent Stops are
// no-ops. After Stop, IsBootstrapped continues to return true so the
// caller can distinguish "never started" from "started and stopped".
func (e *Engine) Stop(_ context.Context) error {
	if e == nil || e.epoch == nil {
		return nil
	}
	if !e.stopOnce.CompareAndSwap(false, true) {
		return nil
	}
	e.epoch.Stop()
	close(e.stopCh)
	return nil
}

// HealthCheck reports whether the wrapped Epoch is in a usable state.
// Returns a small map suitable for the engine-level health endpoint.
// The wrapper does not introspect the Epoch's internal round/wal
// state — bft does not expose that surface. A caller that needs
// deeper introspection should reach into the underlying bft.Epoch
// directly through Epoch().
func (e *Engine) HealthCheck(_ context.Context) (interface{}, error) {
	if e == nil || e.epoch == nil {
		return nil, ErrNilEpoch
	}
	stopped := e.stopOnce.Load()
	bootstrapped := e.bootstrapped.Load()
	return map[string]any{
		"engine":       "bft",
		"bootstrapped": bootstrapped,
		"stopped":      stopped,
	}, nil
}

// IsBootstrapped reports whether Start has completed at least once.
// Mirrors the contract of the Quasar engines: true means "the engine
// has finished its bootstrap phase and is now processing rounds."
func (e *Engine) IsBootstrapped() bool {
	if e == nil {
		return false
	}
	return e.bootstrapped.Load()
}

// Epoch returns the underlying bft.Epoch for callers that need to
// inject messages or advance time. The consensus.Engine surface is
// intentionally narrow; this accessor is the escape hatch.
//
// Returns nil if the wrapper was constructed without an Epoch.
func (e *Engine) Epoch() *bft.Epoch {
	if e == nil {
		return nil
	}
	return e.epoch
}

// AdvanceTime forwards the wall clock to the wrapped Epoch. Exposed
// at the wrapper layer because the engine.Engine interface itself
// does not have an AdvanceTime hook — clock-driven engines like BFT
// need this access point and Quasar engines don't.
func (e *Engine) AdvanceTime(t time.Time) {
	if e == nil || e.epoch == nil {
		return
	}
	e.epoch.AdvanceTime(t)
}

// HandleMessage forwards an incoming consensus message to the wrapped
// Epoch. Sibling to AdvanceTime: clock-driven engines surface this
// hook through the wrapper, sampling-driven Quasar engines don't.
func (e *Engine) HandleMessage(msg *bft.Message, from bft.NodeID) error {
	if e == nil || e.epoch == nil {
		return ErrNilEpoch
	}
	return e.epoch.HandleMessage(msg, from)
}
