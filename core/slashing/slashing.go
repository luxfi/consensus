// Copyright (C) 2025-2026, Lux Industries Inc All rights reserved.
// Package slashing provides equivocation detection and slashing evidence
// for Lux consensus. It detects double-voting, double-signing, and
// downtime violations. Actual stake deduction happens in the platformvm;
// this package provides detection and evidence only.
package slashing

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/ids"
)

// EvidenceType classifies validator misbehavior.
type EvidenceType uint8

const (
	// DoubleVote means a validator voted for two different blocks at the same height.
	DoubleVote EvidenceType = iota + 1
	// DoubleSign means a validator signed two different blocks at the same height.
	DoubleSign
	// Downtime means a validator missed too many consecutive blocks.
	Downtime
)

func (e EvidenceType) String() string {
	switch e {
	case DoubleVote:
		return "double_vote"
	case DoubleSign:
		return "double_sign"
	case Downtime:
		return "downtime"
	default:
		return "unknown"
	}
}

// Evidence represents proof of validator misbehavior.
type Evidence struct {
	Type        EvidenceType
	ValidatorID ids.NodeID
	Height      uint64
	Timestamp   time.Time
	Proof       []byte // serialized conflicting votes/blocks
}

// SlashRecord tracks cumulative slashing state for a validator.
type SlashRecord struct {
	ValidatorID ids.NodeID
	SlashCount  uint32
	JailedUntil time.Time // zero value means not jailed
	Evidence    []Evidence
}

// IsJailed returns true if the validator is currently jailed.
func (r *SlashRecord) IsJailed(now time.Time) bool {
	return !r.JailedUntil.IsZero() && now.Before(r.JailedUntil)
}

// voteRecord tracks a single vote cast by a validator at a height.
type voteRecord struct {
	BlockID ids.ID
	Accept  bool
}

// blockRecord tracks a block signed by a validator at a height.
type blockRecord struct {
	BlockID   ids.ID
	BlockData []byte
}

var (
	ErrAlreadySlashed = errors.New("evidence already recorded for this violation")
)

// Detector watches for equivocation in votes and block proposals.
type Detector struct {
	mu sync.Mutex

	// height -> validatorID -> first vote seen
	votes map[uint64]map[ids.NodeID]voteRecord

	// height -> validatorID -> first block seen
	blocks map[uint64]map[ids.NodeID]blockRecord

	// Downtime: validatorID -> bitmap of recent participation
	// Each bit represents one slot in the window. 1 = participated, 0 = missed.
	uptimeBits   map[ids.NodeID]uint64
	uptimeSlots  map[ids.NodeID]int // slots recorded per validator (capped at uptimeWindow)
	uptimeWindow int                // number of slots to track (max 64)

	// Downtime threshold: fraction of window that must be missed (0.0-1.0)
	downtimeThreshold float64
}

// NewDetector creates an equivocation detector.
// uptimeWindow is the number of recent slots to track (max 64).
// downtimeThreshold is the fraction of missed slots that triggers evidence (e.g. 0.5 = 50%).
func NewDetector(uptimeWindow int, downtimeThreshold float64) *Detector {
	if uptimeWindow <= 0 || uptimeWindow > 64 {
		uptimeWindow = 64
	}
	if downtimeThreshold <= 0 || downtimeThreshold > 1.0 {
		downtimeThreshold = 0.5
	}
	return &Detector{
		votes:             make(map[uint64]map[ids.NodeID]voteRecord),
		blocks:            make(map[uint64]map[ids.NodeID]blockRecord),
		uptimeBits:        make(map[ids.NodeID]uint64),
		uptimeSlots:       make(map[ids.NodeID]int),
		uptimeWindow:      uptimeWindow,
		downtimeThreshold: downtimeThreshold,
	}
}

// CheckVote checks a vote for double-voting. Returns evidence if equivocation detected, nil otherwise.
// A double-vote occurs when a validator votes for two different block IDs at the same height.
func (d *Detector) CheckVote(validatorID ids.NodeID, height uint64, blockID ids.ID, accept bool) *Evidence {
	d.mu.Lock()
	defer d.mu.Unlock()

	heightVotes, exists := d.votes[height]
	if !exists {
		heightVotes = make(map[ids.NodeID]voteRecord)
		d.votes[height] = heightVotes
	}

	prev, seen := heightVotes[validatorID]
	if !seen {
		heightVotes[validatorID] = voteRecord{BlockID: blockID, Accept: accept}
		return nil
	}

	// Same block ID is not equivocation (duplicate vote)
	if prev.BlockID == blockID {
		return nil
	}

	// Different block at same height = equivocation
	proof := fmt.Appendf(nil, "height=%d vote1=%s vote2=%s", height, prev.BlockID, blockID)
	return &Evidence{
		Type:        DoubleVote,
		ValidatorID: validatorID,
		Height:      height,
		Timestamp:   time.Now(),
		Proof:       proof,
	}
}

