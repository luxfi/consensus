// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package conformance writes the finality standard down as data.
//
// Go is the network. The Rust and C++ implementations of Lux consensus are
// correct exactly insofar as they reproduce what this package emits, byte for
// byte. Everything here is READ from the live definitions — CanonicalVoteMessage,
// QuorumCert.MarshalBinary, config.TwoThirdsStakeFloor, config.HalfStakeFloor,
// chain.NovaSignerFloor, config.FeasibleParams, the Finality ladder — never
// restated. There is one definition of each rule and this is its projection, so
// an edit to a rule moves the corpus and the golden test fails loudly at the
// source rather than at a foreign node six months later.
//
// The failure this exists to prevent already happened: the C++ implementation
// signs a 70-byte message under tag "LUX/chain/vote/v1\0" while Go signs 226
// bytes under "LUX/chain/vote/v2\0". Neither side could ever verify the other,
// and nothing said so. A corpus captured from the Go definitions says so on the
// first run.
//
// Determinism is total: no clock, no randomness, no I/O, no map iteration in the
// output. Every uint64 is written as a decimal STRING — a stake total near 2^64
// loses precision in a JSON parser that keeps numbers as doubles, and a
// conformance comparison must never be lost to formatting.
package conformance

import (
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain"
	"github.com/luxfi/constants"
	"github.com/luxfi/ids"
)

// Corpus is the whole standard: what a validator signs, what a certificate looks
// like on the wire, the thresholds that decide finality, the ladder of
// authorizations, and the committee parameters a node sizes itself to.
type Corpus struct {
	Standard  string      `json:"standard"`
	Source    string      `json:"source"`
	Version   uint16      `json:"version"`
	Vote      Vote        `json:"vote"`
	Cert      Cert        `json:"cert"`
	Ladder    []Rung      `json:"ladder"`
	Threshold Threshold   `json:"threshold"`
	Committee []Committee `json:"committee"`
}

// Vote is the signed message: the statement every quorum argument is about.
type Vote struct {
	Tag    string     `json:"tag"`
	QCType uint8      `json:"qcType"`
	Length int        `json:"length"`
	Layout []string   `json:"layout"`
	Cases  []VoteCase `json:"cases"`
}

// VoteCase is one position and the exact bytes a validator's key signs over it.
type VoteCase struct {
	Name               string `json:"name"`
	Note               string `json:"note"`
	ChainID            string `json:"chainID"`
	Height             string `json:"height"`
	Round              uint32 `json:"round"`
	BlockID            string `json:"blockID"`
	ParentID           string `json:"parentID"`
	CanonicalID        string `json:"canonicalID"`
	ParentCanonicalID  string `json:"parentCanonicalID"`
	ExecutionStateRoot string `json:"executionStateRoot"`
	PayloadRoot        string `json:"payloadRoot"`
	ValidatorSetRoot   string `json:"validatorSetRoot"`
	Accept             bool   `json:"accept"`
	Message            string `json:"message"`
}

// Cert is the gossiped finality witness: the wire form of a QuorumCert.
type Cert struct {
	Layout []string   `json:"layout"`
	Cases  []CertCase `json:"cases"`
}

// CertCase is one assembled certificate and its wire bytes. Votes are recorded
// in the order the encoder emits them, which is the order AssembleQuorumCert
// produced — strictly increasing by node id, whatever order they arrived in.
type CertCase struct {
	Name      string     `json:"name"`
	Note      string     `json:"note"`
	Tier      string     `json:"tier"`
	Threshold uint32     `json:"threshold"`
	Position  string     `json:"position"`
	Votes     []CertVote `json:"votes"`
	Wire      string     `json:"wire"`
	Length    int        `json:"length"`
}

// CertVote is one signed record inside a certificate.
type CertVote struct {
	NodeID    string `json:"nodeID"`
	Accept    bool   `json:"accept"`
	Signature string `json:"signature"`
}

