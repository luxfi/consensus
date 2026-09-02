// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// verdict.go — the finality DECISION as data.
//
// The rest of the corpus states what a validator signs and what a certificate
// looks like on the wire. That is necessary and it is not sufficient: an
// implementation can reproduce every byte here and still finalize on the wrong
// number of signers, because encoding and deciding are different questions. The
// failure this section exists to prevent already happened — a C++ build that
// carried no weighted predicate at all reported PASS against a corpus that only
// ever asked it to encode.
//
// So this section asks the other question. Each case states a validator set, the
// distinct signers a certificate carries, and the rung it attests, and records
// what the live predicate DECIDES about it. Nothing here restates the rule: the
// accept flag and the refusal class are produced by running
// chain.QuorumCert.VerifyWeighted and validators.Register, exactly as the
// proof-of-possession vector records what pop.Verify returns. An edit to a
// threshold moves these verdicts and the golden test fails at the source.
//
// SIGNATURES ARE NOT THE SUBJECT HERE. Every vote is resolved as correctly
// signed, so the only thing a case can turn on is the weighted half — the signer
// floor and the stake floor. Signature validity is its own dimension and it is
// already frozen, in the vote cases, the cert cases and the proof-of-possession
// vector. Braiding the two would mean a weighted rule could not be stated
// without a key ceremony, and a runner that lacked one would report PASS by
// skipping the case — which is the whole failure mode this section closes.
package conformance

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain"
	validators "github.com/luxfi/consensus/validator"
	"github.com/luxfi/ids"
)

// verdictEpoch is the P-chain epoch every case is decided at. The sets here do
// not vary by height, so it changes no verdict; it is recorded because the
// predicate reads its tally at an epoch and a case must be reproducible.
const verdictEpoch = 7

// Verdict is what the finality predicate DECIDES, as opposed to what the
// encoder emits. Two doors, because a certificate is only as sound as the set it
// is checked against: Finality is the certificate predicate, Admission is the
// registration predicate that decides which sets exist at all.
type Verdict struct {
	Note      string          `json:"note"`
	Epoch     string          `json:"epoch"`
	Finality  []FinalityCase  `json:"finality"`
	Admission []AdmissionCase `json:"admission"`
}

// Seat is one admitted validator: the identity that signs and the weight behind
// it. Weight is a decimal string for the reason every uint64 here is — a set
// near 2^64 loses precision in a JSON parser that keeps numbers as doubles.
type Seat struct {
	NodeID string `json:"nodeID"`
	Weight string `json:"weight"`
	// Keyless marks a seat the chain carries without a signing key. It holds
	// stake and can never sign, so it is in no floor's denominator — the field is
	// omitted when false, so a set of ordinary seats reads exactly as it always
	// did.
	Keyless bool `json:"keyless,omitempty"`
}

// FinalityCase is one certificate weighed against one validator set at one rung.
//
// SignerFloor and StakeFloor are recorded alongside the verdict on purpose. They
// are read off the same functions the predicate enforces, so a runner that
// disagrees about the DECISION can say in one line whether it disagreed about
// the count floor, the stake floor, or which of the two applies to that rung.
// BOTH are the rung's own: Nova's count floor saturates at three, Quasar's is
// the export supermajority in seats and grows with the set.
// Threshold is what the certificate DECLARES its quorum to be, and every honest
// case declares SignerFloor — the floor the rung derives over this set. It is a
// recorded field rather than a derived one because the derived-authority rows
// declare something else on purpose, and a runner has to be handed the number the
// certificate carried in order to reach the same verdict.
type FinalityCase struct {
	Name        string   `json:"name"`
	Note        string   `json:"note"`
	Rung        string   `json:"rung"`
	Set         []Seat   `json:"set"`
	Total       string   `json:"total"`
	SignerFloor int      `json:"signerFloor"`
	StakeFloor  string   `json:"stakeFloor"`
	Threshold   uint32   `json:"threshold"`
	Signers     []string `json:"signers"`
	Voted       string   `json:"voted"`
	Accept      bool     `json:"accept"`
	Refusal     string   `json:"refusal"`
}

