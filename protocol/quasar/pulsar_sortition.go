// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// pulsar_sortition.go — the UNBIASABLE committee sortition for the Pulsar
// sampled-certificate layer.
//
// The whole security argument of the sampled cert rests on the committees being
// sampled from a seed an adversary CANNOT bias or grind: if the block producer
// (or any proposer) could choose the seed after seeing the block, it would
// reselect committees it controls and forge finality with a single captured
// committee. So the sortition seed is derived ONLY from values that are FIXED
// before this round's block exists:
//
//	sortitionSeed = H( "PULSAR_SORTITION_V1"
//	                   ‖ prevFinalizedBlockID   // already-final, immutable
//	                   ‖ signerSetID            // P-Chain-pinned validator set
//	                   ‖ pChainHeight           // the P-Chain pin height
//	                   ‖ epoch                  // sortition epoch
//	                   ‖ policyID )             // the finality posture
//
// NEVER the current block hash, the proposer's identity, or any proposer-chosen
// VRF output — those are grindable. The randomness is the hash of ALREADY-FINAL
// consensus state, so it is fixed and public the instant the previous block
// finalized, identical for every verifier, and unbiasable by the current proposer.
//
// committeePlanHash then commits the ENTIRE sampling plan so it can be bound into
// the subject M (pulsar_sampled_subject.go); a committee plan that does not hash
// to the planHash in M cannot have its signatures verify, so committees are
// frozen at block-production time and cannot be adaptively reselected.
//
// # Decomplected
//
// This file owns ONLY: the parameter set, the unbiasable seed, the deterministic
// stake-weighted committee selection, and the plan/key-era roots. It does NOT
// own the subject M (pulsar_sampled_subject.go), the cert types
// (pulsar_sampled_cert.go), verification (pulsar_sampled_verify.go), the security
// math (pulsar_sampled_security.go), or the policy posture
// (pulsar_sampled_policy.go).
//
// # Why not reuse protocol/prism StakeWeightedCut
//
// prism.StakeWeightedCut samples with crypto/rand — NON-deterministic, for a node
// privately sampling its own consensus queries. A sampled-certificate committee
// plan must be a DETERMINISTIC, verifiable function of the shared unbiasable seed
// so every verifier derives the byte-identical plan. Different randomness
// contract (verifiable-deterministic vs private-nondeterministic) ⇒ a distinct,
// justified primitive, not duplication. The cumulative-weight-without-replacement
// LOGIC is the same; the randomness SOURCE is fundamentally different.
package quasar

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/luxfi/ids"
	"golang.org/x/crypto/sha3"
)

// Domain-separation tags. Each is the first absorbed part of its TupleHash256,
// AND each derivation uses a distinct cSHAKE customization, so no two
// derivations can ever produce a colliding digest across contexts.
const (
	pulsarSortitionDomain = "PULSAR_SORTITION_V1"
	pulsarPlanDomain      = "PULSAR_PLAN_V1"
	pulsarCommitteeDomain = "PULSAR_COMMITTEE_ID_V1"
	pulsarRootDomain      = "PULSAR_COMMITTEE_ROOT_V1"
	pulsarSubSeedDomain   = "PULSAR_COMMITTEE_SUBSEED_V1"

	pulsarSortitionCustom = "Lux/PulsarSortition/v1"
	pulsarPlanCustom      = "Lux/PulsarPlan/v1"
	pulsarCommitteeCustom = "Lux/PulsarCommitteeID/v1"
	pulsarRootCustom      = "Lux/PulsarCommitteeRoot/v1"
)

// SelectionAlgorithm names a committee-selection algorithm. It is bound into the
// committeePlanHash so a plan can never be reinterpreted under a different
// algorithm than the one the certificate committed to.
type SelectionAlgorithm uint32

const (
	// SelectionStakeWeightedSHAKE256 is the canonical selection: m committees of
	// n members each, each member drawn stake-weighted WITHOUT replacement from a
	// SHAKE256 DRBG seeded by H(subSeedDomain ‖ sortitionSeed ‖ committeeIndex).
	SelectionStakeWeightedSHAKE256 SelectionAlgorithm = 1
)

// SortitionParams is the (n, t, m, r) sampled-certificate parameter set plus the
// selection algorithm and the adversary stake bound f_max (a rational
// FMaxNum/FMaxDen) used by the security tool (pulsar_sampled_security.go).
//
//	n — committee size            (members per committee)
//	t — committee threshold       (2 ≤ t ≤ n; signers needed within a committee)
//	m — total committees sampled  (m ≥ r; liveness slack is m − r)
//	r — required committees       (1 ≤ r ≤ m; the PQ repetition / confidence count)
type SortitionParams struct {
	N                  uint16
	T                  uint16
	M                  uint16
	R                  uint16
	SelectionAlgorithm SelectionAlgorithm
	FMaxNum            uint32 // adversary stake-fraction bound numerator   (0 < num < den)
	FMaxDen            uint32 // adversary stake-fraction bound denominator (> 0)
}

