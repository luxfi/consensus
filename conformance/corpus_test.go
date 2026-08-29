// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// corpus_test.go — two independent guards on the standard.
//
// The first is the golden file: the corpus this build produces must equal
// corpus.json byte for byte, so any edit to a finality rule shows up as a diff
// in the standard rather than as a foreign node that cannot verify our
// signatures.
//
// A golden file alone is weak — it can be re-blessed with -update and the
// change disappears. So the second guard states the load-bearing facts OUTRIGHT,
// transcribed from the spec rather than read from the encoder: the message is
// 226 bytes, the tag is "LUX/chain/vote/v2\0", the two-thirds floor is
// floor(2·total/3) computed in arbitrary precision, and the transport ids are
// absent from the signed bytes. Re-blessing the golden does not get past these.
package conformance

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"math/big"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain"
)

var update = flag.Bool("update", false, "rewrite corpus.json from the live definitions")

const goldenPath = "corpus.json"

func TestCorpusMatchesGolden(t *testing.T) {
	got, err := Marshal(Build())
	if err != nil {
		t.Fatalf("marshal corpus: %v", err)
	}

	if *update {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		t.Logf("wrote %s (%d bytes)", goldenPath, len(got))
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run `go test ./conformance -update` to create it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("the corpus moved: a finality rule changed.\n"+
			"Every other implementation of Lux consensus is now wrong against this build.\n"+
			"If the change is intended, run `go test ./conformance -update`, review the diff as a\n"+
			"protocol change, and re-capture luxfi/conformance from it.\ngot %d bytes, want %d",
			len(got), len(want))
	}
}

// TestCorpusIsDeterministic — two builds in one process must agree. A map
// iterated into the output, a clock, or a random id would show up here.
func TestCorpusIsDeterministic(t *testing.T) {
	a, err := Marshal(Build())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	b, err := Marshal(Build())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("two builds of the corpus differ; something in it is not deterministic")
	}
}

// TestGoldenIsValidJSON — the committed file has to parse in the languages that
// read it, not only in the one that wrote it.
func TestGoldenIsValidJSON(t *testing.T) {
	raw, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var c Corpus
	if err := json.Unmarshal(raw, &c); err != nil {
		t.Fatalf("golden does not parse: %v", err)
	}
	if c.Version != chain.QuorumCertVersion {
		t.Errorf("golden version %d, live QuorumCertVersion %d", c.Version, chain.QuorumCertVersion)
	}
}

// TestSignedBytesStatedOutright transcribes the message layout from the spec and
// rebuilds the reference message by hand. It shares no code with the encoder, so
// a reordered field, a changed width, an edited tag or a dropped binding fails
// here even if the golden was re-blessed in the same commit.
func TestSignedBytesStatedOutright(t *testing.T) {
	const tag = "LUX/chain/vote/v2\x00"

	var want []byte
	want = append(want, tag...)
	want = append(want, 0x00, 0x03) // version 3, big-endian
	want = append(want, 0x01)       // qcType = finality
	want = append(want, bytes.Repeat([]byte{0x11}, 32)...)
	want = append(want, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08) // height
	want = append(want, 0x0A, 0x0B, 0x0C, 0x0D)                         // round
	want = append(want, bytes.Repeat([]byte{0x44}, 32)...)              // canonical
	want = append(want, bytes.Repeat([]byte{0x55}, 32)...)              // parent canonical
	want = append(want, bytes.Repeat([]byte{0x66}, 32)...)              // execution state root
	want = append(want, bytes.Repeat([]byte{0x77}, 32)...)              // payload root
	want = append(want, bytes.Repeat([]byte{0x88}, 32)...)              // validator set root
	want = append(want, 0x01)                                           // accept

	if len(want) != 226 {
		t.Fatalf("the transcribed layout is %d bytes, the spec says 226", len(want))
	}

	got := chain.CanonicalVoteMessage(spec())
	if !bytes.Equal(got, want) {
		t.Fatalf("the signed message is not what the spec says.\n got %x\nwant %x", got, want)
	}

	// The corpus must carry exactly these bytes under "spec".
	if hexOf(t, "spec") != hex.EncodeToString(want) {
		t.Error("the corpus's spec case does not carry the transcribed message")
	}

	// Reject differs in the last byte alone — this is what stops an accept
	// signature from being presented as a reject.
	rej := chain.CanonicalRejectMessage(spec())
	if len(rej) != len(got) {
		t.Fatalf("reject message is %d bytes, accept is %d", len(rej), len(got))
	}
	if !bytes.Equal(rej[:len(rej)-1], got[:len(got)-1]) {
		t.Error("accept and reject differ before the final byte")
	}
	if rej[len(rej)-1] != 0x00 || got[len(got)-1] != 0x01 {
		t.Errorf("decision byte is reject=%#x accept=%#x, want 0x00 and 0x01", rej[len(rej)-1], got[len(got)-1])
	}
}