// AdmissionCase is one weight vector offered at the door.
//
// Door names the entry point that produced the verdict, because the standard has
// two and they do not enforce the same clauses: Register admits a fresh
// registration set and demands possession of every key; FlattenValidatorSet
// reads a set the chain already admitted and therefore has no proof to check.
// The weight clauses are common to both, and which one a case runs through is a
// fact about the standard, so it is recorded rather than assumed.
type AdmissionCase struct {
	Name     string   `json:"name"`
	Note     string   `json:"note"`
	Door     string   `json:"door"`
	Weights  []string `json:"weights"`
	Admitted bool     `json:"admitted"`
	Refusal  string   `json:"refusal"`
}

// seat is the node id of the i-th validator: the index in the last four bytes,
// big-endian, everything above it zero. Ascending index is ascending node id, so
// the order a case lists its signers in is the order the encoder emits them, and
// a reader can name a seat without carrying a table of literals.
func seat(i int) ids.NodeID {
	var n ids.NodeID
	binary.BigEndian.PutUint32(n[len(n)-4:], uint32(i))
	return n
}

// first returns the lowest k seats — the signer set a case uses when it is
// walking a quorum boundary one signer at a time.
func first(k int) []int {
	out := make([]int, 0, k)
	for i := 1; i <= k; i++ {
		out = append(out, i)
	}
	return out
}

// member is one seat of a corpus set: the stake it holds, and whether it holds a
// key. A member with no key is carried by the chain and counted by nothing — it
// is the case the keyless vector is about.
type member struct {
	weight  uint64
	keyless bool
}

// stake is the authoritative validator set a case is weighed against: seat i
// holds stake[i-1]. It is the live StakeSource interface, so the predicate reads
// this set through exactly the seam a node reads the P-chain through.
type stake []member

// signers builds a set in which every seat holds a key — the shape every case
// but the keyless one has.
func signers(ws ...uint64) stake {
	out := make(stake, 0, len(ws))
	for _, w := range ws {
		out = append(out, member{weight: w})
	}
	return out
}

// spectator is a seat that holds stake and no key. It can never sign, so it is
// in no floor's denominator — which is the whole of what the keyless case says.
func spectator(w uint64) member { return member{weight: w, keyless: true} }

// at resolves a node id to its seat index, or -1 for an id outside the set.
func (s stake) at(id ids.NodeID) int {
	i := int(binary.BigEndian.Uint32(id[len(id)-4:])) - 1
	if i < 0 || i >= len(s) {
		return -1
	}
	return i
}

// Weight returns the seat's stake, or zero for an id outside the set and zero
// for a seat that holds no key — an unknown voter must never be able to inflate
// a tally, and neither must a voter whose vote no verifier would accept.
func (s stake) Weight(id ids.NodeID, _ uint64) uint64 {
	i := s.at(id)
	if i < 0 || s[i].keyless {
		return 0
	}
	return s[i].weight
}

// SignerStake returns the stake held by the seats that can sign — the
// denominator both floors are read against. A keyless seat's stake is absent
// from it for the reason it is absent from every tally: no quorum can ever
// contain it, so a floor computed over it is a bar raised against stake that
// could never help clear it.
func (s stake) SignerStake(uint64) uint64 {
	var total uint64
	for _, m := range s {
		if !m.keyless {
			total += m.weight
		}
	}
	return total
}

// SignerCount returns the number of distinct seats that can sign — read over
// the same set as SignerStake, so the count floor and the stake floor are the
// same supermajority in two units and not two supermajorities.
func (s stake) SignerCount(uint64) int {
	n := 0
	for _, m := range s {
		if !m.keyless {
			n++
		}
	}
	return n
}

// CarriedStake returns every seat's weight, keyed or not — the membership roll's
// stake. No floor is read against it, and a case exists precisely to state what
// the two numbers are when they differ.
func (s stake) CarriedStake(uint64) uint64 {
	var total uint64
	for _, m := range s {
		total += m.weight
	}
	return total
}

// trust resolves every vote from a SEAT THAT HOLDS A KEY as correctly signed.
// The weighted half is the subject of this section; see the file comment for why
// the two are separated. A keyless seat is refused, because a signature under a
// key that does not exist is not a signature a corpus may credit — it would let
// a case state a quorum no implementation could ever assemble.
type trust struct{ set stake }

// VerifyVote reports a keyed seat's signature as valid and a keyless seat's as
// what it is: absent.
func (t trust) VerifyVote(id ids.NodeID, _ []byte, _ []byte, _ uint64) bool {
	i := t.set.at(id)
	return i >= 0 && !t.set[i].keyless
}