// PulsarHybridPQv1 is the DEFAULT production sampled-certificate parameter set:
// n=8, t=7, m=12, r=8, f_max=1/3. Sample 12 independent dealerless Mithril
// committees, accept when any 8 emit a valid stock-ML-DSA signature over the same
// M. This gives a per-committee capture p ≈ 2^-8.6 at f=1/3 and an r-of-m
// finality-forgery bound ≈ 2^-59.8 (all-of-r at m=r=8 would be ≈ 2^-68.9) — see
// pulsar_sampled_security.go for the exact binomial computation. The m−r=4
// liveness slack tolerates four offline/slow committees.
var PulsarHybridPQv1 = SortitionParams{
	N: 8, T: 7, M: 12, R: 8,
	SelectionAlgorithm: SelectionStakeWeightedSHAKE256,
	FMaxNum:            1,
	FMaxDen:            3,
}

// Typed sortition errors.
var (
	ErrSortitionParams      = errors.New("quasar: invalid sampled-cert sortition parameters")
	ErrSortitionValidators  = errors.New("quasar: not enough validators to form a committee")
	ErrSortitionWeight      = errors.New("quasar: validator set has zero or overflowing total weight")
	ErrUnknownSelectionAlgo = errors.New("quasar: unknown committee selection algorithm")
)

// Validate fail-closes on any structurally unsound parameter set. It enforces
// the SAMPLED-CERT structural invariants (2 ≤ t ≤ n, 1 ≤ r ≤ m, m ≥ 1) and the
// f_max rational shape. It deliberately does NOT cap n at the dealerless RSS
// keygen bound (rss.MaxParties, currently 6 — see mithril_committee.go): the
// per-committee KEYGEN viability bound is a separate concern enforced at KEY-ERA
// ACTIVATION by ValidateMithrilSigningCommittee, and is being raised by the
// dkg/pulsar RSS extension to admit the n=8 production set. The sampled-cert
// VERIFY layer is orthogonal to keygen: it verifies committee key-eras and
// signatures, it does not run keygen, so it must not couple to keygen's current
// committee-size ceiling.
func (p SortitionParams) Validate() error {
	if p.T < 2 || p.T > p.N {
		return fmt.Errorf("%w: need 2 ≤ t ≤ n, got t=%d n=%d", ErrSortitionParams, p.T, p.N)
	}
	if p.M < 1 || p.R < 1 || p.R > p.M {
		return fmt.Errorf("%w: need 1 ≤ r ≤ m, got r=%d m=%d", ErrSortitionParams, p.R, p.M)
	}
	if p.SelectionAlgorithm != SelectionStakeWeightedSHAKE256 {
		return fmt.Errorf("%w: %d", ErrUnknownSelectionAlgo, p.SelectionAlgorithm)
	}
	if p.FMaxDen == 0 || p.FMaxNum == 0 || p.FMaxNum >= p.FMaxDen {
		return fmt.Errorf("%w: need 0 < f_max < 1, got %d/%d", ErrSortitionParams, p.FMaxNum, p.FMaxDen)
	}
	return nil
}

// SortitionValidator pairs a node identity with its stake weight. The committee
// plan is sampled from the P-Chain-pinned signer set these represent.
type SortitionValidator struct {
	NodeID ids.NodeID
	Weight uint64
}

// SortitionSeed derives the unbiasable committee-sortition seed. Every input is
// fixed before the current block exists (prevFinalizedBlockID is already final;
// signerSetID/pChainHeight pin the P-Chain validator set; epoch and policyID are
// configuration), so the current proposer cannot bias or grind the seed.
func SortitionSeed(prevFinalizedBlockID ids.ID, signerSetID [48]byte, pChainHeight, epoch uint64, policyID uint32) []byte {
	parts := [][]byte{
		[]byte(pulsarSortitionDomain),
		prevFinalizedBlockID[:],
		signerSetID[:],
		u64be(pChainHeight),
		u64be(epoch),
		u32be(policyID),
	}
	return tupleHash256RoundDigest(parts, 32, pulsarSortitionCustom)
}

// SampledCommittee is one sampled committee in the plan: its plan index, its
// deterministic identity, and its stake-weighted membership (sorted).
type SampledCommittee struct {
	Index   int
	ID      ids.ID
	Members []ids.NodeID
}