// Rung is one step of the finality ladder and what it authorizes. The three
// booleans are the whole safety boundary: a block below Quasar may drive local
// execution and may never be exported.
type Rung struct {
	Name                             string `json:"name"`
	Value                            uint8  `json:"value"`
	AuthorizesLocalExecution         bool   `json:"authorizesLocalExecution"`
	AuthorizesExport                 bool   `json:"authorizesExport"`
	AuthorizesIrreversibleSettlement bool   `json:"authorizesIrreversibleSettlement"`
}

// Threshold is every finality predicate as a table of inputs and outputs.
type Threshold struct {
	Stake     []StakeFloor `json:"stake"`
	Count     []CountFloor `json:"count"`
	Weighted  []Weighted   `json:"weighted"`
	StakeNote string       `json:"stakeNote"`
	CountNote string       `json:"countNote"`
}

// StakeFloor pairs a total stake with the two floors read off it. A quorum must
// STRICTLY EXCEED the floor for its rung; equality is not a quorum.
type StakeFloor struct {
	Total      string `json:"total"`
	TwoThirds  string `json:"twoThirds"`
	Half       string `json:"half"`
	QuasarNeed string `json:"quasarNeed"`
	NovaNeed   string `json:"novaNeed"`
}

// CountFloor pairs a live validator count with the count-side thresholds.
type CountFloor struct {
	N                int `json:"n"`
	NovaQuorum       int `json:"novaQuorum"`
	NovaSignerFloor  int `json:"novaSignerFloor"`
	NovaBeta         int `json:"novaBeta"`
	CrashTolerance   int `json:"crashTolerance"`
	EqualStakeQuasar int `json:"equalStakeQuasar"`
}

// Weighted is the minimum vote count that CAN reach the ⅔-by-stake predicate for
// a given weight vector — the count the parameter sizer derives from the same
// floor the certificate verifier enforces.
type Weighted struct {
	Note    string   `json:"note"`
	Weights []string `json:"weights"`
	Count   int      `json:"count"`
}

// Committee is the parameter set a node sizes itself to for a live validator
// count on a network. K and alpha are derived purely from the live set; only
// timing varies by network.
type Committee struct {
	Network         string `json:"network"`
	NetworkID       uint32 `json:"networkID"`
	N               int    `json:"n"`
	K               int    `json:"k"`
	AlphaPreference int    `json:"alphaPreference"`
	AlphaConfidence int    `json:"alphaConfidence"`
	Beta            uint32 `json:"beta"`
	BetaVirtuous    int    `json:"betaVirtuous"`
	BetaRogue       int    `json:"betaRogue"`
	BlockTimeMS     int64  `json:"blockTimeMS"`
	RoundTimeoutMS  int64  `json:"roundTimeoutMS"`
}

// Build reads the live definitions and returns the standard. Deterministic: the
// same binary always returns the same value.
func Build() Corpus {
	return Corpus{
		Standard: "lux consensus finality",
		Source:   "github.com/luxfi/consensus",
		Version:  chain.QuorumCertVersion,
		Vote: Vote{
			Tag:    "LUX/chain/vote/v2\x00",
			QCType: uint8(chain.QCFinality),
			Length: len(chain.CanonicalVoteMessage(chain.VotePosition{})),
			Layout: []string{
				"tag:18", "version:2", "qcType:1",
				"chainID:32", "height:8", "round:4",
				"canonicalID:32", "parentCanonicalID:32",
				"executionStateRoot:32", "payloadRoot:32", "validatorSetRoot:32",
				"accept:1",
			},
			Cases: voteCases(),
		},
		Cert: Cert{
			Layout: []string{
				"version:2", "type:1", "tier:1",
				"chainID:32", "height:8", "round:4",
				"blockID:32", "parentID:32",
				"canonicalID:32", "parentCanonicalID:32",
				"executionStateRoot:32", "payloadRoot:32", "validatorSetRoot:32",
				"threshold:4", "voteCount:4",
				"per vote: nodeID:20 accept:1 sigLen:4 sig:sigLen",
			},
			Cases: certCases(),
		},
		Ladder:    ladder(),
		Threshold: thresholds(),
		Committee: committees(),
	}
}

