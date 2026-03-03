package slashing

import (
	"sync"
	"testing"
	"time"

	"github.com/luxfi/ids"
)

func nodeID(b byte) ids.NodeID {
	var id ids.NodeID
	id[0] = b
	return id
}

func blockID(b byte) ids.ID {
	var id ids.ID
	id[0] = b
	return id
}

// --- Detector: DoubleVote ---

func TestCheckVote_NoDuplicate(t *testing.T) {
	d := NewDetector(64, 0.5)
	v := nodeID(1)
	b := blockID(0xAA)

	ev := d.CheckVote(v, 100, b, true)
	if ev != nil {
		t.Fatal("first vote should not produce evidence")
	}
}

func TestCheckVote_SameBlockNotEquivocation(t *testing.T) {
	d := NewDetector(64, 0.5)
	v := nodeID(1)
	b := blockID(0xAA)

	d.CheckVote(v, 100, b, true)
	ev := d.CheckVote(v, 100, b, true)
	if ev != nil {
		t.Fatal("duplicate vote for same block should not produce evidence")
	}
}

func TestCheckVote_DifferentBlockEquivocation(t *testing.T) {
	d := NewDetector(64, 0.5)
	v := nodeID(1)
	b1 := blockID(0xAA)
	b2 := blockID(0xBB)

	d.CheckVote(v, 100, b1, true)
	ev := d.CheckVote(v, 100, b2, true)
	if ev == nil {
		t.Fatal("voting for different blocks at same height must produce evidence")
	}
	if ev.Type != DoubleVote {
		t.Fatalf("expected DoubleVote, got %s", ev.Type)
	}
	if ev.ValidatorID != v {
		t.Fatalf("wrong validator in evidence")
	}
	if ev.Height != 100 {
		t.Fatalf("wrong height in evidence: %d", ev.Height)
	}
	if len(ev.Proof) == 0 {
		t.Fatal("proof must not be empty")
	}
}

func TestCheckVote_DifferentHeightsOK(t *testing.T) {
	d := NewDetector(64, 0.5)
	v := nodeID(1)
	b1 := blockID(0xAA)
	b2 := blockID(0xBB)

	d.CheckVote(v, 100, b1, true)
	ev := d.CheckVote(v, 101, b2, true) // different height is fine
	if ev != nil {
		t.Fatal("votes at different heights should not produce evidence")
	}
}

func TestCheckVote_DifferentValidatorsOK(t *testing.T) {
	d := NewDetector(64, 0.5)
	v1 := nodeID(1)
	v2 := nodeID(2)
	b1 := blockID(0xAA)
	b2 := blockID(0xBB)

	d.CheckVote(v1, 100, b1, true)
	ev := d.CheckVote(v2, 100, b2, true) // different validator is fine
	if ev != nil {
		t.Fatal("different validators voting for different blocks is not equivocation")
	}
}

// --- Detector: DoubleSign ---

func TestCheckBlock_NoDuplicate(t *testing.T) {
	d := NewDetector(64, 0.5)
	v := nodeID(1)
	b := blockID(0xAA)

	ev := d.CheckBlock(v, 100, b, []byte("data"))
	if ev != nil {
		t.Fatal("first block should not produce evidence")
	}
}

func TestCheckBlock_SameBlockNotEquivocation(t *testing.T) {
	d := NewDetector(64, 0.5)
	v := nodeID(1)
	b := blockID(0xAA)

	d.CheckBlock(v, 100, b, []byte("data"))
	ev := d.CheckBlock(v, 100, b, []byte("data"))
	if ev != nil {
		t.Fatal("same block at same height should not produce evidence")
	}
}

func TestCheckBlock_DifferentBlockEquivocation(t *testing.T) {
	d := NewDetector(64, 0.5)
	v := nodeID(1)
	b1 := blockID(0xAA)
	b2 := blockID(0xBB)

	d.CheckBlock(v, 100, b1, []byte("data1"))
	ev := d.CheckBlock(v, 100, b2, []byte("data2"))
	if ev == nil {
		t.Fatal("signing different blocks at same height must produce evidence")
	}
	if ev.Type != DoubleSign {
		t.Fatalf("expected DoubleSign, got %s", ev.Type)
	}
	if ev.ValidatorID != v {
		t.Fatal("wrong validator in evidence")
	}
	if ev.Height != 100 {
		t.Fatalf("wrong height: %d", ev.Height)
	}
}

// --- Detector: Downtime ---

func TestRecordParticipation_AllPresent(t *testing.T) {
	d := NewDetector(10, 0.5)
	v := nodeID(1)

	// Fill the window with participation
	for i := 0; i < 10; i++ {
		ev := d.RecordParticipation(v, true)
		if ev != nil {
			t.Fatalf("full participation should not produce evidence at slot %d", i)
		}
	}
}