// keyShaped is a key that is present and the right width but is not a point. The
// clauses this section is about are reached before any key is read, and a case
// that carried a real key would be pinning possession — which the
// proof-of-possession vector already pins, once, on its own.
var keyShaped = bytes.Repeat([]byte{0xAB}, 48)

// refusal names the clause a refusal came from, as a class rather than a
// message, so an implementation can map it onto its own error type without
// carrying Go's prose. A refusal the corpus cannot name is a hard failure: the
// standard would otherwise record a decision no other implementation could
// reproduce.
func refusal(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, chain.ErrQCThresholdNotDerived):
		return "thresholdNotDerived"
	case errors.Is(err, chain.ErrQCBelowThreshold):
		return "belowThreshold"
	case errors.Is(err, chain.ErrQCStakeBelowMajority):
		return "stakeBelowMajority"
	case errors.Is(err, chain.ErrQCStakeBelowSupermajority):
		return "stakeBelowSupermajority"
	case errors.Is(err, validators.ErrZeroWeight):
		return "zeroWeight"
	case errors.Is(err, validators.ErrWeightOverflow):
		return "weightOverflow"
	default:
		panic("conformance: a verdict the corpus cannot name: " + err.Error())
	}
}

// finality weighs one certificate that declares the quorum its set DERIVES —
// what an honest assembler stamps on it — and records what the live predicate
// decides.
//
// The declared threshold is not a free variable any more. A certificate's floor
// is a function of the set and the rung, and the predicate now refuses one that
// declares any other number, so "the count clause is neutralised by declaring the
// vote count" is no longer a thing a case may do. What isolates the weighted half
// here is that every vote is resolved as correctly signed and every certificate
// declares the one threshold it is allowed to: a runner that fails one of these
// has failed a floor, not the wire and not a signature.
func finality(name, note string, rung chain.Finality, s stake, signers []int) FinalityCase {
	return weigh(name, note, rung, s, signers,
		chain.SignerFloor(rung, s.SignerCount(verdictEpoch)))
}

// weigh is the body: one certificate, declaring `declared`, weighed against one
// set at one rung. Cases that pass anything but the derived floor are stating the
// derived-authority attack — a certificate naming a quorum of its own choosing.
func weigh(name, note string, rung chain.Finality, s stake, signers []int, declared uint32) FinalityCase {
	votes := make([]chain.SignedVote, 0, len(signers))
	names := make([]string, 0, len(signers))
	var voted uint64
	for _, i := range signers {
		id := seat(i)
		votes = append(votes, chain.SignedVote{NodeID: id, Accept: true, Signature: []byte{0x01}})
		names = append(names, hex.EncodeToString(id[:]))
		voted += s.Weight(id, verdictEpoch)
	}

	// Assembled at the vote count so the canonical ordering and dedup clauses run,
	// then stamped with what this case's certificate DECLARES. Assembly is not the
	// subject here — a verifier is handed a byte string claiming a threshold, and
	// an honest assembler cannot even build the under-quorum certificates a
	// verifier must still refuse.
	cert, err := chain.AssembleQuorumCert(spec(), rung, uint32(len(votes)), votes)
	if err != nil {
		panic("conformance: " + name + ": " + err.Error())
	}
	cert.Threshold = declared

	seats := make([]Seat, 0, len(s))
	for i, m := range s {
		id := seat(i + 1)
		seats = append(seats, Seat{NodeID: hex.EncodeToString(id[:]), Weight: u64(m.weight), Keyless: m.keyless})
	}

	// Both floors are the RUNG's. Recording Nova's count floor on a Quasar row
	// would describe a clause the export rung does not enforce, and a runner
	// reading it would be told the wrong number by the file that is supposed to
	// tell it the right one.
	// n is the SIGNER count, never len(s): a seat that holds no key is not a party
	// whose agreement a count floor is a statement about, and a floor read over the
	// membership roll can demand more signatures than the set is able to produce.
	total := s.SignerStake(verdictEpoch)
	n := s.SignerCount(verdictEpoch)
	floor := config.HalfStakeFloor(total)
	if rung == chain.Quasar {
		floor = config.TwoThirdsStakeFloor(total)
	}
	// Read from the engine's own definition, never restated: signerFloor is the
	// number the derived-authority clause compares a certificate's declaration
	// against, so a corpus that computed it a second way could freeze a floor the
	// predicate does not enforce.
	signerFloor := int(chain.SignerFloor(rung, n))

	// The decision, read from the live predicate — never restated here.
	decided := cert.VerifyWeighted(trust{set: s}, s, verdictEpoch)

	return FinalityCase{
		Name:        name,
		Note:        note,
		Rung:        rung.String(),
		Set:         seats,
		Total:       u64(total),
		SignerFloor: signerFloor,
		StakeFloor:  u64(floor),
		Threshold:   cert.Threshold,
		Signers:     names,
		Voted:       u64(voted),
		Accept:      decided == nil,
		Refusal:     refusal(decided),
	}
}

