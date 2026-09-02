// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// guard_store_edges_test.go — what the durable guard and the regime report do
// when the thing they depend on is not there.
//
// Both are answers about the node's own health, and both have the same failure
// mode if they are lenient: a Persist that could not write must not report that
// it did, and a Mode() that cannot witness finality post-quantum must not report
// a regime that says it can. In each case the wrong answer is the reassuring one,
// which is why it is the one worth testing.
package chain

import (
	"context"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/ids"
)

// TestAPersistThatCannotWriteDoesNotAdvanceTheFloor is the sharpest of these.
// The in-memory floor is advanced only AFTER the durable write commits, so a
// failed write leaves the guard reporting the floor it can actually prove. The
// other order would let a node whose disk is gone report a decided-through it
// would not recover after a restart — the exact window the floor exists to close.
func TestAPersistThatCannotWriteDoesNotAdvanceTheFloor(t *testing.T) {
	// A path whose parent does not exist: reading it is a plain "not there", so
	// the store opens as a fresh signer, and writing it cannot succeed.
	path := filepath.Join(t.TempDir(), "gone", "vote-guard")
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("a missing guard file is a fresh signer, not an error: %v", err)
	}

	if err := store.Persist(map[SlotKey]ids.ID{{Height: 1}: ids.GenerateTestID()}, 9); err == nil {
		t.Fatal("a Persist that could not write reported success")
	}
	if got := store.FinalizedThrough(); got != 0 {
		t.Fatalf("the floor advanced to %d on a write that did not commit", got)
	}
}

// TestAnUnreadableGuardIsARefusal separates the two ways a guard file can fail
// to load. A file that is simply absent is a fresh validator and starts. A file
// that is there and cannot be read is equivocation memory this node holds and
// cannot see, and starting on it is how a restarted signer signs against itself.
func TestAnUnreadableGuardIsARefusal(t *testing.T) {
	dir := t.TempDir()

	// A directory where the guard file should be: present, and not readable as a
	// file. The distinction the code draws is os.ErrNotExist versus anything else.
	if _, err := OpenVoteGuard(dir); err == nil {
		t.Fatal("a guard path that is not a readable file was opened as a fresh signer")
	}

	// And the control, one directory along: genuinely absent is fresh.
	fresh, err := OpenVoteGuard(filepath.Join(dir, "vote-guard"))
	if err != nil {
		t.Fatalf("an absent guard is a fresh signer: %v", err)
	}
	if len(fresh.Snapshot()) != 0 {
		t.Fatal("a fresh signer came back with bindings")
	}
}

// TestALegacySnapshotDecodesAcrossTheUpgrade holds the compatibility clause: a
// snapshot written before the durable floor existed still loads, with the floor
// reading zero. Bricking on it would take a validator's equivocation memory away
// at exactly the moment it upgrades, which is when it restarts.
func TestALegacySnapshotDecodesAcrossTheUpgrade(t *testing.T) {
	canonical := ids.GenerateTestID()

	// The v1 layout by hand: magic | ver=1 | count | records | crc. No floor.
	buf := make([]byte, 0, voteGuardHdrLenV1+voteGuardRecLen+4)
	buf = append(buf, voteGuardMagic...)
	buf = append(buf, voteGuardVersionV1)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], 1)
	buf = append(buf, u32[:]...)
	var height [8]byte
	binary.BigEndian.PutUint64(height[:], 12)
	buf = append(buf, height[:]...)
	buf = append(buf, make([]byte, 32)...) // the reserved slot where the epoch used to live
	buf = append(buf, canonical[:]...)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(buf, voteGuardCRC))
	buf = append(buf, crc[:]...)

	path := filepath.Join(t.TempDir(), "vote-guard")
	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	store, err := OpenVoteGuard(path)
	if err != nil {
		t.Fatalf("a legacy snapshot must decode: %v", err)
	}
	if got := store.Snapshot()[SlotKey{Height: 12}]; got != canonical {
		t.Fatalf("the legacy binding came back as %v, want %v", got, canonical)
	}
	if store.FinalizedThrough() != 0 {
		t.Fatalf("a legacy snapshot carries no floor, got %d", store.FinalizedThrough())
	}

	// The version selects WHICH header to expect, and the length is then checked
	// against that choice — not against the shorter of the two. A v2 frame long
	// enough to be a complete v1 one is still short of a v2 header, and reading
	// it under v1's layout would take the first four bytes of the floor as the
	// record count. That is the one length only the second check can catch.
	shortV2 := append([]byte{}, voteGuardMagic...)
	shortV2 = append(shortV2, voteGuardVersion)
	shortV2 = append(shortV2, make([]byte, voteGuardHdrLen+3-len(shortV2))...)
	if len(shortV2) < voteGuardHdrLenV1+4 || len(shortV2) >= voteGuardHdrLen+4 {
		t.Fatalf("the fixture is %d bytes, which is not between a v1 and a v2 header", len(shortV2))
	}
	if _, _, err := decodeVoteGuard(shortV2); err == nil {
		t.Fatal("a v2 frame shorter than a v2 header decoded")
	}
	if _, _, err := decodeVoteGuard(buf[:voteGuardHdrLenV1]); err == nil {
		t.Fatal("a frame with no room for its checksum decoded")
	}
}