// CheckBlock checks a block proposal for double-signing. Returns evidence if equivocation detected.
// A double-sign occurs when a validator proposes/signs two different blocks at the same height.
func (d *Detector) CheckBlock(validatorID ids.NodeID, height uint64, blockID ids.ID, blockData []byte) *Evidence {
	d.mu.Lock()
	defer d.mu.Unlock()

	heightBlocks, exists := d.blocks[height]
	if !exists {
		heightBlocks = make(map[ids.NodeID]blockRecord)
		d.blocks[height] = heightBlocks
	}

	prev, seen := heightBlocks[validatorID]
	if !seen {
		data := make([]byte, len(blockData))
		copy(data, blockData)
		heightBlocks[validatorID] = blockRecord{BlockID: blockID, BlockData: data}
		return nil
	}

	// Same block is not equivocation
	if prev.BlockID == blockID {
		return nil
	}

	// Different block at same height = equivocation
	proof := fmt.Appendf(nil, "height=%d block1=%s block2=%s", height, prev.BlockID, blockID)
	return &Evidence{
		Type:        DoubleSign,
		ValidatorID: validatorID,
		Height:      height,
		Timestamp:   time.Now(),
		Proof:       proof,
	}
}

// RecordParticipation records whether a validator participated in a slot.
// Call this once per slot per validator. Returns downtime evidence if threshold exceeded.
// The detector does not trigger evidence until at least uptimeWindow slots have been recorded
// for a given validator, preventing false positives during warmup.
func (d *Detector) RecordParticipation(validatorID ids.NodeID, participated bool) *Evidence {
	d.mu.Lock()
	defer d.mu.Unlock()

	bits := d.uptimeBits[validatorID]

	// Shift left, new bit enters at position 0
	bits = bits << 1
	if participated {
		bits |= 1
	}

	// Mask to window size
	mask := uint64((1 << d.uptimeWindow) - 1)
	bits &= mask
	d.uptimeBits[validatorID] = bits

	// Track how many slots we've seen (cap at window size)
	slots := d.uptimeSlots[validatorID] + 1
	if slots > d.uptimeWindow {
		slots = d.uptimeWindow
	}
	d.uptimeSlots[validatorID] = slots

	// Don't evaluate until the window is full
	if slots < d.uptimeWindow {
		return nil
	}

	// Count missed slots
	present := popcount(bits, d.uptimeWindow)
	missed := d.uptimeWindow - present

	if float64(missed)/float64(d.uptimeWindow) > d.downtimeThreshold {
		proof := fmt.Appendf(nil, "window=%d missed=%d present=%d threshold=%.2f",
			d.uptimeWindow, missed, present, d.downtimeThreshold)
		return &Evidence{
			Type:        Downtime,
			ValidatorID: validatorID,
			Height:      0, // downtime is not height-specific
			Timestamp:   time.Now(),
			Proof:       proof,
		}
	}

	return nil
}

// PruneBelow removes vote/block records at heights strictly below the given height.
// Call periodically to bound memory usage.
func (d *Detector) PruneBelow(height uint64) {
	d.mu.Lock()
	defer d.mu.Unlock()

	for h := range d.votes {
		if h < height {
			delete(d.votes, h)
		}
	}
	for h := range d.blocks {
		if h < height {
			delete(d.blocks, h)
		}
	}
}

// popcount counts set bits in the lowest windowSize bits of v.
func popcount(v uint64, windowSize int) int {
	// Mask to window
	mask := uint64((1 << windowSize) - 1)
	v &= mask
	count := 0
	for v != 0 {
		count++
		v &= v - 1 // clear lowest set bit
	}
	return count
}

// DB stores slashing records. In-memory for now; persistence is a future concern
// when this integrates with the platformvm.
type DB struct {
	mu      sync.RWMutex
	records map[ids.NodeID]*SlashRecord

	// Jail duration per slash. Doubles with each successive slash.
	baseJailDuration time.Duration
}

// NewDB creates a slashing database with the given base jail duration.
// Each successive slash doubles the jail time.
func NewDB(baseJailDuration time.Duration) *DB {
	return &DB{
		records:          make(map[ids.NodeID]*SlashRecord),
		baseJailDuration: baseJailDuration,
	}
}

// RecordEvidence stores evidence and updates the validator's slash record.
// Returns the updated record.
func (db *DB) RecordEvidence(ev Evidence) *SlashRecord {
	db.mu.Lock()
	defer db.mu.Unlock()

	rec, exists := db.records[ev.ValidatorID]
	if !exists {
		rec = &SlashRecord{ValidatorID: ev.ValidatorID}
		db.records[ev.ValidatorID] = rec
	}

	rec.Evidence = append(rec.Evidence, ev)
	rec.SlashCount++

	// Jail duration doubles with each slash: base * 2^(count-1)
	jailDuration := db.baseJailDuration
	for i := uint32(1); i < rec.SlashCount; i++ {
		jailDuration *= 2
	}
	rec.JailedUntil = time.Now().Add(jailDuration)

	return rec
}

// GetRecord returns the slash record for a validator, or nil if clean.
func (db *DB) GetRecord(validatorID ids.NodeID) *SlashRecord {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.records[validatorID]
}

// IsJailed returns true if the validator is currently jailed.
func (db *DB) IsJailed(validatorID ids.NodeID) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()

	rec, exists := db.records[validatorID]
	if !exists {
		return false
	}
	return rec.IsJailed(time.Now())
}

// GetAllRecords returns all slash records. The returned slice is a snapshot.
func (db *DB) GetAllRecords() []SlashRecord {
	db.mu.RLock()
	defer db.mu.RUnlock()

	out := make([]SlashRecord, 0, len(db.records))
	for _, rec := range db.records {
		out = append(out, *rec)
	}
	return out
}