// register offers a weight vector at the registration door and records what the
// door decides.
func register(name, note string, ws []uint64) AdmissionCase {
	rs := make([]validators.Registration, 0, len(ws))
	for i, w := range ws {
		rs = append(rs, validators.Registration{NodeID: seat(i + 1), Key: keyShaped, Weight: w})
	}
	_, err := validators.Register(rs)
	return AdmissionCase{
		Name:     name,
		Note:     note,
		Door:     "Register",
		Weights:  decimals(ws),
		Admitted: err == nil,
		Refusal:  refusal(err),
	}
}

// flatten offers a weight vector at the already-admitted door and records what it
// decides. The map is an input to a function that sorts its keys internally, so
// nothing about the output depends on iteration order.
//
// Every seat carries the same shaped key the registration door's cases carry, so
// the two doors are offered the same material and the weight vector is the only
// thing that varies. It has to be present: the clause that refuses a seat with
// no stake is about a seat that could otherwise SIGN, and a keyless seat is
// skipped before it can. It does not have to be a point — the weight clauses are
// reached before any key is decoded, at both doors.
func flatten(name, note string, ws []uint64) AdmissionCase {
	set := make(map[ids.NodeID]*validators.GetValidatorOutput, len(ws))
	for i, w := range ws {
		id := seat(i + 1)
		set[id] = &validators.GetValidatorOutput{NodeID: id, PublicKey: keyShaped, Weight: w}
	}
	_, err := validators.FlattenValidatorSet(set)
	return AdmissionCase{
		Name:     name,
		Note:     note,
		Door:     "FlattenValidatorSet",
		Weights:  decimals(ws),
		Admitted: err == nil,
		Refusal:  refusal(err),
	}
}

// decimals renders a weight vector the way every other uint64 here is rendered.
func decimals(ws []uint64) []string {
	out := make([]string, 0, len(ws))
	for _, w := range ws {
		out = append(out, u64(w))
	}
	return out
}

