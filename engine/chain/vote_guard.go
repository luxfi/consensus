// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"

	"github.com/luxfi/ids"
)

// vote_guard.go — the DURABLE backing for the per-height non-equivocation guard.
//
// The in-memory committedSlot map (engine.go) makes an honest double-vote
// impossible WHILE THE PROCESS LIVES. But a crash/restart between casting a vote
// and finalizing the height clears that map, so the restarted node would forget
// every unfinalized height it already signed and could sign a CONFLICTING sibling
// at one of them — a cross-node fork with ZERO Byzantine intent under correlated
// restarts (rolling upgrade / k8s eviction / OOM at a contested height). This is
// Red's HIGH-1.
//
// The fix mirrors Tendermint's priv_validator_state.json: BEFORE the node signs
// an accept, the (height,epoch)->canonical binding is written to stable storage
// and fsync'd; on startup the surviving bindings are reloaded so the guard's
// memory spans the restart. reserveSlotForSign returns true (permitting the
// signature) ONLY after the durable write commits — a persistence failure FAILS
// CLOSED (the vote is refused, never cast).

// SlotKey identifies one per-(height, epoch) non-equivocation slot. The epoch is
// the block's ValidatorSetRoot — the validator-set commitment ALREADY folded into
// the signed vote message (cert.go: canonicalVoteMessageFor). Keying by
// (height, epoch) rather than height alone matches the consensus2 reference
// (SlotKey = {height, epoch}) and is what lets a contested height be re-proposed
// under a NEW validator set at a set-change boundary without every validator that
// signed the OLD-epoch block refusing the new one (a liveness stall — Red's
// HIGH-2). Cross-epoch SAFETY is preserved independently: the ledger's per-height
// finalize gate (ErrHeightAlreadyFinalized) admits at most one finalized block per
// height, and the validator set at a height is a deterministic function of the
// uniquely-finalized lower chain, so two distinct epochs can co-exist at one
// height only atop a lower-height fork — which the SAME guard prevents by
// induction.
//
// A fixed-set chain leaves Epoch == ids.Empty, so SlotKey degrades to height-only
// — byte-identical to the pre-epoch guard (backward-safe no-op).
type SlotKey struct {
	Height uint64
	Epoch  ids.ID
}

// VoteGuardStore is the durable backing a SIGNING validator uses so a per-height
// binding survives a crash (HIGH-1). It is wired with WithVoteGuard; a node
// constructed without one runs the guard memory-only (correct for tests and
// verify-only nodes, but a crash may forget an unfinalized binding — Start warns
// when a signer has no store).
//
// Persist MUST NOT return until the bytes are on stable storage (fsync). The
// engine casts a vote ONLY after Persist returns nil, so a durable-write failure
// fails CLOSED. Persist is called under the engine's slotMu, so implementations
// need not be safe for concurrent use.
type VoteGuardStore interface {
	// Persist atomically and durably records the COMPLETE set of live
	// (height,epoch)->canonical bindings, fsync'd. It must not mutate the map.
	Persist(bindings map[SlotKey]ids.ID) error
	// Snapshot returns the bindings recovered from stable storage at open time.
	// The engine seeds committedSlot from it so the guard spans a restart.
	Snapshot() map[SlotKey]ids.ID
	// Close releases any held resource.
	Close() error
}

// WithVoteGuard wires the durable non-equivocation backing (HIGH-1) and seeds the
// in-memory guard from whatever survived the last run, so a restarted signer
// refuses to sign a conflicting sibling at any height it had already committed to
// before the crash. Applied either through NewWithConfig's option loop or
// post-construction (NewRuntime) — both run after committedSlot is initialized.
func WithVoteGuard(store VoteGuardStore) Option {
	return func(t *Transitive) {
		t.voteGuard = store
		if store == nil {
			return
		}
		for k, v := range store.Snapshot() {
			t.committedSlot[k] = v
		}
	}
}

// -----------------------------------------------------------------------------
// fileVoteGuard — the one concrete VoteGuardStore: a single atomically-replaced,
// crc-checked snapshot file. The live binding set is small and bounded (it is
// pruned to the unfinalized window on every finalize — see pruneCommittedSlotsBelow),
// so writing the whole set per new binding is a few KB at block cadence — the
// simplest structure that is complete and crash-safe (no WAL/compaction machinery,
// no torn-append edge cases). Atomicity is the standard write-temp + fsync + rename
// + fsync-dir: a crash leaves EITHER the old or the new complete file, never a torn
// one.
// -----------------------------------------------------------------------------

const (
	voteGuardMagic   = "LUXVGUARD"                 // 9 bytes
	voteGuardVersion = byte(1)                     // bumped only on a wire change
	voteGuardHdrLen  = len(voteGuardMagic) + 1 + 4 // magic | ver | count(u32)
	voteGuardRecLen  = 8 + 32 + 32                 // height(u64) | epoch(32) | canonical(32)
)

var voteGuardCRC = crc32.MakeTable(crc32.Castagnoli)

// errVoteGuardCorrupt marks an unreadable/tampered snapshot — surfaced by
// OpenVoteGuard so a signing node fails CLOSED (refuses to start with
// unverifiable equivocation memory rather than silently start with none).
var errVoteGuardCorrupt = errors.New("vote guard snapshot is corrupt")