// TestTransportIDsAreNotSigned holds the property the whole canonical/transport
// split exists for: two nodes that executed one inner block sign identical
// bytes however it was wrapped.
func TestTransportIDsAreNotSigned(t *testing.T) {
	base := spec()
	moved := base
	moved.BlockID = fill(0xE1)
	moved.ParentID = fill(0xE2)

	if !bytes.Equal(chain.CanonicalVoteMessage(base), chain.CanonicalVoteMessage(moved)) {
		t.Fatal("moving the transport ids moved the signed message; they are being signed")
	}
	if hexOf(t, "spec") != hexOf(t, "outer_moved") {
		t.Error("the corpus records different messages for the same execution identity")
	}

	// Every signed axis, moved one at a time, must move the message. A field
	// that is silently dropped from the encoder is caught here and nowhere else.
	for _, m := range []struct {
		name string
		edit func(*chain.VotePosition)
	}{
		{"chainID", func(p *chain.VotePosition) { p.ChainID = fill(0xEE) }},
		{"height", func(p *chain.VotePosition) { p.Height++ }},
		{"round", func(p *chain.VotePosition) { p.Round++ }},
		{"canonicalID", func(p *chain.VotePosition) { p.CanonicalID = fill(0xEE) }},
		{"parentCanonicalID", func(p *chain.VotePosition) { p.ParentCanonicalID = fill(0xEE) }},
		{"executionStateRoot", func(p *chain.VotePosition) { p.ExecutionStateRoot = fill(0xEE) }},
		{"payloadRoot", func(p *chain.VotePosition) { p.PayloadRoot = fill(0xEE) }},
		{"validatorSetRoot", func(p *chain.VotePosition) { p.ValidatorSetRoot = fill(0xEE) }},
	} {
		p := spec()
		m.edit(&p)
		if bytes.Equal(chain.CanonicalVoteMessage(p), chain.CanonicalVoteMessage(spec())) {
			t.Errorf("%s is not bound into the signed message", m.name)
		}
	}
}

// TestStakeFloorsStatedOutright recomputes both floors in arbitrary precision.
// The production functions are written to survive a total near 2^64 where 2·total
// overflows; big.Int has no such problem and therefore no shared mistake.
func TestStakeFloorsStatedOutright(t *testing.T) {
	three := big.NewInt(3)
	two := big.NewInt(2)

	for _, row := range Build().Threshold.Stake {
		total, err := strconv.ParseUint(row.Total, 10, 64)
		if err != nil {
			t.Fatalf("total %q: %v", row.Total, err)
		}

		bt := new(big.Int).SetUint64(total)
		wantTwoThirds := new(big.Int).Div(new(big.Int).Mul(bt, two), three).String()
		wantHalf := new(big.Int).Div(bt, two).String()

		if row.TwoThirds != wantTwoThirds {
			t.Errorf("total=%s: twoThirds=%s, floor(2·total/3)=%s", row.Total, row.TwoThirds, wantTwoThirds)
		}
		if row.Half != wantHalf {
			t.Errorf("total=%s: half=%s, floor(total/2)=%s", row.Total, row.Half, wantHalf)
		}

		// The recorded need is the floor plus one — the predicate is STRICTLY
		// greater than the floor, and "≥ floor" is the classic off-by-one that
		// finalizes on exactly two thirds.
		if row.QuasarNeed != new(big.Int).Add(new(big.Int).SetUint64(config.TwoThirdsStakeFloor(total)), big.NewInt(1)).String() {
			t.Errorf("total=%s: quasarNeed=%s is not twoThirds+1", row.Total, row.QuasarNeed)
		}
		if row.NovaNeed != new(big.Int).Add(new(big.Int).SetUint64(config.HalfStakeFloor(total)), big.NewInt(1)).String() {
			t.Errorf("total=%s: novaNeed=%s is not half+1", row.Total, row.NovaNeed)
		}
	}
}