// verdicts is the section: every case decided by the live predicate.
func verdicts() Verdict {
	// Forty-one equal seats. The set is chosen because both rungs have a sharp
	// edge on it and the two edges are seven seats apart — 21 for the majority,
	// 28 for the supermajority — so an implementation that confuses one floor
	// for the other cannot pass by accident, which it can on a small set where
	// the two coincide.
	equal41 := make(stake, 41)
	for i := range equal41 {
		equal41[i] = member{weight: 1}
	}

	// Four equal seats: the smallest Byzantine-tolerant committee, where the
	// signer floor is 3 and a lone signer is below every gate.
	equal4 := signers(1, 1, 1, 1)

	// One holder of a stake majority and four registrations at the minimum. This
	// is the shape the signer floor exists for.
	whale := signers(100, 1, 1, 1, 1)

	// Three seats that can sign and one that cannot, the keyless one holding two
	// fifths of what the chain carries — past the third at which a denominator
	// read over the whole membership puts the export rung out of reach for good.
	keylessThird := append(signers(100, 100, 100), spectator(200))

	// Six seats that can sign and one that cannot, holding a third of the roll.
	// The COUNT floor is the same number either way here — ⌊2·6/3⌋+1 and ⌊2·7/3⌋+1
	// are both five — so the STAKE denominator is the only thing that decides it.
	keylessStake := append(signers(100, 100, 100, 100, 100, 100), spectator(300))

	// Four seats that can sign and two that cannot, holding one unit each. The
	// keyless weight is a rounding error, so the stake floor is cleared under
	// either denominator and only the COUNT can decide: ⌊2·4/3⌋+1 is three over
	// the signers and ⌊2·6/3⌋+1 is five over the roll, and four signatures is
	// every one this set is able to produce.
	keylessCount := append(signers(100, 100, 100, 100), spectator(1), spectator(1))

	return Verdict{
		Note: "what the predicate DECIDES, not what the encoder emits. Every vote is " +
			"resolved as correctly signed, so a case turns on the weighted half alone: " +
			"the distinct-signer floor and the stake floor. BOTH rungs carry both. Nova " +
			"(local execution) needs signers >= novaSignerFloor(n) AND voted > floor(total/2). " +
			"Quasar (export) needs n >= 4 AND signers >= floor(2*n/3)+1 AND voted > " +
			"floor(2*total/3) — the same supermajority in seats and in stake, and neither " +
			"half is sufficient alone, because stake alone lets one holder of two thirds " +
			"mint export finality on one signature. The n >= 4 clause is a floor on the SET " +
			"and not on the voters: Byzantine tolerance is f = floor((n-1)/3), which is zero " +
			"for n of one, two and three, so a two-thirds supermajority over such a set " +
			"tolerates no fault and one compromised key forges it. Both quorum floors shrink " +
			"with n and neither catches it — floor(2*1/3)+1 is 1 — so it is stated " +
			"separately. n and total are read over the SIGNERS: a seat marked keyless holds " +
			"stake, can never sign, and is in neither denominator. The decision does not " +
			"depend on the position the votes were cast over, so a runner may weigh these " +
			"votes over any position its encoder can build. DERIVED AUTHORITY, the clause " +
			"every row above also states: a certificate DECLARES a threshold, and it must be " +
			"exactly signerFloor — the floor the rung derives over this set. A certificate " +
			"states its quorum; it does not choose it. Without that clause the field is " +
			"load-bearing, because the tier-agnostic count clause counts distinct valid " +
			"accepts against the certificate's OWN declaration, so one declaring 1 clears it " +
			"on a single signature. Equality and not a lower bound: an over-claim names a bar " +
			"this set does not set, and tolerating it would let a certificate redefine the " +
			"rung upward exactly as tolerating an under-claim lets it redefine the rung down. " +
			"The threshold field is recorded per row because three rows declare something " +
			"else on purpose, and a runner must be handed the number the certificate carried.",
		Epoch: u64(verdictEpoch),
		Finality: []FinalityCase{
			finality("nova_below_majority",
				"twenty of forty-one equal seats: exactly the majority floor, and equality is not a quorum",
				chain.Nova, equal41, first(20)),
			finality("nova_majority",
				"twenty-one of forty-one: one seat past the floor is the whole difference",
				chain.Nova, equal41, first(21)),
			finality("quasar_below_supermajority",
				"twenty-seven of forty-one: exactly the supermajority floor, and equality is not "+
					"a quorum here either. Over an EQUAL set the two halves of the export rule "+
					"bind at the same edge, so the certificate is one signature short of the "+
					"twenty-eight it declares and the count clause names it — the stake is "+
					"equally short, and quasar_count_met_stake_short is the lopsided row where "+
					"the stake half binds on its own",
				chain.Quasar, equal41, first(27)),
			finality("quasar_supermajority",
				"twenty-eight of forty-one: the export edge. Note the same twenty-eight signers "+
					"clear Nova seven seats earlier — the rungs are different authorizations",
				chain.Quasar, equal41, first(28)),
			finality("nova_one_of_four",
				"one of four equal seats: below the signer floor of three, refused on the count "+
					"before stake is ever read",
				chain.Nova, equal4, first(1)),
			finality("quasar_one_of_four",
				"one of four equal seats at the export rung. One signature is short of the "+
					"three this set derives and one unit is short of two thirds of four, so "+
					"both halves refuse and the count names it. The rung has TWO floors and "+
					"this row clears neither; quasar_whale_alone isolates the count and "+
					"quasar_count_met_stake_short isolates the stake",
				chain.Quasar, equal4, first(1)),
			finality("nova_whale_alone",
				"the holder of a hundred of a hundred and four signs alone: a stake majority many "+
					"times over, refused on the distinct-signer floor. Stake cannot buy a pass past "+
					"the count — this is what stops a single holder self-igniting",
				chain.Nova, whale, []int{1}),
			finality("nova_whale_with_two",
				"the same holder with two of the minimum registrations: the floor is met and the "+
					"same stake now carries. The floor was the binding clause, not the stake",
				chain.Nova, whale, []int{1, 2, 3}),
			finality("quasar_whale_alone",
				"the same holder signs alone at the EXPORT rung. A hundred of a hundred and "+
					"four clears floor(2*104/3)=69 outright, so the stake half of the export "+
					"rule is satisfied many times over and the certificate is refused on the "+
					"count: one signer where four are needed. This is the whole of the rung's "+
					"claim — export finality is a statement that a Byzantine supermajority of "+
					"PARTIES agreed, and a certificate one key can mint is not that statement "+
					"however much stake stands behind the key",
				chain.Quasar, whale, []int{1}),
			finality("quasar_whale_with_three",
				"the same holder with three of the minimum registrations: four distinct signers "+
					"meet floor(2*5/3)+1 and the same stake now carries. The floor was the binding "+
					"clause, not the stake — and one seat fewer is the case above",
				chain.Quasar, whale, []int{1, 2, 3, 4}),

			finality("quasar_keyless_third",
				"THE KEYLESS DENOMINATOR AND THE COMMITTEE FLOOR, on one set. Three seats "+
					"of a hundred hold keys; a fourth holds two hundred and no key. The "+
					"denominator is the signers, and the recorded floors say so outright: "+
					"the stake floor is floor(2*300/3)=200 over the three hundred that can "+
					"sign, not floor(2*500/3)=333 over the five hundred the chain carries. "+
					"All three signers sign, so both quorum floors are cleared — three "+
					"hundred exceeds two hundred, and three signers meet floor(2*3/3)+1=3. "+
					"The certificate is REFUSED anyway, and on neither of them. Three parties "+
					"are not a Byzantine committee: f=floor((3-1)/3)=0, so this unanimous "+
					"certificate tolerates no fault at all and one compromised key among the "+
					"three forges it. Reading the floors over the signers is what stops a "+
					"spectator stranding a chain; it is not a way for a chain with three "+
					"signers to export. Those are different claims and this row states both",
				chain.Quasar, keylessThird, first(3)),
			finality("quasar_keyless_stake",
				"THE KEYLESS DENOMINATOR, stake half, isolated. Six seats of a hundred "+
					"hold keys; a seventh holds three hundred and none. Every signer signs "+
					"and the export rung admits it on six hundred of six hundred. Read over "+
					"the roll it is stranded and stays stranded: six hundred does not exceed "+
					"floor(2*900/3)=600, and nothing the signers do can reach it, because "+
					"the shortfall is held by a member that can never cast a vote. The COUNT "+
					"floor is deliberately the same number either way — floor(2*6/3)+1 and "+
					"floor(2*7/3)+1 are both five — so the stake denominator is the only "+
					"thing that decides this row, and an implementation that moved only the "+
					"count fails it. Seven members, six signers, above the committee floor",
				chain.Quasar, keylessStake, first(6)),
			// THE DERIVED-AUTHORITY ROWS. Everything above is a certificate that
			// declares the quorum its set gives it; these three declare something
			// else, which is the only remaining way a certificate could name its own
			// authority. They are stated over the sets the rows above already use, so
			// the only variable is the declaration.
			weigh("quasar_threshold_underclaimed",
				"THE UNDER-CLAIM. The accepting twenty-eight-of-forty-one certificate above, "+
					"byte for byte, except that it declares a quorum of ONE. Every signature "+
					"still verifies and the stake is still a supermajority; the only thing "+
					"wrong is the number. It matters because the tier-agnostic count clause "+
					"counts distinct valid accepts against the certificate's OWN declaration, "+
					"so a declaration of one turns 'twenty-eight validators agreed' into 'one "+
					"did' for any reader that has no validator set to check against. The floor "+
					"a certificate is held to is a function of the SET and the RUNG, so the "+
					"declaration is a claim to be checked and never a value to adopt",
				chain.Quasar, equal41, first(28), 1),
			weigh("quasar_threshold_overclaimed",
				"THE OVER-CLAIM, refused for the same reason in the other direction. THIRTY of "+
					"forty-one sign — a supermajority in seats and in stake, so this certificate "+
					"would carry — and it declares a quorum of twenty-nine where the set derives "+
					"twenty-eight. Thirty signatures clear twenty-nine, so the count clause has "+
					"nothing to say and the ONLY thing refusing this row is the derived clause. "+
					"That is the point of stating it: an over-claim ABOVE the vote count is "+
					"already caught by counting, and one below it is caught by nothing else. A "+
					"verifier that tolerated this would let a certificate redefine the rung "+
					"upward exactly as tolerating the under-claim lets it redefine the rung "+
					"down. Neither direction is the set's number, so the rule is equality",
				chain.Quasar, equal41, first(30), 29),
			weigh("nova_threshold_underclaimed",
				"THE UNDER-CLAIM at the accept rung, because the rule is one rule and not an "+
					"export special case. Twenty-one of forty-one carry a stake majority and "+
					"the certificate declares a quorum of one; the accept rung derives three "+
					"over this set — its count floor saturates — and three is what the "+
					"certificate must say",
				chain.Nova, equal41, first(21), 1),

			// THE STAKE HALF, ISOLATED, at each rung. Over an EQUAL set the count and
			// the stake bind at the same edge, so neither can be seen alone there;
			// these two are lopsided on purpose. The whale abstains and the minimum
			// registrations sign, so the quorum is present in SEATS and nowhere near
			// present in weight — which is the clause a count-only implementation
			// passes every other row without ever running.
			finality("quasar_count_met_stake_short",
				"Four of five sign and the holder of a hundred of a hundred and four is not "+
					"among them. Four distinct signers meet floor(2*5/3)+1, so the export count "+
					"floor is satisfied outright and the certificate declares exactly it — and "+
					"four units of stake does not exceed floor(2*104/3)=69, so the row is "+
					"refused on STAKE alone. It is the mirror of quasar_whale_alone, which has "+
					"the stake and not the seats; between them the two halves of the export "+
					"rule are each shown binding on their own",
				chain.Quasar, whale, []int{2, 3, 4, 5}),
			finality("nova_count_met_stake_short",
				"The same shape one rung down. Three of the minimum registrations sign while "+
					"the whale abstains: three distinct signers meet the accept rung's "+
					"saturating floor of three, and three units of a hundred and four is not a "+
					"strict majority of the stake, so the row is refused on STAKE. Nova reads "+
					"its majority in weight and not in heads, and this is the row that says so",
				chain.Nova, whale, []int{2, 3, 4}),

			finality("quasar_keyless_count",
				"THE KEYLESS DENOMINATOR, count half, isolated. Four seats of a hundred "+
					"hold keys; two more hold one unit each and no key. The keyless weight "+
					"is a rounding error on purpose, so the stake floor is cleared under "+
					"EITHER denominator — four hundred exceeds floor(2*400/3)=266 and "+
					"floor(2*402/3)=268 alike — and stake cannot be what decides. The count "+
					"can: floor(2*4/3)+1 is three over the signers and floor(2*6/3)+1 is "+
					"five over the roll, and four signatures is every one this set is able "+
					"to produce. Read over the roll, two members holding two units between "+
					"them strand the export rung of a chain whose four real validators all "+
					"agree. This is the row a stake-only fix passes and a correct one must "+
					"also pass",
				chain.Quasar, keylessCount, first(4)),
		},
		Admission: []AdmissionCase{
			register("zero_weight",
				"a set carrying a validator with no stake is refused whole, never trimmed: a "+
					"signer that raises the count without raising the weight is exactly the "+
					"disagreement the two floors exist to prevent. The seat with no stake sorts "+
					"first, so the refusal lands before any key is read. There is no admitted "+
					"counterpart here: past the zero-weight clause the door asks for possession, "+
					"which is the proof-of-possession vector's question and not this one",
				[]uint64{0, 5}),
			flatten("zero_weight",
				"the same weight vector at the OTHER door. A set the chain already admitted is "+
					"read here, not admitted here, so this door forgives what it cannot check — a "+
					"key it cannot decode is skipped rather than refused. It does not forgive a "+
					"key with no stake behind it. That seat is a phantom signer: it raises the "+
					"count of distinct signers the Nova floor is read against and adds nothing to "+
					"the stake the same certificate is weighed by, which is the disagreement "+
					"between how many signed and how much signed that both floors exist to "+
					"prevent. A door that admitted it would leave the floor meaning one thing at "+
					"registration and another here",
				[]uint64{0, 5}),
			flatten("weight_overflow",
				"two seats whose stake sums past 2^64 are refused: a total that wrapped would make "+
					"every floor read off it meaningless. Checked at the already-admitted door "+
					"because Register demands possession of every key BEFORE it reaches its weight "+
					"clause, so that clause cannot be reached without a key ceremony",
				[]uint64{1 << 63, 1<<63 + 1}),
			flatten("weight_fits",
				"the same two seats one unit lower: the sum is representable and the set stands",
				[]uint64{1 << 63, 1<<63 - 1}),
		},
	}
}