type fileVoteGuard struct {
	path string // the snapshot file
	dir  string // parent dir, fsync'd after each rename so the rename itself is durable
	snap map[SlotKey]ids.ID
}

// OpenVoteGuard opens (or initializes) the durable vote-once guard at path. A
// missing file is a fresh validator (empty snapshot). A present-but-corrupt file
// is a hard error: a signing validator must not start with equivocation memory it
// cannot trust — the caller (the node) refuses to build the chain, fail-closed.
func OpenVoteGuard(path string) (VoteGuardStore, error) {
	g := &fileVoteGuard{
		path: path,
		dir:  filepath.Dir(path),
		snap: map[SlotKey]ids.ID{},
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return g, nil // fresh node: no prior signings
	}
	if err != nil {
		return nil, fmt.Errorf("read vote guard %s: %w", path, err)
	}
	snap, err := decodeVoteGuard(data)
	if err != nil {
		return nil, fmt.Errorf("%w at %s: %v (refusing to start with unverifiable equivocation memory)",
			errVoteGuardCorrupt, path, err)
	}
	g.snap = snap
	return g, nil
}

// Persist writes the complete binding set atomically and durably. It is called
// under the engine's slotMu, so the fixed-name temp file is never contended.
func (g *fileVoteGuard) Persist(bindings map[SlotKey]ids.ID) error {
	buf := encodeVoteGuard(bindings)

	tmp := g.path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(buf); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	// fsync the data BEFORE the rename — the bytes must be on stable storage so a
	// crash after the rename recovers a complete file.
	if err := f.Sync(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// Atomic replace: on POSIX rename(2) is atomic, so a reader (open) sees either
	// the old or the new complete file, never a partial one.
	if err := os.Rename(tmp, g.path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// fsync the directory so the rename itself survives a crash.
	return fsyncDir(g.dir)
}

// Snapshot returns the bindings recovered at open time (used once to seed the
// engine's committedSlot). After open, the engine's map is the source of truth.
func (g *fileVoteGuard) Snapshot() map[SlotKey]ids.ID { return g.snap }

// Close is a no-op: the store holds no file handle open between writes.
func (g *fileVoteGuard) Close() error { return nil }

// encodeVoteGuard serializes the binding set: magic | ver | count | records | crc.
// Record order is unspecified (the decoder rebuilds a map); every field is
// fixed-width so the frame is length-free.
func encodeVoteGuard(bindings map[SlotKey]ids.ID) []byte {
	buf := make([]byte, 0, voteGuardHdrLen+voteGuardRecLen*len(bindings)+4)
	buf = append(buf, voteGuardMagic...)
	buf = append(buf, voteGuardVersion)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(len(bindings)))
	buf = append(buf, u32[:]...)
	var u64 [8]byte
	for k, canonical := range bindings {
		binary.BigEndian.PutUint64(u64[:], k.Height)
		buf = append(buf, u64[:]...)
		buf = append(buf, k.Epoch[:]...)
		buf = append(buf, canonical[:]...)
	}
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(buf, voteGuardCRC))
	buf = append(buf, crc[:]...)
	return buf
}

// decodeVoteGuard is the inverse of encodeVoteGuard. Any framing/CRC mismatch is
// an error (the snapshot is untrustworthy) — never a silent partial read.
func decodeVoteGuard(data []byte) (map[SlotKey]ids.ID, error) {
	if len(data) < voteGuardHdrLen+4 {
		return nil, fmt.Errorf("too short (%d bytes)", len(data))
	}
	if string(data[:len(voteGuardMagic)]) != voteGuardMagic {
		return nil, errors.New("bad magic")
	}
	off := len(voteGuardMagic)
	if data[off] != voteGuardVersion {
		return nil, fmt.Errorf("unsupported version %d", data[off])
	}
	off++
	count := binary.BigEndian.Uint32(data[off : off+4])
	off += 4
	want := voteGuardHdrLen + voteGuardRecLen*int(count) + 4
	if len(data) != want {
		return nil, fmt.Errorf("length %d != expected %d for count %d", len(data), want, count)
	}
	// Verify the CRC over everything preceding the trailing 4 bytes.
	body := data[:len(data)-4]
	gotCRC := binary.BigEndian.Uint32(data[len(data)-4:])
	if crc32.Checksum(body, voteGuardCRC) != gotCRC {
		return nil, errors.New("crc mismatch")
	}
	out := make(map[SlotKey]ids.ID, count)
	for i := uint32(0); i < count; i++ {
		height := binary.BigEndian.Uint64(data[off : off+8])
		off += 8
		var epoch, canonical ids.ID
		copy(epoch[:], data[off:off+32])
		off += 32
		copy(canonical[:], data[off:off+32])
		off += 32
		out[SlotKey{Height: height, Epoch: epoch}] = canonical
	}
	return out, nil
}

// fsyncDir fsyncs a directory so a rename inside it is durable across a crash.
// A directory that cannot be opened for fsync (some platforms/filesystems) is not
// fatal — the data file was already fsync'd; only the rename's durability is at
// stake, and the binding is re-persisted on the next vote regardless.
func fsyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return nil
	}
	defer d.Close()
	if err := d.Sync(); err != nil {
		// EINVAL on directories on some filesystems — tolerate, per above.
		return nil
	}
	return nil
}
