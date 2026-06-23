// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// quorum_guard.go — the fail-closed gate that forbids real-value DEX activation
// on an engine that cannot prove α-of-K quorum finality.
//
// THE LAUNCH RULE (non-negotiable): no real-value native DEX may run until a
// value C+D block finalizes ONLY with a quorum — never on a proposer's lone
// self-vote. This gate is the runtime enforcement: the DEX value-activation
// path calls RequireQuorumFinalityForValueDEX before enabling real-value
// trading, and the node FAILS CLOSED (refuses to activate) if the engine is not
// in quorum-finality mode.
package chain

import (
	"errors"
	"fmt"
)

// ConsensusMode names the finality regime an engine is operating under. It is
// derived from the engine's configuration, not set independently, so it can
// never disagree with the actual finalization path.
type ConsensusMode uint8

const (
	// ModeUnknown is the zero value — never a valid mode for value activation.
	ModeUnknown ConsensusMode = iota

	// ModeSingleValidator is a K==1 engine: the sole validator's own accept IS
	// the 1-of-1 quorum (ForceAccept). Correct for --dev / localnet. NOT a
	// quorum-finality mode for the purposes of multi-party value safety: a
	// single validator can author any history, so real-value DEX across
	// independent parties MUST NOT run here.
	ModeSingleValidator

	// ModeQuorumFinality is a K>1 engine with a vote verifier: a value block
	// finalizes ONLY on a verified α-of-K QuorumCert (no self-finality, no
	// REJECT→ACCEPT flip). This is the ONLY mode in which real-value native DEX
	// may activate.
	ModeQuorumFinality
)

// String renders the mode for logs/errors.
func (m ConsensusMode) String() string {
	switch m {
	case ModeSingleValidator:
		return "single-validator"
	case ModeQuorumFinality:
		return "quorum-finality"
	default:
		return "unknown"
	}
}

// ErrValueDEXRequiresQuorumFinality is returned by
// RequireQuorumFinalityForValueDEX when real-value DEX activation is requested
// on an engine that is not in quorum-finality mode. Fail-closed.
var ErrValueDEXRequiresQuorumFinality = errors.New("chain: real-value native DEX requires quorum-finality consensus (K>1 with a verified alpha-of-K cert path); refusing to activate")

// Mode reports the engine's finality regime, derived from its live
// configuration:
//
//   - K<=1                                    → ModeSingleValidator
//   - K>1 with a vote verifier AND a cert
//     gossiper (the α-of-K topology is
//     actually reachable: votes collected,
//     certs distributed to followers)        → ModeQuorumFinality
//   - K>1 missing the verifier OR the cert
//     gossiper                               → ModeUnknown (degraded — a verifier
//     with no way to distribute the cert can leave followers unable to finalize;
//     the engine refuses Start without the verifier, and the value-DEX guard
//     treats this as not-safe so value never activates over a degraded topology).
//
// HIGH-4: ModeQuorumFinality REQUIRES a present quorum gossiper, not merely
// K>1 && verifier!=nil. Otherwise a node whose network layer never wired the
// vote/cert distribution would report "quorum-finality" and permit value-DEX
// while followers silently cannot finalize on a cert (freeze) — the topology
// must be live, not just the verifier present. Because the mode is computed from
// the SAME fields that select and distribute finality (verifier gates counting,
// certGossiper distributes the proof), an engine cannot report ModeQuorumFinality
// yet be unable to drive cert-witnessed finality to its followers.
func (t *Transitive) Mode() ConsensusMode {
	k := t.consensus.K()
	if k <= 1 {
		return ModeSingleValidator
	}
	t.mu.RLock()
	hasVerifier := t.voteVerifier != nil
	hasGossiper := t.certGossiper != nil
	t.mu.RUnlock()
	if hasVerifier && hasGossiper {
		return ModeQuorumFinality
	}
	return ModeUnknown
}

// RequireQuorumFinalityForValueDEX is the fail-closed gate the DEX value-
// activation path MUST call before enabling real-value trading on this engine.
//
//	if err := engine.RequireQuorumFinalityForValueDEX(dexNativeValueEnabled); err != nil {
//	    return err // node refuses to activate value DEX
//	}
//
// Semantics:
//   - dexNativeValueEnabled == false: no real value at stake; returns nil (the
//     gate only governs REAL-value activation).
//   - dexNativeValueEnabled == true and Mode() == ModeQuorumFinality: returns
//     nil (the only path that permits value DEX).
//   - dexNativeValueEnabled == true and any other mode: returns
//     ErrValueDEXRequiresQuorumFinality — the node fails closed.
//
// This is intentionally a METHOD on the live engine, not a static config check:
// it reads the engine's actual finality regime, so it cannot be satisfied by a
// config flag that does not match the running consensus.
func (t *Transitive) RequireQuorumFinalityForValueDEX(dexNativeValueEnabled bool) error {
	if !dexNativeValueEnabled {
		return nil
	}
	if mode := t.Mode(); mode != ModeQuorumFinality {
		return fmt.Errorf("%w: engine mode=%s K=%d", ErrValueDEXRequiresQuorumFinality, mode, t.consensus.K())
	}
	return nil
}
