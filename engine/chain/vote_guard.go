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

// SlotKey identifies one per-HEIGHT non-equivocation slot. An honest validator
// signs AT MOST ONE canonical block per consensus height — full stop. This is the
// quorum-intersection invariant that makes the α-of-K cert SOUND: two ⅔ certs at
// one height would require |signers(A)| + |signers(B)| ≥ 2·α over n validators, so
// |A ∩ B| ≥ 2α − n honest signers signed BOTH — impossible under this rule unless
// f ≥ 2α − n (beyond the BFT budget). One slot per height is the ONLY correct key.
//
// WHY NOT (height, epoch): an earlier revision keyed the slot on (height, epoch)
// where epoch = the block's ValidatorSetRoot(pChainHeight). That FRAGMENTED the
// slot: two honest sibling blocks at the SAME consensus height can pin DIFFERENT
// proposervm P-chain heights (a bare/pre-fork block reports height 0, a wrapped
// block reports P; or two wrapped blocks pick different heights within the
// proposer window), and ValidatorSetRoot(0) (empty/error ⇒ ids.Empty) ≠
// ValidatorSetRoot(P). Different epoch ⇒ different SlotKey ⇒ the SAME honest
// validator signed BOTH siblings ⇒ two α-of-K certs at one height ⇒ the
// double-finalization fatal on fresh multi-node chains. The P-chain-height epoch is
// a PROPOSER-CHOSEN axis, NOT a function of the finalized value-chain prefix, so
// the "one epoch per height by induction" argument was false. Safety must never be
// keyed on a proposer-controlled value.
//
// The validator-set-root epoch binding still lives where it belongs — in the SIGNED
// vote message (cert.go: canonicalVoteMessageFor folds ValidatorSetRoot) and in
// cert VERIFICATION (topology.go: the set-root cross-check + stake@epoch) — so a
// cert remains bound to the exact set that signed it (anti-cross-epoch-forgery).
// It is removed ONLY from the equivocation SLOT, where it added no safety and only
// fragmented the one-signature-per-height rule.
type SlotKey struct {
	Height uint64
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
	// Persist atomically and durably records the COMPLETE set of live height->canonical
	// bindings AND the decided-through floor (the highest finalized height this node has
	// observed), fsync'd. It must not mutate the map. The floor is what makes the
	// decided-height sign gate DURABLE across a restart: the strictly-below prune persists
	// the removal of below-tip slots, so on reboot only the floor tells a signer that those
	// heights are decided and unsignable (the in-process ledger.Height() is a
	// non-authoritative (0,false) hint until the first post-restart cert — incident-1082814
	// PART-A). fsync'd in the SAME write as the bindings, so the floor and the slot removal
	// are atomic on disk.
	Persist(bindings map[SlotKey]ids.ID, finalizedThrough uint64) error
	// Snapshot returns the bindings recovered from stable storage at open time.
	// The engine seeds committedSlot from it so the guard spans a restart.
	Snapshot() map[SlotKey]ids.ID
	// FinalizedThrough returns the decided-through floor recovered at open time (0 if none
	// / a legacy v1 snapshot). The engine seeds decidedFloor from it so the sign gate
	// refuses signing at any height <= a height this node had already finalized before the
	// crash — even though that height's slot was pruned. A SIGN-GATE-ONLY fail-safe lower
	// bound; it NEVER enters the finality ledger, byHeight, or the equivocation index.
	FinalizedThrough() uint64
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
		// Seed the durable decided-through floor so the sign gate refuses any height this
		// node had already finalized before the crash — even the below-tip heights whose
		// slots the strictly-below prune persisted away (CROSS-RESTART prune-then-resign).
		if f := store.FinalizedThrough(); f > t.decidedFloor {
			t.decidedFloor = f
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
	voteGuardMagic     = "LUXVGUARD" // 9 bytes
	voteGuardVersion   = byte(2)     // v2 adds finalizedThrough(u64) to the header (durable sign-gate floor)
	voteGuardVersionV1 = byte(1)     // legacy: no finalizedThrough field (decoded with floor=0)

	// Header layouts. v1: magic | ver | count(u32). v2 appends finalizedThrough(u64) —
	// the durable decided-through floor, fsync'd atomically with the bindings.
	voteGuardHdrLenV1 = len(voteGuardMagic) + 1 + 4     // magic | ver | count
	voteGuardHdrLen   = len(voteGuardMagic) + 1 + 4 + 8 // v2: magic | ver | count | finalizedThrough(u64)
	voteGuardRecLen   = 8 + 32 + 32                     // height(u64) | epoch(32) | canonical(32)
)

var voteGuardCRC = crc32.MakeTable(crc32.Castagnoli)

// errVoteGuardCorrupt marks an unreadable/tampered snapshot — surfaced by
// OpenVoteGuard so a signing node fails CLOSED (refuses to start with
// unverifiable equivocation memory rather than silently start with none).
var errVoteGuardCorrupt = errors.New("vote guard snapshot is corrupt")

type fileVoteGuard struct {
	path             string // the snapshot file
	dir              string // parent dir, fsync'd after each rename so the rename itself is durable
	snap             map[SlotKey]ids.ID
	finalizedThrough uint64 // decided-through floor recovered at open (0 if none / legacy v1)
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
	snap, finalizedThrough, err := decodeVoteGuard(data)
	if err != nil {
		return nil, fmt.Errorf("%w at %s: %v (refusing to start with unverifiable equivocation memory)",
			errVoteGuardCorrupt, path, err)
	}
	g.snap = snap
	g.finalizedThrough = finalizedThrough
	return g, nil
}

// Persist writes the complete binding set atomically and durably. It is called
// under the engine's slotMu, so the fixed-name temp file is never contended.
func (g *fileVoteGuard) Persist(bindings map[SlotKey]ids.ID, finalizedThrough uint64) error {
	// The floor is monotonic on disk: never write a value below what we already recovered,
	// so a caller that passes a stale-lower floor (e.g. a best-effort prune re-persist that
	// races a higher finalize) cannot regress the durable refusal.
	if finalizedThrough < g.finalizedThrough {
		finalizedThrough = g.finalizedThrough
	}
	buf := encodeVoteGuard(bindings, finalizedThrough)

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
	if err := fsyncDir(g.dir); err != nil {
		return err
	}
	g.finalizedThrough = finalizedThrough // durable write committed — advance the in-memory floor
	return nil
}

// Snapshot returns the bindings recovered at open time (used once to seed the
// engine's committedSlot). After open, the engine's map is the source of truth.
func (g *fileVoteGuard) Snapshot() map[SlotKey]ids.ID { return g.snap }

// FinalizedThrough returns the durable decided-through floor recovered at open (0 if
// none / legacy v1). Seeds the engine's decidedFloor so the sign gate spans a restart.
func (g *fileVoteGuard) FinalizedThrough() uint64 { return g.finalizedThrough }

// Close is a no-op: the store holds no file handle open between writes.
func (g *fileVoteGuard) Close() error { return nil }

// encodeVoteGuard serializes the binding set: magic | ver | count | finalizedThrough(u64)
// | records | crc (v2). Record order is unspecified (the decoder rebuilds a map); every
// field is fixed-width so the frame is length-free. The record retains a 32-byte RESERVED
// field (written as all-zero) where the now-removed epoch used to live, so records are
// byte-identical to the earlier format; v2 adds ONLY the finalizedThrough floor to the
// HEADER (the durable decided-height sign-gate floor). decodeVoteGuard reads legacy v1
// snapshots (no floor → 0) too, so an existing validator's on-disk snapshot decodes across
// the upgrade without a brick.
func encodeVoteGuard(bindings map[SlotKey]ids.ID, finalizedThrough uint64) []byte {
	buf := make([]byte, 0, voteGuardHdrLen+voteGuardRecLen*len(bindings)+4)
	buf = append(buf, voteGuardMagic...)
	buf = append(buf, voteGuardVersion)
	var u32 [4]byte
	binary.BigEndian.PutUint32(u32[:], uint32(len(bindings)))
	buf = append(buf, u32[:]...)
	var floorBuf [8]byte
	binary.BigEndian.PutUint64(floorBuf[:], finalizedThrough) // v2: the durable decided-through floor
	buf = append(buf, floorBuf[:]...)
	var u64 [8]byte
	var reserved [32]byte // reserved (was epoch); always zero on write
	for k, canonical := range bindings {
		binary.BigEndian.PutUint64(u64[:], k.Height)
		buf = append(buf, u64[:]...)
		buf = append(buf, reserved[:]...)
		buf = append(buf, canonical[:]...)
	}
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.Checksum(buf, voteGuardCRC))
	buf = append(buf, crc[:]...)
	return buf
}

