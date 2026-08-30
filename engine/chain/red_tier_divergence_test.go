package chain

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/luxfi/ids"
)

// RED: Go's decoder CARRIES the tier byte and refuses at Verify. Rust's
// Cert::decode judges it (src/finality.rs: `_ => return Err(Refusal::Tier)`),
// so for tier >= 5 the three legs disagree about WHERE the same bytes are
// refused. No corpus row carries a tier outside 0..=4, so nothing catches it.
func TestRedTierByteIsCarriedNotJudged(t *testing.T) {
	b, err := os.ReadFile("/home/z/work/lux/consensus2/vectors/cert_verify.json")
	if err != nil {
		t.Skip("no corpus")
	}
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	var wire []byte
	for _, r := range rows {
		if r["name"] == "verify/ok_4" {
			wire, _ = hex.DecodeString(r["wire"].(string))
		}
	}
	if wire == nil {
		t.Fatal("no ok_4 row")
	}
	for _, tier := range []byte{0, 1, 2, 3, 4, 5, 7, 0xFF} {
		w := append([]byte{}, wire...)
		w[3] = tier // version:2 role:1 -> tier at offset 3
		c, err := UnmarshalQuorumCert(w)
		if err != nil {
			t.Errorf("tier=%d: Go DECODE refused (%v) — Rust-like behaviour", tier, err)
			continue
		}
		verr := c.Verify(alwaysTrueVerifier{}, 0)
		t.Logf("tier=%3d  Go decode=OK  Verify=%v  unknownTier=%v",
			tier, verr, errors.Is(verr, ErrQCUnknownTier))
	}
}

type alwaysTrueVerifier struct{}

func (alwaysTrueVerifier) VerifyVote(_ ids.NodeID, _, _ []byte, _ uint64) bool { return true }