// Marshal renders the corpus as the canonical JSON document the corpus repo
// carries. Indented two spaces, newline-terminated, no HTML escaping — so the
// bytes are readable and a diff is a real diff.
func Marshal(c Corpus) ([]byte, error) {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// fill returns the id whose every byte is b — a value recognisable on sight in a
// hexdump, so a field emitted in the wrong slot reads as wrong rather than
// merely unequal.
func fill(b byte) ids.ID {
	var id ids.ID
	for i := range id {
		id[i] = b
	}
	return id
}

func fillNode(b byte) ids.NodeID {
	var n ids.NodeID
	for i := range n {
		n[i] = b
	}
	return n
}

func u64(v uint64) string { return strconv.FormatUint(v, 10) }

// voteCase records a position and the message the live builder produces for it.
func voteCase(name, note string, pos chain.VotePosition, accept bool) VoteCase {
	var msg []byte
	if accept {
		msg = chain.CanonicalVoteMessage(pos)
	} else {
		msg = chain.CanonicalRejectMessage(pos)
	}
	return VoteCase{
		Name:               name,
		Note:               note,
		ChainID:            pos.ChainID.Hex(),
		Height:             u64(pos.Height),
		Round:              pos.Round,
		BlockID:            pos.BlockID.Hex(),
		ParentID:           pos.ParentID.Hex(),
		CanonicalID:        pos.CanonicalID.Hex(),
		ParentCanonicalID:  pos.ParentCanonicalID.Hex(),
		ExecutionStateRoot: pos.ExecutionStateRoot.Hex(),
		PayloadRoot:        pos.PayloadRoot.Hex(),
		ValidatorSetRoot:   pos.ValidatorSetRoot.Hex(),
		Accept:             accept,
		Message:            hex.EncodeToString(msg),
	}
}

// spec is the reference position: every axis a distinct recognisable value, a
// height and round that exercise every byte of their widths.
func spec() chain.VotePosition {
	return chain.VotePosition{
		ChainID:            fill(0x11),
		Height:             0x0102030405060708,
		Round:              0x0A0B0C0D,
		BlockID:            fill(0x22),
		ParentID:           fill(0x33),
		CanonicalID:        fill(0x44),
		ParentCanonicalID:  fill(0x55),
		ExecutionStateRoot: fill(0x66),
		PayloadRoot:        fill(0x77),
		ValidatorSetRoot:   fill(0x88),
	}
}

func voteCases() []VoteCase {
	// The outer envelope moved, everything signed held fixed. The message must
	// not move with it — that is the whole reason the transport ids are absent.
	outerMoved := spec()
	outerMoved.BlockID = fill(0xE1)
	outerMoved.ParentID = fill(0xE2)

	// A block with no inner/outer split: the canonical slots are unset and the
	// builder binds the outer ids under them. The pair below states the degrade
	// twice — implicitly and explicitly — and both must produce one message.
	degrade := chain.VotePosition{
		ChainID:  fill(0x11),
		Height:   9,
		Round:    1,
		BlockID:  fill(0x22),
		ParentID: fill(0x33),
	}
	degradeExplicit := degrade
	degradeExplicit.CanonicalID = fill(0x22)
	degradeExplicit.ParentCanonicalID = fill(0x33)

	// Every width at its maximum: a builder that truncates a field or writes it
	// little-endian cannot survive this one.
	max := chain.VotePosition{
		ChainID:            fill(0xFF),
		Height:             ^uint64(0),
		Round:              ^uint32(0),
		BlockID:            fill(0xFF),
		ParentID:           fill(0xFF),
		CanonicalID:        fill(0xFF),
		ParentCanonicalID:  fill(0xFF),
		ExecutionStateRoot: fill(0xFF),
		PayloadRoot:        fill(0xFF),
		ValidatorSetRoot:   fill(0xFF),
	}

	return []VoteCase{
		voteCase("zero", "every axis empty, accept — the minimal signed message",
			chain.VotePosition{}, true),
		voteCase("zero_reject", "every axis empty, reject — differs from accept in the final byte alone",
			chain.VotePosition{}, false),
		voteCase("spec", "the reference position; the transport ids (blockID, parentID) do not appear in the message",
			spec(), true),
		voteCase("spec_reject", "the reference position, reject",
			spec(), false),
		voteCase("outer_moved", "the reference position with both transport ids changed — the message is unchanged",
			outerMoved, true),
		voteCase("degrade", "canonical slots unset: the transport ids are bound under them",
			degrade, true),
		voteCase("degrade_explicit", "the same position with canonical == outer stated outright — identical bytes",
			degradeExplicit, true),
		voteCase("max", "every field at its maximum width — catches a truncated or byte-swapped integer",
			max, true),
	}
}

// certCases assembles certificates through the live AssembleQuorumCert and
// records the bytes MarshalBinary produces. Signatures are recognisable fills:
// the codec does not verify them, and a corpus that needed real keys would need
// a key ceremony to be readable.
func certCases() []CertCase {
	pos := spec()

	sig := func(b byte, n int) []byte {
		s := make([]byte, n)
		for i := range s {
			s[i] = b
		}
		return s
	}

	// One vote, the local-execution rung.
	nova := []chain.SignedVote{
		{NodeID: fillNode(0xA1), Accept: true, Signature: sig(0x01, 8)},
	}
	// Three votes handed in DESCENDING order. The encoder must emit them
	// ascending — assembly order is part of the standard, not a detail of the
	// caller, or two nodes holding one quorum would gossip two different certs.
	quasar := []chain.SignedVote{
		{NodeID: fillNode(0xC3), Accept: true, Signature: sig(0x33, 4)},
		{NodeID: fillNode(0xA1), Accept: true, Signature: sig(0x11, 96)},
		{NodeID: fillNode(0xB2), Accept: true, Signature: sig(0x22, 48)},
	}
	// A zero-length signature exercises the length prefix on its own.
	empty := []chain.SignedVote{
		{NodeID: fillNode(0x01), Accept: true, Signature: nil},
	}

	build := func(name, note string, tier chain.Finality, threshold uint32, votes []chain.SignedVote) CertCase {
		cert, err := chain.AssembleQuorumCert(pos, tier, threshold, votes)
		if err != nil {
			panic("conformance: " + name + ": " + err.Error())
		}
		wire, err := cert.MarshalBinary()
		if err != nil {
			panic("conformance: " + name + ": " + err.Error())
		}
		recorded := make([]CertVote, 0, len(cert.Votes))
		for i := range cert.Votes {
			v := &cert.Votes[i]
			recorded = append(recorded, CertVote{
				// Hex, like every other identifier here. NodeID.String() is
				// base58 with a checksum, and a harness in another language
				// should not have to carry that codec to compare an id.
				NodeID:    hex.EncodeToString(v.NodeID[:]),
				Accept:    v.Accept,
				Signature: hex.EncodeToString(v.Signature),
			})
		}
		return CertCase{
			Name:      name,
			Note:      note,
			Tier:      cert.Tier.String(),
			Threshold: cert.Threshold,
			Position:  "spec",
			Votes:     recorded,
			Wire:      hex.EncodeToString(wire),
			Length:    len(wire),
		}
	}

	return []CertCase{
		build("nova_single", "one signer at the local-execution rung — the smallest well-formed certificate",
			chain.Nova, 1, nova),
		build("quasar_sorted", "votes handed in descending order; the wire carries them ascending by node id",
			chain.Quasar, 3, quasar),
		build("empty_signature", "a zero-length signature — the length prefix stands alone",
			chain.Nova, 1, empty),
	}
}

func ladder() []Rung {
	rungs := []chain.Finality{chain.Photon, chain.Wave, chain.Nova, chain.Quasar, chain.Horizon}
	out := make([]Rung, 0, len(rungs))
	for _, f := range rungs {
		out = append(out, Rung{
			Name:                             f.String(),
			Value:                            uint8(f),
			AuthorizesLocalExecution:         f.AuthorizesLocalExecution(),
			AuthorizesExport:                 f.AuthorizesExport(),
			AuthorizesIrreversibleSettlement: f.AuthorizesIrreversibleSettlement(),
		})
	}
	return out
}

func thresholds() Threshold {
	// Totals chosen for the boundaries: the three residues mod 3, the powers,
	// and the top of the range where a naive 2·total overflows.
	totals := []uint64{
		0, 1, 2, 3, 4, 5, 6, 7, 99, 100, 101, 102,
		1 << 20, 1<<32 - 1, 1 << 32,
		4611686018427387904,  // 2^62
		6148914691236517205,  // (2^64-1)/3
		18446744073709551614, // 2^64-2
		18446744073709551615, // 2^64-1 — 2·total overflows, floor(2·total/3) must not
	}
	stake := make([]StakeFloor, 0, len(totals))
	for _, t := range totals {
		two := config.TwoThirdsStakeFloor(t)
		half := config.HalfStakeFloor(t)
		stake = append(stake, StakeFloor{
			Total:      u64(t),
			TwoThirds:  u64(two),
			Half:       u64(half),
			QuasarNeed: u64(two + 1),
			NovaNeed:   u64(half + 1),
		})
	}

	ns := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 11, 20, 21, 22, 100, 1000}
	count := make([]CountFloor, 0, len(ns))
	for _, n := range ns {
		count = append(count, CountFloor{
			N:                n,
			NovaQuorum:       chain.NovaQuorum(n),
			NovaSignerFloor:  chain.NovaSignerFloor(n),
			NovaBeta:         chain.NovaBeta(n),
			CrashTolerance:   chain.CrashTolerance(n),
			EqualStakeQuasar: config.EqualStakeSupermajorityThreshold(n),
		})
	}

	vectors := []struct {
		note    string
		weights []uint64
	}{
		{"equal stake, five validators — collapses to ⌊2n/3⌋+1", []uint64{1, 1, 1, 1, 1}},
		{"equal stake, twenty-one validators — 15, not 14: 14/21 does not strictly exceed ⅔", []uint64{
			1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}},
		{"one validator holds most of the stake", []uint64{100, 1, 1, 1, 1}},
		{"two heavy validators cannot reach ⅔ alone", []uint64{5, 5, 4, 3, 3}},
		{"a dust registration must not move the gate", []uint64{1000, 1000, 1000, 1}},
		{"empty set — no stake model, fail closed", nil},
		{"all weights zero — no stake model, fail closed", []uint64{0, 0, 0}},
	}
	weighted := make([]Weighted, 0, len(vectors))
	for _, v := range vectors {
		ws := make([]string, 0, len(v.weights))
		for _, w := range v.weights {
			ws = append(ws, u64(w))
		}
		weighted = append(weighted, Weighted{
			Note:    v.note,
			Weights: ws,
			Count:   config.WeightedSupermajorityThreshold(v.weights),
		})
	}

	return Threshold{
		Stake:    stake,
		Count:    count,
		Weighted: weighted,
		StakeNote: "a quorum must STRICTLY EXCEED its floor. Quasar (export) needs voted > twoThirds; " +
			"Nova (local execution) needs voted > half. Equality is not a quorum.",
		CountNote: "novaSignerFloor is the count guard the stake predicate cannot give — a lone holder " +
			"of a stake majority must not self-ignite. equalStakeQuasar is the ⅔ count for a uniform set.",
	}
}

func committees() []Committee {
	nets := []struct {
		name string
		id   uint32
	}{
		{"mainnet", constants.MainnetID},
		{"testnet", constants.TestnetID},
		{"local", constants.LocalID},
	}
	ns := []int{1, 4, 5, 11, 21, 100}
	out := make([]Committee, 0, len(nets)*len(ns))
	for _, net := range nets {
		for _, n := range ns {
			p := config.FeasibleParams(net.id, n)
			out = append(out, Committee{
				Network:         net.name,
				NetworkID:       net.id,
				N:               n,
				K:               p.K,
				AlphaPreference: p.AlphaPreference,
				AlphaConfidence: p.AlphaConfidence,
				Beta:            p.Beta,
				BetaVirtuous:    p.BetaVirtuous,
				BetaRogue:       p.BetaRogue,
				BlockTimeMS:     p.BlockTime.Milliseconds(),
				RoundTimeoutMS:  p.RoundTO.Milliseconds(),
			})
		}
	}
	return out
}