// TestFsyncingADirectoryThatIsNotThereIsNotFatal holds the deliberate leniency at
// the end of the atomic write. The data file is already on stable storage by then;
// only the rename's durability rides on the directory fsync, and the binding is
// re-persisted on the next vote regardless. Failing here would turn a filesystem
// that does not support the call into a validator that cannot sign.
func TestFsyncingADirectoryThatIsNotThereIsNotFatal(t *testing.T) {
	if err := fsyncDir(filepath.Join(t.TempDir(), "not-a-directory")); err != nil {
		t.Fatalf("a directory that cannot be opened must be tolerated: %v", err)
	}
	if err := fsyncDir(t.TempDir()); err != nil {
		t.Fatalf("a real directory must fsync: %v", err)
	}
}

// TestAnUnwiredGuardLeavesTheEngineUnguarded covers the option's own nil case.
// The node passes whatever it built, and a nil store must leave the engine with
// no durable memory rather than panic seeding from it — the in-memory vote-once
// rule still holds within the process, and only the memory of what was signed
// BEFORE this process started is missing.
func TestAnUnwiredGuardLeavesTheEngineUnguarded(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()

	e := NewWithConfig(Config{Params: params4()},
		WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
		WithVoteGuard(nil))
	if e.voteGuard != nil {
		t.Fatal("a nil store was wired as a guard")
	}
	if err := e.Start(context.Background(), true); err != nil {
		t.Fatalf("an engine with no durable guard must still start: %v", err)
	}
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
}

// TestAMultiValidatorEngineRefusesToStartWithoutAVerifier is the door the test
// above had to be wired through, and it is the more important of the two: a K>1
// engine that cannot check a signature would count votes it cannot attribute.
// It refuses to start rather than run as a majority of whoever is talking.
func TestAMultiValidatorEngineRefusesToStartWithoutAVerifier(t *testing.T) {
	e := NewWithConfig(Config{Params: params4()})
	if err := e.Start(context.Background(), true); err == nil {
		_ = e.Stop(context.Background())
		t.Fatal("a multi-validator engine started with no way to verify a vote")
	}
}

// TestTheZeroAuthorityTokenCanNeverBePromoted holds the one thing the unexported
// promoter is for: it is used only on paths that have just verified a cert, and
// it refuses nil so the zero VerifiedQuorumCert — which carries no cert and
// would answer nil to every reader — can never become the authority token.
func TestTheZeroAuthorityTokenCanNeverBePromoted(t *testing.T) {
	if _, ok := wrapVerifiedCert(nil); ok {
		t.Fatal("the zero token was promoted to a finality authority")
	}

	f := newCertFixture(t, 4)
	cert := f.cert(t, Quasar, uint32(NovaSignerFloor(4)), 4)
	tok, ok := wrapVerifiedCert(cert)
	if !ok || tok.Cert() != cert {
		t.Fatal("a verified cert must promote to the token carrying it")
	}
	if (VerifiedQuorumCert{}).Cert() != nil {
		t.Fatal("the zero token answered with a cert")
	}
}

// TestAStrictPQChainWithNoWitnessIsNotQuorumFinality is the regime report's own
// fail-closed clause. Everything else is wired — verifier, cert gossip, a stake
// source — and the chain still must not claim quorum finality it cannot witness
// post-quantum, because a bridge reads that word and settles on it.
func TestAStrictPQChainWithNoWitnessIsNotQuorumFinality(t *testing.T) {
	vs := newTestValidatorSet(4)
	chainID := ids.GenerateTestID()

	build := func(strict bool) *Transitive {
		e := NewWithConfig(Config{Params: params4()},
			WithQuorumCert(chainID, vs.nodeID(0), vs, &recordingGossiper{}, vs.signerFor(0)),
			WithStakeWeighting(vs),
			WithStrictPQ(strict))
		if err := e.Start(context.Background(), true); err != nil {
			t.Fatalf("Start: %v", err)
		}
		t.Cleanup(func() { _ = e.Stop(context.Background()) })
		return e
	}

	// The control: the same wiring on a non-strict chain IS quorum finality, so
	// the refusal below is about the profile and not about a missing dependency.
	if got := build(false).Mode(); got != ModeQuorumFinality {
		t.Fatalf("a fully wired non-strict chain reports %s, want quorum-finality", got)
	}
	if got := build(true).Mode(); got != ModeUnknown {
		t.Fatalf("a strict-PQ chain with no witness reports %s, want unknown", got)
	}
}
