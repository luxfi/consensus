// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/luxfi/ids"
)

func blockAt(n int) ids.ID {
	var id ids.ID
	id[0] = byte(n)
	id[1] = byte(n >> 8)
	id[2] = byte(n >> 16)
	return id
}

// TestCertOutlivesTheProcess pins the property the store exists for: a height
// this node decided stays serveable to a peer catching up after the node
// restarts. The engine that finalized the block is thrown away between the write
// and the read, which is what a restart does to certByDecision.
func TestCertOutlivesTheProcess(t *testing.T) {
	dir := t.TempDir()
	blockID, cert := blockAt(1), []byte("finality-cert-for-15499")

	store, err := OpenCerts(dir)
	if err != nil {
		t.Fatalf("OpenCerts: %v", err)
	}
	// The same two steps applyBranchFinalization runs, in the same order: the
	// memory window under the lock, the durable copy after it.
	finalizer := New(WithCerts(store))
	finalizer.mu.Lock()
	finalizer.storeServedCertLocked(blockID, cert)
	finalizer.mu.Unlock()
	finalizer.persistServedCert(blockID, 15499, cert)

	if _, ok := finalizer.certFor(blockID); !ok {
		t.Fatal("the finalizing engine cannot serve the cert it just wrote")
	}

	// Restart: a new process, a new engine, nothing carried over but the disk.
	reopened, err := OpenCerts(dir)
	if err != nil {
		t.Fatalf("OpenCerts after restart: %v", err)
	}
	restarted := New(WithCerts(reopened))

	got, ok := restarted.certFor(blockID)
	if !ok {
		t.Fatal("cert did not survive the restart — a peer whose gap predates our restart could never be served its next block")
	}
	if string(got) != string(cert) {
		t.Fatalf("cert came back changed: got %q want %q", got, cert)
	}
}

// TestCertsWithoutAStoreDieWithTheProcess pins the behaviour the fix exists to
// change, so nobody reads the memory window as durable. This is the pre-fix
// state, asserted deliberately.
func TestCertsWithoutAStoreDieWithTheProcess(t *testing.T) {
	blockID, cert := blockAt(2), []byte("cert")

	finalizer := New()
	finalizer.mu.Lock()
	finalizer.storeServedCertLocked(blockID, cert)
	finalizer.mu.Unlock()
	finalizer.persistServedCert(blockID, 7, cert) // no store wired: a no-op
	if _, ok := finalizer.certFor(blockID); !ok {
		t.Fatal("in-process serve must still work without a store")
	}

	if _, ok := New().certFor(blockID); ok {
		t.Fatal("a memory-only engine must not claim a cert it never assembled")
	}
}

// TestCertStoreKeepsTheNewestWindow: eviction follows the CHAIN (lowest heights
// go first), not write order, so what is retained is the window a straggler can
// actually use.
func TestCertStoreKeepsTheNewestWindow(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenCerts(dir)
	if err != nil {
		t.Fatalf("OpenCerts: %v", err)
	}

	// Seed the window by hand rather than through Put: each Put is an atomic
	// replace with an fsync (~17ms), and 4101 of those is a minute of test time
	// spent measuring the filesystem rather than the eviction rule. The names are
	// the whole index, so writing them directly is the same input Put produces.
	const extra = 5
	for i := 0; i < maxServedCerts+extra-1; i++ {
		name := certName(uint64(i), blockAt(i))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(fmt.Sprintf("cert-%d", i)), 0o600); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	// The last one goes through the real write path, so eviction is exercised from
	// both entry points: recovery at open, and the hot path at Put.
	last := maxServedCerts + extra - 1
	if err := store.Put(blockAt(last), uint64(last), []byte(fmt.Sprintf("cert-%d", last))); err != nil {
		t.Fatalf("Put(%d): %v", last, err)
	}
	if store, err = OpenCerts(dir); err != nil {
		t.Fatalf("reopen: %v", err)
	}

	for i := 0; i < extra; i++ {
		if _, ok := store.Get(blockAt(i)); ok {
			t.Fatalf("height %d should have been evicted below the window", i)
		}
	}
	for i := extra; i < maxServedCerts+extra; i++ {
		if _, ok := store.Get(blockAt(i)); !ok {
			t.Fatalf("height %d is inside the window and must be retained", i)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != maxServedCerts {
		t.Fatalf("store holds %d files, want %d — eviction did not reach disk", len(entries), maxServedCerts)
	}
}

// TestCertStoreRecoversTheWindowOnOpen: the file names carry height and block id,
// so a reopened store knows what it holds with no index to fall out of step.
func TestCertStoreRecoversTheWindowOnOpen(t *testing.T) {
	dir := t.TempDir()
	store, err := OpenCerts(dir)
	if err != nil {
		t.Fatalf("OpenCerts: %v", err)
	}
	for i := 1; i <= 3; i++ {
		if err := store.Put(blockAt(i), uint64(i*100), []byte(fmt.Sprintf("cert-%d", i))); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	// A stray file must neither be adopted nor deleted.
	stray := filepath.Join(dir, "operator-notes.txt")
	if err := os.WriteFile(stray, []byte("do not eat"), 0o600); err != nil {
		t.Fatalf("write stray: %v", err)
	}

	reopened, err := OpenCerts(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	for i := 1; i <= 3; i++ {
		got, ok := reopened.Get(blockAt(i))
		if !ok {
			t.Fatalf("cert %d not recovered on open", i)
		}
		if want := fmt.Sprintf("cert-%d", i); string(got) != want {
			t.Fatalf("cert %d = %q, want %q", i, got, want)
		}
	}
	if _, err := os.Stat(stray); err != nil {
		t.Fatalf("a file the store does not own was removed: %v", err)
	}
}

// TestCertStoreNameRoundTrips: the name IS the index, so it has to survive the
// trip both ways for every height including the uint64 edges.
func TestCertStoreNameRoundTrips(t *testing.T) {
	for _, height := range []uint64{0, 1, 15499, 1 << 32, ^uint64(0)} {
		id := blockAt(int(height % 251))
		gotHeight, gotID, ok := parseCertName(certName(height, id))
		if !ok {
			t.Fatalf("height %d: name did not parse back", height)
		}
		if gotHeight != height || gotID != id {
			t.Fatalf("height %d: round-tripped to (%d, %s)", height, gotHeight, gotID)
		}
	}
	for _, bad := range []string{"", "not-a-cert", "12-abc", filepath.Join("a", "b")} {
		if _, _, ok := parseCertName(bad); ok {
			t.Fatalf("%q must not parse as a cert name", bad)
		}
	}
}