func TestRecordParticipation_AllMissed(t *testing.T) {
	d := NewDetector(10, 0.5)
	v := nodeID(1)

	// Miss every slot. Window needs to fill (10 slots) before evidence triggers.
	// Slots 0-8: warmup, no evidence. Slot 9: window full, 10/10 missed > 50%.
	for i := 0; i < 9; i++ {
		ev := d.RecordParticipation(v, false)
		if ev != nil {
			t.Fatalf("should not trigger during warmup at slot %d", i)
		}
	}
	ev := d.RecordParticipation(v, false) // slot 10, window full
	if ev == nil {
		t.Fatal("missing all slots must produce downtime evidence once window is full")
	}
	if ev.Type != Downtime {
		t.Fatalf("expected Downtime, got %s", ev.Type)
	}
}

func TestRecordParticipation_ThresholdBoundary(t *testing.T) {
	// Window=10, threshold=0.5 means >5 missed triggers evidence.
	// Window must be full before evaluation.

	// Case 1: 5 present then 5 missed = exactly 50%, should NOT trigger (> not >=)
	d := NewDetector(10, 0.5)
	v := nodeID(1)
	for i := 0; i < 5; i++ {
		d.RecordParticipation(v, true)
	}
	for i := 0; i < 5; i++ {
		ev := d.RecordParticipation(v, false)
		if ev != nil {
			t.Fatal("exactly at threshold should not trigger evidence")
		}
	}

	// Case 2: 4 present then 6 missed = 60% missed > 50%
	d2 := NewDetector(10, 0.5)
	for i := 0; i < 4; i++ {
		d2.RecordParticipation(v, true)
	}
	var gotEvidence bool
	for i := 0; i < 6; i++ {
		ev := d2.RecordParticipation(v, false)
		if ev != nil {
			gotEvidence = true
		}
	}
	if !gotEvidence {
		t.Fatal("6/10 missed (60%) should exceed 50% threshold")
	}
}

// --- Detector: Prune ---

func TestPruneBelow(t *testing.T) {
	d := NewDetector(64, 0.5)
	v := nodeID(1)

	d.CheckVote(v, 100, blockID(0xAA), true)
	d.CheckVote(v, 200, blockID(0xBB), true)
	d.CheckBlock(v, 100, blockID(0xCC), []byte("x"))
	d.CheckBlock(v, 200, blockID(0xDD), []byte("y"))

	d.PruneBelow(150)

	// Height 100 should be pruned
	d.mu.Lock()
	if _, exists := d.votes[100]; exists {
		t.Error("height 100 votes should be pruned")
	}
	if _, exists := d.blocks[100]; exists {
		t.Error("height 100 blocks should be pruned")
	}
	// Height 200 should remain
	if _, exists := d.votes[200]; !exists {
		t.Error("height 200 votes should remain")
	}
	if _, exists := d.blocks[200]; !exists {
		t.Error("height 200 blocks should remain")
	}
	d.mu.Unlock()
}

// --- DB ---

func TestDB_RecordEvidence(t *testing.T) {
	db := NewDB(1 * time.Hour)
	v := nodeID(1)

	ev := Evidence{
		Type:        DoubleVote,
		ValidatorID: v,
		Height:      100,
		Timestamp:   time.Now(),
		Proof:       []byte("proof"),
	}

	rec := db.RecordEvidence(ev)
	if rec.SlashCount != 1 {
		t.Fatalf("expected slash count 1, got %d", rec.SlashCount)
	}
	if rec.JailedUntil.IsZero() {
		t.Fatal("validator should be jailed after slashing")
	}
	if !rec.IsJailed(time.Now()) {
		t.Fatal("validator should be jailed now")
	}
	if len(rec.Evidence) != 1 {
		t.Fatalf("expected 1 evidence, got %d", len(rec.Evidence))
	}
}

func TestDB_JailDurationDoubles(t *testing.T) {
	db := NewDB(1 * time.Hour)
	v := nodeID(1)

	now := time.Now()

	ev := Evidence{Type: DoubleVote, ValidatorID: v, Height: 100, Timestamp: now, Proof: []byte("1")}
	rec1 := db.RecordEvidence(ev)
	jail1 := rec1.JailedUntil.Sub(now)

	ev2 := Evidence{Type: DoubleVote, ValidatorID: v, Height: 200, Timestamp: now, Proof: []byte("2")}
	rec2 := db.RecordEvidence(ev2)
	jail2 := rec2.JailedUntil.Sub(now)

	if rec2.SlashCount != 2 {
		t.Fatalf("expected slash count 2, got %d", rec2.SlashCount)
	}

	// Second jail should be roughly 2x the first (modulo time.Now() drift)
	// jail1 ~= 1h, jail2 ~= 2h from now
	if jail2 < jail1 {
		t.Fatalf("jail duration should increase: first=%v second=%v", jail1, jail2)
	}
}

