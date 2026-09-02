// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// vote_wire_guard_test.go — the two durable surfaces a peer can reach: the vote
// frame it sends, and the equivocation memory this node keeps about what it
// signed.
//
// Both are read from bytes this node did not write. The frame comes straight off
// the gossip envelope, and the guard file comes off a disk that may have been
// truncated by a crash mid-write, so both decoders have to be total: a refusal
// for every shape that is not exactly one well-formed message, and no allocation
// sized from a number the input chose.
//
// The guard's refusal is the sharper of the two, because it is the only one that
// stops the node. A snapshot that does not decode is not an empty snapshot — it
// is equivocation memory this node cannot read, and starting without it is how a
// restarted signer signs the sibling it already signed against.
package chain

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// TestTheVoteFrameRoundTrips is the control for the refusals below, and the
// boundary that is easiest to get wrong: a zero-length signature is a
// well-formed frame, because the length prefix is what says so.
func TestTheVoteFrameRoundTrips(t *testing.T) {
	for _, row := range []struct {
		holds string
		sig   []byte
	}{
		{"an ordinary signature", []byte("a signature of some length")},
		{"an empty signature, which the length prefix states rather than implies", nil},
		{"a single byte", []byte{0x00}},
	} {
		t.Run(row.holds, func(t *testing.T) {
			want := ids.GenerateTestNodeID()
			frame, err := encodeSignedVote(want, row.sig)
			if err != nil {
				t.Fatalf("encodeSignedVote: %v", err)
			}
			got, sig, err := decodeSignedVote(frame)
			if err != nil {
				t.Fatalf("decodeSignedVote: %v", err)
			}
			if got != want {
				t.Fatalf("node id came back as %s, want %s", got, want)
			}
			if len(sig) != len(row.sig) || (len(sig) > 0 && string(sig) != string(row.sig)) {
				t.Fatalf("signature came back as %x, want %x", sig, row.sig)
			}
		})
	}
}

// TestTheVoteFrameRefusesEveryOtherShape walks the decoder's refusals. The last
// row is the one that matters most: the declared length is checked against what
// is actually there BEFORE the buffer is allocated, so a frame claiming four
// gigabytes of signature costs a comparison rather than four gigabytes.
func TestTheVoteFrameRefusesEveryOtherShape(t *testing.T) {
	good, err := encodeSignedVote(ids.GenerateTestNodeID(), []byte("signature"))
	if err != nil {
		t.Fatalf("encodeSignedVote: %v", err)
	}

	claiming := func(n uint32, body []byte) []byte {
		frame := make([]byte, 0, ids.NodeIDLen+4+len(body))
		frame = append(frame, make([]byte, ids.NodeIDLen)...)
		var u32 [4]byte
		binary.BigEndian.PutUint32(u32[:], n)
		frame = append(frame, u32[:]...)
		return append(frame, body...)
	}

	for _, row := range []struct {
		holds string
		frame []byte
	}{
		{"nothing at all", nil},
		{"a node id and no length prefix", make([]byte, ids.NodeIDLen)},
		{"a length prefix one byte short", make([]byte, ids.NodeIDLen+3)},
		{"trailing bytes after the signature the prefix declared", append(append([]byte{}, good...), 0xFF)},
		{"one byte fewer than the prefix declared", good[:len(good)-1]},
		{"a length longer than the body", claiming(64, []byte("short"))},
		{"a length shorter than the body", claiming(1, []byte("long enough to notice"))},
		{"four gigabytes of signature that are not there", claiming(math.MaxUint32, []byte{0x01})},
	} {
		t.Run(row.holds, func(t *testing.T) {
			if _, _, err := decodeSignedVote(row.frame); !errors.Is(err, ErrVoteWireCorrupt) {
				t.Fatalf("want ErrVoteWireCorrupt, got %v", err)
			}
		})
	}
}

