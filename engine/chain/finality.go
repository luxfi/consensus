// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// finality.go — the finality ladder in the Lux astrophysics ontology. One story:
// light is emitted, it propagates, a star ignites, it blazes into the brightest
// beacon, it crosses the event horizon.
//
//	Photon → Wave → Nova → Quasar → Horizon
//
// Each rung is a DISTINCT authorization boundary. The whole point of this type is
// that these boundaries are never collapsed into one another — not in code, not in
// RPC, not in metrics, not in tests. A block carries exactly one Finality.
package chain

// Finality is a block's rung on the ladder — the single, highest authority tier it
// has reached.
//
//	Photon  : a vote is emitted and in flight; the block is known and being sampled.
//	          Authorizes NOTHING.
//	Wave    : the sampling wave has made this block the forming preference (the build
//	          tip). Authorizes NOTHING — a preference is not an acceptance.
//	Nova    : β-IGNITION. The sampler's confidence predicate fired — a NovaQuorum
//	          majority sustained NovaBeta rounds. Authorizes LOCAL execution only, and
//	          is REORGABLE until Quasar: a lone equivocator can fork a bare majority,
//	          so Nova is safe to act on locally (crash-fault model) but never to export.
//	Quasar  : a ⅔-STAKE certificate exists — strict >2/3 of total stake by DISTINCT
//	          signers (the one definition: config.TwoThirdsStakeFloor, enforced by
//	          QuorumCert.VerifyWeighted). Byzantine-safe, exportable finality.
//	          Authorizes bridges, DEX settlement, cross-chain messages, validator-set
//	          transitions.
//	Horizon : a post-quantum signature seals the certificate — the event horizon,
//	          irreversible even against a quantum adversary. Authorizes irreversible
//	          settlement.
//
// THE INVARIANT: a block below Quasar can NEVER be exported as deterministically
// final to a bridge, another chain, or an irreversible settlement. Nova authorizes
// local execution; export requires Quasar; quantum-irreversibility requires Horizon.
type Finality uint8

const (
	Photon  Finality = iota // in flight, being sampled — authorizes nothing
	Wave                    // the forming Nova preference (build tip) — authorizes nothing
	Nova                    // β-ignition, sampler accepted — authorizes LOCAL execution
	Quasar                  // ⅔-stake certificate — authorizes bridges / DEX / cross-chain
	Horizon                 // PQ-sealed, irreversible — authorizes irreversible settlement
)

// AuthorizesLocalExecution reports whether a block at this rung may drive local
// execution — Nova ignition or brighter. Nova is crash-safe but reorgable, so a
// caller acting here must be prepared to reorg if the block never reaches Quasar.
func (f Finality) AuthorizesLocalExecution() bool { return f >= Nova }

// AuthorizesExport reports whether a block may be exported as deterministically final
// to a bridge, another chain, or an irreversible settlement — the safety boundary of
// THE INVARIANT. Requires the ⅔ Quasar certificate (Byzantine-safe).
func (f Finality) AuthorizesExport() bool { return f >= Quasar }

// AuthorizesIrreversibleSettlement reports whether a block is quantum-irreversible —
// Horizon, the PQ-sealed event horizon.
func (f Finality) AuthorizesIrreversibleSettlement() bool { return f >= Horizon }

// String is the lowercase ontology name used verbatim in RPC status and metrics
// (photon/wave/nova/quasar/horizon — never a generic finality word).
func (f Finality) String() string {
	switch f {
	case Photon:
		return "photon"
	case Wave:
		return "wave"
	case Nova:
		return "nova"
	case Quasar:
		return "quasar"
	case Horizon:
		return "horizon"
	default:
		return "unknown"
	}
}