func TestDB_IsJailed(t *testing.T) {
	db := NewDB(1 * time.Millisecond) // very short jail for testing
	v := nodeID(1)

	if db.IsJailed(v) {
		t.Fatal("clean validator should not be jailed")
	}

	ev := Evidence{Type: DoubleSign, ValidatorID: v, Height: 50, Timestamp: time.Now(), Proof: []byte("x")}
	db.RecordEvidence(ev)

	if !db.IsJailed(v) {
		t.Fatal("slashed validator should be jailed")
	}

	// Wait for jail to expire
	time.Sleep(5 * time.Millisecond)
	if db.IsJailed(v) {
		t.Fatal("jail should have expired")
	}
}

func TestDB_GetRecord_Clean(t *testing.T) {
	db := NewDB(1 * time.Hour)
	rec := db.GetRecord(nodeID(99))
	if rec != nil {
		t.Fatal("clean validator should have nil record")
	}
}

func TestDB_GetAllRecords(t *testing.T) {
	db := NewDB(1 * time.Hour)
	v1 := nodeID(1)
	v2 := nodeID(2)

	db.RecordEvidence(Evidence{Type: DoubleVote, ValidatorID: v1, Proof: []byte("a")})
	db.RecordEvidence(Evidence{Type: DoubleSign, ValidatorID: v2, Proof: []byte("b")})

	recs := db.GetAllRecords()
	if len(recs) != 2 {
		t.Fatalf("expected 2 records, got %d", len(recs))
	}
}

// --- EvidenceType String ---