// TestLadderBoundary states the invariant in words: nothing below Quasar may be
// exported, and Nova and brighter may drive local execution.
func TestLadderBoundary(t *testing.T) {
	want := map[string][3]bool{
		//                      local   export  irreversible
		"photon":  {false, false, false},
		"wave":    {false, false, false},
		"nova":    {true, false, false},
		"quasar":  {true, true, false},
		"horizon": {true, true, true},
	}
	rungs := Build().Ladder
	if len(rungs) != len(want) {
		t.Fatalf("ladder has %d rungs, want %d", len(rungs), len(want))
	}
	for _, r := range rungs {
		w, ok := want[r.Name]
		if !ok {
			t.Errorf("unknown rung %q on the ladder", r.Name)
			continue
		}
		got := [3]bool{r.AuthorizesLocalExecution, r.AuthorizesExport, r.AuthorizesIrreversibleSettlement}
		if got != w {
			t.Errorf("%s authorizes %v, want %v", r.Name, got, w)
		}
	}
}

// TestNovaSitsBelowQuasar — the two rungs are distinct authorizations, so on any
// set where they can differ the local-execution gate must be the lower one. If
// they ever coincide, Nova has stopped being a bare majority and export has
// stopped being ⅔.
func TestNovaSitsBelowQuasar(t *testing.T) {
	for _, row := range Build().Threshold.Stake {
		total, err := strconv.ParseUint(row.Total, 10, 64)
		if err != nil {
			t.Fatalf("total %q: %v", row.Total, err)
		}
		half := config.HalfStakeFloor(total)
		twoThirds := config.TwoThirdsStakeFloor(total)
		if half > twoThirds {
			t.Errorf("total=%d: the majority floor %d is above the supermajority floor %d", total, half, twoThirds)
		}
		if total >= 6 && half >= twoThirds {
			t.Errorf("total=%d: majority floor %d does not sit below supermajority floor %d", total, half, twoThirds)
		}
	}
}

// TestCertWireIsSortedByNodeID — assembly order is part of the standard. Two
// nodes holding one quorum must gossip one certificate, so the encoder emits
// votes ascending by node id whatever order they were collected in.
func TestCertWireIsSortedByNodeID(t *testing.T) {
	for _, c := range Build().Cert.Cases {
		var prev string
		for i, v := range c.Votes {
			if i > 0 && v.NodeID <= prev {
				t.Errorf("%s: vote %d node id %s does not follow %s", c.Name, i, v.NodeID, prev)
			}
			prev = v.NodeID
		}
	}
}

// hexOf returns the recorded message for a named vote case.
func hexOf(t *testing.T, name string) string {
	t.Helper()
	for _, c := range Build().Vote.Cases {
		if c.Name == name {
			return c.Message
		}
	}
	t.Fatalf("no vote case named %q in the corpus", name)
	return ""
}

// TestEveryCaseIsFullWidth — a message that came out short means a field was
// dropped; every case must be the one fixed length the layout implies.
func TestEveryCaseIsFullWidth(t *testing.T) {
	c := Build()
	for _, vc := range c.Vote.Cases {
		raw, err := hex.DecodeString(vc.Message)
		if err != nil {
			t.Fatalf("%s: %v", vc.Name, err)
		}
		if len(raw) != c.Vote.Length {
			t.Errorf("%s: message is %d bytes, the standard is %d", vc.Name, len(raw), c.Vote.Length)
		}
		if !strings.HasPrefix(vc.Message, hex.EncodeToString([]byte(c.Vote.Tag))) {
			t.Errorf("%s: message does not open with the domain tag", vc.Name)
		}
	}
}