// TestAnUndecodableVoteIsDroppedNotCounted joins the codec to the ingest path: a
// frame the decoder refuses must not reach the verifier, and must not be counted
// toward anything. The runtime carries no verifier here, which is the second
// door and is checked the same way — an engine that cannot check signatures
// counts no votes rather than counting them unchecked.
func TestAnUndecodableVoteIsDroppedNotCounted(t *testing.T) {
	rt, _ := servingNode(t, 1)
	// Wired as the node wires it. Both refusal paths below log the drop, and
	// NewRuntime does not fill in a logger, so a runtime built without one is not
	// the configuration this ingest path is written for.
	rt.config.Logger = log.Noop()

	if rt.HandleIncomingVote(ids.GenerateTestID(), []byte{0x01, 0x02}) {
		t.Fatal("a frame the decoder refused was counted")
	}
	good, err := encodeSignedVote(ids.GenerateTestNodeID(), []byte("signature"))
	if err != nil {
		t.Fatalf("encodeSignedVote: %v", err)
	}
	if rt.HandleIncomingVote(ids.GenerateTestID(), good) {
		t.Fatal("a well-formed vote was counted by an engine with no verifier wired")
	}
}

// TestTheGuardRemembersAcrossARestart is the whole point of the durable guard:
// what this node signed before the crash is what it refuses to contradict after
// it. Both halves have to survive — the per-height bindings and the
// decided-through floor — because the floor is what makes the gate span a
// restart at heights the bindings were already pruned below.
func TestTheGuardRemembersAcrossARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vote-guard")

	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}
	// A fresh signer has bound nothing, and OpenVoteGuard creates no file — so
	// "not there yet" and "destroyed" are the same observation and the operator
	// is told which by GuardConfigured, not by this.
	if len(store.Snapshot()) != 0 || store.FinalizedThrough() != 0 {
		t.Fatal("a fresh guard came back with memory it never wrote")
	}
	locator, ok := store.(VoteGuardLocator)
	if !ok {
		t.Fatal("the production store must be able to say where it writes")
	}
	if locator.Path() != path {
		t.Fatalf("the guard reports %q, want %q", locator.Path(), path)
	}
	if locator.Exists() {
		t.Fatal("opening the guard created a file — the first committed Persist does that")
	}

	first, second := ids.GenerateTestID(), ids.GenerateTestID()
	bindings := map[SlotKey]ids.ID{{Height: 7}: first, {Height: 8}: second}
	if err := store.Persist(bindings, 6); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if !locator.Exists() {
		t.Fatal("a committed Persist left no file on disk")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	restarted, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	snap := restarted.Snapshot()
	if len(snap) != 2 || snap[SlotKey{Height: 7}] != first || snap[SlotKey{Height: 8}] != second {
		t.Fatalf("the bindings did not survive the restart: %v", snap)
	}
	if restarted.FinalizedThrough() != 6 {
		t.Fatalf("the decided-through floor came back as %d, want 6", restarted.FinalizedThrough())
	}
}

// TestTheDurableFloorNeverGoesBackwards holds the monotonicity the on-disk floor
// is written with. A best-effort re-persist that races a higher finalize passes
// a stale-lower floor, and taking it would lower the durable refusal — reopening
// the window the floor exists to close.
func TestTheDurableFloorNeverGoesBackwards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vote-guard")
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("OpenVoteGuard: %v", err)
	}

	if err := store.Persist(map[SlotKey]ids.ID{}, 20); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if err := store.Persist(map[SlotKey]ids.ID{}, 5); err != nil {
		t.Fatalf("Persist: %v", err)
	}
	if got := store.FinalizedThrough(); got != 20 {
		t.Fatalf("the in-memory floor regressed to %d, want 20", got)
	}

	reopened, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got := reopened.FinalizedThrough(); got != 20 {
		t.Fatalf("the durable floor regressed to %d, want 20", got)
	}
}