// decodeVoteGuard is the inverse of encodeVoteGuard. It accepts BOTH the current v2
// layout (with the finalizedThrough floor) and the legacy v1 layout (no floor → returns
// 0), so a snapshot written by a pre-v1.35.4 signer decodes across the upgrade without a
// brick. Any framing/CRC mismatch is an error (the snapshot is untrustworthy) — never a
// silent partial read.
func decodeVoteGuard(data []byte) (map[SlotKey]ids.ID, uint64, error) {
	if len(data) < voteGuardHdrLenV1+4 {
		return nil, 0, fmt.Errorf("too short (%d bytes)", len(data))
	}
	if string(data[:len(voteGuardMagic)]) != voteGuardMagic {
		return nil, 0, errors.New("bad magic")
	}
	off := len(voteGuardMagic)
	ver := data[off]
	off++
	var hdrLen int
	switch ver {
	case voteGuardVersionV1:
		hdrLen = voteGuardHdrLenV1
	case voteGuardVersion:
		hdrLen = voteGuardHdrLen
	default:
		return nil, 0, fmt.Errorf("unsupported version %d", ver)
	}
	if len(data) < hdrLen+4 {
		return nil, 0, fmt.Errorf("too short for version %d header (%d bytes)", ver, len(data))
	}
	count := binary.BigEndian.Uint32(data[off : off+4])
	off += 4
	var finalizedThrough uint64
	if ver == voteGuardVersion { // v2 carries the durable decided-through floor
		finalizedThrough = binary.BigEndian.Uint64(data[off : off+8])
		off += 8
	}
	want := hdrLen + voteGuardRecLen*int(count) + 4
	if len(data) != want {
		return nil, 0, fmt.Errorf("length %d != expected %d for count %d (version %d)", len(data), want, count, ver)
	}
	// Verify the CRC over everything preceding the trailing 4 bytes.
	body := data[:len(data)-4]
	gotCRC := binary.BigEndian.Uint32(data[len(data)-4:])
	if crc32.Checksum(body, voteGuardCRC) != gotCRC {
		return nil, 0, errors.New("crc mismatch")
	}
	out := make(map[SlotKey]ids.ID, count)
	for i := uint32(0); i < count; i++ {
		height := binary.BigEndian.Uint64(data[off : off+8])
		off += 8
		// The 32-byte reserved field (formerly epoch) is read and DISCARDED: the slot
		// is now height-only. A legacy snapshot written by the (height,epoch)-keyed
		// build can therefore hold MULTIPLE records at one height (different epochs);
		// they FOLD into the single height slot here. On a fold collision keep the
		// LOWEST canonical (deterministic) — the recovered height is contested and
		// will be pruned the moment it finalizes; refusing every sibling above the
		// min is the fail-safe direction.
		off += 32
		var canonical ids.ID
		copy(canonical[:], data[off:off+32])
		off += 32
		key := SlotKey{Height: height}
		if prev, ok := out[key]; !ok || canonical.Compare(prev) < 0 {
			out[key] = canonical
		}
	}
	return out, finalizedThrough, nil
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
