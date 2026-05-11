// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

// wave_signer.go -- thin adapter that drives a Pulsar threshold-sign
// ceremony from one Lux round (one Wave Tick over a Prism.Cut sample).
//
// Architecture (matches pulsar/spec/system-model.tex "Committee
// selection and PQ rollup"):
//
//   1. Prism.Cut.Sample(K) picks a fresh K-validator committee from
//      the validator pool each Lux round.
//   2. The K sampled validators run Pulsar threshold (3, 2) (or any
//      (T, K) with T <= K) on the round's item.
//   3. The output is a single FIPS 204 ML-DSA signature on the item.
//   4. Many such per-round signatures roll up via P3Q at the
//      Z-Chain envelope path (consensus/protocol/zchain) to produce
//      the final cert.
//
// This file is the minimum viable surface. The full Wave + Focus
// driver wiring (β-confidence amplification, multi-round
// accumulation) belongs at the Wave consumer layer.

import (
	"context"
	"crypto/rand"
	"errors"

	"github.com/luxfi/consensus/protocol/prism"
	"github.com/luxfi/ids"
	"github.com/luxfi/pulsar-m/ref/go/pkg/pulsarm"
	"golang.org/x/crypto/sha3"
)

// RoundResult is the output of one Lux round of Pulsar threshold
// signing: a FIPS 204 ML-DSA signature on the round item under the
// committee's shared group public key.
type RoundResult struct {
	// Item is the item that was signed (the value passed to the Wave
	// round).
	Item []byte
	// GroupPubkey is the Pulsar group public key (FIPS 204 ML-DSA
	// public key format) under which the signature verifies.
	GroupPubkey *pulsarm.PublicKey
	// Signature is the FIPS 204 ML-DSA signature bytes -- byte-equal
	// to what a single-party FIPS 204 signer would emit on the same
	// (group pubkey, item) pair.
	Signature *pulsarm.Signature
	// Committee is the K-validator sample that produced the
	// signature, in canonical NodeID order.
	Committee []pulsarm.NodeID
}

// RoundSigner drives one Lux round of Pulsar (T, K) threshold
// signing using a Prism.Cut to pick the K-validator committee.
type RoundSigner struct {
	Params    *pulsarm.Params
	Cut       prism.Cut[ids.NodeID] // K-sampler over the validator pool
	K         int                   // Sample size
	Threshold int                   // T -- minimum honest signers to combine
	// Shares maps each potential validator NodeID to its Pulsar
	// KeyShare. The K-sample pulls from this map; missing entries
	// cause that validator to be skipped (it cannot contribute).
	Shares map[ids.NodeID]*pulsarm.KeyShare
	// PrecompileCtx is the FIPS 204 context string the resulting
	// signature is bound to. Pass `[]byte("lux-evm-precompile-pulsar-v1")`
	// for on-chain Pulsar precompile verification; pass a different
	// chain-specific tag for in-protocol verification. Empty / nil
	// produces a context-free signature.
	PrecompileCtx []byte
}

// RunRound performs one Lux round of Pulsar threshold signing.
// Returns the FIPS 204 ML-DSA signature on item under the group
// public key. The signature verifies through unmodified FIPS 204
// ML-DSA.Verify(group_pk, item, signer.PrecompileCtx, sig).
func (s *RoundSigner) RunRound(ctx context.Context, item []byte) (*RoundResult, error) {
	if s.Cut == nil {
		return nil, errors.New("RoundSigner: Cut is nil")
	}
	if s.K < s.Threshold || s.Threshold < 1 {
		return nil, errors.New("RoundSigner: invalid (T, K)")
	}
	// Sample K validators for this Lux round.
	sampled := s.Cut.Sample(s.K)
	if len(sampled) < s.Threshold {
		return nil, errors.New("RoundSigner: sample short of threshold")
	}

	// Translate to Pulsar NodeIDs and collect shares.
	committee := make([]pulsarm.NodeID, 0, len(sampled))
	allShares := make([]*pulsarm.KeyShare, 0, len(sampled))
	for _, id := range sampled {
		var pid pulsarm.NodeID
		copy(pid[:], id[:])
		share, ok := s.Shares[id]
		if !ok {
			continue
		}
		committee = append(committee, pid)
		allShares = append(allShares, share)
		if len(committee) == s.K {
			break
		}
	}
	if len(committee) < s.Threshold {
		return nil, errors.New("RoundSigner: too few shares to meet threshold")
	}
	// Take the first T as the signing quorum (deterministic; the
	// Wave consumer can re-sample if a member is unavailable).
	quorum := committee[:s.Threshold]
	groupPK := allShares[0].Pub

	// Per-round session id binds the Pulsar PRNG to this Lux round.
	// The Wave consumer typically derives this from
	// (epoch, round-counter, item-hash, committee-root); here we
	// take the leading 16 bytes of SHA3-256 over the item + first
	// committee NodeID to keep the signature deterministic across
	// replays of the same Lux round.
	var sessionID [16]byte
	h := sha3.NewLegacyKeccak256()
	h.Write(item)
	h.Write(committee[0][:])
	copy(sessionID[:], h.Sum(nil))

	// Reverse map: pulsarm.NodeID -> ids.NodeID via byte copy, used
	// to look up the share-owner's KeyShare. Both types alias to a
	// 32-byte array but Go's type system requires an explicit copy.
	idsByPulsarID := make(map[pulsarm.NodeID]ids.NodeID, len(committee))
	for id := range s.Shares {
		var pid pulsarm.NodeID
		copy(pid[:], id[:])
		idsByPulsarID[pid] = id
	}

	// Spin up a ThresholdSigner per quorum member.
	signers := make([]*pulsarm.ThresholdSigner, s.Threshold)
	for i := 0; i < s.Threshold; i++ {
		ts, err := pulsarm.NewThresholdSigner(
			s.Params, sessionID, 0, quorum,
			s.Shares[idsByPulsarID[quorum[i]]], item, rand.Reader,
		)
		if err != nil {
			return nil, err
		}
		signers[i] = ts
	}

	// Round 1 -- commits + MACs.
	r1 := make([]*pulsarm.Round1Message, s.Threshold)
	for i, ts := range signers {
		m, err := ts.Round1(item)
		if err != nil {
			return nil, err
		}
		r1[i] = m
	}

	// Round 2 -- reveals.
	r2 := make([]*pulsarm.Round2Message, s.Threshold)
	for i, ts := range signers {
		m, _, err := ts.Round2(r1)
		if err != nil {
			return nil, err
		}
		r2[i] = m
	}

	// Combine -- single FIPS 204 ML-DSA signature output. Pass the
	// caller's precompile / verifier context so the signature is
	// bound to its intended verifier.
	sig, err := pulsarm.Combine(
		s.Params, groupPK, item, s.PrecompileCtx, false,
		sessionID, 0, quorum, s.Threshold, r1, r2, allShares,
	)
	if err != nil {
		return nil, err
	}

	return &RoundResult{
		Item:        item,
		GroupPubkey: groupPK,
		Signature:   sig,
		Committee:   committee,
	}, nil
}
