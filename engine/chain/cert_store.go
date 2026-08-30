// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert_store.go — where a finality cert lives between the moment this node
// assembles it and the moment a peer catching up asks for it.
//
// THE CERT IS THE ONLY WAY BACK. A node that fell behind cannot rejoin by
// voting: the network will not re-vote a height it has already decided. Its one
// route is AcceptCatchupBlock, which finalizes each gap block exclusively by
// running a SERVED cert through HandleIncomingCert. So the set of blocks a node
// can serve is the set of blocks its peers can recover from.
//
// Held in memory, that set is bounded by process lifetime rather than by the
// window maxServedCerts names, which makes recoverability depend on how recently
// the peers happened to start. A cert is durable evidence about a decided height;
// its usefulness outlasts any one process, so its storage should too.
//
// So the cert is kept where the block is: on disk, next to the chain. Retention
// then means what it says — the last maxServedCerts heights, across restarts.
//
// FAILING OPEN IS CORRECT HERE, and the contrast with the vote guard is the
// point. An unreadable vote-guard snapshot must stop the node — acting without
// equivocation memory risks double-signing. An unreadable cert can only cost a
// fetch: the asking peer tries another node, and a cert that decodes to garbage
// is refused by HandleIncomingCert exactly as a forged one is. Nothing downstream
// trusts these bytes because they came from disk.
package chain

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/luxfi/ids"
)

// CertStore is the durable home for finality certs, keyed by the DECISION each
// one proves. The engine writes one at the sole finalizer and reads one to serve
// a peer catching up.
//
// THE KEY IS THE SIGNED IDENTITY, NOT THE ENVELOPE. A cert's signatures cover the
// canonical execution commitment (chain.decision: the inner CanonicalID, or the
// block's own id on a bare chain that records none) — never the outer proposervm
// envelope that happened to carry it. Two envelopes wrapping the same inner block
// are the same decision and are collapsed to duplicates everywhere else in the
// engine; keyed by envelope, they were two store entries for one finality fact,
// and a peer that held the sibling wrapper asked for a cert this node had and was
// told no. Keyed by decision, the cert is found by anything that finalized the
// same thing, which is exactly the set of nodes it is valid for.
//
// Put is idempotent per decision. Get reports absence rather than error — a cert
// this node cannot produce is a cert the peer fetches elsewhere, which is the
// same handling either way.
type CertStore interface {
	// Put durably records cert as the finality proof for decision at height. The
	// height orders eviction: the store keeps the most recent maxServedCerts and
	// drops the lowest heights, so retention follows the chain rather than the
	// order writes happened to arrive.
	Put(decision ids.ID, height uint64, cert []byte) error
	// Get returns the cert recorded for decision, or ok=false when this node never
	// finalized it, it has aged out of the window, or it cannot be read back.
	Get(decision ids.ID) ([]byte, bool)
}

// fileCerts is the one concrete CertStore: a directory holding one file per
// cert, named "<height>-<decision>" with the height zero-padded so lexical order
// IS height order. That name carries everything the store needs, which is why
// there is no index file — nothing to fall out of step with the directory, and
// nothing whose corruption would lose certs that are sitting right there.
//
// Writes are the standard write-temp + fsync + rename + fsync-dir, shared with
// the vote guard so the two durable stores cannot drift on crash-safety.
type fileCerts struct {
	dir string

	mu         sync.Mutex
	order      []certSlot        // ascending height — the eviction order
	byDecision map[ids.ID]string // decision -> file name
}

type certSlot struct {
	height uint64
	id     ids.ID
}

// certNameWidth zero-pads the height so a lexical sort of the directory is a
// numeric sort by height. 20 digits covers the full uint64 range.
const certNameWidth = 20

func certName(height uint64, decision ids.ID) string {
	return fmt.Sprintf("%0*d-%s", certNameWidth, height, decision)
}

// parseCertName recovers (height, decision) from a file name written by certName.
// A name that does not round-trip is not ours; the caller ignores the file rather
// than deleting it, so an operator's notes in the directory survive.
func parseCertName(name string) (uint64, ids.ID, bool) {
	dash := strings.IndexByte(name, '-')
	if dash != certNameWidth {
		return 0, ids.Empty, false
	}
	height, err := strconv.ParseUint(name[:dash], 10, 64)
	if err != nil {
		return 0, ids.Empty, false
	}
	decision, err := ids.FromString(name[dash+1:])
	if err != nil {
		return 0, ids.Empty, false
	}
	return height, decision, true
}

// OpenCerts opens (or creates) the durable cert store at dir and recovers the
// window already on disk. A missing directory is a fresh node. Unreadable
// entries are skipped, not fatal — see the note at the top of this file on why
// this store fails open where the vote guard fails closed.
func OpenCerts(dir string) (CertStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create cert store %s: %w", dir, err)
	}
	c := &fileCerts{dir: dir, byDecision: map[ids.ID]string{}}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read cert store %s: %w", dir, err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		height, decision, ok := parseCertName(e.Name())
		if !ok {
			continue
		}
		c.order = append(c.order, certSlot{height: height, id: decision})
		c.byDecision[decision] = e.Name()
	}
	sort.Slice(c.order, func(i, j int) bool { return c.order[i].height < c.order[j].height })
	c.evictLocked()
	return c, nil
}

func (c *fileCerts) Put(decision ids.ID, height uint64, cert []byte) error {
	if len(cert) == 0 {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.byDecision[decision]; exists {
		return nil
	}
	name := certName(height, decision)
	if err := writeFileAtomic(filepath.Join(c.dir, name), c.dir, cert); err != nil {
		return fmt.Errorf("write cert %s: %w", name, err)
	}
	c.byDecision[decision] = name
	// Finality is monotonic, so appending keeps order ascending on the hot path.
	// A cert arriving out of order (a re-delivered older height) is rare enough
	// that re-sorting it into place costs nothing and keeps eviction honest.
	if n := len(c.order); n > 0 && c.order[n-1].height > height {
		c.order = append(c.order, certSlot{height: height, id: decision})
		sort.Slice(c.order, func(i, j int) bool { return c.order[i].height < c.order[j].height })
	} else {
		c.order = append(c.order, certSlot{height: height, id: decision})
	}
	c.evictLocked()
	return nil
}

func (c *fileCerts) Get(decision ids.ID) ([]byte, bool) {
	c.mu.Lock()
	name, ok := c.byDecision[decision]
	c.mu.Unlock()
	if !ok {
		return nil, false
	}
	b, err := os.ReadFile(filepath.Join(c.dir, name))
	if err != nil || len(b) == 0 {
		return nil, false
	}
	return b, true
}

// evictLocked drops the lowest heights until the store is within maxServedCerts.
// A node lagging beyond that window bootstraps rather than catching up, so the
// certs below it cannot help anyone. The caller holds c.mu.
func (c *fileCerts) evictLocked() {
	for len(c.order) > maxServedCerts {
		ev := c.order[0]
		c.order = c.order[1:]
		name, ok := c.byDecision[ev.id]
		if !ok {
			continue
		}
		delete(c.byDecision, ev.id)
		_ = os.Remove(filepath.Join(c.dir, name))
	}
}