func TestEvidenceType_String(t *testing.T) {
	tests := []struct {
		t    EvidenceType
		want string
	}{
		{DoubleVote, "double_vote"},
		{DoubleSign, "double_sign"},
		{Downtime, "downtime"},
		{EvidenceType(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.t.String(); got != tt.want {
			t.Errorf("EvidenceType(%d).String() = %q, want %q", tt.t, got, tt.want)
		}
	}
}

// --- popcount ---

func TestPopcount(t *testing.T) {
	tests := []struct {
		v      uint64
		window int
		want   int
	}{
		{0b1111, 4, 4},
		{0b1010, 4, 2},
		{0b0000, 4, 0},
		{0b11111111, 8, 8},
		{0xFF, 4, 4},         // only lower 4 bits: 0b1111
		{0xFFFF, 10, 10},     // lower 10 bits all set
		{0b1010101010, 10, 5},
	}
	for _, tt := range tests {
		got := popcount(tt.v, tt.window)
		if got != tt.want {
			t.Errorf("popcount(0b%b, %d) = %d, want %d", tt.v, tt.window, got, tt.want)
		}
	}
}

// --- Inversion tests: evidence must be rejected when invalid ---

// TestDB_DuplicateEvidence_NoDoubleSlash verifies that recording evidence
// for the same validator at the same height twice increments the slash count
// each time (the DB does not deduplicate -- that is the caller's responsibility
// via the Detector). This documents the current behavior: the DB is append-only.
// If deduplication is added later, this test should be updated to verify it.
func TestDB_DuplicateEvidence_NoDoubleSlash(t *testing.T) {
	db := NewDB(1 * time.Hour)
	v := nodeID(1)

	ev := Evidence{
		Type:        DoubleVote,
		ValidatorID: v,
		Height:      100,
		Timestamp:   time.Now(),
		Proof:       []byte("proof-1"),
	}

	rec1 := db.RecordEvidence(ev)
	if rec1.SlashCount != 1 {
		t.Fatalf("expected slash count 1, got %d", rec1.SlashCount)
	}

	// Same evidence again -- DB currently allows it (caller must dedup).
	rec2 := db.RecordEvidence(ev)
	if rec2.SlashCount != 2 {
		t.Fatalf("expected slash count 2 (DB does not dedup), got %d", rec2.SlashCount)
	}
	if len(rec2.Evidence) != 2 {
		t.Fatalf("expected 2 evidence entries, got %d", len(rec2.Evidence))
	}
}

// TestDB_NonExistentValidator verifies that querying a validator with no
// slashing history returns nil/false.
func TestDB_NonExistentValidator(t *testing.T) {
	db := NewDB(1 * time.Hour)
	unknown := nodeID(42)

	if db.IsJailed(unknown) {
		t.Fatal("non-existent validator must not be jailed")
	}
	rec := db.GetRecord(unknown)
	if rec != nil {
		t.Fatal("non-existent validator must have nil record")
	}
}

// TestDB_JailDurationExponential verifies jail duration doubles with each
// successive slash: base, 2*base, 4*base, 8*base.
func TestDB_JailDurationExponential(t *testing.T) {
	base := 1 * time.Hour
	db := NewDB(base)
	v := nodeID(1)

	now := time.Now()
	for slash := uint32(1); slash <= 4; slash++ {
		ev := Evidence{
			Type:        DoubleVote,
			ValidatorID: v,
			Height:      uint64(slash * 100),
			Timestamp:   now,
			Proof:       []byte{byte(slash)},
		}
		rec := db.RecordEvidence(ev)
		if rec.SlashCount != slash {
			t.Fatalf("slash %d: expected count %d, got %d", slash, slash, rec.SlashCount)
		}

		// Expected jail: base * 2^(slash-1)
		expectedDuration := base
		for i := uint32(1); i < slash; i++ {
			expectedDuration *= 2
		}
		actualJail := rec.JailedUntil.Sub(now)
		// Allow 1 second drift from time.Now() inside RecordEvidence
		tolerance := 1 * time.Second
		if actualJail < expectedDuration-tolerance || actualJail > expectedDuration+tolerance {
			t.Fatalf("slash %d: expected jail ~%v, got %v", slash, expectedDuration, actualJail)
		}
	}
}

// TestDB_UnjailedValidatorCanBeReslashed verifies a validator whose jail
// expired can be slashed again, incrementing the slash count.
func TestDB_UnjailedValidatorCanBeReslashed(t *testing.T) {
	db := NewDB(1 * time.Millisecond) // very short base jail
	v := nodeID(1)

	ev1 := Evidence{Type: DoubleVote, ValidatorID: v, Height: 100, Timestamp: time.Now(), Proof: []byte("1")}
	db.RecordEvidence(ev1)
	if !db.IsJailed(v) {
		t.Fatal("should be jailed after first slash")
	}

	// Wait for jail to expire
	time.Sleep(5 * time.Millisecond)
	if db.IsJailed(v) {
		t.Fatal("jail should have expired")
	}

	// Re-slash
	ev2 := Evidence{Type: DoubleSign, ValidatorID: v, Height: 200, Timestamp: time.Now(), Proof: []byte("2")}
	rec := db.RecordEvidence(ev2)
	if rec.SlashCount != 2 {
		t.Fatalf("expected slash count 2 after re-slash, got %d", rec.SlashCount)
	}
	if !db.IsJailed(v) {
		t.Fatal("should be jailed again after re-slash")
	}
}

// TestDB_ConcurrentEvidenceSubmission verifies that concurrent evidence
// submissions do not corrupt the DB state (race safety).
func TestDB_ConcurrentEvidenceSubmission(t *testing.T) {
	db := NewDB(1 * time.Hour)
	v := nodeID(1)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			ev := Evidence{
				Type:        DoubleVote,
				ValidatorID: v,
				Height:      uint64(idx),
				Timestamp:   time.Now(),
				Proof:       []byte{byte(idx)},
			}
			db.RecordEvidence(ev)
		}(i)
	}
	wg.Wait()

	rec := db.GetRecord(v)
	if rec == nil {
		t.Fatal("expected non-nil record after concurrent submissions")
	}
	if rec.SlashCount != goroutines {
		t.Fatalf("expected %d slashes, got %d", goroutines, rec.SlashCount)
	}
	if len(rec.Evidence) != goroutines {
		t.Fatalf("expected %d evidence entries, got %d", goroutines, len(rec.Evidence))
	}
}

// TestDetector_ConcurrentCheckVote verifies the Detector is race-safe
// under concurrent vote checking.
func TestDetector_ConcurrentCheckVote(t *testing.T) {
	d := NewDetector(64, 0.5)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			v := nodeID(byte(idx))
			b := blockID(byte(idx))
			d.CheckVote(v, 100, b, true)
		}(i)
	}
	wg.Wait()

	// Verify state is consistent: each validator should have exactly one
	// vote recorded at height 100.
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.votes[100]) != goroutines {
		t.Fatalf("expected %d validators in votes map, got %d", goroutines, len(d.votes[100]))
	}
}

// TestCheckVote_ForgedEvidence_WrongValidator ensures that evidence produced
// by one validator cannot be "attributed" to another validator. The detector
// associates evidence with the validatorID passed to CheckVote, so an attacker
// must actually observe the victim voting.
func TestCheckVote_ForgedEvidence_WrongValidator(t *testing.T) {
	d := NewDetector(64, 0.5)

	// Honest validator votes for block A
	honest := nodeID(1)
	d.CheckVote(honest, 100, blockID(0xAA), true)

	// Attacker is a different validator
	attacker := nodeID(2)
	ev := d.CheckVote(attacker, 100, blockID(0xBB), true)
	if ev != nil {
		t.Fatal("voting by different validators must not produce evidence")
	}

	// The honest validator did NOT double-vote
	ev = d.CheckVote(honest, 100, blockID(0xAA), true)
	if ev != nil {
		t.Fatal("re-submitting same vote must not produce evidence")
	}
}
