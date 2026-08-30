package chain

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

// RED: the C++ harness's cert_wire_strict section ASSERTS what Go refuses; it
// does not read Go's verdict from the corpus. This runs the identical mutations
// through Go and reports the truth.
func TestRedStrictWireMutations(t *testing.T) {
	b, err := os.ReadFile("/home/z/work/lux/consensus2/vectors/cert_wire.json")
	if err != nil {
		t.Skip("no corpus")
	}
	var rows []map[string]any
	if err := json.Unmarshal(b, &rows); err != nil {
		t.Fatal(err)
	}
	const header = 280
	const nodeLen = 20
	for _, r := range rows {
		name, _ := r["name"].(string)
		w, err := hex.DecodeString(r["wire"].(string))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := UnmarshalQuorumCert(w); err != nil {
			t.Errorf("%s: the corpus wire itself does not decode: %v", name, err)
		}
		// trailing byte
		if _, err := UnmarshalQuorumCert(append(append([]byte{}, w...), 0x00)); err == nil {
			t.Errorf("%s: Go ACCEPTED a trailing byte", name)
		}
		// every truncation
		bad := 0
		for n := 0; n < len(w); n++ {
			if _, err := UnmarshalQuorumCert(w[:n]); err == nil {
				bad++
			}
		}
		if bad != 0 {
			t.Errorf("%s: Go ACCEPTED %d truncations", name, bad)
		}
	}
	// mutations that need a vote present
	w, _ := hex.DecodeString(rows[0]["wire"].(string))
	if len(w) <= header {
		t.Fatal("first row has no vote")
	}
	huge := append([]byte{}, w...)
	for i := header - 4; i < header; i++ {
		huge[i] = 0xFF
	}
	if _, err := UnmarshalQuorumCert(huge); err == nil {
		t.Error("Go ACCEPTED vote_count=0xFFFFFFFF")
	}
	junkAccepted := 0
	for v := 2; v < 256; v++ {
		j := append([]byte{}, w...)
		j[header+nodeLen] = byte(v)
		if _, err := UnmarshalQuorumCert(j); err == nil {
			junkAccepted++
		}
	}
	if junkAccepted != 0 {
		t.Errorf("Go ACCEPTED %d of 254 non-canonical accept bytes", junkAccepted)
	}
	t.Logf("Go verdicts match the C++ assertions: trailing/truncation/huge-count/accept-byte all REFUSED")
}