// CommitteePlan is the full deterministic output of sortition: the m committees
// in plan order, the committeeKeyEraRoot committing them, and the committeePlanHash
// that is bound into the subject M.
type CommitteePlan struct {
	Params         SortitionParams
	Seed           []byte
	Committees     []SampledCommittee
	KeyEraRoot     []byte // committeeKeyEraRoot
	PlanHash       []byte // committeePlanHash
	committeeIndex map[ids.ID]int
}

// InPlan reports whether a committee id is one of the m sampled committees.
func (cp *CommitteePlan) InPlan(id ids.ID) (int, bool) {
	i, ok := cp.committeeIndex[id]
	return i, ok
}

// DeriveCommitteePlan deterministically samples the m committees from validators
// using sortitionSeed, then computes the committeeKeyEraRoot and committeePlanHash.
// It is a pure function of (params, sortitionSeed, validators): every verifier
// computes the byte-identical plan. Fail-closed on bad params, an empty/overflowing
// validator set, or a validator set too small to form an n-member committee.
func DeriveCommitteePlan(params SortitionParams, sortitionSeed []byte, validators []SortitionValidator) (*CommitteePlan, error) {
	if err := params.Validate(); err != nil {
		return nil, err
	}
	// Filter zero-weight validators and compute total weight (overflow-checked).
	filtered := make([]SortitionValidator, 0, len(validators))
	var total uint64
	for _, v := range validators {
		if v.Weight == 0 {
			continue
		}
		next := total + v.Weight
		if next < total {
			return nil, fmt.Errorf("%w: total weight overflows uint64", ErrSortitionWeight)
		}
		total = next
		filtered = append(filtered, v)
	}
	if total == 0 {
		return nil, ErrSortitionWeight
	}
	if len(filtered) < int(params.N) {
		return nil, fmt.Errorf("%w: have %d, need n=%d", ErrSortitionValidators, len(filtered), params.N)
	}

	committees := make([]SampledCommittee, int(params.M))
	index := make(map[ids.ID]int, int(params.M))
	for j := 0; j < int(params.M); j++ {
		members := sampleCommittee(sortitionSeed, j, filtered, int(params.N))
		// Canonicalise membership order so the committee id is independent of the
		// draw order (the draw is deterministic anyway; this is belt-and-braces).
		sort.Slice(members, func(a, b int) bool {
			return bytes.Compare(members[a][:], members[b][:]) < 0
		})
		id := committeeID(sortitionSeed, j, members)
		committees[j] = SampledCommittee{Index: j, ID: id, Members: members}
		index[id] = j
	}

	keyEraRoot := committeeKeyEraRoot(committees)
	planHash := committeePlanHash(params, sortitionSeed, keyEraRoot)
	return &CommitteePlan{
		Params:         params,
		Seed:           sortitionSeed,
		Committees:     committees,
		KeyEraRoot:     keyEraRoot,
		PlanHash:       planHash,
		committeeIndex: index,
	}, nil
}

// sampleCommittee draws n members stake-weighted WITHOUT replacement for the
// committee at plan index j, from a SHAKE256 DRBG seeded by
// H(subSeedDomain ‖ sortitionSeed ‖ j). Deterministic and unbiased (the
// cumulative-weight target is drawn with rejection sampling, no modulo bias).
func sampleCommittee(sortitionSeed []byte, j int, pool []SortitionValidator, n int) []ids.NodeID {
	d := newSortitionDRBG(sortitionSeed, j)

	// Local mutable copy of the pool so removal-without-replacement does not
	// disturb the caller's slice. Track remaining total weight.
	rem := make([]SortitionValidator, len(pool))
	copy(rem, pool)
	var remWeight uint64
	for _, v := range rem {
		remWeight += v.Weight
	}

	out := make([]ids.NodeID, 0, n)
	for len(out) < n {
		target := d.below(remWeight) // uniform in [0, remWeight)
		// Find the validator whose cumulative-weight interval contains target.
		var cum uint64
		pick := -1
		for i := range rem {
			cum += rem[i].Weight
			if target < cum {
				pick = i
				break
			}
		}
		// pick is always set: target < remWeight = Σ weights.
		out = append(out, rem[pick].NodeID)
		remWeight -= rem[pick].Weight
		// Remove pick without replacement (swap-with-last, order irrelevant —
		// membership is canonicalised by the caller).
		last := len(rem) - 1
		rem[pick] = rem[last]
		rem = rem[:last]
	}
	return out
}