// TestAGuardThatDoesNotDecodeStopsTheNode is the fail-closed rule, walked shape
// by shape. Every one of these is a file that EXISTS, so treating it as an empty
// snapshot would look like a fresh validator and start a signer with no memory
// of what it already signed. The refusal is what makes the node stop instead.
func TestAGuardThatDoesNotDecodeStopsTheNode(t *testing.T) {
	// A real snapshot, to corrupt one way at a time.
	good := encodeVoteGuard(map[SlotKey]ids.ID{{Height: 1}: ids.GenerateTestID()}, 3)

	withBadCRC := append([]byte{}, good...)
	withBadCRC[len(withBadCRC)-1] ^= 0xFF

	badMagic := append([]byte{}, good...)
	badMagic[0] ^= 0xFF

	// A count the body does not carry: reframe the header to claim two records
	// and re-checksum, so the framing check is what refuses it rather than the CRC.
	overCount := append([]byte{}, good...)
	binary.BigEndian.PutUint32(overCount[len(voteGuardMagic)+1:len(voteGuardMagic)+5], 2)
	binary.BigEndian.PutUint32(overCount[len(overCount)-4:],
		crc32.Checksum(overCount[:len(overCount)-4], voteGuardCRC))

	badVersion := append([]byte{}, good...)
	badVersion[len(voteGuardMagic)] = 0xFE

	for _, row := range []struct {
		holds string
		data  []byte
	}{
		{"an empty file", nil},
		{"a file shorter than a header", good[:4]},
		{"a file whose magic is not ours", badMagic},
		{"a version this build does not know", badVersion},
		{"a record count the body does not carry", overCount},
		{"a body that does not match its checksum", withBadCRC},
		{"a snapshot truncated by a crash mid-write", good[:len(good)-6]},
	} {
		t.Run(row.holds, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "vote-guard")
			if err := os.WriteFile(path, row.data, 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			store, err := OpenVoteGuard(path)
			if !errors.Is(err, errVoteGuardCorrupt) {
				t.Fatalf("want the corrupt-guard refusal, got %v", err)
			}
			if store != nil {
				t.Fatal("a refused open returned a store — the node would start on it")
			}
		})
	}

	// And the control: the uncorrupted snapshot opens, so each row above is
	// about the one thing it changed.
	path := filepath.Join(t.TempDir(), "vote-guard")
	if err := os.WriteFile(path, good, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := OpenVoteGuard(path); err != nil {
		t.Fatalf("the uncorrupted snapshot must open: %v", err)
	}
}

// TestTheGuardLocatorIsTotalOnATypedNil is the shape that defeats every
// `store == nil` guard: an interface holding a nil *fileVoteGuard is not nil.
// Introspection asks these two on whatever it was handed, so both must answer
// rather than panic — and Exists must answer false, because a store that is not
// there has nothing on disk.
func TestTheGuardLocatorIsTotalOnATypedNil(t *testing.T) {
	var typedNil *fileVoteGuard
	var locator VoteGuardLocator = typedNil

	if locator == nil {
		t.Fatal("a typed nil in an interface is not nil — the premise of this test")
	}
	if got := locator.Path(); got != "" {
		t.Fatalf("a store that is not there reports the path %q", got)
	}
	if locator.Exists() {
		t.Fatal("a store that is not there reported a snapshot on disk")
	}
}

// TestAtomicWriteLeavesNoTemporaryBehind holds the crash-safety path's tidiness:
// the replace goes through a fixed-name temp file, and a successful write must
// leave the destination complete and the temp gone. A temp left behind is the
// next write's silently-truncated input.
func TestAtomicWriteLeavesNoTemporaryBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "snapshot")

	if err := writeFileAtomic(path, dir, []byte("first")); err != nil {
		t.Fatalf("writeFileAtomic: %v", err)
	}
	if err := writeFileAtomic(path, dir, []byte("second, longer")); err != nil {
		t.Fatalf("writeFileAtomic (replace): %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "second, longer" {
		t.Fatalf("the file holds %q, want the second write", got)
	}
	if _, err := os.Stat(path + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the temp file survived a successful write")
	}

	// A destination whose directory does not exist is an error, not a partial
	// write somewhere else.
	if err := writeFileAtomic(filepath.Join(dir, "missing", "snapshot"), dir, []byte("x")); err == nil {
		t.Fatal("writing into a directory that does not exist reported success")
	}
}