// committeeID = H("PULSAR_COMMITTEE_ID_V1" ‖ sortitionSeed ‖ index ‖ member…)
// over the sorted membership — the committee's deterministic identity, binding
// both which seed sampled it and exactly which nodes it contains.
func committeeID(sortitionSeed []byte, index int, sortedMembers []ids.NodeID) ids.ID {
	parts := make([][]byte, 0, 3+len(sortedMembers))
	parts = append(parts, []byte(pulsarCommitteeDomain), sortitionSeed, u32be(uint32(index)))
	for _, m := range sortedMembers {
		parts = append(parts, m[:])
	}
	digest := tupleHash256RoundDigest(parts, 32, pulsarCommitteeCustom)
	var id ids.ID
	copy(id[:], digest)
	return id
}

// committeeKeyEraRoot commits the m sampled committees in plan order. It commits
// the committee SET, and via each CommitteeID (a hash of that committee's
// membership) the membership of every committee — so a plan with substituted or
// reordered committees produces a different root, hence a different planHash and
// a different M. The committee GROUP KEYS are bound separately and completely:
// each PulsarCommitteeCert carries PubKeyHash and the verifier checks it against
// the key the trusted CommitteeKeyResolver supplies. Identity here, key there —
// orthogonal bindings that together pin both who the committee is and which key
// it signs under.
func committeeKeyEraRoot(committees []SampledCommittee) []byte {
	parts := make([][]byte, 0, 1+len(committees))
	parts = append(parts, []byte(pulsarRootDomain))
	for _, c := range committees {
		parts = append(parts, c.ID[:])
	}
	return tupleHash256RoundDigest(parts, 32, pulsarRootCustom)
}

// committeePlanHash = H("PULSAR_PLAN_V1" ‖ n ‖ t ‖ m ‖ r ‖ selectionAlgorithmID
// ‖ sortitionSeed ‖ committeeKeyEraRoot). The single commitment to the entire
// sampling plan, bound into the subject M.
func committeePlanHash(p SortitionParams, sortitionSeed, keyEraRoot []byte) []byte {
	parts := [][]byte{
		[]byte(pulsarPlanDomain),
		u16be(p.N),
		u16be(p.T),
		u16be(p.M),
		u16be(p.R),
		u32be(uint32(p.SelectionAlgorithm)),
		sortitionSeed,
		keyEraRoot,
	}
	return tupleHash256RoundDigest(parts, 32, pulsarPlanCustom)
}

// CommitteePlanHash is the exported pure-function form, for callers (e.g. the
// subject builder, the producer) that have a seed and key-era root in hand.
func CommitteePlanHash(p SortitionParams, sortitionSeed, keyEraRoot []byte) []byte {
	return committeePlanHash(p, sortitionSeed, keyEraRoot)
}

// ----------------------------------------------------------------------------
// sortitionDRBG — a SHAKE256-backed deterministic bit generator with an unbiased
// bounded sampler. Seeded by H(subSeedDomain ‖ sortitionSeed ‖ committeeIndex).
// ----------------------------------------------------------------------------

type sortitionDRBG struct{ sh sha3.ShakeHash }

func newSortitionDRBG(sortitionSeed []byte, committeeIndex int) *sortitionDRBG {
	sh := sha3.NewShake256()
	_, _ = sh.Write([]byte(pulsarSubSeedDomain))
	_, _ = sh.Write(sortitionSeed)
	_, _ = sh.Write(u32be(uint32(committeeIndex)))
	return &sortitionDRBG{sh: sh}
}

func (d *sortitionDRBG) uint64() uint64 {
	var b [8]byte
	_, _ = d.sh.Read(b[:])
	return binary.BigEndian.Uint64(b[:])
}

// below returns a uniform value in [0, n) with NO modulo bias, by rejection
// sampling: it rejects the lowest (2^64 mod n) of the 2^64 possible draws so the
// accepted range is an exact multiple of n. n must be > 0.
func (d *sortitionDRBG) below(n uint64) uint64 {
	if n == 0 {
		return 0 // unreachable: callers guarantee n = remaining weight > 0
	}
	// thresh = 2^64 mod n, computed as (-n) mod n in unsigned arithmetic.
	thresh := (-n) % n
	for {
		v := d.uint64()
		if v >= thresh {
			return v % n
		}
	}
}

// ---- small fixed-width big-endian encoders (local, allocation-light) ----

// u64be is provided by p3q_codec.go (reused here — DRY).
func u16be(x uint16) []byte { var b [2]byte; binary.BigEndian.PutUint16(b[:], x); return b[:] }
func u32be(x uint32) []byte { var b [4]byte; binary.BigEndian.PutUint32(b[:], x); return b[:] }
